# Zitadel Phase 3c-1 — Close the Storefront Gaps Phase 3b Deferred

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the three storefront-side gaps that phases 3a and 3b explicitly deferred to 3c and that do **not** depend on the Google-through-Zitadel decision, so they ship while that work proceeds separately.

**Architecture:** Three independent changes. (1) Add a `kind` pin to `exchange-code.ts`, the one HMAC code module that lacks one. (2) Give the storefront a TOTP entry screen so a customer with an authenticator can finish signing in, mirroring the admin flow built in 3a. (3) Stop the storefront's three "Continue with Google" entry points from sending customers into the GIP trampoline when the Zitadel flag is on.

**Tech Stack:** Next.js 16 / React 19 server actions, `@repo/ui` shared auth modules, vitest.

**Spec:** `docs/superpowers/specs/2026-09-03-zitadel-migration-design.md`

## Global Constraints

- **The Zitadel flag rule is one literal string.** `NEXT_PUBLIC_AUTH_PROVIDER === "zitadel"` and nothing else. Unset, empty, `"Zitadel"`, `"true"` all mean GIP. Copy the existing ternary exactly; never invent a looser check.
- **With the flag unset, every byte of behaviour must be unchanged.** This is the whole safety property of the migration to date. Any test that passes only when the flag is set must have a sibling proving the GIP path is untouched.
- **Never render an internal error string to a shopper.** `AuthBffCustomerError.message` is `"auth-bff customer endpoint error: <code> (status <n>)"`. Log it server-side; return a generic message. See the `catch` block in `apps/storefront/app/sign-in/actions.ts`.
- **Never tell a customer their password is wrong when it isn't.** Each non-`complete` outcome keeps its own truthful message. This was a real 3b bug; do not regress it.
- **No secret, token, password, or authenticator code in any log line.**
- **`SESSION_ENCRYPT_KEY` signs all four HMAC code modules.** That shared key is exactly why the `kind` pin in Task 1 matters.
- Do not touch `services/auth-bff` in this phase. All four customer/merchant endpoints already exist and are guarded; this phase is storefront + `@repo/ui` only.

---

### Task 1: Pin the exchange code to its purpose

`packages/ui/src/auth/exchange-code.ts` mints and verifies the HMAC code that bounces a verified Google sign-in from the `mark8ly.com` trampoline back to a tenant store. Its two siblings (`admin-handoff-code.ts`, `zitadel-totp-code.ts`) both carry a `kind` field and reject a code whose `kind` doesn't match. This one does not — and all of them are signed with the same `SESSION_ENCRYPT_KEY`, so a code minted for one purpose currently passes this verifier's signature check.

There is exactly one minter and one verifier, both found by grep:
- mint: `apps/onboarding/app/auth/google/actions.ts:49`
- verify: `apps/storefront/app/auth/google/finish/route.ts:33`

**Deployment note for the implementer:** this changes the wire format. Both apps ship in the same Kargo Freight (all 7 mark8ly images promote together), and codes carry a 30-second TTL, so the exposure is one rollout's pod-start skew against a 30s window. Ship minter and verifier together in this one task — do **not** split them across tasks or commits.

**Files:**
- Modify: `packages/ui/src/auth/exchange-code.ts`
- Modify: `packages/ui/src/auth/exchange-code.test.ts`
- Modify: `apps/onboarding/app/auth/google/actions.ts`
- Modify: `apps/storefront/app/auth/google/finish/route.ts` (only if it constructs claims itself; it should need no change)

**Interfaces:**
- Produces: `EXCHANGE_CODE_KIND = "google_exchange_v1"`, exported from `@repo/ui/auth/exchange-code`; `ExchangeCodeClaims` gains `kind: typeof EXCHANGE_CODE_KIND`.

- [ ] **Step 1: Write the failing tests**

Add to `packages/ui/src/auth/exchange-code.test.ts`:

```ts
import { ADMIN_HANDOFF_KIND, mintAdminHandoffCode } from "./admin-handoff-code";

const KEY = "thirtytwo-bytes-for-testing-only";

it("round-trips a minted code and exposes its kind", () => {
  const code = mintExchangeCode(
    { idToken: "t", storeSlug: "shop", returnTo: "/", intent: "signin" },
    KEY,
    30,
  );
  const claims = verifyExchangeCode(code, KEY);
  expect(claims.kind).toBe(EXCHANGE_CODE_KIND);
  expect(claims.storeSlug).toBe("shop");
});

it("rejects a code minted by a sibling module with the same key", () => {
  const foreign = mintAdminHandoffCode(
    { tenant_id: "t1", multiple_tenants: false },
    KEY,
    30,
  );
  expect(() => verifyExchangeCode(foreign, KEY)).toThrow(
    expect.objectContaining({ code: "wrong_kind" }),
  );
});

it("rejects a legacy code that carries no kind at all", () => {
  // Hand-built payload in the pre-kind format, signed with the real key.
  const legacy = { idToken: "t", storeSlug: "shop", returnTo: "/", intent: "signin",
                   exp: Math.floor(Date.now() / 1000) + 30 };
  const payload = Buffer.from(JSON.stringify(legacy)).toString("base64url");
  const sig = createHmac("sha256", KEY).update(payload).digest("hex");
  expect(() => verifyExchangeCode(`${payload}.${sig}`, KEY)).toThrow(
    expect.objectContaining({ code: "wrong_kind" }),
  );
});
```

Check the sibling's real export names before writing this — if `mintAdminHandoffCode` has a different signature, adapt the call, do not change the sibling.

- [ ] **Step 2: Run them and watch them fail**

Run: `cd apps/storefront && npx vitest run ../../packages/ui/src/auth/exchange-code.test.ts`
(or the repo's usual path for `@repo/ui` tests — find how `admin-handoff-code.test.ts` is currently run and use that.)
Expected: FAIL — `EXCHANGE_CODE_KIND` is not exported.

- [ ] **Step 3: Add the kind, mirroring `admin-handoff-code.ts` exactly**

```ts
export const EXCHANGE_CODE_KIND = "google_exchange_v1" as const;

export interface ExchangeCodeClaims {
  kind: typeof EXCHANGE_CODE_KIND;
  idToken: string;
  // ...unchanged fields
}
```

In `mintExchangeCode`, set `kind: EXCHANGE_CODE_KIND` on the claims object. In `verifyExchangeCode`, after the JSON parse and **before** the expiry check, add the same guard shape the sibling uses:

```ts
if (claims.kind !== EXCHANGE_CODE_KIND) {
  throw new ExchangeCodeError(
    "wrong_kind",
    `expected kind ${EXCHANGE_CODE_KIND}, got ${claims.kind}`,
  );
}
```

- [ ] **Step 4: Run the full storefront and onboarding suites**

Run: `cd apps/storefront && npx vitest run` and `cd apps/onboarding && npx vitest run`
Expected: PASS, including the pre-existing trampoline tests. If a pre-existing test hand-builds a claims object, update it to include `kind` — that is the one legitimate reason to touch an existing test here.

- [ ] **Step 5: Typecheck both apps**

Run: `cd apps/storefront && npx tsc --noEmit` and `cd apps/onboarding && npx tsc --noEmit`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "fix(ui): pin the Google exchange code to its own purpose"
```

---

### Task 2: A TOTP entry screen for storefront customers

Phase 3b left an honest dead end: a customer with TOTP enrolled gets `totp_required` and a message saying this page can't collect a code. `auth-bff`'s `/auth/customer/totp` endpoint already exists and is guarded by `X-Internal-Auth`; `verifyCustomerCredential`'s `totp_required` outcome already carries `sessionId` and `sessionToken`. Only the storefront UI and the second server action are missing.

**Mirror the admin flow built in phase 3a** — `apps/admin/app/login/actions.ts` (see `confirmZitadelTotp`, and the `totp_required` case near line 377) and `apps/admin/components/auth/SignInForm.tsx`. Read both before writing anything. Carrying `sessionId`/`sessionToken` through the client between the two server actions matches admin and is bounded: `PATCH /v2/sessions/{id}` also requires the instance login-client PAT, which only auth-bff holds.

**Files:**
- Modify: `apps/storefront/app/sign-in/actions.ts` (add `confirmCustomerTotp`; change the `totp_required` case)
- Modify: `apps/storefront/components/auth/CustomerSignInForm.tsx` (code entry step)
- Modify/Create: `apps/storefront/app/sign-in/actions.test.ts` (or the existing customer-login test file)
- Modify: `apps/storefront/lib/auth/auth-bff-customer.ts` **only if** it has no TOTP submit function yet — check first; 3b may already have added one.

**Interfaces:**
- Consumes: `verifyCustomerCredential` (`totp_required` → `{ kind, sessionId, sessionToken }`), the `/auth/customer/totp` client.
- Produces: `confirmCustomerTotp(input: { storeSlug: string; sessionId: string; sessionToken: string; code: string }): Promise<Result>` — the same `Result` union `customerSignIn` returns.

- [ ] **Step 1: Write the failing tests**

Cover, at minimum:
- a valid code completes: `mp_customer_session` is set, and `ensureCustomerProfile`/`ensureLoyaltyEnrollment` run exactly as on the password path;
- an invalid code returns a truthful, non-generic message and sets **no** cookie;
- the authenticator code never appears in any `console.error` argument;
- an `AuthBffCustomerError` from the TOTP call returns the generic "temporarily unavailable" message, not the internal string;
- with the flag unset, `confirmCustomerTotp` is unreachable from the form (the GIP path never yields `totp_required`).

- [ ] **Step 2: Run and watch them fail**

Run: `cd apps/storefront && npx vitest run app/sign-in`
Expected: FAIL — `confirmCustomerTotp` is not exported.

- [ ] **Step 3: Implement `confirmCustomerTotp`**

Everything after credential verification — host resolution, `resolveStore`, `encodeSession`, the cookie with `domain: cookieHost`, the profile and loyalty side effects, the referral-cookie burn — is **identical** to `customerSignIn`. Extract that tail into one private helper and call it from both, rather than duplicating it; a second copy will drift.

- [ ] **Step 4: Change the `totp_required` case to surface the step**

It must stop returning a dead-end message and instead return the data the form needs to render the code entry step. Keep the `handoff` case exactly as it is — that one is still a genuine dead end.

- [ ] **Step 5: Add the code entry step to `CustomerSignInForm`**

Follow `SignInForm.tsx`'s shape. Requirements: `inputMode="numeric"`, `autoComplete="one-time-code"`, a labelled input, the error rendered accessibly, and a way back to the password step. Honour the storefront's design tokens — paper/ink/moss, no new colours.

- [ ] **Step 6: Run the full suite and typecheck**

Run: `cd apps/storefront && npx vitest run && npx tsc --noEmit`

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "feat(storefront): let a customer finish sign-in with an authenticator code"
```

---

### Task 3: Stop the Google buttons leading customers into GIP under the Zitadel flag

The storefront has three entry points that redirect to `mark8ly.com/auth/google`, the GIP trampoline:
- `apps/storefront/components/auth/CustomerSignInForm.tsx:98` (`handleGoogle`)
- `apps/storefront/components/auth/CreateAccountForm.tsx:94` (`handleGoogle`)
- `apps/storefront/app/account/security/SecurityClient.tsx:52` (`handleLinkGoogle`)

Under `NEXT_PUBLIC_AUTH_PROVIDER=zitadel` that trampoline authenticates against GIP, which is the store we are migrating off. Google-through-Zitadel is phase 3c-2 and does not exist yet. Until it does, these must not be offered.

`CustomerSignInForm` already has the `AUTH_PROVIDER` constant (line 16); the other two files do not and need the identical ternary, copied verbatim.

**Hide the control entirely** rather than disabling it — a disabled "Continue with Google" invites support tickets. On `/account/security`, where the panel explains linked sign-in methods, keep the panel and omit only the Google link action.

**Files:**
- Modify: all three files above
- Modify/Create: their test files

**Interfaces:**
- Consumes: `process.env.NEXT_PUBLIC_AUTH_PROVIDER`, via the exact ternary in `CustomerSignInForm.tsx:16`.

- [ ] **Step 1: Write the failing tests**

For each of the three components, a pair:
- flag unset → the Google control renders and still points at `mark8ly.com/auth/google` (proves the GIP path is untouched);
- flag `"zitadel"` → the control is absent from the tree.

Plus one guard test: flag `"Zitadel"` (wrong case) behaves as GIP.

- [ ] **Step 2: Run and watch them fail**

Run: `cd apps/storefront && npx vitest run components/auth app/account`
Expected: FAIL — the control renders in both cases.

- [ ] **Step 3: Add the constant to the two files that lack it and gate the controls**

Copy the ternary and its explanatory comment from `CustomerSignInForm.tsx:11-17`. Do not export it from one component and import it into another — these are client components with their own env reads; matching the existing local-constant pattern keeps them independently readable.

- [ ] **Step 4: Run the full suite and typecheck**

Run: `cd apps/storefront && npx vitest run && npx tsc --noEmit`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "fix(storefront): don't offer Google sign-in while it still means GIP"
```

---

### Task 4: Record what 3c-1 closed and what 3c-2 still owes

**Files:**
- Modify: `services/auth-bff/internal/zitadellogin/README.md`

- [ ] **Step 1: Update the deferred-items sections**

Three entries in that file now describe closed work and must say so: the customer-TOTP "KNOWN LIMITATION" (Task 2 closes the TOTP half; the `handoff` half stays open), and "Carried from Phase 3a: `exchange-code.ts` Has No `kind` Field" (Task 1 closes it). Rewrite them as closed with the commit that closed each; do not delete the history.

Then state plainly what 3c-2 still owes: Google sign-in through Zitadel's existing org IDP (`386381087862948767`, TESSERIX org, auto-creation on, email auto-linking), an Apple IDP, and re-enabling the three storefront Google controls once that path works.

Note for accuracy: the Zitadel Google IDP's OAuth client id differs from the apps' `NEXT_PUBLIC_GOOGLE_CLIENT_ID`, but both are in Google project `849928263410` and Google's `sub` is stable per project, so IDP links still resolve; email auto-linking is a second backstop.

- [ ] **Step 2: Commit**

```bash
git add -A && git commit -m "docs(auth-bff): record what phase 3c-1 closed and what 3c-2 still owes"
```
