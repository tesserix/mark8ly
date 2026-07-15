# mobile-admin Account Linking (login-time) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make mobile match the web — when a merchant who registered with a password (or Google) signs in with Google/Apple on the same email, link the provider into their existing account instead of erroring.

**Architecture:** Native RN Firebase SDK only (GIP-direct, no BFF, no REST). `signInWithCredential` conflicts are caught and returned as a typed `needs-link` outcome carrying the credential we already built; the login screen prompts the user to re-authenticate with their existing method, then `user.linkWithCredential(pending)` attaches the new provider. `onAuthStateChanged` (fired by the re-auth sign-in) drives routing as today.

**Tech Stack:** `@react-native-firebase/auth` 24.1.1 (`linkWithCredential` — User.js:98; `fetchSignInMethodsForEmail` — index.js:447), Expo 56 / RN 0.85.3, nativewind, jest-expo.

## Global Constraints

- **GIP-direct — no BFF.** Never add `completeBFFLogin`/`setAuthResponse`. `onAuthStateChanged` drives routing.
- **Tenant setting unchanged** — `MP-Internal-e986p` stays *one-account-per-email* (matches web's security posture).
- **Demo backend must never hit the link path:** demo `signInWithGoogle/Apple` always return `{ status: "signed-in" }`, `completeLink*` are no-ops, `existingSignInMethods` returns `[]`.
- **Apple "Hide My Email" is OUT OF SCOPE** (a relay email can't match at login) — deferred to the Settings "Connected accounts" phase. Do not attempt to solve it here.
- **Enumeration protection:** `fetchSignInMethodsForEmail` may return `[]`. The UI must never dead-end — default to the password field plus a secondary "I used Google / Apple instead" option.
- **TypeScript:** explicit types on exports; no `any` (use `unknown` + narrow); `interface` for object shapes, `type` for unions.
- **Immutability:** no in-place mutation; spread for state updates.
- **Commits:** single-line conventional, **NO signatures**, direct to `main`. No PR.
- **Monorepo landmines — do NOT touch:** `metro.config.js`, `tsconfig.json`, `jest.config.js`, `babel.config.js`, tailwind/nativewind wiring, `app.config.js`, `eas.json`. **No `.test.*` under `apps/mobile-admin/app/`** — tests live in `apps/mobile-admin/__tests__/`.
- **jest.mock factories must build their mock fns INSIDE the factory** (babel hoists ESM imports above outer `const`/`var`, so outer-scope refs are `undefined` at factory time) and read them back off the imported default export. See `apps/mobile-admin/__tests__/gip.test.tsx`.

Run tests from the app: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest`.
Typecheck: `npx tsc --noEmit` — **ignore the 2 pre-existing unrelated errors in `app/(tabs)/_layout.tsx`** (expo-notifications).

---

### Task 1: `social-credentials.ts` — typed outcome, conflict → needs-link

**Files:**
- Modify: `packages/mobile-shared/auth/social-credentials.ts`
- Modify: `apps/mobile-admin/__tests__/social-credentials.test.tsx`

**Interfaces:**
- Produces: `SocialSignInOutcome` (exported union); `signInWithGoogleCredential(idToken, accessToken?): Promise<SocialSignInOutcome>`; `signInWithAppleCredential(idToken, rawNonce, fullName?): Promise<SocialSignInOutcome>`. `AppleFullName` unchanged.

- [ ] **Step 1: Write the failing tests**

Add to `apps/mobile-admin/__tests__/social-credentials.test.tsx`. The existing `jest.mock("@react-native-firebase/auth", …)` factory builds its mocks inside — extend it so `signInWithCredential` is reachable and can be made to reject. Keep the existing two credential-mapping tests unchanged. Add:

```tsx
  it("returns signed-in when the credential sign-in succeeds", async () => {
    const outcome = await signInWithGoogleCredential("gtok");
    expect(outcome).toEqual({ status: "signed-in" });
  });

  it("maps an account-exists conflict to needs-link with the pending credential (google)", async () => {
    const conflict = Object.assign(
      new Error("account exists"),
      { code: "auth/account-exists-with-different-credential", email: "merchant@store.com" },
    );
    mockedAuth().signInWithCredential.mockRejectedValueOnce(conflict);

    const outcome = await signInWithGoogleCredential("gtok");

    expect(outcome).toEqual({
      status: "needs-link",
      email: "merchant@store.com",
      provider: "google.com",
      pendingCredential: { provider: "google", idToken: "gtok" },
    });
  });

  it("maps an account-exists conflict to needs-link (apple)", async () => {
    const conflict = Object.assign(
      new Error("account exists"),
      { code: "auth/account-exists-with-different-credential", email: "merchant@store.com" },
    );
    mockedAuth().signInWithCredential.mockRejectedValueOnce(conflict);

    const outcome = await signInWithAppleCredential("atok", "nonce123", null);

    expect(outcome).toEqual({
      status: "needs-link",
      email: "merchant@store.com",
      provider: "apple.com",
      pendingCredential: { provider: "apple", idToken: "atok", nonce: "nonce123" },
    });
  });

  it("rethrows non-conflict errors", async () => {
    mockedAuth().signInWithCredential.mockRejectedValueOnce(
      Object.assign(new Error("network"), { code: "auth/network-request-failed" }),
    );
    await expect(signInWithGoogleCredential("gtok")).rejects.toThrow("network");
  });
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest social-credentials`
Expected: FAIL — the functions currently return a `UserCredential`, not an outcome, and rethrow the conflict.

- [ ] **Step 3: Implement the outcome**

Rewrite `packages/mobile-shared/auth/social-credentials.ts`:

```ts
import auth, { FirebaseAuthTypes } from "@react-native-firebase/auth";

export interface AppleFullName {
  givenName?: string | null;
  familyName?: string | null;
}

/**
 * Result of a social sign-in. `needs-link` means the tenant is
 * one-account-per-email and an account already exists for this email under a
 * different provider — the caller must have the user re-authenticate with
 * their existing method, then link `pendingCredential` onto it.
 */
export type SocialSignInOutcome =
  | { status: "signed-in" }
  | {
      status: "needs-link";
      email: string;
      provider: "google.com" | "apple.com";
      pendingCredential: FirebaseAuthTypes.AuthCredential;
    };

const ACCOUNT_EXISTS = "auth/account-exists-with-different-credential";

function isAccountExistsConflict(
  e: unknown,
): e is { code: string; email?: string } {
  return (
    typeof e === "object" &&
    e !== null &&
    (e as { code?: unknown }).code === ACCOUNT_EXISTS
  );
}

export async function signInWithGoogleCredential(
  idToken: string,
  accessToken?: string,
): Promise<SocialSignInOutcome> {
  const cred = auth.GoogleAuthProvider.credential(idToken, accessToken);
  try {
    await auth().signInWithCredential(cred);
    return { status: "signed-in" };
  } catch (e: unknown) {
    if (isAccountExistsConflict(e)) {
      return {
        status: "needs-link",
        email: e.email ?? "",
        provider: "google.com",
        pendingCredential: cred,
      };
    }
    throw e;
  }
}

export async function signInWithAppleCredential(
  idToken: string,
  rawNonce: string,
  fullName?: AppleFullName | null,
): Promise<SocialSignInOutcome> {
  const cred = auth.AppleAuthProvider.credential(idToken, rawNonce);
  let result: FirebaseAuthTypes.UserCredential;
  try {
    result = await auth().signInWithCredential(cred);
  } catch (e: unknown) {
    if (isAccountExistsConflict(e)) {
      return {
        status: "needs-link",
        email: e.email ?? "",
        provider: "apple.com",
        pendingCredential: cred,
      };
    }
    throw e;
  }
  const displayName = buildDisplayName(fullName);
  if (displayName && !result.user.displayName) {
    try {
      await result.user.updateProfile({ displayName });
    } catch {
      // Best-effort: name capture is non-fatal; the user stays signed in.
    }
  }
  return { status: "signed-in" };
}

function buildDisplayName(fullName?: AppleFullName | null): string {
  if (!fullName) return "";
  return [fullName.givenName, fullName.familyName]
    .map((p) => (p ?? "").trim())
    .filter((p) => p.length > 0)
    .join(" ");
}
```

- [ ] **Step 4: Run to verify they pass**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest social-credentials`
Expected: PASS (existing credential-mapping tests + the 4 new ones).

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/auth/social-credentials.ts apps/mobile-admin/__tests__/social-credentials.test.tsx
git commit -m "feat(mobile-shared): return typed social sign-in outcome with needs-link on account conflict"
```

---

### Task 2: `link.ts` — re-auth + link helpers

**Files:**
- Create: `packages/mobile-shared/auth/link.ts`
- Create: `apps/mobile-admin/__tests__/link.test.tsx`

**Interfaces:**
- Consumes: `FirebaseAuthTypes.AuthCredential` (the `pendingCredential` from Task 1).
- Produces:
  - `completeLinkWithPassword(email: string, password: string, pending: FirebaseAuthTypes.AuthCredential): Promise<void>`
  - `completeLinkWithGoogle(googleIdToken: string, pending: FirebaseAuthTypes.AuthCredential): Promise<void>`
  - `completeLinkWithApple(appleIdToken: string, rawNonce: string, pending: FirebaseAuthTypes.AuthCredential): Promise<void>`
  - `existingSignInMethods(email: string): Promise<string[]>`

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/link.test.tsx`:

```tsx
// Mocks are built INSIDE the jest.mock factory (babel hoists imports above
// outer const/var) and read back off the imported `auth` default export.
jest.mock("@react-native-firebase/auth", () => {
  const linkWithCredential = jest.fn().mockResolvedValue(undefined);
  const user = { linkWithCredential };
  const instance = {
    signInWithEmailAndPassword: jest.fn().mockResolvedValue({ user }),
    signInWithCredential: jest.fn().mockResolvedValue({ user }),
    fetchSignInMethodsForEmail: jest.fn().mockResolvedValue(["password"]),
  };
  const authFn = () => instance;
  authFn.GoogleAuthProvider = {
    credential: jest.fn((idToken: string) => ({ provider: "google", idToken })),
  };
  authFn.AppleAuthProvider = {
    credential: jest.fn((idToken: string, nonce: string) => ({
      provider: "apple",
      idToken,
      nonce,
    })),
  };
  return { __esModule: true, default: authFn };
});

import auth from "@react-native-firebase/auth";
import {
  completeLinkWithPassword,
  completeLinkWithGoogle,
  completeLinkWithApple,
  existingSignInMethods,
} from "@repo/mobile-shared/auth/link";

interface MockedInstance {
  signInWithEmailAndPassword: jest.Mock;
  signInWithCredential: jest.Mock;
  fetchSignInMethodsForEmail: jest.Mock;
}
const mockedAuth = auth as unknown as (() => MockedInstance) & {
  GoogleAuthProvider: { credential: jest.Mock };
  AppleAuthProvider: { credential: jest.Mock };
};
const PENDING = { provider: "google", idToken: "pending-tok" } as never;

describe("account linking", () => {
  beforeEach(() => jest.clearAllMocks());

  it("password re-auth signs in then links the pending credential", async () => {
    await completeLinkWithPassword("merchant@store.com", "pw", PENDING);
    const instance = mockedAuth();
    expect(instance.signInWithEmailAndPassword).toHaveBeenCalledWith(
      "merchant@store.com",
      "pw",
    );
    const linked = await instance.signInWithEmailAndPassword.mock.results[0]!.value;
    expect(linked.user.linkWithCredential).toHaveBeenCalledWith(PENDING);
  });

  it("google re-auth builds the existing credential, signs in, then links", async () => {
    await completeLinkWithGoogle("existing-gtok", PENDING);
    expect(mockedAuth.GoogleAuthProvider.credential).toHaveBeenCalledWith("existing-gtok");
    const instance = mockedAuth();
    expect(instance.signInWithCredential).toHaveBeenCalledWith({
      provider: "google",
      idToken: "existing-gtok",
    });
    const linked = await instance.signInWithCredential.mock.results[0]!.value;
    expect(linked.user.linkWithCredential).toHaveBeenCalledWith(PENDING);
  });

  it("apple re-auth builds the existing credential, signs in, then links", async () => {
    await completeLinkWithApple("existing-atok", "nonce", PENDING);
    expect(mockedAuth.AppleAuthProvider.credential).toHaveBeenCalledWith(
      "existing-atok",
      "nonce",
    );
    const instance = mockedAuth();
    expect(instance.signInWithCredential).toHaveBeenCalledWith({
      provider: "apple",
      idToken: "existing-atok",
      nonce: "nonce",
    });
  });

  it("existingSignInMethods returns the native result", async () => {
    await expect(existingSignInMethods("merchant@store.com")).resolves.toEqual(["password"]);
    expect(mockedAuth().fetchSignInMethodsForEmail).toHaveBeenCalledWith("merchant@store.com");
  });

  it("existingSignInMethods returns [] when enumeration protection blocks it", async () => {
    mockedAuth().fetchSignInMethodsForEmail.mockResolvedValueOnce([]);
    await expect(existingSignInMethods("merchant@store.com")).resolves.toEqual([]);
  });
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest link.test`
Expected: FAIL — module `@repo/mobile-shared/auth/link` not found.

- [ ] **Step 3: Create `link.ts`**

Create `packages/mobile-shared/auth/link.ts`:

```ts
// Completes the one-account-per-email merge on mobile: the user re-authenticates
// with the method their account already has, then the pending provider credential
// is linked onto that account. Mirrors the web admin's link handshake
// (apps/admin/lib/gip/link.ts) using the native SDK instead of REST.

import auth, { FirebaseAuthTypes } from "@react-native-firebase/auth";

/** Re-auth with the account's existing password, then attach `pending`. */
export async function completeLinkWithPassword(
  email: string,
  password: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  const result = await auth().signInWithEmailAndPassword(email, password);
  await result.user.linkWithCredential(pending);
}

/** Re-auth with the account's existing Google identity, then attach `pending`. */
export async function completeLinkWithGoogle(
  googleIdToken: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  const existing = auth.GoogleAuthProvider.credential(googleIdToken);
  const result = await auth().signInWithCredential(existing);
  await result.user.linkWithCredential(pending);
}

/** Re-auth with the account's existing Apple identity, then attach `pending`. */
export async function completeLinkWithApple(
  appleIdToken: string,
  rawNonce: string,
  pending: FirebaseAuthTypes.AuthCredential,
): Promise<void> {
  const existing = auth.AppleAuthProvider.credential(appleIdToken, rawNonce);
  const result = await auth().signInWithCredential(existing);
  await result.user.linkWithCredential(pending);
}

/**
 * Sign-in methods already registered for `email` — e.g. ["password"],
 * ["google.com"]. Returns [] when the tenant has email-enumeration protection
 * enabled, in which case the caller must ask the user which method they used.
 */
export async function existingSignInMethods(email: string): Promise<string[]> {
  return auth().fetchSignInMethodsForEmail(email);
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest link.test`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/auth/link.ts apps/mobile-admin/__tests__/link.test.tsx
git commit -m "feat(mobile-shared): add re-auth + linkWithCredential helpers for account merge"
```

---

### Task 3: `gip.ts` — return the outcome, expose link helpers

**Files:**
- Modify: `packages/mobile-shared/auth/gip.ts`
- Modify: `apps/mobile-admin/__tests__/gip.test.tsx`

**Interfaces:**
- Consumes: `SocialSignInOutcome`, `signInWith{Google,Apple}Credential` (Task 1); `completeLinkWith{Password,Google,Apple}`, `existingSignInMethods` (Task 2).
- Produces: `createGIPAuth(...)` return gains `completeLinkWithPassword`, `completeLinkWithGoogle`, `completeLinkWithApple`, `existingSignInMethods`; `signInWithGoogle`/`signInWithApple` now resolve to `SocialSignInOutcome`. The existing `tenantReady` await stays on every sign-in/link path.

- [ ] **Step 1: Add the link surface to `gip.ts`**

In `packages/mobile-shared/auth/gip.ts`, extend the imports:

```ts
import {
  signInWithGoogleCredential,
  signInWithAppleCredential,
  type AppleFullName,
  type SocialSignInOutcome,
} from "./social-credentials";
import {
  completeLinkWithPassword,
  completeLinkWithGoogle,
  completeLinkWithApple,
  existingSignInMethods,
} from "./link";
import type { FirebaseAuthTypes } from "@react-native-firebase/auth";
```

`signInWithGoogle`/`signInWithApple` keep their `await tenantReady` and now simply return the credential helpers' `SocialSignInOutcome` (no signature change needed beyond the inferred return type). Add these to the returned object, each awaiting the tenant first so the link happens in the right pool:

```ts
    completeLinkWithPassword: async (
      email: string,
      password: string,
      pending: FirebaseAuthTypes.AuthCredential,
    ) => {
      await tenantReady;
      return completeLinkWithPassword(email, password, pending);
    },
    completeLinkWithGoogle: async (
      googleIdToken: string,
      pending: FirebaseAuthTypes.AuthCredential,
    ) => {
      await tenantReady;
      return completeLinkWithGoogle(googleIdToken, pending);
    },
    completeLinkWithApple: async (
      appleIdToken: string,
      rawNonce: string,
      pending: FirebaseAuthTypes.AuthCredential,
    ) => {
      await tenantReady;
      return completeLinkWithApple(appleIdToken, rawNonce, pending);
    },
    existingSignInMethods: async (email: string) => {
      await tenantReady;
      return existingSignInMethods(email);
    },
```

Also re-export the type so consumers have one import site:

```ts
export type { SocialSignInOutcome } from "./social-credentials";
```

- [ ] **Step 2: Extend `gip.test.tsx` to cover the tenant gate on linking**

The existing file already mocks `@react-native-firebase/auth` (with the throwing `tenantId` setter) and `@repo/mobile-shared/auth/social-credentials`. Add a mock for the link module next to it:

```tsx
jest.mock("@repo/mobile-shared/auth/link", () => ({
  completeLinkWithPassword: jest.fn().mockResolvedValue(undefined),
  completeLinkWithGoogle: jest.fn().mockResolvedValue(undefined),
  completeLinkWithApple: jest.fn().mockResolvedValue(undefined),
  existingSignInMethods: jest.fn().mockResolvedValue(["password"]),
}));
```

and import + assert:

```tsx
import { completeLinkWithPassword } from "@repo/mobile-shared/auth/link";

  it("awaits the tenant before completing a password link", async () => {
    const gip = createGIPAuth({ tenantId: "T5" });
    const pending = { provider: "google", idToken: "p" } as never;
    await gip.completeLinkWithPassword("merchant@store.com", "pw", pending);
    expect(instance.setTenantId).toHaveBeenCalledWith("T5");
    expect(completeLinkWithPassword as jest.Mock).toHaveBeenCalledWith(
      "merchant@store.com",
      "pw",
      pending,
    );
  });
```

Keep every existing test in the file unchanged.

- [ ] **Step 3: Run the tests**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest gip`
Expected: PASS (the 5 existing tests + the new link test).

- [ ] **Step 4: Typecheck**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx tsc --noEmit 2>&1 | grep -E "auth/gip|auth/link|social-credentials" || echo "TYPE-CLEAN"`
Expected: `TYPE-CLEAN`.

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/auth/gip.ts apps/mobile-admin/__tests__/gip.test.tsx
git commit -m "feat(mobile-shared): expose account-link helpers on the GIP client"
```

---

### Task 4: `provider.tsx` — AuthState surface + demo no-ops

**Files:**
- Modify: `packages/mobile-shared/auth/provider.tsx`

**Interfaces:**
- Consumes: `gip.completeLinkWith*`, `gip.existingSignInMethods`, `SocialSignInOutcome` (Task 3).
- Produces: `AuthState` (and `AuthBackend`) gain `completeLinkWithPassword(email, password, pending): Promise<void>`, `completeLinkWithGoogle(googleIdToken, pending): Promise<void>`, `completeLinkWithApple(appleIdToken, rawNonce, pending): Promise<void>`, `existingSignInMethods(email): Promise<string[]>`; and `signInWithGoogle`/`signInWithApple` now return `Promise<SocialSignInOutcome>` instead of `Promise<void>`.

- [ ] **Step 1: Update both interfaces**

In `packages/mobile-shared/auth/provider.tsx`, add the type-only import (keep it type-only — the firebase native module must never load in Expo Go/demo):

```ts
import type { AppleFullName, SocialSignInOutcome } from "./social-credentials";
import type { FirebaseAuthTypes } from "@react-native-firebase/auth";
```

Change the social signatures in **both** `AuthState` and `AuthBackend` from `Promise<void>` to `Promise<SocialSignInOutcome>`, and add to both:

```ts
  completeLinkWithPassword: (
    email: string,
    password: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => Promise<void>;
  completeLinkWithGoogle: (
    googleIdToken: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => Promise<void>;
  completeLinkWithApple: (
    appleIdToken: string,
    rawNonce: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => Promise<void>;
  existingSignInMethods: (email: string) => Promise<string[]>;
```

- [ ] **Step 2: Demo backend — never conflicts**

In `createDemoBackend`, change the two social methods to return `{ status: "signed-in" }` after setting the demo user (keep the existing `active = {...}; for (const cb of subs) cb(active);` bodies) and add:

```ts
    completeLinkWithPassword: async () => {},
    completeLinkWithGoogle: async () => {},
    completeLinkWithApple: async () => {},
    existingSignInMethods: async () => [],
```

- [ ] **Step 3: Firebase backend — delegate**

In `createFirebaseBackend`, the social methods now return the gip outcome directly (drop the `await`-and-discard so the outcome reaches the caller):

```ts
    signInWithGoogle: (idToken, accessToken) => gip.signInWithGoogle(idToken, accessToken),
    signInWithApple: (idToken, rawNonce, fullName) =>
      gip.signInWithApple(idToken, rawNonce, fullName),
    completeLinkWithPassword: (email, password, pending) =>
      gip.completeLinkWithPassword(email, password, pending),
    completeLinkWithGoogle: (googleIdToken, pending) =>
      gip.completeLinkWithGoogle(googleIdToken, pending),
    completeLinkWithApple: (appleIdToken, rawNonce, pending) =>
      gip.completeLinkWithApple(appleIdToken, rawNonce, pending),
    existingSignInMethods: (email) => gip.existingSignInMethods(email),
```

- [ ] **Step 4: Surface on the context**

Add wrappers on `AuthProvider` mirroring the existing `signIn` wrapper style, and include all four new members plus the updated social methods in the `AuthContext.Provider value`. The social wrappers must **return** the outcome:

```ts
  const signInWithGoogle = (idToken: string, accessToken?: string) =>
    backend.signInWithGoogle(idToken, accessToken);
  const signInWithApple = (
    idToken: string,
    rawNonce: string,
    fullName?: AppleFullName | null,
  ) => backend.signInWithApple(idToken, rawNonce, fullName);
  const completeLinkWithPassword = (
    email: string,
    password: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => backend.completeLinkWithPassword(email, password, pending);
  const completeLinkWithGoogle = (
    googleIdToken: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => backend.completeLinkWithGoogle(googleIdToken, pending);
  const completeLinkWithApple = (
    appleIdToken: string,
    rawNonce: string,
    pending: FirebaseAuthTypes.AuthCredential,
  ) => backend.completeLinkWithApple(appleIdToken, rawNonce, pending);
  const existingSignInMethods = (email: string) => backend.existingSignInMethods(email);
```

- [ ] **Step 5: Typecheck + full suite**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx tsc --noEmit 2>&1 | grep -E "auth/provider" || echo "provider TYPE-CLEAN"`
Expected: `provider TYPE-CLEAN`.
Run: `npx jest`
Expected: all suites still green.

- [ ] **Step 6: Commit**

```bash
git add packages/mobile-shared/auth/provider.tsx
git commit -m "feat(mobile-shared): expose account-link surface on AuthProvider (demo + firebase)"
```

---

### Task 5: `LinkAccountPrompt` modal

**Files:**
- Create: `apps/mobile-admin/components/auth/LinkAccountPrompt.tsx`
- Create: `apps/mobile-admin/__tests__/LinkAccountPrompt.test.tsx`

**Interfaces:**
- Consumes: `useAuth().existingSignInMethods`, `.completeLinkWithPassword`, `.completeLinkWithGoogle`, `.completeLinkWithApple` (Task 4); `signInWithGoogleNative`, `signInWithAppleNative`, `configureGoogleSignin` from `@/lib/social-auth`.
- Produces:

```ts
export interface LinkAccountPromptProps {
  visible: boolean;
  email: string;
  provider: "google.com" | "apple.com";
  pendingCredential: FirebaseAuthTypes.AuthCredential;
  onCancel: () => void;
  onLinked: () => void;
}
export function LinkAccountPrompt(props: LinkAccountPromptProps): React.ReactElement;
```

- [ ] **Step 1: Write the failing tests**

Create `apps/mobile-admin/__tests__/LinkAccountPrompt.test.tsx`:

```tsx
const mockAuth: Record<string, unknown> = {};
jest.mock("@repo/mobile-shared/auth/provider", () => ({
  useAuth: () => mockAuth,
}));
jest.mock("@/lib/social-auth", () => ({
  configureGoogleSignin: jest.fn(),
  signInWithGoogleNative: jest.fn().mockResolvedValue("existing-gtok"),
  signInWithAppleNative: jest
    .fn()
    .mockResolvedValue({ idToken: "existing-atok", rawNonce: "", fullName: null }),
}));

import { fireEvent, render, waitFor } from "@testing-library/react-native";
import { LinkAccountPrompt } from "../components/auth/LinkAccountPrompt";

const PENDING = { provider: "google", idToken: "pending-tok" } as never;

function setAuth(overrides: Record<string, unknown> = {}) {
  Object.keys(mockAuth).forEach((k) => delete mockAuth[k]);
  Object.assign(
    mockAuth,
    {
      existingSignInMethods: jest.fn().mockResolvedValue(["password"]),
      completeLinkWithPassword: jest.fn().mockResolvedValue(undefined),
      completeLinkWithGoogle: jest.fn().mockResolvedValue(undefined),
      completeLinkWithApple: jest.fn().mockResolvedValue(undefined),
    },
    overrides,
  );
}

// `provider` is the provider being LINKED (the one that hit the conflict).
function renderPrompt(
  provider: 'google.com' | 'apple.com' = 'google.com',
  onLinked = jest.fn(),
  onCancel = jest.fn(),
) {
  return {
    onLinked,
    onCancel,
    ...render(
      <LinkAccountPrompt
        visible
        email="merchant@store.com"
        provider={provider}
        pendingCredential={PENDING}
        onCancel={onCancel}
        onLinked={onLinked}
      />,
    ),
  };
}

describe("LinkAccountPrompt", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setAuth();
  });

  it("shows the conflicting email and the provider being linked", async () => {
    const { getByText } = renderPrompt();
    await waitFor(() => expect(getByText(/merchant@store.com/)).toBeTruthy());
    expect(getByText(/Google/)).toBeTruthy();
  });

  // Linking Google onto an existing password account (the common case).
  it("password method: submitting links with the password", async () => {
    const { getByLabelText, onLinked } = renderPrompt("google.com");
    await waitFor(() => expect(getByLabelText("Password")).toBeTruthy());
    fireEvent.changeText(getByLabelText("Password"), "hunter2");
    fireEvent.press(getByLabelText("Sign in and link"));
    await waitFor(() =>
      expect(mockAuth.completeLinkWithPassword).toHaveBeenCalledWith(
        "merchant@store.com",
        "hunter2",
        PENDING,
      ),
    );
    await waitFor(() => expect(onLinked).toHaveBeenCalled());
  });

  // Linking APPLE onto an account whose existing method is Google — the only
  // coherent way a Google re-auth button appears (you can never re-auth with
  // the same provider you are linking).
  it("google method: offers a Google re-auth button that links", async () => {
    setAuth({ existingSignInMethods: jest.fn().mockResolvedValue(["google.com"]) });
    const { getByLabelText, onLinked } = renderPrompt("apple.com");
    await waitFor(() => expect(getByLabelText("Continue with Google to link")).toBeTruthy());
    fireEvent.press(getByLabelText("Continue with Google to link"));
    await waitFor(() =>
      expect(mockAuth.completeLinkWithGoogle).toHaveBeenCalledWith("existing-gtok", PENDING),
    );
    await waitFor(() => expect(onLinked).toHaveBeenCalled());
  });

  // Enumeration protection hides the answer: offer password + every OTHER
  // provider, but never the provider being linked.
  it("enumeration-protected ([]): shows password plus the other providers, not the one being linked", async () => {
    setAuth({ existingSignInMethods: jest.fn().mockResolvedValue([]) });
    const { getByLabelText, queryByLabelText } = renderPrompt("google.com");
    await waitFor(() => expect(getByLabelText("Password")).toBeTruthy());
    expect(getByLabelText("Continue with Apple to link")).toBeTruthy();
    expect(queryByLabelText("Continue with Google to link")).toBeNull();
  });

  it("shows an error and stays open when the re-auth fails", async () => {
    setAuth({
      completeLinkWithPassword: jest.fn().mockRejectedValue(new Error("Wrong password")),
    });
    const { getByLabelText, findByText, onLinked } = renderPrompt();
    await waitFor(() => expect(getByLabelText("Password")).toBeTruthy());
    fireEvent.changeText(getByLabelText("Password"), "nope");
    fireEvent.press(getByLabelText("Sign in and link"));
    expect(await findByText("Wrong password")).toBeTruthy();
    expect(onLinked).not.toHaveBeenCalled();
  });

  it("cancel closes without linking", async () => {
    const { getByLabelText, onCancel } = renderPrompt();
    await waitFor(() => expect(getByLabelText("Password")).toBeTruthy());
    fireEvent.press(getByLabelText("Cancel"));
    expect(onCancel).toHaveBeenCalled();
    expect(mockAuth.completeLinkWithPassword).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest LinkAccountPrompt`
Expected: FAIL — component does not exist.

- [ ] **Step 3: Create the component**

Create `apps/mobile-admin/components/auth/LinkAccountPrompt.tsx`. Use react-native `Modal` (as `components/StoreSelector.tsx` does) but nativewind classes (as `app/login.tsx` does). Methods are fetched on mount; `[]` (enumeration protection) shows password **and** both social options so the user can never dead-end.

```tsx
import { useEffect, useState } from 'react';
import { Modal, Pressable, TextInput, View } from 'react-native';
import type { FirebaseAuthTypes } from '@react-native-firebase/auth';
import { useAuth } from '@repo/mobile-shared/auth/provider';
import {
  configureGoogleSignin,
  signInWithAppleNative,
  signInWithGoogleNative,
} from '@/lib/social-auth';
import { Text } from '../ui/Text';

export interface LinkAccountPromptProps {
  visible: boolean;
  email: string;
  provider: 'google.com' | 'apple.com';
  pendingCredential: FirebaseAuthTypes.AuthCredential;
  onCancel: () => void;
  onLinked: () => void;
}

const PROVIDER_LABEL: Record<LinkAccountPromptProps['provider'], string> = {
  'google.com': 'Google',
  'apple.com': 'Apple',
};

function getErrorMessage(e: unknown): string {
  return e instanceof Error && e.message
    ? e.message
    : 'Could not link your account. Try again.';
}

export function LinkAccountPrompt({
  visible,
  email,
  provider,
  pendingCredential,
  onCancel,
  onLinked,
}: LinkAccountPromptProps) {
  const {
    existingSignInMethods,
    completeLinkWithPassword,
    completeLinkWithGoogle,
    completeLinkWithApple,
  } = useAuth();
  const [methods, setMethods] = useState<string[] | null>(null);
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const found = await existingSignInMethods(email);
        if (!cancelled) setMethods(found);
      } catch {
        // Fail open — unknown methods render every option.
        if (!cancelled) setMethods([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [email, existingSignInMethods]);

  // `[]` means enumeration protection hid the answer — offer everything.
  const unknown = methods !== null && methods.length === 0;
  const showPassword = methods === null || unknown || methods.includes('password');
  const showGoogle =
    provider !== 'google.com' && (unknown || (methods?.includes('google.com') ?? false));
  const showApple =
    provider !== 'apple.com' && (unknown || (methods?.includes('apple.com') ?? false));

  async function run(fn: () => Promise<void>) {
    if (busy) return;
    setError(null);
    setBusy(true);
    try {
      await fn();
      onLinked();
    } catch (e: unknown) {
      setError(getErrorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onCancel}>
      <View className="flex-1 justify-end bg-ink/40">
        <View className="rounded-t-xl bg-paper-elevated px-6 pb-10 pt-6">
          <Text preset="h3">Link your account</Text>
          <Text preset="body" className="mt-2 text-ink-muted">
            An account already exists for {email}. Sign in to connect{' '}
            {PROVIDER_LABEL[provider]}.
          </Text>

          {showPassword ? (
            <View className="mt-6 gap-3">
              <TextInput
                accessibilityLabel="Password"
                className="min-h-touch rounded border border-border bg-paper px-4 font-sans text-body text-ink"
                placeholder="Password"
                placeholderTextColor="#7A766E"
                secureTextEntry
                value={password}
                onChangeText={setPassword}
              />
              <Pressable
                accessibilityRole="button"
                accessibilityLabel="Sign in and link"
                disabled={busy}
                onPress={() =>
                  void run(() => completeLinkWithPassword(email, password, pendingCredential))
                }
                className="min-h-touch items-center justify-center rounded bg-ink active:opacity-90"
              >
                <Text preset="bodyEmphasis" className="text-paper">
                  {busy ? 'Linking…' : 'Sign in and link'}
                </Text>
              </Pressable>
            </View>
          ) : null}

          {showGoogle ? (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel="Continue with Google to link"
              disabled={busy}
              onPress={() =>
                void run(async () => {
                  configureGoogleSignin();
                  const idToken = await signInWithGoogleNative();
                  await completeLinkWithGoogle(idToken, pendingCredential);
                })
              }
              className="mt-3 min-h-touch items-center justify-center rounded border border-border bg-paper active:opacity-90"
            >
              <Text preset="bodyEmphasis">Continue with Google to link</Text>
            </Pressable>
          ) : null}

          {showApple ? (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel="Continue with Apple to link"
              disabled={busy}
              onPress={() =>
                void run(async () => {
                  const { idToken, rawNonce } = await signInWithAppleNative();
                  await completeLinkWithApple(idToken, rawNonce, pendingCredential);
                })
              }
              className="mt-3 min-h-touch items-center justify-center rounded bg-ink active:opacity-90"
            >
              <Text preset="bodyEmphasis" className="text-paper">
                Continue with Apple to link
              </Text>
            </Pressable>
          ) : null}

          {error ? (
            <Text
              preset="caption"
              className="mt-3 text-danger"
              accessibilityRole="alert"
              accessibilityLiveRegion="polite"
            >
              {error}
            </Text>
          ) : null}

          <Pressable
            accessibilityRole="button"
            accessibilityLabel="Cancel"
            disabled={busy}
            onPress={onCancel}
            className="mt-4 min-h-touch items-center justify-center rounded"
          >
            <Text preset="body" className="text-ink-muted">
              Cancel
            </Text>
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest LinkAccountPrompt`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/components/auth/LinkAccountPrompt.tsx apps/mobile-admin/__tests__/LinkAccountPrompt.test.tsx
git commit -m "feat(mobile-admin): add LinkAccountPrompt for cross-provider account merge"
```

---

### Task 6: `login.tsx` — open the prompt on a needs-link outcome

**Files:**
- Modify: `apps/mobile-admin/app/login.tsx`
- Modify: `apps/mobile-admin/__tests__/login.test.tsx`

**Interfaces:**
- Consumes: `SocialSignInOutcome` from `useAuth().signInWithGoogle/signInWithApple` (Task 4); `LinkAccountPrompt` (Task 5).

- [ ] **Step 1: Write the failing tests**

In `apps/mobile-admin/__tests__/login.test.tsx`, the existing configurable `mockUseAuth(overrides)` helper drives `useAuth`. Keep every existing test. Add a `LinkAccountPrompt` mock next to the existing `@/lib/social-auth` mock:

```tsx
jest.mock('../components/auth/LinkAccountPrompt', () => ({
  LinkAccountPrompt: ({ email }: { email: string }) => {
    const { Text } = require('react-native');
    return <Text testID="link-prompt">{`link:${email}`}</Text>;
  },
}));
```

and add:

```tsx
  it('opens the link prompt when Google sign-in needs linking', async () => {
    const signInWithGoogle = jest.fn().mockResolvedValue({
      status: 'needs-link',
      email: 'merchant@store.com',
      provider: 'google.com',
      pendingCredential: { provider: 'google', idToken: 'gtok' },
    });
    mockUseAuth({ signInWithGoogle });
    const { getByLabelText, findByTestId } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Continue with Google'));
    expect(await findByTestId('link-prompt')).toBeTruthy();
  });

  it('does not open the link prompt on a normal signed-in outcome', async () => {
    const signInWithGoogle = jest.fn().mockResolvedValue({ status: 'signed-in' });
    mockUseAuth({ signInWithGoogle });
    const { getByLabelText, queryByTestId } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Continue with Google'));
    await waitFor(() => expect(signInWithGoogle).toHaveBeenCalled());
    expect(queryByTestId('link-prompt')).toBeNull();
  });
```

> The existing social tests assert `signInWithGoogle` is called with `'gtok'`; give their `mockUseAuth` defaults a `{ status: 'signed-in' }` resolution so they keep passing.

- [ ] **Step 2: Run to verify they fail**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest login`
Expected: FAIL — no link prompt is rendered.

- [ ] **Step 3: Wire the outcome in `login.tsx`**

Add the import and a `linkTarget` state:

```tsx
import { LinkAccountPrompt } from '../components/auth/LinkAccountPrompt';
import type { SocialSignInOutcome } from '@repo/mobile-shared/auth/social-credentials';

type LinkTarget = Extract<SocialSignInOutcome, { status: 'needs-link' }>;
```

Inside `LoginScreen`:

```tsx
  const [linkTarget, setLinkTarget] = useState<LinkTarget | null>(null);
```

Change both social handlers to inspect the outcome. Google:

```tsx
  async function handleGoogleSignIn() {
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      let outcome: SocialSignInOutcome;
      if (DEMO_AUTH) {
        outcome = await signInWithGoogle('demo-google-token');
      } else {
        configureGoogleSignin();
        const idToken = await signInWithGoogleNative();
        outcome = await signInWithGoogle(idToken);
      }
      if (outcome.status === 'needs-link') setLinkTarget(outcome);
    } catch (e: unknown) {
      setError(getErrorMessage(e));
    } finally {
      setSubmitting(false);
    }
  }
```

Apple, identically:

```tsx
  async function handleAppleSignIn() {
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      let outcome: SocialSignInOutcome;
      if (DEMO_AUTH) {
        outcome = await signInWithApple('demo-apple-token', '', null);
      } else {
        const { idToken, rawNonce, fullName } = await signInWithAppleNative();
        outcome = await signInWithApple(idToken, rawNonce, fullName);
      }
      if (outcome.status === 'needs-link') setLinkTarget(outcome);
    } catch (e: unknown) {
      setError(getErrorMessage(e));
    } finally {
      setSubmitting(false);
    }
  }
```

Render the prompt at the end of the `SafeAreaView`, after the Apple button's closing tag and before `</View></SafeAreaView>`:

```tsx
        {linkTarget ? (
          <LinkAccountPrompt
            visible
            email={linkTarget.email}
            provider={linkTarget.provider}
            pendingCredential={linkTarget.pendingCredential}
            onCancel={() => setLinkTarget(null)}
            onLinked={() => setLinkTarget(null)}
          />
        ) : null}
```

- [ ] **Step 4: Run to verify they pass**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest login`
Expected: PASS (all existing login tests + the 2 new ones).

- [ ] **Step 5: Full suite + typecheck**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest && npx tsc --noEmit 2>&1 | grep -vE "app/\(tabs\)/_layout" | grep "error" || echo "ALL GREEN"`
Expected: all suites pass; no new type errors.

- [ ] **Step 6: Commit**

```bash
git add apps/mobile-admin/app/login.tsx apps/mobile-admin/__tests__/login.test.tsx
git commit -m "feat(mobile-admin): prompt to link accounts when social sign-in hits an existing email"
```

---

## Final verification

- [ ] **Full mobile suite:** `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest` → all green.
- [ ] **Typecheck:** `npx tsc --noEmit` → only the 2 pre-existing `app/(tabs)/_layout.tsx` expo-notifications errors.
- [ ] **Demo build unaffected:** `EXPO_PUBLIC_AUTH_BACKEND=demo npx expo config --json | grep -c googleServicesFile` → `0`; demo social buttons still route to the demo backend (no link path).
- [ ] **Manual (needs a real device/sim + a password account):** sign in with Google using the email of an existing password account → the link prompt appears → enter the password → linked → dashboard. This closes web parity.
