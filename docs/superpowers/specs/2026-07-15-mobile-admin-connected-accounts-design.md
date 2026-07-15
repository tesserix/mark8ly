# mobile-admin Settings → Security ("Connected accounts") — design

**Date:** 2026-07-15
**App:** `apps/mobile-admin` + `@repo/mobile-shared/auth`
**Status:** approved design → ready for implementation plan
**Follows:** login-time account linking (shipped, `1ef9f134..509da51c`). GIP-direct, no BFF.

## Problem

Login-time linking merges a social provider onto an existing account **by matching email**.
That covers password↔Google, but it **cannot** cover Apple **"Hide My Email"**: Apple hands
back a relay address (`…@privaterelay.appleid.com`), which matches no existing account, so no
conflict fires and the merchant silently gets a **second, separate account**. There is also no
way for a merchant to see which sign-in methods their account has, or to remove one.

## Solution

A **signed-in** linking screen. Because the user is already authenticated we link straight to
`currentUser` via `linkWithCredential` — **no re-auth, no password prompt, and no email
matching**, so Apple's relay address is irrelevant. This is the reliable Hide-My-Email path.

Note this is *better* than the web, which (a) has **no unlink** (`SecurityClient.tsx:99`
returns a "contact support" error) and (b) still replays the login-time `needConfirmation` +
password handshake to link from settings — a REST limitation the native SDK doesn't have.

## Decisions (locked)

| Decision | Choice |
| --- | --- |
| Placement | New route `app/(tabs)/more/security.tsx` + a Security row in the More hub (mirrors web's `settings/security`; keeps Account focused on profile/store/sign-out) |
| Unlink | **Supported**, with a **last-method guard** enforced in the auth layer (not just the UI) |
| Linking mechanism | `currentUser.linkWithCredential(cred)` — signed-in, no re-auth |
| Password method | **Status + Remove only.** Adding a password from here would need a set-password flow — out of scope |
| Style | `StyleSheet` + `theme` + `@/components/ui` primitives (matches `more/account.tsx` and `more/index.tsx`), NOT nativewind — the More section's idiom |
| Demo backend | Canned `['password']`; link/unlink are no-ops; the screen takes a `DEMO_AUTH` branch so it never invokes the real native Google/Apple sheets |

## Design

### Auth layer — `packages/mobile-shared/auth/link.ts` (extended)

```ts
/** Firebase provider ids on the signed-in user: "password" | "google.com" | "apple.com". */
export async function linkedProviderIds(): Promise<string[]>;

/** Thrown by unlinkProvider when removing the user's only remaining sign-in method. */
export class LastSignInMethodError extends Error {}

/** Attach Google to the CURRENT user — email/relay irrelevant. */
export async function linkGoogleToCurrentUser(idToken: string): Promise<void>;

/** Attach Apple to the CURRENT user — this is the Hide-My-Email path. */
export async function linkAppleToCurrentUser(idToken: string, rawNonce: string): Promise<void>;

/** Detach a provider. Throws LastSignInMethodError if it is the last one. */
export async function unlinkProvider(providerId: string): Promise<void>;
```

- All require a signed-in user; throw a plain `Error("Not signed in")` when `currentUser` is null.
- `unlinkProvider` guards on `currentUser.providerData.length <= 1` **before** calling
  `unlink` — a merchant must never be able to lock themselves out. The UI also disables the
  control, but the auth layer is the enforcement point.
- No explicit `reload()`: RN Firebase's `linkWithCredential`/`unlink` already update
  `currentUser` (`User.js:98`, `:215`); the screen re-reads `linkedProviderIds()` after each
  mutation.

### `gip.ts` / `provider.tsx`

`gip.ts` exposes all five behind the existing `tenantReady` await. `provider.tsx` adds them to
both `AuthState` and `AuthBackend`; the **demo** backend returns `['password']` and no-ops the
link/unlink; the **firebase** backend delegates. `FirebaseAuthTypes` imports stay **type-only**
(the Expo Go / demo isolation invariant).

### Screen — `app/(tabs)/more/security.tsx`

- `BackHeader eyebrow="SECURITY" title="Sign-in methods"`.
- Fetches `linkedProviderIds()` on mount; shows a loading state, then one row per method
  (**Password**, **Google**, **Apple**) with its linked state.
- **Google / Apple, not linked** → "Link" → (demo: skip native) native sign-in →
  `link{Google,Apple}ToCurrentUser` → re-fetch.
- **Any method, linked** → "Remove" → confirm `Alert` → `unlinkProvider` → re-fetch. Disabled
  with explanatory copy when it is the only method.
- **Password** → status + Remove only (no Link).
- One in-flight guard shared across the screen's actions; errors render inline.

### More hub + layout

`more/index.tsx` gains a Security `Row` (lucide `ShieldCheck`) → `/(tabs)/more/security`;
`more/_layout.tsx` gains `<Stack.Screen name="security" />`.

## Error handling

| Case | Copy |
| --- | --- |
| `auth/credential-already-in-use` | "That account is already linked to a different Mark8ly account." |
| `auth/provider-already-linked` | "That's already linked to your account." |
| `auth/requires-recent-login` | "For security, sign out and sign in again, then retry." |
| `LastSignInMethodError` | "You can't remove your only sign-in method." |
| Anything else | The error message, else "Something went wrong. Try again." |

`requires-recent-login` is real — Firebase requires a fresh session for link/unlink. v1
surfaces guidance rather than building a re-auth flow (out of scope, noted below).

## Testing

- **`link.test.tsx` (extended):** `linkedProviderIds` maps `providerData`; returns `[]` when
  signed out. `linkGoogleToCurrentUser`/`linkAppleToCurrentUser` build the right credential and
  call `currentUser.linkWithCredential`. `unlinkProvider` **throws `LastSignInMethodError` and
  does NOT call `unlink`** when `providerData.length <= 1`; unlinks when there are 2+. All throw
  when signed out.
- **`security.test.tsx` (new):** renders each method's linked state; Link Google runs
  native→link→re-fetch; Remove confirms then unlinks; the only-method Remove is blocked with the
  guard copy; `credential-already-in-use` renders its mapped copy.

## Out of scope

- Adding a **password** method from this screen (needs a set-password/verify flow).
- A re-auth flow for `auth/requires-recent-login` (guidance copy only).
- MFA / passkeys.
- Web admin changes (its unlink stays unimplemented).
- Enabling the Apple provider in the GIP console / Apple Developer setup (operational, not code).
