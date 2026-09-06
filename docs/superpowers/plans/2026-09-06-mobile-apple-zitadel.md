# Mobile Apple sign-in on Zitadel (#771)

Migrate `handleAppleSignIn` in the mobile admin app off Firebase/GIP and onto
Zitadel's IDP-intent browser flow, the same one mobile Google uses (#756).

Tracking issue: tesserix/mark8ly#771. Part of the #524 epic (milestone
"Auth: GIP → Zitadel").

## Why

Under Zitadel, Apple sign-in currently authenticates against Firebase, sets the
Firebase provider's `user`, and bounces the merchant back to `/login` — `AuthGate`
reads `zitadelSignedIn` and `api-client` reads `zitadelSession`, and a Firebase
sign-in populates neither. #764 hid the button rather than fix it.

Apple's App Review guideline 4.8 requires Sign in with Apple wherever another
social provider is offered, and Google is offered. So the app cannot be submitted
with the button hidden, and cannot pass review with it shown and broken.

## The chain, and what hardcodes Google in it

The flow is four hops, and **three of them pin Google today**:

```
apps/mobile-admin/app/login.tsx
  → packages/mobile-shared/auth/zitadel-signin.ts   signInWithGoogle
  → packages/mobile-shared/auth/zitadel-client.ts   idpStart / idpFinish
  → marketplace-api  /api/v1/mobile/admin/auth/idp/{start,finish}
  → auth-bff         /auth/zitadel/mobile/idp/{start,finish}
  → Zitadel → Apple → admin web bridge → mark8ly-admin://auth/idp
```

1. `zitadel-client.ts`'s `idpFinish(intentId, intentToken)` sends **no `provider`
   at all**. Server-side an absent provider means Google.
2. `marketplace-api`'s `MobileIDPHandler.Finish` **ignores the request body's
   provider entirely** and passes `authbffclient.ProviderGoogle` verbatim. Its
   `Start` rejects anything that is not `ProviderGoogle`.
3. `auth-bff`'s `idpIDForProvider` has one case, `"" | "google"`.

Every one of these must change or an Apple intent gets finished as Google and is
refused by the IDP pin with a misleading error.

## What must NOT change

**Do not re-register, edit, or re-create the Apple IDP in Zitadel, and do not
touch the `idps:` block in `tesserix-k8s/charts/apps/zitadel-bootstrap/values.yaml`.**
The IDP is registered with the Services ID `com.tesserix.signin.web`, grouped under
the primary App ID `com.tesserix.signin`. That grouping is what keys Apple's
private-relay addresses; changing it gives every already-linked user a different
relay address, i.e. they return as new people with no account. The IDP already
exists on the TESSERIX org: id `389173155337339395`.

This is also why the flow is the **browser** flow and not
`expo-apple-authentication`'s native sheet: the web flow authenticates as the
Services ID, the native sheet would authenticate as the Bundle ID.

**Do not weaken the IDP pin or the `email_verified` gate.** `idpFinish` resolves
the expected IDP id from the request's provider *before* calling Zitadel and
checks `identity.IDPID` against it. Accepting "whatever IDP the intent carried"
is an account-takeover primitive: start an intent against the weaker provider,
register `victim@merchant.com` there, and the endpoint links it onto the victim's
account. Adding Apple to the switch is a deliberate act of trusting Apple; it is
not a licence to stop pinning.

**Do not touch `customer_handler.go`.** The storefront/customer path has its own
hardcoded `googleIDPID` and offers no Apple button. Leaving it Google-only is
correct.

## Global Constraints

- Commits: conventional, **single-line**, no body. **No `Co-Authored-By`, no
  `Claude-Session`, no attribution trailers of any kind.**
- Work only on branch `feat/771-mobile-apple-zitadel` in the worktree
  `/Users/Mahesh.Sangawar/personal/tesserix-new/m8-wt-686`. Verify
  `git rev-parse --show-toplevel` ends in `m8-wt-686` before the first commit.
- **Do not run `npm install`** in the worktree — it fails on
  `@tesserix/otto-widget` (needs `NODE_AUTH_TOKEN`). `node_modules` is already
  present and working.
- TypeScript changes MUST be verified with
  `cd packages/mobile-shared && npx tsc --noEmit`. That package sets
  `noUncheckedIndexedAccess` via the root tsconfig; `apps/mobile-admin` does not.
  A file can pass the app's typecheck and all jest tests and still fail this
  gate — it broke `main` once already.
- Go changes: `go build ./... && go vet ./... && go test ./...` in the changed
  service. Go works fine in a worktree via `go.work`.
- Two pre-existing `@tesserix/otto-widget` unresolved-module errors in
  `apps/admin` and `apps/storefront` are **not yours** — ignore them.
- Never invent the Apple IDP id. It is `389173155337339395`, and it is read from
  config, never hardcoded in Go.

## Task 1 — tesserix-k8s: ship the Apple IDP id before the code that reads it

Repo: `/Users/Mahesh.Sangawar/personal/tesserix-new/tesserix-k8s` (separate repo,
separate PR, **merged and deployed first**).

Adding required config is **k8s-first**: Task 2 makes `ZITADEL_APPLE_IDP_ID`
boot-required, so auth-bff will refuse to start without it. The env var must be
live before that code deploys.

Changes in `charts/apps/mark8ly-auth-bff/`:

- `values.yaml`: add `appleIdpId: "389173155337339395"` next to the existing
  `googleIdpId: "386381087862948767"` under `zitadel:`.
- `templates/deployment.yaml`: add, mirroring the `ZITADEL_GOOGLE_IDP_ID` block
  immediately above it:
  ```yaml
  - name: ZITADEL_APPLE_IDP_ID
    value: {{ .Values.zitadel.appleIdpId | quote }}
  ```
- `Chart.yaml`: **bump `version`.** `ct lint` fails on an unbumped chart version
  and local `helm lint` will not catch it.

Verify with `helm template` (or `helm lint`) that the rendered Deployment carries
the new env var with the right value.

That repo requires a review approval on the PR. Branch from `origin/main` — it has
concurrent sessions; never rewrite another branch.

## Task 2 — auth-bff: trust Apple as a second IDP, and read Apple's `email_verified`

Repo: mark8ly, worktree `m8-wt-686`. Service: `services/auth-bff`.

### 2a. Provider switch

`internal/zitadellogin/handler.go`:

- Add `const providerApple = "apple"` beside `providerGoogle`.
- Add an `appleIDPID string` field beside `googleIDPID`, and a `WithAppleIDPID`
  builder mirroring `WithGoogleIDPID` exactly.
- Add the `case providerApple:` arm to `idpIDForProvider`, returning
  `errIDPProviderNotConfigured` when `appleIDPID == ""`, exactly as the Google
  arm does. **Do not** change the `default:` arm — an unknown provider must still
  be `errUnsupportedIDPProvider`, and the empty string must still mean Google
  (the web callers predate the field and rely on it).
- Update `idpIDForProvider`'s doc comment: it currently says an Apple IDP exists
  but is not accepted. It is accepted now, and the comment should say what
  trusting it means rather than being deleted.

### 2b. Config

`pkg/config/config.go`:

- `ZitadelAppleIDPID string \`envconfig:"ZITADEL_APPLE_IDP_ID"\`` with a doc
  comment in the style of `ZitadelGoogleIDPID`'s.
- Add to `Validate`'s missing-fields check, same as `ZITADEL_GOOGLE_IDP_ID`.
  Boot-required is deliberate: it makes "deployed the code without the config"
  impossible, and Task 1 lands the value first.

`cmd/server/main.go`: chain `.WithAppleIDPID(cfg.ZitadelAppleIDPID)` onto the
**merchant `Handler`** construction only (the one that already gets
`WithGoogleIDPID` at ~line 122 or ~143 — whichever builds the `Handler`, not the
`CustomerHandler`). Read both call sites and wire only the merchant one. Update
`main_test.go`'s config fixture accordingly.

### 2c. Apple's `email_verified` claim shape

`internal/zitadellogin/idpintent.go`, `readRawEmail`: today it is
`raw["email_verified"].(bool)` and fails soft to `false` on anything else. Apple
documents that claim as **String or Boolean** and has historically sent the
string `"true"`. If Zitadel passes it through unnormalised, every Apple link
attempt refuses with `email_not_verified` — and it will read as a policy decision,
not a type bug.

Accept a boolean, or the exact strings `"true"` / `"false"`. Anything else — a
number, an absent claim, `"TRUE"`, `"yes"` — still means **unverified**. This is a
normalisation of a documented claim type, not a loosening of the gate: keep the
fail-soft-to-false default and say so in the comment.

Leave `readRawName` alone.

### 2d. Tests

- `idpIDForProvider`: `"apple"` resolves to the Apple id; `"Apple"` and
  `" apple "` too (the switch already lowercases/trims); `""` and `"google"` still
  resolve to the Google id; an unknown provider is still
  `errUnsupportedIDPProvider`; an unconfigured Apple id is
  `errIDPProviderNotConfigured`.
- **The cross-provider pin, both directions:** an intent whose `identity.IDPID` is
  the Apple id, finished with `provider: "google"`, must be refused; and the Google
  id finished with `provider: "apple"` must be refused. This is the security
  property of the whole task — it needs an explicit test, not an implied one.
- `readRawEmail`: `true` → verified; `"true"` → verified; `false`, `"false"`,
  absent, `"TRUE"`, `1`, `nil` → unverified.
- `Validate` reports `ZITADEL_APPLE_IDP_ID` when it is missing.

## Task 3 — marketplace-api: stop pinning Google at the public edge

Repo: mark8ly, worktree `m8-wt-686`. File:
`services/marketplace-api/internal/handlers/admin/mobile_auth_idp.go` (plus
`internal/authbffclient/` for the provider constant).

### 3a. Accept Apple on start

`Start` currently refuses anything that is not `authbffclient.ProviderGoogle`.
Add `ProviderApple` beside `ProviderGoogle` in `authbffclient` and accept both.
Keep the allowlist shape — an unknown provider is still a 400
`unsupported_provider`. The existing comment ("adding a provider must be a
deliberate change in both, never something a request opts into") is the rule being
followed here, not broken; update it to name both providers.

### 3b. Forward the provider on finish — the actual bug

`mobileIDPFinishRequest` has no `Provider` field, and `Finish` passes
`authbffclient.ProviderGoogle` as a literal. Add the field, validate it against
the same two-value allowlist `Start` uses, and pass the request's provider to
`h.backend.IDPFinish`.

**An absent/empty provider on finish must keep meaning Google** — do not make the
field required. Older app builds in the wild send no provider and must keep
working exactly as they do today.

### 3c. Provider-aware error copy

`idpErrorCopy` hardcodes "Google" in merchant-facing strings:

- `email_not_verified` → "Google hasn't verified that email address, so we can't
  sign you in with it."
- `unexpected_idp` → "Couldn't sign you in with Google. Try again."

Shown to an Apple user these are simply wrong. Make the copy carry the provider
the request actually named. Keep the existing statuses and stable `error` codes
byte-for-byte — only the human-facing `message` varies. Do not collapse the
distinct codes; the comment above the map explains why they are deliberately not
collapsed, and that reasoning still holds.

### 3d. Tests

- `Start` accepts `"apple"` and `"google"`, still 400s on `"facebook"` and on `""`.
- `Finish` forwards `"apple"` to the backend when the request names it; forwards
  `"google"` when the request names it; forwards `"google"` when the request omits
  it (the back-compat case — assert this explicitly).
- `Finish` 400s on an unknown provider without calling the backend at all.
- Error copy for `email_not_verified` names Apple on an Apple request and Google on
  a Google request, with the status and `error` code unchanged in both.

## Task 4 — the app: route Apple through Zitadel and show the button

Repo: mark8ly, worktree `m8-wt-686`.

### 4a. `packages/mobile-shared/auth/zitadel-client.ts`

- `export type IdpProvider = "google" | "apple";`
- `idpFinish` takes the provider and sends it: `idpFinish(provider, intentId,
  intentToken)` posting `{ provider, intent_id, intent_token }`. Update
  `idpStart`'s call site type if needed. **Every existing caller must be updated**
  — a missed one silently finishes an Apple intent as Google.

### 4b. `packages/mobile-shared/auth/zitadel-signin.ts`

Add `signInWithApple`, mirroring `signInWithGoogle` exactly: same
`IdpBrowserOptions` shape, same `parseIdpCallback` handling, same cancellation
handling, same step-up routing, same `redirectUrl`. Differences are only the
`provider` constant (`"apple"`) and the error copy/codes.

Where `signInWithGoogle` throws `"google_sign_in_failed"` with "Couldn't sign you
in with Google. Try again.", the Apple path throws an Apple-named equivalent. Add
the new code to whatever union/type enumerates them, and to the screen's error
mapper (Task 4c) — an unmapped code falls through to the generic message, which
is a silent quality regression, not a crash.

**Factor the shared body rather than copying it** if the two functions come out
near-identical: verbatim duplication of a logic block is a review defect. A small
private helper taking the provider and its copy is the right shape. Use judgment —
do not contort the code to avoid three duplicated lines.

### 4c. `apps/mobile-admin/app/login.tsx`

- `handleAppleSignIn`: add the `isZitadelProvider()` branch **first**, mirroring
  `handleGoogleSignIn`'s — `createZitadelSignIn(env.apiBaseUrl).signInWithApple(...)`
  with `redirectUrl: IDP_REDIRECT_URL` and the same `openAuthSession` wiring,
  `routeStepUp(out)`, then `router.replace('/(tabs)')`. Leave the existing
  GIP/demo path untouched below it for non-Zitadel builds.
- The catch block needs the same `isZitadelProvider() && e instanceof
  ZitadelAuthError` arm `handleGoogleSignIn` has.
- Extend `zitadelErrorMessage` to map the Apple failure code.
- **Remove the `!isZitadelProvider()` gate on the Apple button** and the #764
  comment block above it explaining why it was hidden. Keep the
  `Platform.OS === 'ios'` condition — Sign in with Apple is iOS-only here.

### 4d. Tests

Mirror `apps/mobile-admin/__tests__/zitadel-google-signin.test.ts` for Apple:
start → browser handoff → finish → complete, the cancellation path, and the
step-up routing. Add a test asserting `idpFinish` sends `provider: "apple"` — that
is the defect this task exists to prevent.

Then, and this is required before the task is done:

```
cd packages/mobile-shared && npx tsc --noEmit
```

## Verification that is NOT possible here, and must be said plainly

- **`next build` cannot run in this worktree** (no full install). Task 4 does not
  touch a Next.js server component, so this is not expected to matter — but do not
  claim a production build passed.
- **The Apple IDP has never been exercised end to end.** Zitadel stores the `.p8`
  private key without validating it, and Apple only rejects a derived client
  secret when a real sign-in is attempted. The first real Apple sign-in is the
  first test of that key.
- **Device verification needs an Expo/EAS build, which the owner has asked to be
  consulted about before triggering.** Do not start one. The task ends at "merged
  and deployed"; the on-device leg is a separate, owner-gated step.
- **Apple private relay** ("Hide My Email") yields
  `…@privaterelay.appleid.com`, which will not match a merchant's account email,
  so `idp/finish`'s link-only path will refuse. That is correct behaviour and out
  of scope to change here — but it means the first device test should use a real,
  shared Apple address, and the product decision about relay is #771's open
  question, not this branch's.
