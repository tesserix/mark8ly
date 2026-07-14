# mobile-admin Phase 1 — Real Auth (+ Social Login) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the demo login into real GIP authentication — email/password + **Continue with Google** + **Sign in with Apple** — and verify the live login → tenant resolution → dashboard path against the real backend.

**Architecture:** The domain layer already exists and renders (api-client, data hooks, `TenantGate`, `StorePicker`, dashboard — proven in Phase 0's demo). Phase 1 adds the real auth methods to `@repo/mobile-shared/auth` (mirroring Home-Chef's `signInWith{Google,Apple}Credential` → GIP `signInWithCredential`), wires social buttons into the login screen, adds the native config, and verifies the real path. mark8ly uses GIP **directly** (no BFF): after `signInWithCredential`, Firebase `onAuthStateChanged` fires and the existing `AuthGate` routes — so social handlers are just credential→sign-in.

**Tech Stack:** `@react-native-firebase/auth` (GoogleAuthProvider / AppleAuthProvider credentials), `@react-native-google-signin/google-signin`, `expo-apple-authentication`, existing `@repo/mobile-shared/auth` provider, nativewind Paper·Ink·Moss.

## Global Constraints

- **Providers:** email/password (exists) + **Google** (mirror web admin's "Continue with Google") + **Apple** (Apple mandates Sign-in-with-Apple on iOS once another social provider is offered). No others. (Design decision, this session)
- **No BFF:** mark8ly auth is GIP-direct via `@repo/mobile-shared/auth` — `onAuthStateChanged` drives routing. Do NOT add Home-Chef's `completeBFFLogin`/`setAuthResponse` calls. (Spec §Approach — Auth seam)
- **Demo toggle stays:** `EXPO_PUBLIC_AUTH_BACKEND=demo` remains a dev-only escape hatch; real auth is the default. The demo backend + `lib/demo-api-client.ts` stay for credential-free sim testing. (Design decision, this session)
- **Existing screen styling preserved:** Phase 1 does NOT restyle the tab/detail screens — the full Paper·Ink·Moss restyle is Phase 2. Only the login screen gains social buttons. (Design decision, this session)
- **Secrets are env-driven, never hardcoded:** `EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID`, `EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID`, `GoogleService-Info.plist`, and the reversed-client-id `iosUrlScheme` come from env / gitignored files. No secret in source. (Global security rules)
- **Google credential call:** `auth.GoogleAuthProvider.credential(idToken)` → `auth().signInWithCredential(cred)`. **Apple:** `auth.AppleAuthProvider.credential(idToken, rawNonce)` → `signInWithCredential`, backfilling `displayName` from Apple's first-authorization `fullName`. (Home-Chef `packages/mobile-shared/src/auth/sign-in.ts`)
- **Commits:** single-line conventional, no signatures. (Global git rules)

---

# Phase 1a — Credential-free (build + review now)

Everything here is buildable and testable WITHOUT the real plist / client IDs (native sign-in is mocked in tests; the demo backend covers the sim). Reviewable and mergeable before credentials arrive.

## File Structure (1a)

| File | Responsibility | Action |
|---|---|---|
| `packages/mobile-shared/auth/gip.ts` | add `signInWithGoogle`/`signInWithApple` to the GIP client | Modify |
| `packages/mobile-shared/auth/provider.tsx` | expose `signInWithGoogle`/`signInWithApple` on `AuthState`; demo + firebase backends | Modify |
| `packages/mobile-shared/auth/social-credentials.ts` | `signInWithGoogleCredential`/`signInWithAppleCredential` helpers | Create |
| `apps/mobile-admin/package.json` | add `@react-native-google-signin/google-signin`, `expo-apple-authentication` | Modify |
| `apps/mobile-admin/app/login.tsx` | Google + Apple buttons + handlers | Modify |
| `apps/mobile-admin/lib/social-auth.ts` | `configureGoogleSignin()` + `signInWithGoogleNative()` + `signInWithAppleNative()` wrappers | Create |
| `apps/mobile-admin/app.config.js` | google-signin + apple plugins (conditional), `usesAppleSignIn`, `iosUrlScheme` from env | Modify |
| `apps/mobile-admin/eas.json` | `EXPO_PUBLIC_GOOGLE_*` env keys (placeholder-empty) | Modify |
| `apps/mobile-admin/__tests__/login.test.tsx` | render + handler tests (mock native modules) | Modify |
| `packages/mobile-shared/auth/social-credentials.test.ts` | credential-mapping unit test | Create |

### Task 1a.1: Credential helpers + GIP client social methods

**Files:**
- Create: `packages/mobile-shared/auth/social-credentials.ts`
- Create: `packages/mobile-shared/auth/social-credentials.test.ts`
- Modify: `packages/mobile-shared/auth/gip.ts`

**Interfaces:**
- Produces:
  - `social-credentials.ts` → `signInWithGoogleCredential(idToken: string, accessToken?: string): Promise<FirebaseAuthTypes.UserCredential>` and `signInWithAppleCredential(idToken: string, rawNonce: string, fullName?: { givenName?: string | null; familyName?: string | null } | null): Promise<FirebaseAuthTypes.UserCredential>`.
  - `gip.ts` `createGIPAuth(...)` return gains `signInWithGoogle(idToken, accessToken?)` and `signInWithApple(idToken, rawNonce, fullName?)`.

- [ ] **Step 1: Write the failing credential test** `packages/mobile-shared/auth/social-credentials.test.ts`

```typescript
const signInWithCredential = jest.fn().mockResolvedValue({ user: { displayName: "X", updateProfile: jest.fn() } });
const googleCredential = jest.fn((idToken: string) => ({ provider: "google", idToken }));
const appleCredential = jest.fn((idToken: string, nonce: string) => ({ provider: "apple", idToken, nonce }));

jest.mock("@react-native-firebase/auth", () => {
  const authFn = () => ({ signInWithCredential });
  authFn.GoogleAuthProvider = { credential: googleCredential };
  authFn.AppleAuthProvider = { credential: appleCredential };
  return { __esModule: true, default: authFn };
});

import { signInWithGoogleCredential, signInWithAppleCredential } from "./social-credentials";

it("maps a Google id_token to a GIP credential sign-in", async () => {
  await signInWithGoogleCredential("gtok");
  expect(googleCredential).toHaveBeenCalledWith("gtok", undefined);
  expect(signInWithCredential).toHaveBeenCalledWith({ provider: "google", idToken: "gtok" });
});

it("maps an Apple id_token + nonce to a GIP credential sign-in", async () => {
  await signInWithAppleCredential("atok", "nonce123", null);
  expect(appleCredential).toHaveBeenCalledWith("atok", "nonce123");
  expect(signInWithCredential).toHaveBeenCalledWith({ provider: "apple", idToken: "atok", nonce: "nonce123" });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd apps/mobile-admin && npx jest ../../packages/mobile-shared/auth/social-credentials.test.ts`
Expected: FAIL — module not found.

> Note: `@repo/mobile-shared` has no jest runner of its own; run its tests through the mobile-admin jest config (which already resolves the package). Confirm the path resolves; if not, add the test under `apps/mobile-admin/__tests__/` importing `@repo/mobile-shared/auth/social-credentials`.

- [ ] **Step 3: Create `social-credentials.ts`** (verbatim port of Home-Chef `packages/mobile-shared/src/auth/sign-in.ts`, trimmed to the two helpers)

```typescript
import auth, { FirebaseAuthTypes } from "@react-native-firebase/auth";

export async function signInWithGoogleCredential(
  idToken: string,
  accessToken?: string,
): Promise<FirebaseAuthTypes.UserCredential> {
  const cred = auth.GoogleAuthProvider.credential(idToken, accessToken);
  return auth().signInWithCredential(cred);
}

export interface AppleFullName {
  givenName?: string | null;
  familyName?: string | null;
}

export async function signInWithAppleCredential(
  idToken: string,
  rawNonce: string,
  fullName?: AppleFullName | null,
): Promise<FirebaseAuthTypes.UserCredential> {
  const cred = auth.AppleAuthProvider.credential(idToken, rawNonce);
  const result = await auth().signInWithCredential(cred);
  const displayName = buildDisplayName(fullName);
  if (displayName && !result.user.displayName) {
    try {
      await result.user.updateProfile({ displayName });
    } catch {
      // Best-effort: name capture is non-fatal; the user stays signed in.
    }
  }
  return result;
}

function buildDisplayName(fullName?: AppleFullName | null): string {
  if (!fullName) return "";
  return [fullName.givenName, fullName.familyName]
    .map((p) => (p ?? "").trim())
    .filter((p) => p.length > 0)
    .join(" ");
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/mobile-admin && npx jest social-credentials`
Expected: PASS (2 tests).

- [ ] **Step 5: Add social methods to `gip.ts`** — read `packages/mobile-shared/auth/gip.ts` first; add to the object returned by `createGIPAuth`:

```typescript
    signInWithGoogle: (idToken: string, accessToken?: string) =>
      signInWithGoogleCredential(idToken, accessToken),
    signInWithApple: (
      idToken: string,
      rawNonce: string,
      fullName?: AppleFullName | null,
    ) => signInWithAppleCredential(idToken, rawNonce, fullName),
```

with `import { signInWithGoogleCredential, signInWithAppleCredential, type AppleFullName } from "./social-credentials";` at the top. (The GIP client already calls `auth()` for tenantId; these reuse the default app.)

- [ ] **Step 6: Commit**

```bash
git add packages/mobile-shared/auth/social-credentials.ts packages/mobile-shared/auth/social-credentials.test.ts packages/mobile-shared/auth/gip.ts
git commit -m "feat(mobile-shared): GIP Google + Apple credential sign-in helpers"
```

### Task 1a.2: Expose social sign-in on the AuthProvider (both backends)

**Files:**
- Modify: `packages/mobile-shared/auth/provider.tsx`

**Interfaces:**
- Consumes: `gip.ts` `signInWithGoogle`/`signInWithApple` (Task 1a.1).
- Produces: `AuthState` (returned by `useAuth()`) gains `signInWithGoogle(idToken: string, accessToken?: string): Promise<void>` and `signInWithApple(idToken: string, rawNonce: string, fullName?: AppleFullName | null): Promise<void>`. The **demo** backend implements them by signing in the demo user (so demo builds' social buttons work); the **firebase** backend delegates to `gip.*`.

- [ ] **Step 1: Extend `AuthBackend` + `AuthState`** — read `provider.tsx`; add to both interfaces:

```typescript
  signInWithGoogle: (idToken: string, accessToken?: string) => Promise<void>;
  signInWithApple: (
    idToken: string,
    rawNonce: string,
    fullName?: AppleFullName | null,
  ) => Promise<void>;
```

- [ ] **Step 2: Implement in `createDemoBackend`** — both resolve by setting the demo user (mirrors the existing demo `signIn`):

```typescript
    signInWithGoogle: async () => {
      active = { uid: "expo-go-demo:google", email: "demo@mark8ly.com", displayName: "Demo Admin" };
      for (const cb of subs) cb(active);
    },
    signInWithApple: async () => {
      active = { uid: "expo-go-demo:apple", email: "demo@mark8ly.com", displayName: "Demo Admin" };
      for (const cb of subs) cb(active);
    },
```

- [ ] **Step 3: Implement in `createFirebaseBackend`** — delegate to the GIP client:

```typescript
    signInWithGoogle: (idToken, accessToken) => gip.signInWithGoogle(idToken, accessToken),
    signInWithApple: (idToken, rawNonce, fullName) => gip.signInWithApple(idToken, rawNonce, fullName),
```

- [ ] **Step 4: Surface on the context** — add `signInWithGoogle`/`signInWithApple` wrappers (like the existing `signIn`) and include them in the `AuthContext.Provider value`.

- [ ] **Step 5: Type-check**

Run: `cd apps/mobile-admin && npx tsc --noEmit 2>&1 | grep "auth/provider" || echo "provider TYPE-CLEAN"`
Expected: `provider TYPE-CLEAN`.

- [ ] **Step 6: Commit**

```bash
git add packages/mobile-shared/auth/provider.tsx
git commit -m "feat(mobile-shared): expose signInWithGoogle/Apple on AuthProvider (demo + firebase)"
```

### Task 1a.3: Native social wrappers (Google config + Apple nonce)

**Files:**
- Modify: `apps/mobile-admin/package.json` (add deps)
- Create: `apps/mobile-admin/lib/social-auth.ts`

**Interfaces:**
- Produces: `configureGoogleSignin(): void` (idempotent, reads env client IDs), `signInWithGoogleNative(): Promise<string>` (returns Google id_token), `signInWithAppleNative(): Promise<{ idToken: string; rawNonce: string; fullName: AppleFullName | null }>`.

- [ ] **Step 1: Add deps** to `apps/mobile-admin/package.json` dependencies, then `npx expo install @react-native-google-signin/google-signin expo-apple-authentication` (pins SDK-correct versions).

- [ ] **Step 2: Create `lib/social-auth.ts`**

```typescript
import { GoogleSignin } from "@react-native-google-signin/google-signin";
import * as AppleAuthentication from "expo-apple-authentication";
import type { AppleFullName } from "@repo/mobile-shared/auth/social-credentials";

let configured = false;

export function configureGoogleSignin(): void {
  if (configured) return;
  const webClientId = process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID;
  if (!webClientId) {
    throw new Error("EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID is not configured");
  }
  GoogleSignin.configure({
    webClientId,
    iosClientId: process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID,
  });
  configured = true;
}

export async function signInWithGoogleNative(): Promise<string> {
  await GoogleSignin.hasPlayServices({ showPlayServicesUpdateDialog: true });
  const result = (await GoogleSignin.signIn()) as {
    data?: { idToken?: string | null };
    idToken?: string | null;
  };
  const idToken = result?.data?.idToken ?? result?.idToken;
  if (!idToken) throw new Error("Google sign-in failed: no ID token");
  return idToken;
}

export async function signInWithAppleNative(): Promise<{
  idToken: string;
  rawNonce: string;
  fullName: AppleFullName | null;
}> {
  const cred = await AppleAuthentication.signInAsync({
    requestedScopes: [
      AppleAuthentication.AppleAuthenticationScope.FULL_NAME,
      AppleAuthentication.AppleAuthenticationScope.EMAIL,
    ],
  });
  if (!cred.identityToken) throw new Error("Apple sign-in failed: no identity token");
  // Home-Chef passes an empty rawNonce (GIP verifies Apple's token without a
  // client nonce in their setup); keep parity. Revisit if GIP rejects it in 1b.
  return { idToken: cred.identityToken, rawNonce: "", fullName: cred.fullName };
}
```

- [ ] **Step 3: Type-check the new file**

Run: `cd apps/mobile-admin && npx tsc --noEmit 2>&1 | grep "lib/social-auth" || echo "social-auth TYPE-CLEAN"`
Expected: `social-auth TYPE-CLEAN`.

- [ ] **Step 4: Commit**

```bash
git add apps/mobile-admin/package.json apps/mobile-admin/package-lock.json apps/mobile-admin/lib/social-auth.ts
git commit -m "feat(mobile-admin): native Google + Apple sign-in wrappers"
```

### Task 1a.4: Login screen — Google + Apple buttons (TDD)

**Files:**
- Modify: `apps/mobile-admin/app/login.tsx`
- Modify: `apps/mobile-admin/__tests__/login.test.tsx`

**Interfaces:**
- Consumes: `useAuth().signInWithGoogle/signInWithApple` (1a.2); `configureGoogleSignin/signInWithGoogleNative/signInWithAppleNative` (1a.3).

- [ ] **Step 1: Write failing tests** — add to `__tests__/login.test.tsx` (mock the native wrappers + `useAuth`):

```tsx
jest.mock("@/lib/social-auth", () => ({
  configureGoogleSignin: jest.fn(),
  signInWithGoogleNative: jest.fn().mockResolvedValue("gtok"),
  signInWithAppleNative: jest.fn().mockResolvedValue({ idToken: "atok", rawNonce: "", fullName: null }),
}));

it("signs in with Google when the Google button is pressed", async () => {
  const signInWithGoogle = jest.fn().mockResolvedValue(undefined);
  mockUseAuth({ signInWithGoogle });
  const { getByLabelText } = render(<LoginScreen />);
  fireEvent.press(getByLabelText("Continue with Google"));
  await waitFor(() => expect(signInWithGoogle).toHaveBeenCalledWith("gtok"));
});

it("signs in with Apple when the Apple button is pressed", async () => {
  const signInWithApple = jest.fn().mockResolvedValue(undefined);
  mockUseAuth({ signInWithApple });
  const { getByLabelText } = render(<LoginScreen />);
  fireEvent.press(getByLabelText("Sign in with Apple"));
  await waitFor(() => expect(signInWithApple).toHaveBeenCalledWith("atok", "", null));
});
```

> `mockUseAuth` is the existing `@repo/mobile-shared/auth/provider` mock in this test file — extend it to accept `signInWithGoogle`/`signInWithApple`. Keep the existing email/password + error tests.

- [ ] **Step 2: Run to verify fail** — Run: `cd apps/mobile-admin && npx jest login`. Expected: the two new tests FAIL (no such buttons).

- [ ] **Step 3: Add buttons + handlers to `login.tsx`** — read the current file; call `configureGoogleSignin()` in a mount effect, add below the email/password `Sign in` button a hairline "or" divider then:
  - **Continue with Google** — Paper button (`bg-paper-elevated border border-border`), `accessibilityLabel="Continue with Google"`, handler: `signInWithGoogleNative()` → `signInWithGoogle(idToken)`, wrapped in the existing try/catch → `setError`.
  - **Sign in with Apple** — Ink button (`bg-ink`, Paper label, per Apple HIG), `accessibilityLabel="Sign in with Apple"`, handler: `signInWithAppleNative()` → `signInWithApple(idToken, rawNonce, fullName)`, same try/catch. Reuse the `submitting` guard so all three paths share the in-flight state.

- [ ] **Step 4: Run to verify pass** — Run: `cd apps/mobile-admin && npx jest login`. Expected: all login tests PASS (existing + 2 new).

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/app/login.tsx apps/mobile-admin/__tests__/login.test.tsx
git commit -m "feat(mobile-admin): Google + Apple sign-in buttons on login screen"
```

### Task 1a.5: Native config scaffolding (env-driven, no secrets)

**Files:**
- Modify: `apps/mobile-admin/app.config.js`
- Modify: `apps/mobile-admin/eas.json`

**Interfaces:**
- Produces: an Expo config that (in non-demo builds) includes `@react-native-firebase/app`, `expo-apple-authentication`, and `@react-native-google-signin/google-signin` (with `iosUrlScheme` from `EXPO_PUBLIC_GOOGLE_IOS_URL_SCHEME`), sets `ios.usesAppleSignIn: true`, and references `ios.googleServicesFile` when present.

- [ ] **Step 1: Update `app.config.js`** — extend the existing `USE_DEMO_AUTH` conditional plugins:

```javascript
    plugins: [
      'expo-router',
      'expo-font',
      'expo-secure-store',
      'expo-local-authentication',
      'expo-image-picker',
      'expo-notifications',
      ['expo-build-properties', { ios: { newArchEnabled: true, useFrameworks: 'static' } }],
      ...(USE_DEMO_AUTH
        ? []
        : [
            '@react-native-firebase/app',
            'expo-apple-authentication',
            [
              '@react-native-google-signin/google-signin',
              { iosUrlScheme: process.env.EXPO_PUBLIC_GOOGLE_IOS_URL_SCHEME || '' },
            ],
          ]),
    ],
```

Add to the iOS block: `usesAppleSignIn: true`, and `googleServicesFile: process.env.GOOGLE_SERVICES_PLIST || './GoogleService-Info.plist'` (the file itself is gitignored; provided in 1b).

- [ ] **Step 2: Add env keys to `eas.json`** — add empty-string `EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID`, `EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID`, `EXPO_PUBLIC_GOOGLE_IOS_URL_SCHEME` to the `development`/`preview` profiles' `env` (real values injected in 1b / via local env).

- [ ] **Step 3: Verify config resolves in demo mode (still no firebase/google plugins)**

Run: `cd apps/mobile-admin && EXPO_PUBLIC_AUTH_BACKEND=demo npx expo config --json > /dev/null && echo "DEMO CONFIG OK"`
Expected: `DEMO CONFIG OK` (demo builds still skip the native-auth plugins → prebuild works without a plist).

- [ ] **Step 4: Add `GoogleService-Info.plist` to `.gitignore`**

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/app.config.js apps/mobile-admin/eas.json apps/mobile-admin/.gitignore
git commit -m "chore(mobile-admin): env-driven Google/Apple/Firebase native config (no secrets)"
```

## Self-Review (1a)

**Spec coverage:** credential helpers (1a.1) ✓; provider surface for both backends (1a.2) ✓; native wrappers (1a.3) ✓; login UI + tests (1a.4) ✓; env-driven config, demo build unaffected (1a.5) ✓. Real-auth verification is deliberately deferred to 1b.
**Placeholder scan:** no TBD; the one open item (Apple `rawNonce=''` parity) is flagged for 1b confirmation, not hand-waved.
**Type consistency:** `AppleFullName` shape identical across `social-credentials.ts`, `gip.ts`, `provider.tsx`, `social-auth.ts`. `signInWithGoogle(idToken, accessToken?)` / `signInWithApple(idToken, rawNonce, fullName?)` signatures match across the GIP client, both backends, the context, and the login handlers.

---

# Phase 1b — Real-auth enablement + verification (needs credentials)

Execute once the user provides the **`GoogleService-Info.plist`** (mark8ly GIP iOS app, bundle `com.mark8ly.admin`) and the **Google OAuth client IDs** (web + iOS) + reversed-client-id URL scheme.

### Task 1b.1: Drop in credentials

- [ ] Place `GoogleService-Info.plist` at `apps/mobile-admin/GoogleService-Info.plist` (gitignored).
- [ ] Set locally (or in the EAS profile `env`): `EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID`, `EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID`, `EXPO_PUBLIC_GOOGLE_IOS_URL_SCHEME` (the `com.googleusercontent.apps.<REVERSED_CLIENT_ID>` value).
- [ ] Confirm the GIP tenant has **Google** and **Apple** providers enabled in the Firebase/Identity Platform console (mirrors web admin's Google; Apple is new).

### Task 1b.2: Real native build (default auth, not demo)

- [ ] `cd apps/mobile-admin && rm -rf ios && npx expo run:ios --device "iPhone 17 Pro"` (NO `EXPO_PUBLIC_AUTH_BACKEND=demo` → real firebase + google-signin + apple plugins, real plist).
- [ ] Confirm prebuild + pod install + Xcode build succeed with the Firebase/Google/Apple pods (`useFrameworks: static` already set in Phase 0).
- [ ] Failure playbook: Apple `signInWithCredential` rejecting the empty `rawNonce` → generate a nonce (`expo-crypto` SHA-256 of a random string, pass the raw to `signInAsync({ nonce: hashed })` and the raw to the credential); wire it in `social-auth.ts` + `signInWithAppleNative`.

### Task 1b.3: Verify the live path end-to-end

- [ ] **Email/password:** sign in with a real GIP `mp-internal` test account → lands past login.
- [ ] **Continue with Google:** Google sheet → GIP credential → signed in.
- [ ] **Sign in with Apple:** Apple sheet → GIP credential → signed in; first-auth name captured.
- [ ] **Tenant resolution:** `TenantGate` fetches real `/stores`; 1 store auto-selects, 2+ shows `StorePicker`; the picked store's id flows to the api-client header.
- [ ] **Live dashboard:** real `/dashboard` data renders (not the demo canned data); a 401 correctly refreshes the token / signs out (api-client self-correction).
- [ ] Screenshot each of login (with social buttons), store pick (if applicable), and the live dashboard; attach to the phase hand-off.

### Task 1b.4: Confirm demo build still works

- [ ] `EXPO_PUBLIC_AUTH_BACKEND=demo npx expo run:ios` still boots the demo (no plist needed) — the escape hatch is preserved.

## Self-Review (1b)

**Spec coverage:** credentials in place (1b.1) ✓; real build (1b.2) ✓; all three auth methods + tenant + live dashboard verified (1b.3) ✓; demo escape hatch preserved (1b.4) ✓.
**Risk:** Apple nonce — Home-Chef uses an empty rawNonce; if GIP rejects it, 1b.2's playbook adds real nonce generation. Flagged, not hand-waved.
