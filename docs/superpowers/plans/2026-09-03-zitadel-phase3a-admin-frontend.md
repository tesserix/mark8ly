# Zitadel Migration Phase 3a — Merchant Admin Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `apps/admin` sign a merchant in through `auth-bff`'s Zitadel endpoints instead of calling Google Identity Platform from the browser — behind a flag that is off by default, with GIP remaining the live path.

**Architecture:** The GIP path is untouched. A parallel path is added: the browser posts credentials to a new server action, which calls `POST /auth/zitadel/login`, normalises the response, and returns the *same* `SignInSuccess` shape the existing `SignInForm` already handles. The OIDC authorization request that Zitadel requires is obtained by redirecting `/login` to Zitadel and being sent back with `?authRequest=`.

**Tech Stack:** Next.js App Router (server actions), React, TypeScript, Vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-09-03-zitadel-migration-design.md` (see D3 and D10)

## Global Constraints

- **Scope is `apps/admin` only.** The storefront and its trampoline are phase 3b, because customer login never touches `auth-bff` (spec D10). Do not touch `apps/storefront` or `apps/onboarding`.
- **GIP stays live and untouched.** Every existing GIP call site, server action and test must still work identically. The flag defaults to GIP.
- **Do not change `services/auth-bff`.** Phase 2 shipped it and it is merged. If the contract seems wrong, report BLOCKED rather than editing it — a change there is a separate, reviewed piece of work.
- **The response envelope is inconsistent and must be normalised in exactly one place.** See Task 1. Three shapes exist today:
  - `POST /auth/auto-login` success → `{"data": {uid, email, tenant_id, mfa_required?, email_otp_required?}}`
  - `POST /auth/otp/verify` success → `{uid, tenant_id}` **top-level, no `data`**
  - every error → `{"error": "...", "message": "..."}` **flat**
  and phase 2 added a fourth: `POST /auth/zitadel/login` returns `totp_required`, `session_id`, `session_token`, `handoff_url` and `callback_url` **top-level**, while its step-up flags come back nested as `{"data": {..., mfa_required, email_otp_required}}`.
- **Two different second factors now exist and they are not the same thing.** Zitadel's own TOTP arrives as top-level `totp_required`; auth-bff's `usermfa` gate arrives as nested `data.mfa_required`. A client that handles one and not the other reproduces #493/#502, where auth-bff returned `email_otp_required`, the client read only `mfa_required`, and no merchant could log in.
- **Beware the stacked `data` envelopes.** The server action's own `Result<T>` is `{ok: true, data: T}`, and auth-bff's body is also `{data: {...}}`. `r.data.mfaRequired` in `SignInForm` reads the *server action's* envelope, not auth-bff's. Do not collapse them.
- **Never log or return a credential.** Not the password, not `session_token`, not the Zitadel PAT.

---

### Task 1: One normaliser for every auth-bff login response

Today each caller parses a slightly different envelope inline. Adding a fifth shape without consolidating is how the next login outage happens.

**Files:**
- Create: `apps/admin/lib/auth/login-response.ts`
- Test: `apps/admin/lib/auth/login-response.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces:

```ts
export type LoginOutcome =
  | { kind: "complete"; uid: string; email: string; tenantId: string; callbackUrl?: string }
  | { kind: "totp_required"; sessionId: string; sessionToken: string }   // Zitadel's own TOTP
  | { kind: "mfa_required" }                                             // auth-bff usermfa gate
  | { kind: "email_otp_required" }
  | { kind: "handoff"; handoffUrl: string };

export function parseLoginResponse(body: unknown): LoginOutcome;
```

Task 3 consumes `parseLoginResponse`.

- [ ] **Step 1: Write the failing tests**

```ts
import { describe, it, expect } from "vitest";
import { parseLoginResponse } from "./login-response";

describe("parseLoginResponse", () => {
  it("reads the nested auto-login success envelope", () => {
    expect(parseLoginResponse({ data: { uid: "u1", email: "a@b.test", tenant_id: "t1" } }))
      .toEqual({ kind: "complete", uid: "u1", email: "a@b.test", tenantId: "t1" });
  });

  it("reads mfa_required from INSIDE data, not the top level", () => {
    // Regression guard: #493/#502 were caused by reading the wrong level.
    expect(parseLoginResponse({ data: { uid: "u1", email: "a@b.test", tenant_id: "t1", mfa_required: true } }))
      .toEqual({ kind: "mfa_required" });
  });

  it("reads email_otp_required from INSIDE data", () => {
    expect(parseLoginResponse({ data: { uid: "u1", email: "a@b.test", tenant_id: "t1", email_otp_required: true } }))
      .toEqual({ kind: "email_otp_required" });
  });

  it("reads Zitadel's totp_required from the TOP level, not from data", () => {
    // Zitadel's own TOTP and auth-bff's usermfa gate are different mechanisms
    // returned at different nesting levels. Both must be handled.
    expect(parseLoginResponse({ totp_required: true, session_id: "s1", session_token: "tok" }))
      .toEqual({ kind: "totp_required", sessionId: "s1", sessionToken: "tok" });
  });

  it("reads a handoff", () => {
    expect(parseLoginResponse({ handoff_url: "https://auth.tesserix.app/ui/v2/login", auth_request_id: "V2_1" }))
      .toEqual({ kind: "handoff", handoffUrl: "https://auth.tesserix.app/ui/v2/login" });
  });

  it("carries callback_url through on a completed Zitadel login", () => {
    const out = parseLoginResponse({
      callback_url: "https://admin.mark8ly.com/auth/callback?code=c&state=s",
      data: { uid: "u1", email: "a@b.test", tenant_id: "t1" },
    });
    expect(out).toEqual({
      kind: "complete", uid: "u1", email: "a@b.test", tenantId: "t1",
      callbackUrl: "https://admin.mark8ly.com/auth/callback?code=c&state=s",
    });
  });

  it("prefers a step-up over completion when both are somehow present", () => {
    // Fail closed: never treat a login as done while a factor is outstanding.
    const out = parseLoginResponse({
      callback_url: "https://admin.mark8ly.com/auth/callback?code=c",
      data: { uid: "u1", email: "a@b.test", tenant_id: "t1", mfa_required: true },
    });
    expect(out.kind).toBe("mfa_required");
  });

  it("throws on an unrecognisable body rather than guessing", () => {
    expect(() => parseLoginResponse({ something: "else" })).toThrow();
    expect(() => parseLoginResponse(null)).toThrow();
  });

  it("does not treat a false flag as a step-up", () => {
    expect(parseLoginResponse({ data: { uid: "u1", email: "a@b.test", tenant_id: "t1", mfa_required: false } }).kind)
      .toBe("complete");
  });
});
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd apps/admin && npx vitest run lib/auth/login-response.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

Create `apps/admin/lib/auth/login-response.ts`. Order matters: step-ups are checked before completion so a body carrying both fails closed.

```ts
/**
 * Normalises every shape auth-bff returns on a login path into one union.
 *
 * Four shapes exist, for historical reasons rather than good ones:
 *   - /auth/auto-login success  -> { data: { uid, email, tenant_id, mfa_required?, email_otp_required? } }
 *   - /auth/otp/verify success  -> { uid, tenant_id }            (top level, no data)
 *   - any error                 -> { error, message }            (flat)
 *   - /auth/zitadel/login       -> { totp_required, session_id, session_token } or
 *                                  { handoff_url, auth_request_id } or
 *                                  { callback_url, data: { ... } }
 *
 * Two DIFFERENT second factors are represented here. Zitadel's own TOTP arrives
 * as top-level `totp_required`; auth-bff's usermfa gate arrives as nested
 * `data.mfa_required`. Handling one and not the other is exactly the defect that
 * took merchant login down in #493/#502 — auth-bff said email_otp_required, the
 * client read only mfa_required, and the code-entry screen never rendered.
 */
export type LoginOutcome =
  | { kind: "complete"; uid: string; email: string; tenantId: string; callbackUrl?: string }
  | { kind: "totp_required"; sessionId: string; sessionToken: string }
  | { kind: "mfa_required" }
  | { kind: "email_otp_required" }
  | { kind: "handoff"; handoffUrl: string };

export class LoginResponseError extends Error {}

export function parseLoginResponse(body: unknown): LoginOutcome {
  if (typeof body !== "object" || body === null) {
    throw new LoginResponseError("login response was not an object");
  }
  const top = body as Record<string, unknown>;
  const data = (typeof top.data === "object" && top.data !== null
    ? (top.data as Record<string, unknown>)
    : {}) as Record<string, unknown>;

  // Step-ups first: a body carrying both a factor requirement and a completion
  // must never be read as complete.
  if (top.totp_required === true) {
    const sessionId = typeof top.session_id === "string" ? top.session_id : "";
    const sessionToken = typeof top.session_token === "string" ? top.session_token : "";
    if (!sessionId || !sessionToken) {
      throw new LoginResponseError("totp_required without a session to continue");
    }
    return { kind: "totp_required", sessionId, sessionToken };
  }
  if (data.mfa_required === true) return { kind: "mfa_required" };
  if (data.email_otp_required === true) return { kind: "email_otp_required" };

  if (typeof top.handoff_url === "string" && top.handoff_url) {
    return { kind: "handoff", handoffUrl: top.handoff_url };
  }

  const uid = typeof data.uid === "string" ? data.uid : typeof top.uid === "string" ? top.uid : "";
  const tenantId =
    typeof data.tenant_id === "string" ? data.tenant_id
    : typeof top.tenant_id === "string" ? top.tenant_id : "";
  const email = typeof data.email === "string" ? data.email : "";
  if (!uid || !tenantId) {
    throw new LoginResponseError("login response carried neither a step-up nor an identity");
  }
  const callbackUrl = typeof top.callback_url === "string" ? top.callback_url : undefined;
  return { kind: "complete", uid, email, tenantId, ...(callbackUrl ? { callbackUrl } : {}) };
}
```

- [ ] **Step 4: Run the tests**

Run: `cd apps/admin && npx vitest run lib/auth/login-response.test.ts`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/lib/auth/login-response.ts apps/admin/lib/auth/login-response.test.ts
git commit -m "feat(admin): normalise every auth-bff login response shape in one place"
```

---

### Task 2: Client functions for the Zitadel endpoints

**Files:**
- Modify: `apps/admin/lib/auth/auth-bff.ts`
- Test: `apps/admin/lib/auth/auth-bff.zitadel.test.ts`

**Interfaces:**
- Consumes: Task 1's `parseLoginResponse`.
- Produces:

```ts
export async function zitadelLogin(req: {
  authRequestId: string; loginName: string; password: string; workspaceTenant: string;
  userAgent?: string; forwardedFor?: string;
}): Promise<{ outcome: LoginOutcome; setCookies: string[] }>;

export async function zitadelTotp(req: {
  authRequestId: string; sessionId: string; sessionToken: string; code: string; workspaceTenant: string;
}): Promise<{ outcome: LoginOutcome; setCookies: string[] }>;
```

Task 3 consumes both.

- [ ] **Step 1: Write the failing tests**

Mock `fetch`. Assert: the request body uses the snake_case field names auth-bff expects (`auth_request_id`, `login_name`, `password`, `workspace_tenant`); a 401 raises `AuthBffError` with the flat `{error, message}` shape; `Set-Cookie` headers are collected via the existing `readAllSetCookies` helper; and the parsed outcome comes back. Match the style of the existing tests in this directory — if there are none for `auth-bff.ts`, use the repo's Vitest conventions from `apps/admin/lib/auth/sign-in-href.test.ts`.

Include one test asserting the **password is not present in any thrown error's message**.

- [ ] **Step 2: Run to verify they fail**

Run: `cd apps/admin && npx vitest run lib/auth/auth-bff.zitadel.test.ts`
Expected: FAIL — functions not exported.

- [ ] **Step 3: Implement**

Add both functions to `apps/admin/lib/auth/auth-bff.ts`, following the file's existing idiom exactly: `fetch` against `config.authBffUrl`, `cache: "no-store"`, non-2xx → `AuthBffError(res.status, body.error ?? …, body.message ?? …)`, success → `parseLoginResponse(await res.json())`. Forward `User-Agent` and `X-Forwarded-For` from the incoming request so `deviceguard` and the email-OTP limiter see the real client — phase 2 shipped a fix for exactly this, and dropping them here re-creates it on the browser side.

- [ ] **Step 4: Run the tests**

Run: `cd apps/admin && npx vitest run lib/auth/`
Expected: all PASS, including pre-existing tests.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/lib/auth/
git commit -m "feat(admin): add auth-bff client functions for the Zitadel login endpoints"
```

---

### Task 3: Server actions for the Zitadel path

**Files:**
- Modify: `apps/admin/app/login/actions.ts`
- Test: `apps/admin/app/login/actions.zitadel.test.ts`

**Interfaces:**
- Consumes: Task 2's `zitadelLogin` / `zitadelTotp`.
- Produces: server actions `signInWithZitadel(input)` and `confirmZitadelTotp(input)`, both returning the **existing** `Result<SignInSuccess>` union so `SignInForm` needs no new result shape.

- [ ] **Step 1: Write the failing tests**

Cover, with `zitadelLogin` mocked:
- a complete outcome returns `{ok: true, data: {mfaRequired: false, emailOtpRequired: false, …}}` and forwards `Set-Cookie`s the same way the GIP path does;
- an `mfa_required` outcome returns `mfaRequired: true` and **no** `callbackUrl`;
- an `email_otp_required` outcome returns `emailOtpRequired: true`;
- a `totp_required` outcome surfaces the session so the UI can collect a code, and asserts **no session cookie was minted**;
- an `AuthBffError` maps to `{ok: false, code, message}` via the existing `fail()` helper;
- the returned message never contains the submitted password.

- [ ] **Step 2: Run to verify they fail**

Run: `cd apps/admin && npx vitest run app/login/actions.zitadel.test.ts`
Expected: FAIL.

- [ ] **Step 3: Implement**

`signInWithZitadel({ email, password, authRequestId })`:
1. Resolve `workspaceTenant` exactly as `signIn` does today — `listMemberTenants` plus the host-slug-aware primary selection. **Reuse that code, do not duplicate it**; extract a helper if needed so the two paths cannot diverge on which tenant they pick.
2. Call `zitadelLogin`.
3. Map the `LoginOutcome` onto `SignInSuccess`, preserving the field names `SignInForm` already reads (`mfaRequired`, `emailOtpRequired`, `multipleTenants`).
4. Forward `Set-Cookie` headers to the browser the same way `signIn` does.

`confirmZitadelTotp({ authRequestId, sessionId, sessionToken, code })` calls `zitadelTotp` and maps the same way.

- [ ] **Step 4: Run the tests**

Run: `cd apps/admin && npx vitest run app/login/ && npx tsc --noEmit`
Expected: PASS and no type errors.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/app/login/
git commit -m "feat(admin): server actions for the Zitadel login and TOTP steps"
```

---

### Task 4: Get an auth request — `/login` redirect and `/auth/callback`

Zitadel's login-client model needs an `auth_request_id`, which only exists after Zitadel's `/authorize` sends the browser back. Today `/login` is entered directly, so this is a genuinely new step.

**Files:**
- Modify: `apps/admin/app/login/page.tsx`
- Create: `apps/admin/app/auth/callback/route.ts`
- Test: `apps/admin/app/auth/callback/route.test.ts`

**Interfaces:**
- Consumes: Task 6's provider flag.
- Produces: `/login?authRequest=V2_…` reaching `SignInForm` as a prop; `GET /auth/callback` completing the OIDC round trip.

- [ ] **Step 1: Understand the flow before writing code**

Zitadel is configured with `loginBaseUri = https://admin.mark8ly.com` and appends `/login` itself, so:

```
browser -> /login                        (no authRequest, provider=zitadel)
        -> 302 to Zitadel /oauth/v2/authorize?client_id=…&redirect_uri=…/auth/callback&…
Zitadel -> 302 back to /login?authRequest=V2_…
user    -> submits credentials -> signInWithZitadel(authRequestId)
auth-bff-> mints m8_session AND returns callback_url
browser -> follows callback_url -> /auth/callback?code=…&state=…
        -> 303 to the post-login destination
```

The session is already minted before `/auth/callback` runs. The callback exists to complete Zitadel's side of the flow and to land the user somewhere; it does **not** need to exchange the code for our session.

- [ ] **Step 2: Write the failing route tests**

Cover: a request with a valid `state` redirects (303) to the sanitised destination; a request whose `state` does not match the cookie is rejected; an **open-redirect attempt** in the destination is rejected — reuse the existing `returnUrl` sanitiser from `app/login/page.tsx` rather than writing a second one.

- [ ] **Step 3: Implement `/auth/callback`**

Verify `state` against an httpOnly cookie set at redirect time, then 303 to the sanitised destination. Do not exchange the code. Do not trust any host from the query string.

- [ ] **Step 4: Implement the `/login` redirect**

In `app/login/page.tsx`, when the provider is Zitadel and no `authRequest` param is present, redirect to Zitadel's `/authorize` with `client_id`, `redirect_uri`, `response_type=code`, `scope=openid`, PKCE parameters and a `state` that carries the sanitised `returnUrl`. Store the PKCE verifier and state in httpOnly cookies. When `authRequest` **is** present, pass it to `SignInForm` and render as normal.

- [ ] **Step 5: Verify**

Run: `cd apps/admin && npx vitest run app/auth/callback/ && npx tsc --noEmit && npm run build`
Expected: PASS, no type errors, build succeeds.

- [ ] **Step 6: Commit**

```bash
git add apps/admin/app/login/page.tsx apps/admin/app/auth/callback/
git commit -m "feat(admin): obtain a Zitadel auth request and complete the OIDC callback"
```

---

### Task 5: Branch `SignInForm` on the provider

**Files:**
- Modify: `apps/admin/components/auth/SignInForm.tsx`

**Interfaces:**
- Consumes: Tasks 3 and 4.
- Produces: no new exports; a `provider` and `authRequestId` prop.

- [ ] **Step 1: Implement the branch**

In `onValid`, when the provider is Zitadel, skip `signInWithPassword` entirely and call `signInWithZitadel({ email, password, authRequestId })`. Everything after that — the `mfaRequired`/`emailOtpRequired` branch, `goToDestination`, the error rendering — is unchanged, because Task 3 returns the same `SignInSuccess` shape.

Add one new screen state for Zitadel's own TOTP (`kind: "totp_required"`), reusing the existing code-entry UI. Submit it with `confirmZitadelTotp`. Under GIP this state is unreachable.

Hide the Google and Apple buttons when the provider is Zitadel — those paths still call GIP and are out of scope for 3a.

- [ ] **Step 2: Verify**

Run: `cd apps/admin && npx tsc --noEmit && npm run build && npx vitest run`
Expected: clean.

- [ ] **Step 3: Confirm the GIP path is untouched**

Run: `cd apps/admin && npx playwright test tests/e2e/sign-in.spec.ts`
Expected: both existing tests pass unchanged. If either needed editing, the GIP path was altered — revert and redo.

- [ ] **Step 4: Commit**

```bash
git add apps/admin/components/auth/SignInForm.tsx
git commit -m "feat(admin): sign in through Zitadel when the provider flag selects it"
```

---

### Task 6: The provider flag, defaulting to GIP

**Files:**
- Modify: `apps/admin/lib/config.ts`
- Modify: `charts/apps/mark8ly-admin/values.yaml` and `templates/deployment.yaml` in `tesserix-k8s` — **note this is the other repo**; if that is out of scope for this run, stop after the app change and report it as a follow-up rather than editing a repo the plan did not set up.

**Interfaces:**
- Produces: `publicConfig.authProvider: "gip" | "zitadel"`, defaulting to `"gip"`.

- [ ] **Step 1: Add the flag**

```ts
// Which identity provider the login screen drives. Defaults to GIP: Zitadel is
// opt-in until the phase-6 cutover, and an unset variable must never silently
// switch the live login path.
authProvider: (process.env.NEXT_PUBLIC_AUTH_PROVIDER === "zitadel" ? "zitadel" : "gip") as "gip" | "zitadel",
```

Plus `NEXT_PUBLIC_ZITADEL_ISSUER` and `NEXT_PUBLIC_ZITADEL_ADMIN_CLIENT_ID`, both optional and unread unless the provider is Zitadel.

- [ ] **Step 2: Verify the default**

Add a Vitest case asserting `authProvider === "gip"` when `NEXT_PUBLIC_AUTH_PROVIDER` is unset **and** when it is set to any unrecognised value.

- [ ] **Step 3: Verify and commit**

Run: `cd apps/admin && npx vitest run && npm run build`

```bash
git add apps/admin/lib/config.ts
git commit -m "feat(admin): add the auth provider flag, defaulting to GIP"
```

---

## Phase 3a completion criteria

- With `NEXT_PUBLIC_AUTH_PROVIDER` unset, `apps/admin` behaves exactly as today and both existing Playwright sign-in tests pass unedited.
- `parseLoginResponse` handles all five outcomes, with regression tests naming #493/#502 for the nesting-level trap.
- A step-up is never read as a completed login, even if a body carries both.
- `npx tsc --noEmit`, `npm run build` and the full Vitest suite are clean.

## Not in this phase

The storefront, the `/auth/google` trampoline, and the customer login path that skips the FGA check (all phase 3b, spec D10). Google and Apple sign-in under Zitadel. Deleting any GIP code — that is the phase 6 cutover.
