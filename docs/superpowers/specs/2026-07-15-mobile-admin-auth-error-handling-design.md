# mobile-admin — auth error handling — Design

**Date:** 2026-07-15
**Status:** Approved
**Scope:** `apps/mobile-admin` + `packages/mobile-shared`

## Goal

Never show a user a raw native/Firebase error string, never show an error for an action
they cancelled, and never sign a user out without telling them why.

## Problems (all verified in code, 2026-07-15)

### P1 — Three error mappers, two of them naive passthroughs

| File | Current | Verdict |
|---|---|---|
| `apps/mobile-admin/app/login.tsx:14` | `e.message` passthrough | broken |
| `apps/mobile-admin/components/auth/LinkAccountPrompt.tsx:26` | `e.message` passthrough | broken |
| `apps/mobile-admin/app/(tabs)/more/security.tsx` | real `errorMessage()` code-mapper | good; becomes the seed of the shared one |

Observed on a simulator with no Apple ID signed in — the login screen rendered, verbatim:

```
RequestUnknownException: The authorization attempt failed for an unknown reason
(at ExpoAppleAuthentication/AppleAuthenticationExceptions.swift:61)
```

Corollary: **cancelling** the Google or Apple sheet throws (`ERR_REQUEST_CANCELED`), so
backing out of the sheet currently shows the user an error for doing nothing wrong.

### P2 — Silent sign-out

`apps/mobile-admin/lib/api-client.ts:41`:

```ts
onUnauthorized: async () => {
  await signOut();   // no message, anywhere
},
```

Chain: social sign-in succeeds (GIP auto-provisions the Firebase user) → `app/_layout.tsx:53`
sees `user` → routes to tabs → `TenantGate` → `useTenantResolver` → `/stores` → API rejects an
identity with no merchant record → `onUnauthorized` → `signOut()` → `_layout` sees `!user` →
`router.replace('/login')`. The user lands back on login with **zero explanation**, and
`TenantGate`'s intended "No store yet → create one on mark8ly.com" screen is bypassed.

### P3 — `auth/invalid-credential` is ambiguous, and the prescribed fix was never built

Mark8ly GIP tenants (`MP-Internal-e986p`) run **email-enumeration protection**, so:

- `auth/wrong-password` **never fires** — any branch on it is dead code.
- `auth/invalid-credential` means **both** "wrong password" **and** "expired OAuth credential".
  Mapping it to "expired, start over" makes a mistyped password unrecoverable.

A prior review prescribed tagging by *which call threw*. **Verified 2026-07-15: never
implemented.** No `reauth` string exists in `packages/mobile-shared/auth/link.ts` or anywhere
under `apps/mobile-admin/`. A shared mapper is therefore *unsafe* without the tagging — the
code alone cannot disambiguate.

## Key insight — the 401 distinction already exists

`packages/mobile-shared/api/client.ts`:

- **line 85** — no token at all → `onUnauthorized()` → throws `"Not authenticated"`
- **lines 90–99** — 401 → refresh → retry → still 401 → `onUnauthorized()` → throws `"Session expired"`
- **line 103** — `if (init.isStoreScoped && (status === 403 || status === 404))` → `onTenantInvalid`

Two consequences:

1. **`"Session expired"` is factually wrong.** In that branch we minted a *fresh* token and the
   server still refused. Fresh token + 401 = **access denied**, not expiry. So `no-session` vs
   `access-denied` is derivable from existing control flow — **no time-since-sign-in heuristic**.
2. **`/stores` is not store-scoped** (it is the request that *discovers* stores), so a 403 there
   misses `onTenantInvalid` entirely and surfaces as `TenantGate`'s "Check your connection and
   try again" — wrong copy for a permissions failure.

**Unverified:** whether the API 401s or 403s for a valid token with no membership. The design
handles **both** correctly rather than betting on one.

## Design

### C1 — `packages/mobile-shared/auth/errors.ts` (exists; stays firebase-free)

Already holds `LastSignInMethodError`. The mapper only reads `.code` strings, so **no firebase
import** — this preserves the Expo Go / demo isolation invariant (see landmines).

```ts
export interface AuthErrorContext {
  /** Which method the user re-authenticated with. Absent = password. */
  provider?: "google.com" | "apple.com";
}

/** Returns null when the user cancelled — callers show NOTHING. Never returns e.message raw. */
export function authErrorMessage(e: unknown, ctx?: AuthErrorContext): string | null;
```

**Contract:** returns `null` **only** for cancellation. Never returns a raw `e.message`.

**Disambiguation is by tag, never by code** (per P3): the *only* thing that distinguishes
"wrong password" from "expired credential" is C2's `auth/reauth-failed` tag. There is
deliberately **no `step` field** — a `step` parameter would be a second, competing mechanism
for the same decision, and the two would drift.

| Condition | Message |
|---|---|
| `AuthCancelledError` (see C1a); Apple `ERR_REQUEST_CANCELED` (safety net) | `null` (show nothing) |
| Apple `ERR_REQUEST_UNKNOWN` | Couldn't complete Apple sign-in. Make sure you're signed in to iCloud on this device. |
| `auth/reauth-failed` **and** no `ctx.provider` (password re-auth) | That password is incorrect. |
| `auth/reauth-failed` **and** `ctx.provider` is set (social re-auth) | Couldn't verify that account. Try again. |
| `auth/invalid-credential` (i.e. **not** from re-auth — untagged) | Couldn't sign you in. Check your details and try again. |
| `auth/credential-already-in-use` | That account is already linked to a different Mark8ly account. |
| `auth/provider-already-linked` | That's already linked to your account. |
| `auth/requires-recent-login` | For security, sign out and sign in again, then retry. |
| `auth/network-request-failed` | No connection. Check your network and try again. |
| `auth/too-many-requests` | Too many attempts. Try again in a few minutes. |
| `LastSignInMethodError` | You can't remove your only sign-in method. |
| fallback | Something went wrong. Try again. |

**Never** map `auth/invalid-credential` to anything mentioning "expired" — that is the
unrecoverable loop described in P3. **Never** branch on `auth/wrong-password` — it never fires.

Guard the `.code` read with `typeof e === "object" && e !== null` (a `throw null` from a native
module must not make the mapper itself throw).

### C1a — Cancellation must be normalised at the native boundary

**Verified 2026-07-15 (this corrects the approved design):** `GoogleSignin.signIn()` **does not
throw on cancel.** `node_modules/@react-native-google-signin/google-signin/src/signIn/GoogleSignin.ts:60`
routes the native rejection through `translateCancellationError`, which **returns**
`cancelledResult` = `{ type: "cancelled", data: null }`.

`apps/mobile-admin/lib/social-auth.ts:signInWithGoogleNative` then finds no `idToken` and throws
`new Error("Google sign-in failed: no ID token")` — a **plain Error with no `code`**. Consequences:

1. Cancelling Google today shows the user `"Google sign-in failed: no ID token"`.
2. A mapper row keyed on `SIGN_IN_CANCELLED` **would never fire**, and `SIGN_IN_CANCELLED` is a
   *native runtime constant* (`NativeModule.getConstants()`) — matching it would force a native
   import into `errors.ts` and break its native-free invariant.

**Therefore:** cancellation is translated to a domain error at the native boundary, and
`errors.ts` owns the sentinel (no native import):

```ts
// packages/mobile-shared/auth/errors.ts
/** The user dismissed a native sign-in sheet. Callers show NOTHING. */
export class AuthCancelledError extends Error {
  constructor() {
    super("Sign-in cancelled");
    this.name = "AuthCancelledError";
  }
}
```

`apps/mobile-admin/lib/social-auth.ts`:

- `signInWithGoogleNative` — when `result.type === "cancelled"`, throw `new AuthCancelledError()`
  **before** the no-idToken check.
- `signInWithAppleNative` — catch `code === "ERR_REQUEST_CANCELED"` and rethrow
  `new AuthCancelledError()`; let everything else propagate.

`errors.ts` maps `AuthCancelledError` → `null`, and keeps a bare `ERR_REQUEST_CANCELED` → `null`
row as a safety net for any raw Apple error that bypasses the wrapper.

### C2 — Reauth tagging in `packages/mobile-shared/auth/link.ts`

Wrap **only** the re-authentication call in `completeLinkWithPassword` / `completeLinkWithGoogle`
/ `completeLinkWithApple`; rethrow tagged with `code: "auth/reauth-failed"` preserving `cause`.
Let the subsequent `linkWithCredential` errors propagate **untouched**.

This tag is the **sole** disambiguator for C1's re-auth rows, and is the design the prior review
prescribed but which was never built. Without it, a shared mapper cannot safely map
`auth/invalid-credential` at all.

`LinkAccountPrompt` passes `{ provider }` to `authErrorMessage` **only** on its social re-auth
paths, so C1 picks the right copy; the password path passes no context.

### C3 — Explained sign-out

`packages/mobile-shared/api/client.ts`:

```ts
export type UnauthorizedReason = "no-session" | "access-denied";
onUnauthorized?: (reason: UnauthorizedReason) => void | Promise<void>;
```

- line 85 (no token) → `"no-session"`
- lines 97–99 (refreshed, still 401) → `"access-denied"`; **also fix the `ApiError` message** —
  `"Session expired"` → `"Access denied"`.

New `packages/mobile-shared/stores/auth-notice.ts` — a tiny zustand store mirroring the existing
`stores/tenant-store.ts` pattern:

```ts
type AuthNotice = "no-session" | "access-denied";
{ notice: AuthNotice | null; setNotice(n): void; clearNotice(): void }
```

`apps/mobile-admin/lib/api-client.ts` — `onUnauthorized: async (reason) => { setNotice(reason); await signOut(); }`.

`apps/mobile-admin/app/login.tsx` — on mount, read the notice, render it, **clear it** (so it
does not survive into the next sign-in attempt).

| Reason | Copy |
|---|---|
| `access-denied` | That account doesn't have access to a Mark8ly admin account. |
| `no-session` | Your session ended. Sign in again. |

**Copy rationale:** `access-denied` speaks to *authorization*, never account existence — safe
under enumeration protection.

### C4 — Call sites adopt the shared mapper

`login.tsx`, `LinkAccountPrompt.tsx` (with `step: "reauth"` on the re-auth path), and
`security.tsx` all delete their local mappers and call `authErrorMessage`. Every call site:

```ts
const msg = authErrorMessage(e, ctx);
if (msg) setError(msg);   // null → show nothing (cancel)
```

### C5 — `TenantGate` 403

A permissions failure loading `/stores` renders access copy, not "Check your connection and try
again." Keep the existing retry affordance for genuine network/5xx errors.

## Out of scope

- Verifying which status the API returns for a valid token with no membership (design covers
  both). If it proves to be a backend defect, the real fix is server-side and this is the safety net.
- Apple sign-in device verification (parked — needs a device build).
- The `rawNonce: ""` question in `signInWithAppleNative()`.
- `extra.eas.projectId` placeholder.

## Testing

All tests live in `apps/mobile-admin/__tests__/` — **never** under `apps/mobile-admin/app/`.

- **`social-auth.test.tsx`** — `signInWithGoogleNative` throws `AuthCancelledError` (not
  "no ID token") when `signIn()` resolves `{ type: "cancelled" }`; `signInWithAppleNative` turns
  `ERR_REQUEST_CANCELED` into `AuthCancelledError` and leaves other errors untouched.
- **`errors.test.tsx`** — mapper table: `AuthCancelledError` → `null`; a tagged `auth/reauth-failed` and a bare
  `auth/invalid-credential` produce *different* copy; `auth/reauth-failed` with vs without
  `ctx.provider` produce *different* copy; no input yields a raw `e.message`; `throw null`
  doesn't throw; **no mapped message contains the word "expired"** (guards the P3 loop).
- **`link.test.tsx`** — reauth failure is tagged `auth/reauth-failed`; a `linkWithCredential`
  failure is **not** tagged (proves only the re-auth call is wrapped).
- **`login.test.tsx`** — cancel shows nothing; a pending notice renders and is cleared.
- **`client.test.ts`** — no token → `no-session`; refreshed-then-401 → `access-denied`.
- **`security.test.tsx`** — existing 7 stay green through the mapper swap.

Baseline: **54/54 jest**. `npx tsc --noEmit` has 2 pre-existing `app/(tabs)/_layout.tsx`
expo-notifications errors — expected.

## Landmines

- `provider.tsx` firebase imports stay `import type`; `require("./gip")` stays lazy.
- `auth/errors.ts` must stay **firebase-free** — `app/(tabs)/more/security.tsx` imports it, and
  expo-router requires every route file at boot.
- A pure `export { X } from "./y"` creates **no local binding** — use `import` + `export {}`.
- jest.mock factories build their mock fns **inside** the factory (babel hoists imports above
  outer `const`).
- Demo backend never touches firebase.
- Do **not** touch: `metro.config.js`, `tsconfig.json`, `jest.config.js`, `babel.config.js`,
  tailwind/nativewind wiring, `app.config.js`, `eas.json`.
- Never run `npm ci` / `npm install` / `npm install --package-lock-only` / `rm -rf node_modules`.
