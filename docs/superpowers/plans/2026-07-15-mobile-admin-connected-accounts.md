# mobile-admin Settings → Security (Connected accounts) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Let a signed-in merchant see their sign-in methods and link/remove Google + Apple — the reliable path for Apple "Hide My Email" (links to `currentUser`, so the relay email is irrelevant).

**Architecture:** Native RN Firebase, GIP-direct, no BFF. Signed-in linking via `currentUser.linkWithCredential(cred)` — no re-auth, no email matching. Unlink via `currentUser.unlink(providerId)` with a **last-method guard enforced in the auth layer**.

**Tech Stack:** `@react-native-firebase/auth` 24.1.1 (`User.linkWithCredential` User.js:98, `User.unlink` User.js:215, `User.providerData`), Expo 56, expo-router, jest-expo.

## Global Constraints

- **GIP-direct — no BFF.** Never add `completeBFFLogin`/`setAuthResponse`.
- **`provider.tsx` firebase imports stay `import type`** and the lazy `require("./gip")` stays lazy — the Expo Go / demo isolation invariant.
- **Demo backend never touches firebase:** `linkedProviderIds` → `["password"]`; link/unlink are no-ops. The **screen** must also take a `DEMO_AUTH` branch (`process.env.EXPO_PUBLIC_AUTH_BACKEND === 'demo'`) so it never invokes the real native Google/Apple sheets — mirror `app/login.tsx`.
- **Last-method guard lives in `link.ts`** (`providerData.length <= 1` → throw `LastSignInMethodError` BEFORE calling `unlink`). The UI also disables it, but the auth layer is the enforcement point.
- **Style:** the More section uses `StyleSheet` + `theme` + `@/components/ui` primitives (see `more/account.tsx`), **NOT** nativewind classes.
- **TypeScript:** explicit types on exports; no `any` (narrow `unknown`). Immutability.
- **Commits:** single-line conventional, **NO signatures**, direct to `main`.
- **Landmines — do NOT touch:** `metro.config.js`, `tsconfig.json`, `jest.config.js`, `babel.config.js`, tailwind/nativewind wiring, `app.config.js`, `eas.json`. **No `.test.*` under `apps/mobile-admin/app/`** — tests live in `apps/mobile-admin/__tests__/`.
- **jest.mock factories must build their mock fns INSIDE the factory** (babel hoists imports above outer `const`/`var`) and read them back off the imported default export. See `__tests__/link.test.tsx`.

Tests: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest`.
Typecheck: `npx tsc --noEmit` — **ignore the 2 pre-existing `app/(tabs)/_layout.tsx` expo-notifications errors.**

---

### Task 1: `link.ts` — signed-in link/unlink + last-method guard

**Files:**
- Modify: `packages/mobile-shared/auth/link.ts`
- Modify: `apps/mobile-admin/__tests__/link.test.tsx`

**Interfaces — Produces:**
- `linkedProviderIds(): Promise<string[]>`
- `class LastSignInMethodError extends Error`
- `linkGoogleToCurrentUser(idToken: string): Promise<void>`
- `linkAppleToCurrentUser(idToken: string, rawNonce: string): Promise<void>`
- `unlinkProvider(providerId: string): Promise<void>`

- [ ] **Step 1: Write the failing tests**

The existing `jest.mock("@react-native-firebase/auth", …)` factory in `__tests__/link.test.tsx` builds mocks inside itself and exposes `GoogleAuthProvider`/`AppleAuthProvider`. **Extend that factory** so the instance has a mutable `currentUser` with `linkWithCredential`, `unlink`, and `providerData`. Keep every existing test passing. Add:

```tsx
describe("connected accounts", () => {
  beforeEach(() => jest.clearAllMocks());

  it("linkedProviderIds maps providerData", async () => {
    setCurrentUser([{ providerId: "password" }, { providerId: "google.com" }]);
    await expect(linkedProviderIds()).resolves.toEqual(["password", "google.com"]);
  });

  it("linkedProviderIds returns [] when signed out", async () => {
    setCurrentUser(null);
    await expect(linkedProviderIds()).resolves.toEqual([]);
  });

  it("linkGoogleToCurrentUser links the built credential to currentUser", async () => {
    const user = setCurrentUser([{ providerId: "password" }]);
    await linkGoogleToCurrentUser("gtok");
    expect(mockedAuth.GoogleAuthProvider.credential).toHaveBeenCalledWith("gtok");
    expect(user.linkWithCredential).toHaveBeenCalledWith({ provider: "google", idToken: "gtok" });
  });

  it("linkAppleToCurrentUser links the built credential to currentUser", async () => {
    const user = setCurrentUser([{ providerId: "password" }]);
    await linkAppleToCurrentUser("atok", "nonce");
    expect(mockedAuth.AppleAuthProvider.credential).toHaveBeenCalledWith("atok", "nonce");
    expect(user.linkWithCredential).toHaveBeenCalledWith({
      provider: "apple",
      idToken: "atok",
      nonce: "nonce",
    });
  });

  it("unlinkProvider removes a provider when more than one remains", async () => {
    const user = setCurrentUser([{ providerId: "password" }, { providerId: "google.com" }]);
    await unlinkProvider("google.com");
    expect(user.unlink).toHaveBeenCalledWith("google.com");
  });

  it("unlinkProvider REFUSES to remove the last method and never calls unlink", async () => {
    const user = setCurrentUser([{ providerId: "password" }]);
    await expect(unlinkProvider("password")).rejects.toBeInstanceOf(LastSignInMethodError);
    expect(user.unlink).not.toHaveBeenCalled();
  });

  it("link/unlink throw when signed out", async () => {
    setCurrentUser(null);
    await expect(linkGoogleToCurrentUser("g")).rejects.toThrow(/not signed in/i);
    await expect(unlinkProvider("google.com")).rejects.toThrow(/not signed in/i);
  });
});
```

Add a `setCurrentUser(providerData: { providerId: string }[] | null)` helper in the test that assigns a fresh user object (with `jest.fn()` `linkWithCredential`/`unlink` and the given `providerData`) onto the mocked auth instance's `currentUser`, returning it; `null` clears it.

- [ ] **Step 2: Run to verify RED** — `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/mobile-admin && npx jest link.test` → FAIL (exports don't exist).

- [ ] **Step 3: Implement** — append to `packages/mobile-shared/auth/link.ts`:

```ts
/** Thrown when unlinking would remove the user's only remaining sign-in method. */
export class LastSignInMethodError extends Error {
  constructor() {
    super("Cannot remove the only sign-in method");
    this.name = "LastSignInMethodError";
  }
}

function requireCurrentUser(): FirebaseAuthTypes.User {
  const user = auth().currentUser;
  if (!user) throw new Error("Not signed in");
  return user;
}

/** Provider ids on the signed-in user: "password" | "google.com" | "apple.com". */
export async function linkedProviderIds(): Promise<string[]> {
  const user = auth().currentUser;
  if (!user) return [];
  return user.providerData.map((p) => p.providerId);
}

/** Attach Google to the CURRENT user — no re-auth, no email matching. */
export async function linkGoogleToCurrentUser(idToken: string): Promise<void> {
  const user = requireCurrentUser();
  await user.linkWithCredential(auth.GoogleAuthProvider.credential(idToken));
}

/**
 * Attach Apple to the CURRENT user. This is the Apple "Hide My Email" path:
 * because we link to the signed-in user, the relay address Apple may return
 * never has to match anything.
 */
export async function linkAppleToCurrentUser(
  idToken: string,
  rawNonce: string,
): Promise<void> {
  const user = requireCurrentUser();
  await user.linkWithCredential(auth.AppleAuthProvider.credential(idToken, rawNonce));
}

/** Detach a provider. Refuses to remove the last one (would lock the user out). */
export async function unlinkProvider(providerId: string): Promise<void> {
  const user = requireCurrentUser();
  if (user.providerData.length <= 1) throw new LastSignInMethodError();
  await user.unlink(providerId);
}
```

(`FirebaseAuthTypes` is already imported in this file.)

- [ ] **Step 4: GREEN** — `npx jest link.test` → all pass. Then `npx jest` (full) → green.

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/auth/link.ts apps/mobile-admin/__tests__/link.test.tsx
git commit -m "feat(mobile-shared): signed-in provider link/unlink with last-method guard"
```

---

### Task 2: Expose on `gip.ts` + `provider.tsx` (+ demo stubs)

**Files:**
- Modify: `packages/mobile-shared/auth/gip.ts`
- Modify: `packages/mobile-shared/auth/provider.tsx`

**Interfaces:**
- Consumes: Task 1's five exports.
- Produces: `useAuth()` gains `linkedProviderIds()`, `linkGoogleToCurrentUser(idToken)`, `linkAppleToCurrentUser(idToken, rawNonce)`, `unlinkProvider(providerId)`.

- [ ] **Step 1: `gip.ts`** — import the four new fns from `./link` and add to `createGIPAuth`'s returned object, **each awaiting `tenantReady` first** (same as the existing link methods):

```ts
    linkedProviderIds: async () => {
      await tenantReady;
      return linkedProviderIds();
    },
    linkGoogleToCurrentUser: async (idToken: string) => {
      await tenantReady;
      return linkGoogleToCurrentUser(idToken);
    },
    linkAppleToCurrentUser: async (idToken: string, rawNonce: string) => {
      await tenantReady;
      return linkAppleToCurrentUser(idToken, rawNonce);
    },
    unlinkProvider: async (providerId: string) => {
      await tenantReady;
      return unlinkProvider(providerId);
    },
```

Do NOT reintroduce `firebaseAuth.tenantId = …` (read-only getter in v22+; there's a regression test).

- [ ] **Step 2: `provider.tsx`** — add to **both** `AuthState` and `AuthBackend`:

```ts
  linkedProviderIds: () => Promise<string[]>;
  linkGoogleToCurrentUser: (idToken: string) => Promise<void>;
  linkAppleToCurrentUser: (idToken: string, rawNonce: string) => Promise<void>;
  unlinkProvider: (providerId: string) => Promise<void>;
```

**Demo backend** (never touches firebase):

```ts
    linkedProviderIds: async () => ["password"],
    linkGoogleToCurrentUser: async () => {},
    linkAppleToCurrentUser: async () => {},
    unlinkProvider: async () => {},
```

**Firebase backend** — delegate:

```ts
    linkedProviderIds: () => gip.linkedProviderIds(),
    linkGoogleToCurrentUser: (idToken) => gip.linkGoogleToCurrentUser(idToken),
    linkAppleToCurrentUser: (idToken, rawNonce) => gip.linkAppleToCurrentUser(idToken, rawNonce),
    unlinkProvider: (providerId) => gip.unlinkProvider(providerId),
```

Add matching context wrappers (mirroring the existing ones) and include all four in the `AuthContext.Provider value`.

- [ ] **Step 3: Gates** — `npx tsc --noEmit 2>&1 | grep -E "auth/provider|auth/gip" || echo "TYPE-CLEAN"` → `TYPE-CLEAN`. `npx jest` → green.

- [ ] **Step 4: Commit**

```bash
git add packages/mobile-shared/auth/gip.ts packages/mobile-shared/auth/provider.tsx
git commit -m "feat(mobile-shared): expose connected-accounts surface on the GIP client and AuthProvider"
```

---

### Task 3: `more/security.tsx` screen

**Files:**
- Create: `apps/mobile-admin/app/(tabs)/more/security.tsx`
- Create: `apps/mobile-admin/__tests__/security.test.tsx`

**Interfaces:**
- Consumes: `useAuth().linkedProviderIds/linkGoogleToCurrentUser/linkAppleToCurrentUser/unlinkProvider`; `LastSignInMethodError` from `@repo/mobile-shared/auth/link`; `configureGoogleSignin`, `signInWithGoogleNative`, `signInWithAppleNative` from `@/lib/social-auth`.

- [ ] **Step 1: Write the failing tests** — create `apps/mobile-admin/__tests__/security.test.tsx`:

```tsx
const mockAuth: Record<string, unknown> = {};
jest.mock("@repo/mobile-shared/auth/provider", () => ({ useAuth: () => mockAuth }));
jest.mock("@/lib/social-auth", () => ({
  configureGoogleSignin: jest.fn(),
  signInWithGoogleNative: jest.fn().mockResolvedValue("gtok"),
  signInWithAppleNative: jest
    .fn()
    .mockResolvedValue({ idToken: "atok", rawNonce: "", fullName: null }),
}));
jest.mock("react-native/Libraries/Alert/Alert", () => ({
  alert: jest.fn((_t, _m, buttons) => {
    // auto-press the destructive/confirm button
    const confirm = (buttons ?? []).find((b: { style?: string }) => b.style === "destructive");
    confirm?.onPress?.();
  }),
}));

import { fireEvent, render, waitFor } from "@testing-library/react-native";
import { LastSignInMethodError } from "@repo/mobile-shared/auth/link";
import SecurityScreen from "../app/(tabs)/more/security";

function setAuth(overrides: Record<string, unknown> = {}) {
  Object.keys(mockAuth).forEach((k) => delete mockAuth[k]);
  Object.assign(
    mockAuth,
    {
      linkedProviderIds: jest.fn().mockResolvedValue(["password"]),
      linkGoogleToCurrentUser: jest.fn().mockResolvedValue(undefined),
      linkAppleToCurrentUser: jest.fn().mockResolvedValue(undefined),
      unlinkProvider: jest.fn().mockResolvedValue(undefined),
    },
    overrides,
  );
}

describe("SecurityScreen", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setAuth();
  });

  it("shows a Link action for providers that are not linked", async () => {
    const { getByLabelText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Link Google")).toBeTruthy());
    expect(getByLabelText("Link Apple")).toBeTruthy();
  });

  it("links Google via the native flow then refreshes", async () => {
    const { getByLabelText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Link Google")).toBeTruthy());
    fireEvent.press(getByLabelText("Link Google"));
    await waitFor(() =>
      expect(mockAuth.linkGoogleToCurrentUser).toHaveBeenCalledWith("gtok"),
    );
    await waitFor(() =>
      expect((mockAuth.linkedProviderIds as jest.Mock).mock.calls.length).toBeGreaterThan(1),
    );
  });

  it("links Apple via the native flow (Hide-My-Email path)", async () => {
    const { getByLabelText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Link Apple")).toBeTruthy());
    fireEvent.press(getByLabelText("Link Apple"));
    await waitFor(() =>
      expect(mockAuth.linkAppleToCurrentUser).toHaveBeenCalledWith("atok", ""),
    );
  });

  it("removes a linked provider after confirming", async () => {
    setAuth({ linkedProviderIds: jest.fn().mockResolvedValue(["password", "google.com"]) });
    const { getByLabelText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Remove Google")).toBeTruthy());
    fireEvent.press(getByLabelText("Remove Google"));
    await waitFor(() => expect(mockAuth.unlinkProvider).toHaveBeenCalledWith("google.com"));
  });

  it("shows the guard copy and does not unlink the only method", async () => {
    setAuth({
      linkedProviderIds: jest.fn().mockResolvedValue(["password"]),
      unlinkProvider: jest.fn().mockRejectedValue(new LastSignInMethodError()),
    });
    const { getByLabelText, findByText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Remove Password")).toBeTruthy());
    fireEvent.press(getByLabelText("Remove Password"));
    expect(await findByText(/only sign-in method/i)).toBeTruthy();
  });

  it("maps credential-already-in-use to friendly copy", async () => {
    setAuth({
      linkGoogleToCurrentUser: jest
        .fn()
        .mockRejectedValue(Object.assign(new Error("x"), { code: "auth/credential-already-in-use" })),
    });
    const { getByLabelText, findByText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Link Google")).toBeTruthy());
    fireEvent.press(getByLabelText("Link Google"));
    expect(await findByText(/already linked to a different Mark8ly account/i)).toBeTruthy();
  });
});
```

- [ ] **Step 2: RED** — `npx jest security` → FAIL (screen doesn't exist).

- [ ] **Step 3: Implement** — create `apps/mobile-admin/app/(tabs)/more/security.tsx`:

```tsx
import { useCallback, useEffect, useState } from "react";
import { Alert, StyleSheet, TouchableOpacity, View } from "react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { LastSignInMethodError } from "@repo/mobile-shared/auth/link";
import {
  configureGoogleSignin,
  signInWithAppleNative,
  signInWithGoogleNative,
} from "@/lib/social-auth";
import { BackHeader, Card, Eyebrow, Hairline, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

const DEMO_AUTH = process.env.EXPO_PUBLIC_AUTH_BACKEND === "demo";

type ProviderId = "password" | "google.com" | "apple.com";

const METHODS: { id: ProviderId; label: string; linkable: boolean }[] = [
  { id: "password", label: "Password", linkable: false },
  { id: "google.com", label: "Google", linkable: true },
  { id: "apple.com", label: "Apple", linkable: true },
];

function errorMessage(e: unknown): string {
  if (e instanceof LastSignInMethodError) {
    return "You can't remove your only sign-in method.";
  }
  const code = (e as { code?: unknown }).code;
  if (code === "auth/credential-already-in-use") {
    return "That account is already linked to a different Mark8ly account.";
  }
  if (code === "auth/provider-already-linked") return "That's already linked to your account.";
  if (code === "auth/requires-recent-login") {
    return "For security, sign out and sign in again, then retry.";
  }
  return e instanceof Error && e.message ? e.message : "Something went wrong. Try again.";
}

export default function SecurityScreen() {
  const {
    linkedProviderIds,
    linkGoogleToCurrentUser,
    linkAppleToCurrentUser,
    unlinkProvider,
  } = useAuth();
  const [linked, setLinked] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    const ids = await linkedProviderIds();
    setLinked(ids);
  }, [linkedProviderIds]);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const ids = await linkedProviderIds();
        if (!cancelled) setLinked(ids);
      } catch {
        if (!cancelled) setLinked([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [linkedProviderIds]);

  const run = useCallback(
    async (fn: () => Promise<void>) => {
      if (busy) return;
      setError(null);
      setBusy(true);
      try {
        await fn();
        await refresh();
      } catch (e: unknown) {
        setError(errorMessage(e));
      } finally {
        setBusy(false);
      }
    },
    [busy, refresh],
  );

  const handleLink = useCallback(
    (id: ProviderId) => {
      void run(async () => {
        if (id === "google.com") {
          if (DEMO_AUTH) return linkGoogleToCurrentUser("demo-google-token");
          configureGoogleSignin();
          const idToken = await signInWithGoogleNative();
          return linkGoogleToCurrentUser(idToken);
        }
        if (DEMO_AUTH) return linkAppleToCurrentUser("demo-apple-token", "");
        const { idToken, rawNonce } = await signInWithAppleNative();
        return linkAppleToCurrentUser(idToken, rawNonce);
      });
    },
    [run, linkGoogleToCurrentUser, linkAppleToCurrentUser],
  );

  const handleRemove = useCallback(
    (id: ProviderId, label: string) => {
      Alert.alert("Remove sign-in method", `Remove ${label} from your account?`, [
        { text: "Cancel", style: "cancel" },
        {
          text: "Remove",
          style: "destructive",
          onPress: () => void run(() => unlinkProvider(id)),
        },
      ]);
    },
    [run, unlinkProvider],
  );

  return (
    <Screen>
      <BackHeader eyebrow="SECURITY" title="Sign-in methods" />
      <Eyebrow label="Ways to sign in" />
      <Card padding={0} style={styles.card}>
        {METHODS.map((m, i) => {
          const isLinked = linked?.includes(m.id) ?? false;
          return (
            <View key={m.id}>
              {i > 0 ? <Hairline /> : null}
              <View style={styles.row}>
                <View style={styles.rowInfo}>
                  <Text preset="bodyEmphasis" color="text">
                    {m.label}
                  </Text>
                  <Text preset="caption" color="textTertiary">
                    {linked === null ? "Checking…" : isLinked ? "Connected" : "Not connected"}
                  </Text>
                </View>
                {linked === null ? null : isLinked ? (
                  <TouchableOpacity
                    disabled={busy}
                    onPress={() => handleRemove(m.id, m.label)}
                    accessibilityRole="button"
                    accessibilityLabel={`Remove ${m.label}`}
                  >
                    <Text preset="bodyEmphasis" color="danger">
                      Remove
                    </Text>
                  </TouchableOpacity>
                ) : m.linkable ? (
                  <TouchableOpacity
                    disabled={busy}
                    onPress={() => handleLink(m.id)}
                    accessibilityRole="button"
                    accessibilityLabel={`Link ${m.label}`}
                  >
                    <Text preset="bodyEmphasis" color="accent">
                      Link
                    </Text>
                  </TouchableOpacity>
                ) : null}
              </View>
            </View>
          );
        })}
      </Card>

      {error ? (
        <View style={styles.errorWrap}>
          <Text
            preset="caption"
            color="danger"
            accessibilityRole="alert"
            accessibilityLiveRegion="polite"
          >
            {error}
          </Text>
        </View>
      ) : null}
    </Screen>
  );
}

const styles = StyleSheet.create({
  card: { marginHorizontal: theme.spacing.lg },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.md,
    minHeight: 56,
  },
  rowInfo: { flex: 1, gap: 2 },
  errorWrap: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.md },
});
```

> Token check (already verified — use as written, don't substitute): `TextColor` in `components/ui/Text.tsx:36` includes `text`, `textSecondary`, `textTertiary`, `textMuted`, `inverse`, `accent`, `success`, `danger`, `warning`. So `color="accent"` and `color="danger"` are both valid.

- [ ] **Step 4: GREEN** — `npx jest security` → pass. `npx jest` (full) → green. `npx tsc --noEmit 2>&1 | grep -E "more/security" || echo "TYPE-CLEAN"`.

- [ ] **Step 5: Commit**

```bash
git add "apps/mobile-admin/app/(tabs)/more/security.tsx" apps/mobile-admin/__tests__/security.test.tsx
git commit -m "feat(mobile-admin): add Security screen to link and remove sign-in methods"
```

---

### Task 4: Wire the route into the More hub

**Files:**
- Modify: `apps/mobile-admin/app/(tabs)/more/_layout.tsx`
- Modify: `apps/mobile-admin/app/(tabs)/more/index.tsx`

- [ ] **Step 1: `_layout.tsx`** — add `<Stack.Screen name="security" />` after `account`.

- [ ] **Step 2: `more/index.tsx`** — import `ShieldCheck` from `lucide-react-native` and add a `Row` **after the Account row** (with a `<Hairline inset={theme.spacing.huge + theme.spacing.xs} />` before it, matching the existing pattern):

```tsx
        <Row
          icon={<ShieldCheck size={18} color={theme.colors.text} strokeWidth={1.75} />}
          label="Security"
          accessibilityLabel="Security and sign-in methods"
          onPress={() => router.push("/(tabs)/more/security")}
        />
```

- [ ] **Step 3: Gates** — `npx jest` → green. `npx tsc --noEmit 2>&1 | grep -E "more/index|more/_layout" || echo "TYPE-CLEAN"`.

- [ ] **Step 4: Commit**

```bash
git add "apps/mobile-admin/app/(tabs)/more/_layout.tsx" "apps/mobile-admin/app/(tabs)/more/index.tsx"
git commit -m "feat(mobile-admin): surface Security in the More hub"
```

---

## Final verification

- [ ] `cd apps/mobile-admin && npx jest` → all green.
- [ ] `npx tsc --noEmit` → only the 2 pre-existing `app/(tabs)/_layout.tsx` errors.
- [ ] `EXPO_PUBLIC_AUTH_BACKEND=demo npx expo config --json | grep -c googleServicesFile` → `0` (demo prebuild stays credential-free).
- [ ] **Manual (needs a real build + the Apple provider enabled):** sign in → More → Security → Link Apple with "Hide My Email" → Apple appears as Connected on the SAME account (the whole point). Remove Google → gone. Try removing the last method → blocked.
