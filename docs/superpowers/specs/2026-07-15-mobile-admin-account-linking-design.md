# mobile-admin account-linking (login-time) — design

**Date:** 2026-07-15
**App:** `apps/mobile-admin` + `@repo/mobile-shared/auth`
**Status:** approved design → ready for implementation plan
**Depends on:** Phase 1a social login (shipped), Phase 1b real-auth boot (verified). GIP-direct (no BFF).

## Problem

The **web** admin keeps password + Google in sync: the `MP-Internal-e986p` tenant is
*one-account-per-email*, and when a password user signs in with Google the web runs an
explicit link handshake (`apps/admin/lib/gip/{signup,link}.ts`) so both providers land on
**one** account. **Mobile has no such handling** — `social-credentials.ts` calls
`auth().signInWithCredential()` and, on a same-email conflict, throws
`auth/account-exists-with-different-credential`, which the login screen shows as a raw error.

Result (mobile, today) — all divergent from web:

| Scenario | Web | Mobile now |
| --- | --- | --- |
| password user → Google | merges | error |
| password user → Apple | (n/a — web has no Apple) | error |
| Google user → Apple | (n/a) | error |

A merchant who registered with a password on the web then taps "Continue with Google"/"Sign
in with Apple" on mobile cannot get into their account.

## Scope

**In:** login-time email-conflict linking on mobile, native-SDK based, mirroring the web's
one-account-per-email merge. **Out (documented follow-up):** a Settings "Connected accounts"
screen (signed-in linking) — that is the reliable path for Apple **Hide My Email** and is
deferred to its own phase.

## Decisions (locked)

| Decision | Choice |
| --- | --- |
| Behaviour | Parity with web — merge same-email cross-provider into one account |
| Mechanism | Native RN Firebase SDK (`signInWithCredential` → `linkWithCredential`); no BFF, no REST session juggling |
| Tenant setting | Unchanged — stays *one-account-per-email* (matches web's security posture) |
| Apple Hide-My-Email | Deferred to the Settings connected-accounts phase (a relay email can't match at login); documented, not silently broken |
| Existing-method detection | `fetchSignInMethodsForEmail`; **defensive fallback** for the empty result (email-enumeration protection) — let the user pick / default to password |
| Demo backend | No-op link surface so the demo build never hits this path |

## Design

### Auth layer — `@repo/mobile-shared/auth`

**`social-credentials.ts`** — stop throwing on conflict; return a typed outcome:

```ts
export type SocialSignInOutcome =
  | { status: "signed-in" }
  | {
      status: "needs-link";
      email: string;
      provider: "google.com" | "apple.com";
      pendingCredential: FirebaseAuthTypes.AuthCredential;
    };
```

`signInWithGoogleCredential` / `signInWithAppleCredential` build the credential as today,
call `signInWithCredential`, and:
- success → `{ status: "signed-in" }` (existing `onAuthStateChanged` routes the user);
- catch `auth/account-exists-with-different-credential` → `{ status: "needs-link", email:
  error.email, provider, pendingCredential: <the credential we just built> }`. We retain the
  credential we constructed (RN Firebase does not reliably expose `error.credential`, but we
  built it, so we keep it). Any other error still throws.

**New `link.ts`** — complete the link after the user re-authenticates with their existing
method, then attach the pending provider:

```ts
export async function completeLinkWithPassword(
  email: string, password: string, pending: FirebaseAuthTypes.AuthCredential,
): Promise<void>;                    // signInWithEmailAndPassword → currentUser.linkWithCredential(pending)

export async function completeLinkWithGoogle(
  googleIdToken: string, pending: FirebaseAuthTypes.AuthCredential,
): Promise<void>;                    // signInWithGoogleCredential(existing) → linkWithCredential(pending)

export async function completeLinkWithApple(
  appleIdToken: string, rawNonce: string, pending: FirebaseAuthTypes.AuthCredential,
): Promise<void>;                    // existing Apple → linkWithCredential(pending)

/** ['password' | 'google.com' | 'apple.com'] or [] when enumeration-protected. */
export async function existingSignInMethods(email: string): Promise<string[]>;
```

**`gip.ts`** — `signInWithGoogle`/`signInWithApple` return `SocialSignInOutcome` (await the
tenant first, as today) and expose the `link.ts` helpers + `existingSignInMethods`.

**`provider.tsx`** — `AuthState.signInWithGoogle`/`signInWithApple` return
`Promise<SocialSignInOutcome>` (both interfaces + wrappers). Add `completeLink*` +
`existingSignInMethods` to the context. **Firebase backend** delegates to `gip.*`. **Demo
backend** always returns `{ status: "signed-in" }` and its `completeLink*` are no-ops
(demo never conflicts).

### UI — `apps/mobile-admin/app/login.tsx` + a modal

- `handleGoogleSignIn`/`handleAppleSignIn` inspect the outcome: `signed-in` → nothing (routed);
  `needs-link` → open a **`LinkAccountPrompt`** modal (new component, following the existing
  `components/StoreSelector.tsx` `Modal` pattern).
- **`LinkAccountPrompt`** — headline *"An account already exists for {email}. Sign in to
  connect {Google|Apple}."* Determine the re-auth method:
  - call `existingSignInMethods(email)`;
  - contains `password` → password field → `completeLinkWithPassword`;
  - contains an OAuth provider (and not password) → a "Continue with {that}" button →
    `completeLinkWith{Google,Apple}`;
  - **empty (enumeration protection)** → default to the password field **plus** a secondary
    "I used Google / Apple instead" affordance, and if a re-auth choice conflicts again, show
    "That wasn't the method you signed up with — try another." Never dead-ends.
- On success the modal closes; `onAuthStateChanged` (fired by the re-auth sign-in) routes in.
- Shares the login screen's `submitting`/`getErrorMessage` pattern; the modal has its own
  in-flight + error state.

## Error handling

- Non-conflict sign-in errors keep the current behaviour (throw → `setError`).
- Re-auth failures inside the modal (wrong password, cancelled Google/Apple) show a modal-local
  error; the pending credential is retained so the user can retry, and is dropped on cancel.
- Pending OAuth credentials are short-lived — if `linkWithCredential` fails "credential expired,"
  the modal tells the user to start over (re-tap the social button).

## Testing

Unit (jest, mock `@react-native-firebase/auth` per the existing `gip.test.tsx` pattern):
- `social-credentials`: success → `signed-in`; `account-exists-with-different-credential` →
  `needs-link` with email/provider/pendingCredential; other errors rethrow.
- `link.ts`: `completeLinkWithPassword` calls `signInWithEmailAndPassword` then
  `currentUser.linkWithCredential(pending)`, in that order; Google/Apple variants likewise;
  `existingSignInMethods` returns the native result and `[]` on the enumeration-protected case.
- `login.test.tsx`: a `needs-link` outcome opens `LinkAccountPrompt`; a `signed-in` outcome
  does not.
- `LinkAccountPrompt`: renders the right re-auth control per detected method; empty-methods
  fallback shows password + secondary option; submit calls the right `completeLink*`.

## Out of scope

- Settings "Connected accounts" screen (signed-in linking; the reliable Hide-My-Email path) —
  next phase.
- Android Google sign-in (needs an Android OAuth client + signing SHA-1).
- Changing the tenant's account-linking setting.
- Web admin changes.
