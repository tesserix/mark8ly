# Zitadel Phase 3c-2 — Google Sign-In Through Zitadel on the Web

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Google sign-in off GIP and onto Zitadel's IDP-intent flow for both web surfaces, and re-enable the storefront Google controls that phase 3c-1 hid.

**Architecture:** Zitadel already holds an active Google IDP (`386381087862948767`, TESSERIX org, auto-creation on, email auto-linking). `auth-bff` gains two endpoint pairs — start and finish — for the merchant and customer paths. Start creates a Zitadel IDP intent and returns Google's `authUrl`; finish exchanges the returned intent for an identity. The merchant path then runs today's gauntlet and mints `m8_session`; the customer path returns an identity and mints nothing, exactly as phase 3b ruled.

This **removes** the `mark8ly.com` trampoline from the Google path. That hop existed because Google's OAuth client has one fixed registered origin; Zitadel takes the return URL per request, so a tenant storefront can be redirected to directly.

**Tech Stack:** Go 1.26 / Gin (`auth-bff`), Next.js 16 / React 19 server actions, `@repo/ui`, vitest.

**Spec:** `docs/superpowers/specs/2026-09-03-zitadel-migration-design.md`

## Verified API facts — use these verbatim, do not re-derive

All confirmed against the live instance or Zitadel source on 2026-09-04.

- **Start:** `POST /v2/idp_intents`, body `{"idpId": "...", "urls": {"successUrl": "...", "failureUrl": "..."}}`. Response carries **`authUrl`** and `details`. There is **no explicit intent-id field** — the id appears as the `state` query param inside `authUrl`, and is returned to us on the redirect, so nothing needs to parse `authUrl`.
- **Success redirect params** (from `internal/api/idp/idp.go`): **`id`** (intent id), **`token`** (intent token), **`user`** (userId, present ONLY when the identity is already linked).
- **Failure redirect params:** **`id`**, **`error`**, **`error_description`**.
- **Retrieve:** `POST /v2/idp_intents/{id}`, body `{"idpIntentToken": "<token>"}`.
- **Session from intent:** `checks.idpIntent = {"idpIntentId": "...", "idpIntentToken": "..."}`.
- **Zitadel does NOT validate `successUrl` or `failureUrl`.** Verified: an intent pointing at `https://evil.example.com/x` was accepted and returned a working Google `authUrl`.
- Google's `redirect_uri` is always `https://auth.tesserix.app/idps/callback` — fixed, already registered, nothing to change at Google.

## Global Constraints

- **The `user` query param is NEVER an identity.** It is attacker-controlled — it arrives in a URL the browser followed. The authoritative identity comes only from `POST /v2/idp_intents/{id}` with the token. Treat `user` as a hint at most; ignore it entirely if unused. Trusting it is account takeover.
- **`auth-bff` must allowlist the return host itself**, because Zitadel validates nothing. An unvalidated `successUrl` turns these endpoints into an open redirect that hands a completed sign-in to an attacker's domain. The `X-Internal-Auth` guard is a second line of defence, not a substitute.
- Every new `auth-bff` endpoint is guarded by `internalauth` as the **first statement**, before the body is read — same as the four existing Zitadel endpoints.
- The flag rule is one literal string: `NEXT_PUBLIC_AUTH_PROVIDER === "zitadel"`. With the flag unset, every byte of GIP behaviour is unchanged, including the existing `mark8ly.com/auth/google` trampoline, which stays untouched and working.
- The customer path returns an identity and **mints nothing** — no `finalize`, no authorization code. The storefront mints `mp_customer_session` through the existing `completeCustomerSignIn` helper.
- No secret, session token, intent token, password, or authenticator code in any log line.
- **`npm run build` must pass for every touched Next.js app.** A `"use server"` module may export only async functions; `vitest` and `tsc --noEmit` both miss violations and a build break shipped to CI in phase 3c-1 because of exactly this.
- Test key literals stay low-entropy (`"thirtytwo-bytes-for-testing-only"`); a 32-hex-looking literal near an auth keyword trips gitleaks/GitGuardian.

---

### Task 1: The intent client in `auth-bff`

**Files:**
- Create: `services/auth-bff/internal/zitadellogin/idpintent.go`
- Create: `services/auth-bff/internal/zitadellogin/idpintent_test.go`

**Interfaces:**
- Produces: `(*Client).StartIDPIntent(ctx, idpID string, successURL, failureURL string) (authURL string, err error)` and `(*Client).RetrieveIDPIntent(ctx, intentID, intentToken string) (IDPIdentity, error)`, where `IDPIdentity` carries at least the Zitadel `userId` (empty when unlinked), the email, and whether the address is verified.

Follow the existing `client.go` conventions exactly — same request helper, same `requestOption` shape (`func(*requestOptions)`), same `readZitadelErrorID` error extraction. Read that file first.

- [ ] **Step 1: Write the failing tests** against an `httptest` fake, mirroring `fakeZitadelHandler` in the existing tests: a start that returns `authUrl`; a start whose response omits `authUrl` (must error, not return ""); a retrieve returning a linked identity; a retrieve returning an unlinked one; a Zitadel error surfacing its error id.
- [ ] **Step 2: Run them, confirm they fail for the stated reason**
- [ ] **Step 3: Implement**
- [ ] **Step 4: `go test ./... && go vet ./...` in `services/auth-bff`**
- [ ] **Step 5: Commit** — `feat(auth-bff): client for Zitadel IDP intents`

---

### Task 2: Return-URL allowlisting

The single most security-sensitive piece in this plan. Zitadel accepts any `successUrl`; this is the only thing standing between these endpoints and an open redirect.

**Files:**
- Create: `services/auth-bff/internal/zitadellogin/returnurl.go`
- Create: `services/auth-bff/internal/zitadellogin/returnurl_test.go`

**Interfaces:**
- Produces: a validator that takes a candidate return URL and the configured allowlist and returns the URL to use or an error. Configuration arrives as an env-driven list on `Config`; follow how existing `auth-bff` config values are declared and validated.

- [ ] **Step 1: Write the failing tests.** Cover, at minimum: an exact allowed host passes; a permitted tenant subdomain passes; `http://` is rejected (https only); a host that merely *contains* an allowed host is rejected (`mark8ly.com.evil.tld`); a subdomain of an allowed host is rejected unless subdomains are explicitly permitted; userinfo (`https://user@evil.tld`), a port, and an embedded credential are handled deliberately; a scheme-relative URL (`//evil.tld`) is rejected; an empty or unparseable URL is rejected. Assert on the *decision*, not on error text.
- [ ] **Step 2: Run them, confirm they fail**
- [ ] **Step 3: Implement.** Compare parsed host equality or an explicit suffix rule — never `strings.Contains` or a prefix test.
- [ ] **Step 4: Tests + vet**
- [ ] **Step 5: Commit** — `feat(auth-bff): allowlist IDP return URLs`

---

### Task 3: The merchant endpoints

**Files:**
- Modify: `services/auth-bff/internal/zitadellogin/handler.go`
- Modify: `services/auth-bff/internal/zitadellogin/handler_test.go`
- Modify: `services/auth-bff/cmd/auth-bff/main.go` (route mounting, behind the existing flag)

Two endpoints: start (validate return URL → `StartIDPIntent` → return `authUrl`) and finish (`RetrieveIDPIntent` → create a session with `checks.idpIntent` → run the existing gauntlet → mint `m8_session`).

- [ ] **Step 1: Write the failing tests** — including one asserting an unauthenticated caller never reaches Zitadel (mirror `unreachableZitadel` from `internal_auth_test.go`), and one asserting a `user` query param is never used as identity.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement.** `internalauth` guard first statement in both.
- [ ] **Step 4: Tests + vet**
- [ ] **Step 5: Commit** — `feat(auth-bff): merchant Google sign-in through Zitadel`

---

### Task 3b: Registration from an unlinked federated identity

**Added mid-plan.** Task 3 revealed that nothing auto-creates a user in the login-client model: `isAutoCreation` is honoured by Zitadel's own hosted login UI, not by the API flow we drive. So `RetrieveIDPIntent` returns an empty `ZitadelUserID` for anyone who has not signed in through Zitadel before — which today is everyone — and the finish endpoint rejects them. Without this task the Google path cannot be exercised end to end by anybody.

The retrieve response's `add_human_user` field is **deprecated**, so do not use it. Create the user explicitly.

**The security rule that governs this task.** An unlinked identity may be attached to an **existing** Zitadel account by email address **only when the provider asserts that email is verified**. If it is not verified, refuse — do not create, do not link. Linking an unverified provider email to an existing account is account takeover: anyone who can register that address at any federated provider inherits the account. `IDPIdentity.EmailVerified` is read soft from raw claims and defaults to false when absent, so treat a false or missing value as "refuse", never as "probably fine".

**Files:**
- Modify: `services/auth-bff/internal/zitadellogin/idpintent.go` (or a new file in the package)
- Modify: `handler.go` and its tests
- Modify: `pkg/config/config.go` — the Google IDP id becomes configuration, not the package constant Task 3 introduced

- [ ] **Step 1: Write the failing tests.** Cover: an unlinked identity with a verified email is created and linked, then signs in; an unlinked identity with `EmailVerified` false is refused with a distinct outcome and no user is created; an unlinked identity with the email claim absent entirely is refused the same way; an already-linked identity takes the existing path untouched; the created user carries the IDP link so the next sign-in resolves via `user_id`.
- [ ] **Step 2: Run them, confirm they fail**
- [ ] **Step 3: Implement**
- [ ] **Step 4: `go test ./... && go vet ./...`**
- [ ] **Step 5: Commit** — `feat(auth-bff): register an unlinked Google identity`

---

### Task 4: The customer endpoints

**Files:**
- Modify: `services/auth-bff/internal/zitadellogin/customer_handler.go`
- Modify: its test file

Same two endpoints, and the same registration rule as Task 3b — an unlinked identity with a verified email is created and linked; an unverified or absent email is refused. Finish then **returns an identity and mints nothing** — no session creation, no finalize. Mirror the decide-only shape the existing customer login already uses, and keep that file's import discipline (stdlib + gin only).

- [ ] **Step 1: Write the failing tests**, including: finish returns an identity without creating any Zitadel session; the guard is first; `user` is not trusted; an unlinked identity (no `user` param) still resolves via retrieve.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement**
- [ ] **Step 4: Tests + vet**
- [ ] **Step 5: Commit** — `feat(auth-bff): customer Google sign-in through Zitadel`

---

### Task 5: Storefront wiring, and re-enabling the buttons

**Files:**
- Modify: `apps/storefront/lib/auth/auth-bff-customer.ts` (start/finish clients, sending `X-Internal-Auth`)
- Create: `apps/storefront/app/auth/idp/finish/route.ts`
- Modify: `apps/storefront/app/sign-in/actions.ts`
- Modify: `apps/storefront/lib/auth/provider.ts` and the three components 3c-1 gated
- Modify/create: the corresponding tests

Under the Zitadel flag the three Google controls become available again and point at the new start endpoint instead of `mark8ly.com/auth/google`. With the flag unset they must still point at the trampoline exactly as today.

- [ ] **Step 1: Write the failing tests** — flag set: control renders and targets the new flow; flag unset: control renders and targets `mark8ly.com/auth/google` unchanged; the finish route mints `mp_customer_session` via the existing shared helper and sets no cookie on failure; a tampered `user` param changes nothing.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement.** Reuse `completeCustomerSignIn` — do not write a second cookie-minting path.
- [ ] **Step 4: `npx vitest run`, `npx tsc --noEmit`, and `npm run build` — report the build's exit status explicitly**
- [ ] **Step 5: Commit** — `feat(storefront): Google sign-in through Zitadel`

---

### Task 6: Admin wiring

The merchant endpoints from Task 3 are unreachable until the admin login page uses them. Admin has a fixed host, so its return URL is a single allowlisted value rather than a per-tenant one.

**Files:**
- Modify: `apps/admin/lib/auth/auth-bff.ts` (start/finish clients, sending `X-Internal-Auth`)
- Create: `apps/admin/app/auth/idp/finish/route.ts`
- Modify: `apps/admin/components/auth/SignInForm.tsx` and `apps/admin/app/login/actions.ts`
- Modify/create: the corresponding tests

Under the Zitadel flag the Google button drives the new start endpoint; with the flag unset it keeps using GIP's GSI path (`apps/admin/lib/gip/google-gsi.ts`) byte-for-byte.

Read how phase 3a wired `/login/authorize` and `/auth/callback` first — the finish route should sit alongside them and reuse the same session-minting path, not a second one. Note the Next.js 16 constraint 3a hit: `cookies().set()` is forbidden during a Server Component render, which is why that work lives in a route handler.

- [ ] **Step 1: Write the failing tests** — flag set: the button targets the new flow and a successful finish mints `m8_session`; flag unset: the GSI path is untouched; a tampered `user` param changes nothing; no cookie on any failure path.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement**
- [ ] **Step 4: `npx vitest run`, `npx tsc --noEmit`, and `npm run build` — report the build's exit status explicitly**
- [ ] **Step 5: Commit** — `feat(admin): Google sign-in through Zitadel`

---

### Task 7: Documentation

**Files:**
- Modify: `services/auth-bff/internal/zitadellogin/README.md`

- [ ] **Step 1:** Record that the Google path no longer uses the trampoline under Zitadel and why; that `successUrl` is unvalidated by Zitadel so the allowlist is load-bearing; that the `user` param is never an identity; and what remains for cutover — customer sign-up is still GIP-only, and the storefront CSP still allowlists `accounts.google.com/gsi/client` for the GIP path and can drop it once GIP is gone. Correct anything the previous phase's section now states falsely.
- [ ] **Step 2: Commit** — `docs(auth-bff): record the Zitadel Google path`
