# Android release parity — plan

**Goal:** release mobile-admin on Android and iOS together.

**Status of the code:** increment 3 is complete, reviewed and pushed (18 commits,
121/121 suites, 1659 tests). Android has been built, installed and sanity-passed
on a Pixel 8 Pro emulator — login, GIP sign-in, Dashboard, Order detail (including
the "Confirm order" sticky bar), More/GroupedList, Account and Products all render
correctly. Nothing below is about broken layout; it is about the release path and
the behaviours a sanity pass cannot reach.

---

## 🔴 Blocker discovered — Google Sign-In is broken on Android today

`google-services.json` contains `com.mark8ly.admin`, but its only `oauth_client`
entry is **type 3 (web)**. There is **no type-1 Android OAuth client with a
certificate SHA-1**.

`@react-native-google-signin` on Android requires an Android OAuth client
registered against the signing certificate's SHA-1. Without it `signIn()` fails
with `DEVELOPER_ERROR` (code 10). The button renders — I saw it on the emulator —
but it cannot succeed.

This was invisible to every gate and to the sanity pass, because the pass used
email/password. **Do not ship Android without fixing this**: the login screen
offers Google, and a merchant who taps it gets an opaque failure.

Two SHA-1s are needed, not one: the **EAS-managed upload/release keystore** and,
if you want Google Sign-In to work in `development`/`preview` builds, the **debug
keystore** too.

---

## Phase A — Release configuration (blocking)

Each item is currently absent, verified in-repo.

| # | Gap | Evidence | Work |
|---|---|---|---|
| A1 | No Android OAuth client / SHA-1 | `google-services.json` has only `client_type: 3` for `com.mark8ly.admin` | Create Android OAuth client in Firebase project `849928263410`; register EAS release SHA-1 (+ debug SHA-1); re-download `google-services.json` |
| A2 | `eas.json` `production` has no `android` block | `eas.json` production = `{channel, autoIncrement, ios, env}` | Add `android: { buildType: "app-bundle" }` |
| A3 | No Android Google client id in production `env` | production `env` carries only `EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID` | Add `EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID` once A1 exists |
| A4 | `google-services.json` is gitignored, CI can't see it | `.gitignore:19`; `app.config.js:97` reads `process.env.GOOGLE_SERVICES_JSON` | Upload as an EAS **file** secret; same for iOS `GoogleService-Info.plist` — **verify how the iOS build currently gets it, it may already be broken** |
| A5 | Play submit key is a local path | `submit.production.android.serviceAccountKeyPath: ./play-service-account.json` | Create Play service account, upload JSON as EAS secret, point config at it |
| A6 | No Android keystore in EAS | — | `eas credentials` → Android → generate/upload keystore (one-time, mirrors the documented iOS cert flow) |
| A7 | Workflow tag path is iOS-only | `mobile-admin-build.yml`: "Tag builds are always iOS / production / --auto-submit" | Make the tag path build **both** platforms, or matrix it |

**A1 is the long pole** — it needs Firebase console access and the EAS keystore
to exist first (A6), because the SHA-1 comes from the keystore.

**Ordering:** A6 → A1 → A3 → A2/A4/A5 → A7.

---

## Phase B — Android behaviours never exercised

The sanity pass covered rendering. These are the divergences that a first-run
render check structurally cannot reach, and each is a plausible release defect.

| # | Item | Why it is a risk |
|---|---|---|
| B1 | **TalkBack + `ActionFailureNotice`** | It *deliberately* skips `announceForAccessibility` on Android and relies on the assertive live region. Never once run against TalkBack. If the live region doesn't fire, mutation failures are silent to screen-reader users — the exact defect Task 18 existed to remove |
| B2 | **`SwipeRow` tap-vs-drag arbitration** | Android's gesture responder differs from iOS. Convention is drag-right = constructive, drag-left = destructive, and **this app has no undo** |
| B3 | **Dynamic Type at accessibility sizes** | Android reaches larger multipliers than iOS. The whole increment rests on `minHeight` boxes growing correctly |
| B4 | **Hardware / gesture Back** | Android-only concept. Sheets, modals and the product-create flow all need sane back behaviour; never tested |
| B5 | **Dock vs Android navigation bar** | Dock is absolutely positioned on safe-area insets. Gesture-nav and 3-button nav give different insets — test both |
| B6 | **Edge-to-edge / display cutout** | Android 15+ enforces edge-to-edge; content can slide under system bars |

Each fix follows the increment's existing discipline: guard with a test that
fails when the fix is removed.

---

## Phase C — Play Store readiness

| # | Item | Notes |
|---|---|---|
| C1 | Play Console app for `com.mark8ly.admin`, internal track | `submit` config already targets `track: "internal"` |
| C2 | Data safety form, content rating, target-audience | Play-specific; no iOS equivalent |
| C3 | Store listing — description, screenshots (phone + tablet), feature graphic | Screenshots must come from the **release** build |
| C4 | Account deletion | Play requires an in-app path **and** a public web URL. In-app exists (verified on device: Account → Delete account). **The public URL still needs confirming** |
| C5 | Privacy policy URL reachable | Play validates it |

---

## Phase D — Joint release

1. Full device pass, **both platforms**, at default **and** `accessibility-large`,
   on one cleared bundle each. Includes the checks Phase B adds.
2. **Fire the Confirm mutation.** The sticky bar is verified as rendering; the
   mutation itself is still unexercised. It consumes the seeded pending order
   `#M-THE-260729-00001`, so seed a replacement first (the ownership fix in
   `docs/ops/2026-07-29-bondi-sequence-ownership.md` makes this repeatable now).
3. Align `version` across platforms; `autoIncrement` handles build numbers.
4. Tag `mobile-admin-v*` → both builds → TestFlight + Play internal.
5. Verify from the store artifacts, not the dev build.

---

## Sequencing recommendation

Phase A and Phase B are **independent** and can run concurrently — A is
config/console work, B is app code. C can start any time. D needs all three.

Critical path is **A6 → A1** (keystore, then OAuth client), because everything
about Google Sign-In on Android hangs off the release SHA-1.

## Deliberately out of scope

- Android tablet/foldable layouts — not attempted on iOS either.
- Wear/Auto surfaces.
- The 45 known `useCallback` violations outside `SWEEP_A_FILES` — ratcheted, not
  a release blocker.
- Storefront checkout (needs carrier + gateway keys; deferred by decision), and
  therefore anything requiring an order **with a shipment**: `EmailLabelSheet`,
  `RefundSheet`'s carrier warning, `CancelReasonSheet`'s shipment sentence.
