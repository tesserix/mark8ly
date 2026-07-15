# mobile-admin Auth Error Handling — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Never show a raw native/Firebase error string, never show an error for a cancelled action, and never sign a user out without telling them why.

**Architecture:** One firebase-free mapper (`packages/mobile-shared/auth/errors.ts`) owns every user-facing auth message. Cancellation is normalised to an `AuthCancelledError` at the native boundary (`lib/social-auth.ts`) because the Google SDK *resolves* on cancel rather than throwing. Re-auth failures are tagged `auth/reauth-failed` in `link.ts` — the only way to tell GIP's ambiguous `auth/invalid-credential` apart. `onUnauthorized(reason)` distinguishes a dead session from an access denial, carried to login by a small zustand store.

**Tech Stack:** Expo 56, expo-router, React Native Firebase 24.1.1, zustand v5, jest-expo, `@testing-library/react-native`.

**Spec:** `docs/superpowers/specs/2026-07-15-mobile-admin-auth-error-handling-design.md` (commit `8a4234bd`)
**Base commit:** `8a4234bd`

## Global Constraints

- **Commits:** single-line conventional, **NO signatures**, direct to `main`. Do not create a branch.
- **`packages/mobile-shared/auth/errors.ts` must stay FIREBASE-FREE and NATIVE-FREE.** No `@react-native-firebase/*`, no `@react-native-google-signin/*`, no `expo-apple-authentication` imports — not even type-only. `app/(tabs)/more/security.tsx` imports it and expo-router requires every route file at boot. The mapper only reads `.code` strings and `instanceof` its own classes.
- **`authErrorMessage` must NEVER return `e.message`.** Unknown → `"Something went wrong. Try again."`
- **`authErrorMessage` returns `null` ONLY for cancellation.** Callers: `const msg = authErrorMessage(e, ctx); if (msg) setError(msg);`
- **NEVER map any code to a message containing the word "expired"** for a credential error. GIP tenants run email-enumeration protection: `auth/wrong-password` never fires, and `auth/invalid-credential` means BOTH "wrong password" AND "expired credential". Telling a user who mistyped their password to "start over" makes it unrecoverable.
- **NEVER branch on `auth/wrong-password`** — dead code under enumeration protection.
- **Disambiguation is by tag, never by code.** The `auth/reauth-failed` tag is the ONLY thing separating "wrong password" from "expired credential". There is deliberately **no `step` field** on `AuthErrorContext`.
- **`provider.tsx` firebase imports stay `import type`**; `require("./gip")` stays lazy.
- **A pure `export { X } from "./y"` creates NO local binding** — if a file both re-exports and uses a symbol, use `import { X } from "./y"; export { X };` (two statements). TS and babel accept the pure form silently and it throws `ReferenceError` at runtime.
- **Demo backend never touches firebase.**
- **Tests live in `apps/mobile-admin/__tests__/` ONLY.** A `.test.*` under `apps/mobile-admin/app/` gets bundled by expo-router into the app bundle → runtime redbox.
- **jest.mock factories must build their mock fns INSIDE the factory** (babel hoists imports above outer `const`), and read them back off the imported module.
- **Do NOT touch:** `metro.config.js`, `tsconfig.json`, `jest.config.js`, `babel.config.js`, tailwind/nativewind wiring, `app.config.js`, `eas.json`.
- **NEVER run** `npm ci`, `npm install`, `npm install --package-lock-only`, `rm -rf node_modules` — a metro dev server may be running against this tree.
- **Style:** `StyleSheet` + `theme` + `@/components/ui` primitives, NOT nativewind classes.
- **TypeScript:** explicit types on exports; no `any` (narrow `unknown`); immutability.

**Gates (run from `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin`):**
- `npx jest` → green. Baseline **54/54**.
- `npx tsc --noEmit` → **only** the 2 pre-existing `app/(tabs)/_layout.tsx` expo-notifications errors. Anything else is yours.

## File Structure

| File | Responsibility |
|---|---|
| `packages/mobile-shared/auth/errors.ts` | **modify** — `LastSignInMethodError` (exists), new `AuthCancelledError`, `AuthErrorContext`, `authErrorMessage`. Firebase-free. |
| `apps/mobile-admin/lib/social-auth.ts` | **modify** — normalise cancel → `AuthCancelledError` at the native boundary. |
| `packages/mobile-shared/auth/link.ts` | **modify** — tag re-auth failures `auth/reauth-failed`. |
| `packages/mobile-shared/stores/auth-notice.ts` | **create** — zustand store carrying the sign-out reason. |
| `packages/mobile-shared/api/client.ts` | **modify** — `onUnauthorized(reason)`, fix bogus `"Session expired"`. |
| `apps/mobile-admin/lib/api-client.ts` | **modify** — set notice before `signOut()`. |
| `apps/mobile-admin/app/login.tsx` | **modify** — adopt mapper; render + clear notice. |
| `apps/mobile-admin/components/auth/LinkAccountPrompt.tsx` | **modify** — adopt mapper with `ctx.provider` on social re-auth. Its test also asserts raw passthrough — see Step 1a-ii. |
| `apps/mobile-admin/app/(tabs)/more/security.tsx` | **modify** — delete local `errorMessage`, adopt mapper. |
| `apps/mobile-admin/components/TenantGate.tsx` | **modify** — 403 → access copy, no retry button. |

---

### Task 1: `errors.ts` — `AuthCancelledError` + `authErrorMessage`

**Files:**
- Modify: `packages/mobile-shared/auth/errors.ts`
- Create: `apps/mobile-admin/__tests__/errors.test.tsx`

**Interfaces — Produces:**
- `class AuthCancelledError extends Error`
- `interface AuthErrorContext { provider?: "google.com" | "apple.com" }`
- `function authErrorMessage(e: unknown, ctx?: AuthErrorContext): string | null`

- [ ] **Step 1: Write the failing tests** — create `apps/mobile-admin/__tests__/errors.test.tsx`.

No mocks needed — `errors.ts` is pure.

```tsx
import {
  AuthCancelledError,
  LastSignInMethodError,
  authErrorMessage,
} from "@repo/mobile-shared/auth/errors";

function withCode(code: string): Error {
  return Object.assign(new Error("raw native text that must never be shown"), { code });
}

describe("authErrorMessage", () => {
  it("returns null for a cancelled sheet", () => {
    expect(authErrorMessage(new AuthCancelledError())).toBeNull();
  });

  it("returns null for a raw Apple ERR_REQUEST_CANCELED (safety net)", () => {
    expect(authErrorMessage(withCode("ERR_REQUEST_CANCELED"))).toBeNull();
  });

  it("maps ERR_REQUEST_UNKNOWN to iCloud guidance", () => {
    expect(authErrorMessage(withCode("ERR_REQUEST_UNKNOWN"))).toMatch(/signed in to iCloud/i);
  });

  it("maps LastSignInMethodError to the guard copy", () => {
    expect(authErrorMessage(new LastSignInMethodError())).toMatch(/only sign-in method/i);
  });

  it("maps a TAGGED reauth failure without a provider to wrong-password copy", () => {
    expect(authErrorMessage(withCode("auth/reauth-failed"))).toMatch(/password is incorrect/i);
  });

  it("maps a TAGGED reauth failure WITH a provider to account copy, not password copy", () => {
    const msg = authErrorMessage(withCode("auth/reauth-failed"), { provider: "google.com" });
    expect(msg).toMatch(/couldn't verify that account/i);
    expect(msg).not.toMatch(/password/i);
  });

  it("gives an UNTAGGED invalid-credential different copy from a tagged reauth failure", () => {
    const untagged = authErrorMessage(withCode("auth/invalid-credential"));
    const tagged = authErrorMessage(withCode("auth/reauth-failed"));
    expect(untagged).toMatch(/check your details/i);
    expect(untagged).not.toEqual(tagged);
  });

  it("maps credential-already-in-use", () => {
    expect(authErrorMessage(withCode("auth/credential-already-in-use"))).toMatch(
      /already linked to a different Mark8ly account/i,
    );
  });

  it("maps provider-already-linked", () => {
    expect(authErrorMessage(withCode("auth/provider-already-linked"))).toMatch(
      /already linked to your account/i,
    );
  });

  it("maps requires-recent-login", () => {
    expect(authErrorMessage(withCode("auth/requires-recent-login"))).toMatch(/sign out and sign in/i);
  });

  it("maps network-request-failed", () => {
    expect(authErrorMessage(withCode("auth/network-request-failed"))).toMatch(/no connection/i);
  });

  it("maps too-many-requests", () => {
    expect(authErrorMessage(withCode("auth/too-many-requests"))).toMatch(/too many attempts/i);
  });

  it("NEVER leaks a raw error message", () => {
    const msg = authErrorMessage(new Error("RequestUnknownException: AppleAuthentication.swift:61"));
    expect(msg).toBe("Something went wrong. Try again.");
    expect(msg).not.toMatch(/swift|Exception/i);
  });

  it("does not throw when the rejection is null or undefined", () => {
    expect(() => authErrorMessage(null)).not.toThrow();
    expect(() => authErrorMessage(undefined)).not.toThrow();
    expect(authErrorMessage(null)).toBe("Something went wrong. Try again.");
  });

  it("never tells a credential error to start over because it 'expired'", () => {
    // GIP collapses wrong-password into auth/invalid-credential. Any "expired"
    // copy makes a mistyped password unrecoverable.
    for (const code of ["auth/invalid-credential", "auth/reauth-failed"]) {
      expect(authErrorMessage(withCode(code))).not.toMatch(/expired/i);
    }
  });
});
```

- [ ] **Step 2: Run to verify RED**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest errors.test`
Expected: FAIL — `authErrorMessage`/`AuthCancelledError` are not exported.
**Paste the verbatim failure output into your report.**

- [ ] **Step 3: Implement** — append to `packages/mobile-shared/auth/errors.ts` (keep the existing header comment and `LastSignInMethodError` untouched):

```ts
/** The user dismissed a native sign-in sheet. Callers show NOTHING. */
export class AuthCancelledError extends Error {
  constructor() {
    super("Sign-in cancelled");
    this.name = "AuthCancelledError";
  }
}

export interface AuthErrorContext {
  /** Which method the user re-authenticated with. Absent = password. */
  provider?: "google.com" | "apple.com";
}

function errorCode(e: unknown): unknown {
  return typeof e === "object" && e !== null ? (e as { code?: unknown }).code : undefined;
}

/**
 * The single source of user-facing auth copy.
 *
 * Returns `null` ONLY when the user cancelled — callers must render nothing.
 * Never returns a raw `e.message`: native SDK strings (Swift file paths, GIP
 * internals) must never reach a user.
 *
 * Disambiguation is by TAG, never by code: GIP tenants run email-enumeration
 * protection, so `auth/wrong-password` never fires and `auth/invalid-credential`
 * means BOTH "wrong password" AND "expired credential". Only `link.ts`'s
 * `auth/reauth-failed` tag can tell them apart — which is why no message here
 * may ever say "expired".
 */
export function authErrorMessage(e: unknown, ctx?: AuthErrorContext): string | null {
  if (e instanceof AuthCancelledError) return null;
  if (e instanceof LastSignInMethodError) {
    return "You can't remove your only sign-in method.";
  }

  const code = errorCode(e);

  // Safety net for a raw Apple error that bypassed the social-auth wrapper.
  if (code === "ERR_REQUEST_CANCELED") return null;

  if (code === "ERR_REQUEST_UNKNOWN") {
    return "Couldn't complete Apple sign-in. Make sure you're signed in to iCloud on this device.";
  }
  if (code === "auth/reauth-failed") {
    return ctx?.provider
      ? "Couldn't verify that account. Try again."
      : "That password is incorrect.";
  }
  if (code === "auth/invalid-credential") {
    return "Couldn't sign you in. Check your details and try again.";
  }
  if (code === "auth/credential-already-in-use") {
    return "That account is already linked to a different Mark8ly account.";
  }
  if (code === "auth/provider-already-linked") {
    return "That's already linked to your account.";
  }
  if (code === "auth/requires-recent-login") {
    return "For security, sign out and sign in again, then retry.";
  }
  if (code === "auth/network-request-failed") {
    return "No connection. Check your network and try again.";
  }
  if (code === "auth/too-many-requests") {
    return "Too many attempts. Try again in a few minutes.";
  }
  return "Something went wrong. Try again.";
}
```

- [ ] **Step 4: GREEN**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest errors.test` → all pass.
Then: `npx jest` → 54 + 15 = **69 passing**.
Then: `npx tsc --noEmit 2>&1 | grep -E "auth/errors" || echo "TYPE-CLEAN"` → `TYPE-CLEAN`.
Then confirm the firebase-free invariant:
`grep -cE "react-native-firebase|google-signin|expo-apple-authentication" ../../packages/mobile-shared/auth/errors.ts` → must print `0`.

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/auth/errors.ts apps/mobile-admin/__tests__/errors.test.tsx
git commit -m "feat(mobile-shared): add the single auth error mapper and cancellation sentinel"
```

---

### Task 2: `social-auth.ts` — normalise cancellation at the native boundary

**Files:**
- Modify: `apps/mobile-admin/lib/social-auth.ts`
- Create: `apps/mobile-admin/__tests__/social-auth.test.tsx`

**Interfaces:**
- Consumes: `AuthCancelledError` from `@repo/mobile-shared/auth/errors` (Task 1).
- Produces: `signInWithGoogleNative()` / `signInWithAppleNative()` throw `AuthCancelledError` on cancel.

**Why this task exists:** `GoogleSignin.signIn()` **does not throw on cancel** — it *resolves* with `{ type: "cancelled", data: null }` (verified: `node_modules/@react-native-google-signin/google-signin/src/signIn/GoogleSignin.ts:60` → `translateCancellationError` → `constants.ts` `cancelledResult`). The current code then finds no `idToken` and throws `Error("Google sign-in failed: no ID token")` — which is what a cancelling user sees today.

- [ ] **Step 1: Write the failing tests** — create `apps/mobile-admin/__tests__/social-auth.test.tsx`.

Mock fns are built INSIDE each factory and read back off the imported module:

```tsx
jest.mock("@react-native-google-signin/google-signin", () => ({
  GoogleSignin: {
    configure: jest.fn(),
    hasPlayServices: jest.fn().mockResolvedValue(true),
    signIn: jest.fn(),
  },
}));

jest.mock("expo-apple-authentication", () => ({
  signInAsync: jest.fn(),
  AppleAuthenticationScope: { FULL_NAME: 1, EMAIL: 0 },
}));

import { GoogleSignin } from "@react-native-google-signin/google-signin";
import * as AppleAuthentication from "expo-apple-authentication";
import { AuthCancelledError } from "@repo/mobile-shared/auth/errors";
import { signInWithAppleNative, signInWithGoogleNative } from "@/lib/social-auth";

const mockGoogleSignIn = GoogleSignin.signIn as jest.Mock;
const mockAppleSignIn = AppleAuthentication.signInAsync as jest.Mock;

beforeEach(() => jest.clearAllMocks());

describe("signInWithGoogleNative", () => {
  it("throws AuthCancelledError when the SDK RESOLVES with type:cancelled", async () => {
    // The SDK does not reject on cancel — it resolves with this shape.
    mockGoogleSignIn.mockResolvedValue({ type: "cancelled", data: null });
    await expect(signInWithGoogleNative()).rejects.toBeInstanceOf(AuthCancelledError);
  });

  it("does NOT report a cancel as a missing-token failure", async () => {
    mockGoogleSignIn.mockResolvedValue({ type: "cancelled", data: null });
    await expect(signInWithGoogleNative()).rejects.not.toThrow(/no ID token/i);
  });

  it("returns the idToken on success", async () => {
    mockGoogleSignIn.mockResolvedValue({ type: "success", data: { idToken: "gtok" } });
    await expect(signInWithGoogleNative()).resolves.toBe("gtok");
  });

  it("still throws when a non-cancelled response carries no idToken", async () => {
    mockGoogleSignIn.mockResolvedValue({ type: "success", data: { idToken: null } });
    await expect(signInWithGoogleNative()).rejects.toThrow(/no ID token/i);
  });
});

describe("signInWithAppleNative", () => {
  it("turns ERR_REQUEST_CANCELED into AuthCancelledError", async () => {
    mockAppleSignIn.mockRejectedValue(
      Object.assign(new Error("The user canceled"), { code: "ERR_REQUEST_CANCELED" }),
    );
    await expect(signInWithAppleNative()).rejects.toBeInstanceOf(AuthCancelledError);
  });

  it("leaves other Apple errors untouched", async () => {
    const original = Object.assign(new Error("RequestUnknownException"), {
      code: "ERR_REQUEST_UNKNOWN",
    });
    mockAppleSignIn.mockRejectedValue(original);
    await expect(signInWithAppleNative()).rejects.toBe(original);
  });

  it("returns the identity token on success", async () => {
    mockAppleSignIn.mockResolvedValue({ identityToken: "atok", fullName: null });
    await expect(signInWithAppleNative()).resolves.toEqual({
      idToken: "atok",
      rawNonce: "",
      fullName: null,
    });
  });
});
```

- [ ] **Step 2: RED**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest social-auth.test`
Expected: the two cancel tests FAIL (Google throws "no ID token"; Apple rethrows the raw error).
**Paste the verbatim failure output into your report.**

- [ ] **Step 3: Implement** — in `apps/mobile-admin/lib/social-auth.ts`, add the import and replace both functions.

Add to the imports at the top:

```ts
import { AuthCancelledError } from "@repo/mobile-shared/auth/errors";
```

Replace `signInWithGoogleNative`:

```ts
export async function signInWithGoogleNative(): Promise<string> {
  await GoogleSignin.hasPlayServices({ showPlayServicesUpdateDialog: true });
  const result = (await GoogleSignin.signIn()) as {
    type?: string;
    data?: { idToken?: string | null };
    idToken?: string | null;
  };
  // The SDK RESOLVES with {type:"cancelled"} rather than rejecting, so a
  // cancel would otherwise fall through to the "no ID token" throw below and
  // be shown to the user as a failure.
  if (result?.type === "cancelled") throw new AuthCancelledError();
  const idToken = result?.data?.idToken ?? result?.idToken;
  if (!idToken) throw new Error("Google sign-in failed: no ID token");
  return idToken;
}
```

Replace `signInWithAppleNative`:

```ts
export async function signInWithAppleNative(): Promise<{
  idToken: string;
  rawNonce: string;
  fullName: AppleFullName | null;
}> {
  let cred: AppleAuthentication.AppleAuthenticationCredential;
  try {
    cred = await AppleAuthentication.signInAsync({
      requestedScopes: [
        AppleAuthentication.AppleAuthenticationScope.FULL_NAME,
        AppleAuthentication.AppleAuthenticationScope.EMAIL,
      ],
    });
  } catch (e: unknown) {
    const code = typeof e === "object" && e !== null ? (e as { code?: unknown }).code : undefined;
    if (code === "ERR_REQUEST_CANCELED") throw new AuthCancelledError();
    throw e;
  }
  if (!cred.identityToken) throw new Error("Apple sign-in failed: no identity token");
  // Home-Chef passes an empty rawNonce (GIP verifies Apple's token without a
  // client nonce in their setup); keep parity. Revisit if GIP rejects it.
  return { idToken: cred.identityToken, rawNonce: "", fullName: cred.fullName };
}
```

- [ ] **Step 4: GREEN**

Run: `npx jest social-auth.test` → pass. Then `npx jest` → **76 passing**.
Then: `npx tsc --noEmit 2>&1 | grep -E "social-auth" || echo "TYPE-CLEAN"` → `TYPE-CLEAN`.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/lib/social-auth.ts apps/mobile-admin/__tests__/social-auth.test.tsx
git commit -m "fix(mobile-admin): treat a dismissed Google or Apple sheet as cancellation, not failure"
```

---

### Task 3: `link.ts` — tag re-auth failures

**Files:**
- Modify: `packages/mobile-shared/auth/link.ts`
- Modify: `apps/mobile-admin/__tests__/link.test.tsx`

**Interfaces — Produces:** `completeLinkWithPassword` / `completeLinkWithGoogle` / `completeLinkWithApple` throw an error carrying `code: "auth/reauth-failed"` when **the re-auth step** fails. `linkWithCredential` failures propagate **untouched**.

- [ ] **Step 1: Write the failing tests** — append this `describe` to `apps/mobile-admin/__tests__/link.test.tsx`.

The file's existing `jest.mock("@react-native-firebase/auth", …)` factory already exposes the mocked instance and a `user` with `linkWithCredential`. Read the file first and reuse its existing helpers/handles rather than adding a second factory.

```tsx
describe("reauth tagging", () => {
  beforeEach(() => jest.clearAllMocks());

  it("tags a FAILED password re-auth with auth/reauth-failed", async () => {
    mockedAuth().signInWithEmailAndPassword.mockRejectedValueOnce(
      Object.assign(new Error("bad"), { code: "auth/invalid-credential" }),
    );
    await expect(
      completeLinkWithPassword("a@b.com", "pw", { provider: "google" } as never),
    ).rejects.toMatchObject({ code: "auth/reauth-failed" });
  });

  it("preserves the original error as `cause`", async () => {
    const original = Object.assign(new Error("bad"), { code: "auth/invalid-credential" });
    mockedAuth().signInWithEmailAndPassword.mockRejectedValueOnce(original);
    await expect(
      completeLinkWithPassword("a@b.com", "pw", { provider: "google" } as never),
    ).rejects.toMatchObject({ cause: original });
  });

  it("does NOT tag a linkWithCredential failure — only the re-auth call is wrapped", async () => {
    const linkErr = Object.assign(new Error("nope"), {
      code: "auth/credential-already-in-use",
    });
    const user = { linkWithCredential: jest.fn().mockRejectedValueOnce(linkErr) };
    mockedAuth().signInWithEmailAndPassword.mockResolvedValueOnce({ user });
    await expect(
      completeLinkWithPassword("a@b.com", "pw", { provider: "google" } as never),
    ).rejects.toBe(linkErr);
  });

  it("tags a FAILED Google re-auth", async () => {
    mockedAuth().signInWithCredential.mockRejectedValueOnce(new Error("bad"));
    await expect(
      completeLinkWithGoogle("gtok", { provider: "apple" } as never),
    ).rejects.toMatchObject({ code: "auth/reauth-failed" });
  });

  it("tags a FAILED Apple re-auth", async () => {
    mockedAuth().signInWithCredential.mockRejectedValueOnce(new Error("bad"));
    await expect(
      completeLinkWithApple("atok", "", { provider: "google" } as never),
    ).rejects.toMatchObject({ code: "auth/reauth-failed" });
  });
});
```

- [ ] **Step 2: RED**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest link.test`
Expected: the tagging tests FAIL (errors pass through untagged).
**Paste the verbatim failure output into your report.**

- [ ] **Step 3: Implement** — in `packages/mobile-shared/auth/link.ts`, add the helper above `completeLinkWithPassword` and rewrite the three functions. **Wrap ONLY the re-auth call.**

```ts
/**
 * Tags a failure of the RE-AUTH step. GIP tenants run email-enumeration
 * protection, which collapses "wrong password" and "no such user" into the
 * single ambiguous `auth/invalid-credential` — and that same code also means
 * "expired credential" when it comes from the LINK step. Tagging by which
 * call threw is the only way to tell them apart; never branch on the code.
 */
function reauthFailed(cause: unknown): Error {
  return Object.assign(new Error("Re-authentication failed"), {
    code: "auth/reauth-failed",
    cause,
  });
}

/** Re-auth with the account's existing password, then attach `pending`. */
export async function completeLinkWithPassword(
  email: string,
  password: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  let result: FirebaseAuthTypes.UserCredential;
  try {
    result = await auth().signInWithEmailAndPassword(email, password);
  } catch (e: unknown) {
    throw reauthFailed(e);
  }
  await result.user.linkWithCredential(pending);
}

/** Re-auth with the account's existing Google identity, then attach `pending`. */
export async function completeLinkWithGoogle(
  googleIdToken: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  const existing = auth.GoogleAuthProvider.credential(googleIdToken);
  let result: FirebaseAuthTypes.UserCredential;
  try {
    result = await auth().signInWithCredential(existing);
  } catch (e: unknown) {
    throw reauthFailed(e);
  }
  await result.user.linkWithCredential(pending);
}

/** Re-auth with the account's existing Apple identity, then attach `pending`. */
export async function completeLinkWithApple(
  appleIdToken: string,
  rawNonce: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  const existing = auth.AppleAuthProvider.credential(appleIdToken, rawNonce);
  let result: FirebaseAuthTypes.UserCredential;
  try {
    result = await auth().signInWithCredential(existing);
  } catch (e: unknown) {
    throw reauthFailed(e);
  }
  await result.user.linkWithCredential(pending);
}
```

- [ ] **Step 4: GREEN**

Run: `npx jest link.test` → pass. Then `npx jest` → **81 passing**.
Then: `npx tsc --noEmit 2>&1 | grep -E "auth/link" || echo "TYPE-CLEAN"` → `TYPE-CLEAN`.

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/auth/link.ts apps/mobile-admin/__tests__/link.test.tsx
git commit -m "fix(mobile-shared): tag re-auth failures so wrong password is distinguishable"
```

---

### Task 4: `client.ts` reason + `auth-notice` store

**Files:**
- Create: `packages/mobile-shared/stores/auth-notice.ts`
- Modify: `packages/mobile-shared/api/client.ts`
- Create: `apps/mobile-admin/__tests__/api-client-unauthorized.test.tsx`

**Interfaces — Produces:**
- `type UnauthorizedReason = "no-session" | "access-denied"` (from `@repo/mobile-shared/api/client`)
- `ApiClientConfig.onUnauthorized?: (reason: UnauthorizedReason) => void | Promise<void>`
- `type AuthNotice`, `useAuthNoticeStore` (from `@repo/mobile-shared/stores/auth-notice`)

**The distinction (no heuristics):** in `client.ts`'s 401 branch, `refresh()` is attempted. If it returned a token and the retry is **still** 401, the token is fresh and valid — the server is denying **access**, not reporting expiry. If refresh returned nothing, the session is genuinely gone.

- [ ] **Step 1: Write the failing tests** — create `apps/mobile-admin/__tests__/api-client-unauthorized.test.tsx`:

```tsx
import { createApiClient } from "@repo/mobile-shared/api/client";

function jsonResponse(status: number): Response {
  return { status, ok: status < 400, json: async () => ({}), text: async () => "" } as Response;
}

describe("onUnauthorized reason", () => {
  const realFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it("reports no-session when there is no token at all", async () => {
    const onUnauthorized = jest.fn();
    globalThis.fetch = jest.fn() as unknown as typeof fetch;
    const client = createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => null,
      getStoreId: () => null,
      onUnauthorized,
    });
    await expect(client.get("/stores")).rejects.toBeTruthy();
    expect(onUnauthorized).toHaveBeenCalledWith("no-session");
  });

  it("reports no-session when the refresh cannot mint a token", async () => {
    const onUnauthorized = jest.fn();
    globalThis.fetch = jest.fn().mockResolvedValue(jsonResponse(401)) as unknown as typeof fetch;
    const client = createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => "stale",
      refreshToken: async () => null,
      getStoreId: () => null,
      onUnauthorized,
    });
    await expect(client.get("/stores")).rejects.toBeTruthy();
    expect(onUnauthorized).toHaveBeenCalledWith("no-session");
  });

  it("reports access-denied when a FRESH token is still rejected", async () => {
    // A freshly minted token that the server still 401s is not expiry —
    // it is the server refusing this identity.
    const onUnauthorized = jest.fn();
    globalThis.fetch = jest.fn().mockResolvedValue(jsonResponse(401)) as unknown as typeof fetch;
    const client = createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => "stale",
      refreshToken: async () => "fresh",
      getStoreId: () => null,
      onUnauthorized,
    });
    await expect(client.get("/stores")).rejects.toBeTruthy();
    expect(onUnauthorized).toHaveBeenCalledWith("access-denied");
  });

  it("does not call onUnauthorized when the retry succeeds", async () => {
    const onUnauthorized = jest.fn();
    globalThis.fetch = jest
      .fn()
      .mockResolvedValueOnce(jsonResponse(401))
      .mockResolvedValueOnce(jsonResponse(200)) as unknown as typeof fetch;
    const client = createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => "stale",
      refreshToken: async () => "fresh",
      getStoreId: () => null,
      onUnauthorized,
    });
    await client.get("/stores").catch(() => undefined);
    expect(onUnauthorized).not.toHaveBeenCalled();
  });
});
```

> Verified against `packages/mobile-shared/api/client.ts:158` — `createApiClient` returns
> `{ get, getTenant, post, patch, delete, uploadMedia }`, with
> `get: <T>(path: string, params?: Record<string, string>, schema?: z.ZodType<T>)`. So
> `client.get("/stores")` above is correct as written.

- [ ] **Step 2: RED**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest api-client-unauthorized`
Expected: FAIL — `onUnauthorized` is called with no argument.
**Paste the verbatim failure output into your report.**

- [ ] **Step 3a: Implement the store** — create `packages/mobile-shared/stores/auth-notice.ts`:

```ts
import { create } from "zustand";

/** Why the user was signed out involuntarily. Rendered once on /login. */
export type AuthNotice = "no-session" | "access-denied";

interface AuthNoticeState {
  notice: AuthNotice | null;
  setNotice: (notice: AuthNotice) => void;
  clearNotice: () => void;
}

/**
 * Carries the reason for an involuntary sign-out across the redirect to
 * /login. Deliberately not persisted: a stale reason must never surface on a
 * later, unrelated sign-in attempt.
 */
export const useAuthNoticeStore = create<AuthNoticeState>((set) => ({
  notice: null,
  setNotice: (notice) => set({ notice }),
  clearNotice: () => set({ notice: null }),
}));
```

- [ ] **Step 3b: Implement the client change** — in `packages/mobile-shared/api/client.ts`:

Add above `export interface ApiClientConfig`:

```ts
/**
 * Why a request was rejected as unauthenticated.
 *
 * - `no-session`   — no token, or the refresh could not mint one.
 * - `access-denied` — a FRESHLY minted token was still rejected. The token is
 *   valid; the server is refusing this identity. This is NOT expiry.
 */
export type UnauthorizedReason = "no-session" | "access-denied";
```

Change the `onUnauthorized` member of `ApiClientConfig` to:

```ts
  /**
   * Called when the API rejects the (possibly refreshed) token with a
   * 401. The caller normally signs the user out and routes back to /login.
   */
  onUnauthorized?: (reason: UnauthorizedReason) => void | Promise<void>;
```

In `execute`, change the no-token branch:

```ts
    const token = await config.getToken();
    if (!token) {
      // No token at all — surface immediately so the caller can route to /login.
      await config.onUnauthorized?.("no-session");
      throw new ApiError(401, "unauthorized", "Not authenticated");
    }
```

And the refresh branch:

```ts
    let res = await send(token, init);
    if (res.status === 401) {
      // Try a single forced token refresh. GIP id_tokens expire after an
      // hour and may have stale custom claims — refresh first, then retry.
      const refreshed = await refresh();
      if (refreshed) {
        res = await send(refreshed, init);
      }
      if (res.status === 401) {
        // A fresh token that is STILL rejected is not expiry — the server is
        // refusing this identity. Only a failed refresh means "session gone".
        const reason: UnauthorizedReason = refreshed ? "access-denied" : "no-session";
        await config.onUnauthorized?.(reason);
        throw new ApiError(
          401,
          "unauthorized",
          refreshed ? "Access denied" : "Session expired",
        );
      }
    }
```

- [ ] **Step 4: GREEN**

Run: `npx jest api-client-unauthorized` → pass. Then `npx jest` → **85 passing**.
Then: `npx tsc --noEmit 2>&1 | grep -c "error TS"` → **2** (per-file greps miss errors in the NEW test file — count instead).

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/api/client.ts packages/mobile-shared/stores/auth-notice.ts apps/mobile-admin/__tests__/api-client-unauthorized.test.tsx
git commit -m "feat(mobile-shared): distinguish a denied identity from a dead session on 401"
```

---

### Task 5: Call sites adopt the shared mapper

**Files:**
- Modify: `apps/mobile-admin/app/login.tsx`
- Modify: `apps/mobile-admin/components/auth/LinkAccountPrompt.tsx`
- Modify: `apps/mobile-admin/app/(tabs)/more/security.tsx`
- Modify: `apps/mobile-admin/__tests__/login.test.tsx`

**Interfaces:**
- Consumes: `authErrorMessage`, `AuthErrorContext`, `AuthCancelledError` (Task 1).

**Rule at every call site:** `const msg = authErrorMessage(e, ctx); if (msg) setError(msg);` — a `null` means the user cancelled and **nothing** renders.

- [ ] **Step 1a: REPLACE the existing test that asserts the bug**

`apps/mobile-admin/__tests__/login.test.tsx` currently contains:

```tsx
  it('shows the error message when signIn rejects', async () => {
    mockSignIn.mockRejectedValue(new Error('Wrong password'));
    const { getByLabelText, findByText } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Sign in'));
    expect(await findByText('Wrong password')).toBeTruthy();
  });
```

**This test asserts the raw `e.message` passthrough — the exact defect this task removes.** It
will fail once the mapper lands, and it must NOT be "fixed" by preserving passthrough. Replace it
in place with a test that proves *mapping*:

```tsx
  it('shows mapped copy — never the raw message — when signIn rejects', async () => {
    mockSignIn.mockRejectedValue(
      Object.assign(new Error('INVALID_LOGIN_CREDENTIALS'), { code: 'auth/invalid-credential' }),
    );
    const { getByLabelText, findByText, queryByText } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Sign in'));
    expect(await findByText(/check your details/i)).toBeTruthy();
    expect(queryByText('INVALID_LOGIN_CREDENTIALS')).toBeNull();
  });
```

- [ ] **Step 1a-ii: REPLACE the SECOND passthrough-asserting test**

`apps/mobile-admin/__tests__/LinkAccountPrompt.test.tsx:127-137` contains the *same* trap:

```tsx
  it("shows an error and stays open when the re-auth fails", async () => {
    setAuth({
      completeLinkWithPassword: jest.fn().mockRejectedValue(new Error("Wrong password")),
    });
    …
    expect(await findByText("Wrong password")).toBeTruthy();
    expect(onLinked).not.toHaveBeenCalled();
  });
```

Replace it in place, using the shape the re-auth path now actually produces (Task 3 tags it
`auth/reauth-failed`; the password path passes no context, so the mapper yields "That password is
incorrect."). Keep the `onLinked` assertion — "stays open" is the part worth preserving:

```tsx
  it("shows mapped copy — never the raw message — and stays open when the re-auth fails", async () => {
    setAuth({
      completeLinkWithPassword: jest
        .fn()
        .mockRejectedValue(
          Object.assign(new Error("INVALID_LOGIN_CREDENTIALS"), { code: "auth/reauth-failed" }),
        ),
    });
    const { getByLabelText, findByText, queryByText, onLinked } = renderPrompt();
    await waitFor(() => expect(getByLabelText("Password")).toBeTruthy());
    fireEvent.changeText(getByLabelText("Password"), "nope");
    fireEvent.press(getByLabelText("Sign in and link"));
    expect(await findByText("That password is incorrect.")).toBeTruthy();
    expect(queryByText("INVALID_LOGIN_CREDENTIALS")).toBeNull();
    expect(onLinked).not.toHaveBeenCalled();
  });
```

This is the stronger test: it proves Task 1 + Task 3 + Task 5 compose end-to-end through the
component. Both replacements are REPLACEMENTS, not additions — the suite total stays 87.

> Swept 2026-07-15: these two are the ONLY tests asserting raw passthrough. `security.test.tsx:73`
> and `:149` use `mockRejectedValue(new Error("network down"))` but assert the mount effect's fixed
> "Couldn't load your sign-in methods." copy, which never goes through the mapper — unaffected.

- [ ] **Step 1b: Add the new failing tests**

The file already has `jest.mock('@/lib/social-auth', …)` whose factory builds its mock fns inside
itself, a `mockUseAuth(overrides)` helper, and `import LoginScreen from '../app/login'`. Reuse
them. Add these imports at the top (the mocked module must be imported to reach its mock fns):

```tsx
import { signInWithGoogleNative } from '@/lib/social-auth';
import { AuthCancelledError } from '@repo/mobile-shared/auth/errors';
```

Then append:

```tsx
describe('error copy', () => {
  it('shows NOTHING when the user cancels the Google sheet', async () => {
    (signInWithGoogleNative as jest.Mock).mockRejectedValueOnce(new AuthCancelledError());
    const { getByLabelText, queryByText } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Continue with Google'));
    await waitFor(() => expect(signInWithGoogleNative).toHaveBeenCalled());
    expect(queryByText(/cancel/i)).toBeNull();
    expect(queryByText(/something went wrong/i)).toBeNull();
  });

  it('never shows a raw native error string', async () => {
    (signInWithGoogleNative as jest.Mock).mockRejectedValueOnce(
      new Error('RequestUnknownException: AppleAuthenticationExceptions.swift:61'),
    );
    const { getByLabelText, findByText, queryByText } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Continue with Google'));
    expect(await findByText('Something went wrong. Try again.')).toBeTruthy();
    expect(queryByText(/swift/i)).toBeNull();
  });
});
```

Labels are verified against `app/login.tsx`: `"Sign in"`, `"Continue with Google"`,
`"Sign in with Apple"`, `"Email"`, `"Password"`.

- [ ] **Step 2: RED**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest login.test`
Expected: FAIL — the raw message is rendered; cancel shows an error.
**Paste the verbatim failure output into your report.**

> If a test you did NOT touch starts failing, stop and report it — do not "fix" it by restoring
> raw-message passthrough.

- [ ] **Step 3a: `login.tsx`** — delete the local `getErrorMessage` (lines ~14-18) entirely and import the shared one:

```ts
import { authErrorMessage } from '@repo/mobile-shared/auth/errors';
```

Replace **every** `setError(getErrorMessage(e));` in the file with:

```ts
      const msg = authErrorMessage(e);
      if (msg) setError(msg);
```

- [ ] **Step 3b: `LinkAccountPrompt.tsx`** — delete the local `getErrorMessage` (lines ~26-30) and import:

```ts
import { authErrorMessage, type AuthErrorContext } from '@repo/mobile-shared/auth/errors';
```

Change `run` to take the context and use the mapper:

```ts
  async function run(fn: () => Promise<void>, ctx?: AuthErrorContext) {
    if (busy) return;
    setError(null);
    setBusy(true);
    try {
      await fn();
      onLinked();
    } catch (e: unknown) {
      const msg = authErrorMessage(e, ctx);
      if (msg) setError(msg);
    } finally {
      setBusy(false);
    }
  }
```

At the **password** re-auth call site (currently `void run(() => completeLinkWithPassword(email, password, pendingCredential))`), pass **no** context — absent `provider` selects the "That password is incorrect." copy.

At the **Google** re-auth call site (the `void run(async () => { … })` that calls `completeLinkWithGoogle`), pass the context:

```ts
                void run(async () => {
                  /* existing body unchanged */
                }, { provider: 'google.com' });
```

At the **Apple** re-auth call site (the `void run(async () => { … })` that calls `completeLinkWithApple`):

```ts
                void run(async () => {
                  /* existing body unchanged */
                }, { provider: 'apple.com' });
```

- [ ] **Step 3c: `security.tsx`** — delete the local `errorMessage` function entirely. Replace its import line

```ts
import { LastSignInMethodError } from "@repo/mobile-shared/auth/errors";
```

with

```ts
import { authErrorMessage } from "@repo/mobile-shared/auth/errors";
```

and in `run`, replace `setError(errorMessage(e));` with:

```ts
        const msg = authErrorMessage(e);
        if (msg) setError(msg);
```

`LastSignInMethodError` is no longer referenced in this file — remove it from the import. (`authErrorMessage` handles it internally.)

- [ ] **Step 4: GREEN**

Run: `npx jest login.test` → pass. Then `npx jest security.test LinkAccountPrompt.test` → pass.
Then `npx jest` (full) → **87 passing** (85 + 2 new; the passthrough test was replaced, not added).
Then: `npx tsc --noEmit 2>&1 | grep -E "login|LinkAccountPrompt|more/security" || echo "TYPE-CLEAN"` → `TYPE-CLEAN`.
Then confirm no mapper survives: `grep -rn "function getErrorMessage\|function errorMessage" app components | grep -v node_modules` → **no output**.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/app/login.tsx apps/mobile-admin/components/auth/LinkAccountPrompt.tsx "apps/mobile-admin/app/(tabs)/more/security.tsx" apps/mobile-admin/__tests__/login.test.tsx
git commit -m "fix(mobile-admin): route every auth error through one mapper, stay silent on cancel"
```

---

### Task 6: Explain the sign-out, and TenantGate's 403

**Files:**
- Modify: `apps/mobile-admin/lib/api-client.ts`
- Modify: `apps/mobile-admin/app/login.tsx`
- Modify: `apps/mobile-admin/components/TenantGate.tsx`
- Modify: `apps/mobile-admin/__tests__/login.test.tsx`

**Interfaces:**
- Consumes: `useAuthNoticeStore`, `AuthNotice`, `UnauthorizedReason` (Task 4).

- [ ] **Step 1: Write the failing tests** — append to `apps/mobile-admin/__tests__/login.test.tsx`:

```tsx
import { useAuthNoticeStore } from "@repo/mobile-shared/stores/auth-notice";

describe("involuntary sign-out notice", () => {
  afterEach(() => useAuthNoticeStore.getState().clearNotice());

  it("explains an access-denied sign-out", async () => {
    useAuthNoticeStore.getState().setNotice("access-denied");
    const { findByText } = render(<LoginScreen />);
    expect(await findByText(/doesn't have access to a Mark8ly admin account/i)).toBeTruthy();
  });

  it("explains a dead session", async () => {
    useAuthNoticeStore.getState().setNotice("no-session");
    const { findByText } = render(<LoginScreen />);
    expect(await findByText(/your session ended/i)).toBeTruthy();
  });

  it("clears the notice so it cannot resurface on a later attempt", async () => {
    useAuthNoticeStore.getState().setNotice("access-denied");
    const { findByText } = render(<LoginScreen />);
    await findByText(/doesn't have access/i);
    await waitFor(() => expect(useAuthNoticeStore.getState().notice).toBeNull());
  });

  it("shows nothing when there is no notice", () => {
    const { queryByText } = render(<LoginScreen />);
    expect(queryByText(/session ended|doesn't have access/i)).toBeNull();
  });
});
```

- [ ] **Step 2: RED**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest login.test`
Expected: FAIL — no notice is rendered.
**Paste the verbatim failure output into your report.**

- [ ] **Step 3a: `lib/api-client.ts`** — add the import:

```ts
import { useAuthNoticeStore } from "@repo/mobile-shared/stores/auth-notice";
```

Replace the `onUnauthorized` member:

```ts
        onUnauthorized: async (reason) => {
          // Record WHY before tearing the session down — /login reads this and
          // explains itself instead of bouncing the user with no message.
          useAuthNoticeStore.getState().setNotice(reason);
          await signOut();
        },
```

> Use `.getState()` — reading the store reactively here would re-create the client on every notice change.

- [ ] **Step 3b: `app/login.tsx`** — add imports:

```ts
import { useEffect } from 'react';
import { useAuthNoticeStore, type AuthNotice } from '@repo/mobile-shared/stores/auth-notice';
```

(If `useEffect` is already imported from `react`, extend the existing import instead of adding a second one.)

Add above the component:

```ts
const NOTICE_COPY: Record<AuthNotice, string> = {
  'access-denied': "That account doesn't have access to a Mark8ly admin account.",
  'no-session': 'Your session ended. Sign in again.',
};
```

Inside `LoginScreen`, after the existing `useState` declarations:

```ts
  const notice = useAuthNoticeStore((s) => s.notice);
  const clearNotice = useAuthNoticeStore((s) => s.clearNotice);

  useEffect(() => {
    if (!notice) return;
    setError(NOTICE_COPY[notice]);
    clearNotice();
  }, [notice, clearNotice]);
```

The existing `error` element renders it — no new UI.

- [ ] **Step 3c: `components/TenantGate.tsx`** — add the import:

```ts
import { ApiError } from "@repo/mobile-shared/api/client";
```

Replace the whole `if (error) { … }` block with:

```tsx
  if (error) {
    // A 403 here is a permissions verdict, not a network problem — "check your
    // connection" would send the user chasing the wrong fix, and retrying a
    // denial just fails again.
    const denied = error instanceof ApiError && error.status === 403;
    return (
      <Screen>
        <View style={styles.center}>
          <EmptyState
            title={denied ? "No access" : "Couldn't load your store"}
            message={
              denied
                ? "That account doesn't have access to a Mark8ly admin account."
                : "Check your connection and try again."
            }
          />
          {denied ? null : (
            <TouchableOpacity
              onPress={() => refetch()}
              style={styles.primaryBtn}
              activeOpacity={0.85}
              accessibilityRole="button"
              accessibilityLabel="Retry"
            >
              <Text preset="bodyEmphasis" color="inverse">
                Retry
              </Text>
            </TouchableOpacity>
          )}
        </View>
      </Screen>
    );
  }
```

- [ ] **Step 4: GREEN**

Run: `npx jest login.test` → pass. Then `npx jest` (full) → **91 passing**.
Then: `npx tsc --noEmit 2>&1 | grep -E "api-client|login|TenantGate" || echo "TYPE-CLEAN"` → `TYPE-CLEAN`.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/lib/api-client.ts apps/mobile-admin/app/login.tsx apps/mobile-admin/components/TenantGate.tsx apps/mobile-admin/__tests__/login.test.tsx
git commit -m "fix(mobile-admin): explain why a session ended instead of bouncing to login in silence"
```

---

## Final verification

- [ ] `cd apps/mobile-admin && npx jest` → all green (~91).
- [ ] `npx tsc --noEmit` → only the 2 pre-existing `app/(tabs)/_layout.tsx` errors.
- [ ] `grep -E "^\s*import|require\(" packages/mobile-shared/auth/errors.ts | grep -E "firebase|google-signin|apple-authentication"` → no output (the isolation invariant — check IMPORTS, not mentions; the file header legitimately names the packages in prose).
- [ ] `grep -rn "function getErrorMessage\|function errorMessage" apps/mobile-admin/app apps/mobile-admin/components` → no output (one mapper only).
- [ ] `EXPO_PUBLIC_AUTH_BACKEND=demo npx expo config --json | grep -c googleServicesFile` → `0` (demo prebuild stays credential-free).
- [ ] **Manual (deferred — needs a device build):** cancel the Google sheet → no error shown. Sign in with a Google account that has no merchant record → login explains it.
