# Zitadel Migration Phase 3b — Storefront Customer Password Login

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a storefront customer sign in with a password against Zitadel instead of Google Identity Platform, behind a flag that is off by default, without changing what mints their session or how it is scoped.

**Architecture:** `auth-bff` gains a customer endpoint that verifies a credential against Zitadel and returns `{uid, email}` — it mints nothing, touches no cookie, and never enters the merchant gauntlet (spec D11). The storefront keeps resolving the host and store, minting `mp_customer_session` in its own HMAC format scoped to the exact host, and driving profile and loyalty enrolment. Only the "verify the token" step moves.

**Tech Stack:** Go 1.26 (auth-bff), Next.js App Router server actions (storefront), stdlib `testing` with `httptest`, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-03-zitadel-migration-design.md` — D10, and **D11 which supersedes D10's framing**.

## Scope — read this before starting

**In scope:** storefront customer **password** sign-in.

**Explicitly NOT in scope, and deferred to phase 3c:** the Google sign-in trampoline at `apps/onboarding/app/auth/google`. Zitadel's federated-IdP flow is shaped differently from GIP's `signInWithIdp`, the trampoline carries its own HMAC exchange-code protocol and host-matching defences, and the onboarding app's CSP allowlists `accounts.google.com/gsi/client` by host with **no `strict-dynamic`** — so any replacement script origin needs an explicit CSP change. Bundling that with password login would produce one branch nobody can review well.

Also not in scope: deleting any GIP code (phase 6), and customer new-device detection or email OTP (spec D11 records why).

## Global Constraints

- **GIP stays live and default.** Every change is additive and flag-gated. Existing storefront tests must pass unedited.
- **`auth-bff` must not mint a customer cookie.** It returns an identity. The storefront mints `mp_customer_session` exactly as it does today, scoped to the exact request host so one store's session cannot be sent to another. Do not "improve" this by centralising cookie minting.
- **The customer endpoint must not call `completeLogin`, `CompleteForProvider`, or OpenFGA.** If a task seems to need them, stop and report BLOCKED — that would reintroduce the weaker-second-path cost D11 exists to avoid.
- **The login-client PAT is instance-level** and must never reach a customer-facing app. It stays in `auth-bff`; that is the whole reason this endpoint exists.
- **Reuse `internal/zitadellogin`.** Its client, sufficiency decision, witness type and archtests are merged and reviewed. The customer path uses them; it does not fork them. The three archtests scan every non-test file in that package, so anything added there must still satisfy them.
- **Never log a credential** — password, session token, PAT.
- Instance: `https://auth.tesserix.app`. Storefront project `mark8ly-storefront` = `389070377390703107`, `access.mode: public`.

---

### Task 1: The customer verification endpoint

**Files:**
- Create: `services/auth-bff/internal/zitadellogin/customer_handler.go`
- Test: `services/auth-bff/internal/zitadellogin/customer_handler_test.go`

**Interfaces:**
- Consumes: the merged `Client` (`CreatePasswordSession`, `VerifyTOTP`, `UserEmail`), `CompleteIfSufficient`, `CompleteAfterFactor`.
- Produces: `func NewCustomerHandler(c *Client) *CustomerHandler` and `Register(r *gin.RouterGroup)` mounting `POST /customer/login` and `POST /customer/totp`. Task 2 mounts it; Task 3 calls it.

- [ ] **Step 1: Write the failing tests**

Mirror `handler_test.go`'s style — `httptest.Server` fake Zitadel, inline JSON literals copied from observed responses.

Cover:
- a successful password login returns `200 {"data": {"uid": …, "email": …}}` and **sets no cookie** — assert `rec.Result().Cookies()` is empty, because this is the property that distinguishes the customer path
- `ErrBadCredentials` and `ErrUserNotFound` produce an **identical** `401 {"error":"invalid_credentials"}` — a different answer for "no such user" is an account-enumeration oracle on a public storefront
- a `totp_required` outcome returns the session for the UI and still sets no cookie
- a handoff outcome returns the hosted-login URL
- neither the password nor the Zitadel session token appears in any error body
- the email is resolved from Zitadel via `UserEmail`, **not** taken from the request body — the same defect fixed on the merchant path in phase 2

- [ ] **Step 2: Run to verify they fail**

Run: `cd services/auth-bff && go test ./internal/zitadellogin/`
Expected: build failure — `NewCustomerHandler` undefined.

- [ ] **Step 3: Implement**

`login` reads `{auth_request_id, login_name, password}`. It calls `CreatePasswordSession`, then `CompleteIfSufficient` with `federated: false`, then on `OutcomeComplete` resolves the user via `SessionFactors` + `UserEmail` and returns the identity. On `OutcomeFactorRequired` it returns the session so the caller can collect a code. On `OutcomeHandoff` it returns the hosted-login URL.

It **mints no session, sets no cookie, and calls nothing in `internal/autologin`.** Add a file-level comment saying so and why, citing D11 — the next reader's instinct will be to "finish" it by minting something.

`totp` mirrors the merchant path: `VerifyTOTP`, then `CompleteAfterFactor`, then the same identity response.

- [ ] **Step 4: Run the tests and the archtests**

Run: `cd services/auth-bff && go test ./internal/zitadellogin/ -v`
Expected: all pass, **including the three archtests** — the new file must not call `finalize` or construct `sufficient{}`.

- [ ] **Step 5: Commit**

```bash
git add services/auth-bff/internal/zitadellogin/
git commit -m "feat(auth-bff): verify a storefront customer credential and return an identity"
```

---

### Task 2: Mount it

**Files:**
- Modify: `services/auth-bff/cmd/server/main.go`

**Interfaces:**
- Consumes: Task 1's `NewCustomerHandler`.
- Produces: `POST /auth/customer/login` and `POST /auth/customer/totp`, live only when the Zitadel client is configured.

- [ ] **Step 1: Mount conditionally**

In the route block (~line 294-302), following the existing `if zitadelHandler != nil { zitadelHandler.Register(v1) }` idiom exactly:

```go
	if zitadelClient != nil {
		zitadellogin.NewCustomerHandler(zitadelClient).Register(v1)
	}
```

Do not introduce a second flag. The existing `ZITADEL_ENABLED` gate already governs whether the client exists.

- [ ] **Step 2: Verify**

Run: `cd services/auth-bff && go build ./... && go vet ./... && go test ./...`
Expected: clean, all packages green.

- [ ] **Step 3: Commit**

```bash
git add services/auth-bff/cmd/server/main.go
git commit -m "feat(auth-bff): mount the customer login routes behind the existing flag"
```

---

### Task 3: Storefront client for the endpoint

**Files:**
- Create: `apps/storefront/lib/auth/auth-bff-customer.ts`
- Test: `apps/storefront/lib/auth/auth-bff-customer.test.ts`

**Interfaces:**
- Produces: `verifyCustomerCredential({authRequestId, loginName, password})` and `verifyCustomerTotp({...})`, each returning a discriminated union mirroring the endpoint's outcomes. Task 4 consumes them.

- [ ] **Step 1: Write the failing tests**

Mock `fetch`. Assert the snake_case wire fields, that a 401 maps to a single `invalid_credentials` outcome regardless of which underlying error occurred, and that **the password never appears in a thrown error or rejection value**.

- [ ] **Step 2: Run to verify they fail, then implement**

Follow whatever conventions `apps/storefront/lib` already uses for outbound HTTP — read a sibling first. The `auth-bff` base URL is server-side config; this must only ever be called from a server action, never the browser.

- [ ] **Step 3: Verify and commit**

Run: `cd apps/storefront && npx vitest run lib/auth/ && npx tsc --noEmit`

```bash
git add apps/storefront/lib/auth/
git commit -m "feat(storefront): client for auth-bff's customer credential check"
```

---

### Task 4: Branch `customerSignIn` on the provider

**Files:**
- Modify: `apps/storefront/app/sign-in/actions.ts`
- Modify: `apps/storefront/components/auth/CustomerSignInForm.tsx`
- Test: `apps/storefront/app/sign-in/actions.zitadel.test.ts`

**Interfaces:**
- Consumes: Task 3's client.
- Produces: no new exports; `customerSignIn` gains a Zitadel branch.

- [ ] **Step 1: Understand what must NOT change**

Read `customerSignIn` end to end first. Under Zitadel, **only** the `verifyGIPIdToken` step is replaced. Everything else stays byte-for-byte: `sanitizeHost`, `resolveStore`, `encodeSession`, the cookie set with `domain: cookieHost` and its exact-host comment, `ensureCustomerProfile`, `ensureLoyaltyEnrollment`.

- [ ] **Step 2: Write the failing tests**

Assert:
- with the flag unset, `customerSignIn` calls `verifyGIPIdToken` and not the new client
- with the flag on, it calls the new client and **not** `verifyGIPIdToken`
- **in both cases the cookie is set with `domain` equal to the resolved request host** — the per-store isolation property, asserted directly rather than assumed
- a failed verification sets no cookie
- the form posts the password to the server action rather than to Zitadel from the browser

- [ ] **Step 3: Implement the branch**

Add a `NEXT_PUBLIC_AUTH_PROVIDER`-style flag read, defaulting to GIP, matching how `apps/admin` does it — check that implementation and use the same defaulting rule, where anything other than the literal `"zitadel"` means GIP.

Under Zitadel the form collects the password and posts it to the server action; the browser no longer calls Identity Toolkit.

- [ ] **Step 4: Verify**

Run: `cd apps/storefront && npx vitest run && npx tsc --noEmit && npm run build`
Expected: clean, and every pre-existing storefront test passes unedited.

- [ ] **Step 5: Commit**

```bash
git add apps/storefront/
git commit -m "feat(storefront): sign customers in through auth-bff when the flag selects Zitadel"
```

---

### Task 5: Prove the flow is reachable end to end

**This task exists because of what happened in phase 3a.** Six tasks each passed their own review, and the feature was still unreachable: a flag was computed and never passed, a callback route was built and never called, and both were invisible in every individual diff. A per-task review sees a diff; a missing connection appears in no diff. This task is the check that would have caught it, run before the final review rather than after.

**Files:**
- Create: `apps/storefront/app/sign-in/customer-login-flow.test.ts`

- [ ] **Step 1: Write an integration test that traverses every seam**

With the flag on, and with only the network boundary mocked, assert in one test that a customer password sign-in:

1. reaches `verifyCustomerCredential` with the submitted credential
2. does **not** call `verifyGIPIdToken`
3. results in `encodeSession` being called with the uid and email that came back from `auth-bff` — not from any client-supplied field
4. sets `mp_customer_session` with `domain` equal to the resolved request host
5. triggers the profile and loyalty side effects

Each assertion pins one seam. If any step is wired to nothing, this test fails — which is exactly the failure mode a per-file review cannot see.

- [ ] **Step 2: Write the same test for the flag unset**, asserting the GIP path is traversed instead and the Zitadel client is never called.

- [ ] **Step 3: Verify and commit**

Run: `cd apps/storefront && npx vitest run`

```bash
git add apps/storefront/app/sign-in/
git commit -m "test(storefront): pin the customer login flow end to end for both providers"
```

---

### Task 6: Record what this phase did not do

**Files:**
- Modify: `services/auth-bff/internal/zitadellogin/README.md`

- [ ] **Step 1: Add a customer-path section**

Record, with reasoning:
1. The customer endpoint verifies and returns an identity. It mints nothing and never enters the merchant gauntlet (D11). The next reader's instinct will be to "finish" it; say why that would be wrong.
2. `mp_customer_session` is minted by the storefront, in a different format from `m8_session`, scoped to the exact host so one store's session cannot reach another. The two share a key *name* and nothing else.
3. Storefront customers get no new-device detection and no email-OTP step-up, because they have neither today. Adding them is a product decision, not a migration detail.
4. The Google trampoline is untouched and is phase 3c — with the CSP host-allowlist constraint noted, since there is no `strict-dynamic`.
5. `packages/ui/src/auth/exchange-code.ts` still has no `kind` pin while its two siblings do, and all three share `SESSION_ENCRYPT_KEY`. Carried from phase 3a; it sits on the trampoline path this phase deliberately did not touch.

- [ ] **Step 2: Commit**

```bash
git add services/auth-bff/internal/zitadellogin/README.md
git commit -m "docs(auth-bff): record the customer path's boundaries and what 3b deferred"
```

---

## Phase 3b completion criteria

- With the flag unset, storefront login behaves exactly as today and every pre-existing test passes unedited.
- The customer endpoint mints no cookie, calls no OpenFGA, and touches nothing in `internal/autologin` — asserted by tests, not by inspection.
- `mp_customer_session` is still minted by the storefront, still scoped to the exact request host, on both providers.
- Unknown-user and wrong-password are indistinguishable to the caller.
- Task 5's end-to-end tests pass for both providers.
- The three `zitadellogin` archtests still pass with the new file present.

## Not in this phase

The Google trampoline (3c), `marketplace-api`'s verifier and the `tenant_id` claim (4), `gipadmin` (5), and the cutover (6).
