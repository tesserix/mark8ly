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
| A5 | ~~Play submit key is a local path~~ **done 2026-08-14** | — | `eas-play-submit@tesseracthub-480811` key registered in EAS credentials; `serviceAccountKeyPath` removed so submits use it |
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

## Phase C — Play Store readiness — AUDITED 2026-07-30, effectively DONE

Audited via the Play Developer API (service-account read) plus live URL checks.
Evidence in `.superpowers/sdd/progress.md`.

| # | Item | Status |
|---|---|---|
| C1 | Play Console app, internal track | ✅ production **and** internal both carry versionCode 7, status `completed` |
| C2 | Data safety, content rating, target-audience | ✅ inferred soundly — Play **blocks** production publishing until these are complete, and v7 is live on production |
| C3 | Store listing — text, screenshots, feature graphic | ⚠️ **the one real action.** Text + required assets all present (title 13/30, short 68/80, full 268/4000, 4 phone shots, feature graphic, icon). But the 4 live screenshots are **stale** — they show the pre-increment-3 app |
| C4 | Account deletion | ✅ `mark8ly.com/delete-account` is 200, indexable, and covers all four required elements (what's deleted, retention, how to request, in-app path) |
| C5 | Privacy policy URL | ✅ `/privacy` + `/terms` 200; correctly names Tesserix Pty Ltd, NSW Australia |

**C3 detail — why the screenshots must be refreshed.** All four downloaded and
inspected. They show the **old light dock** (now the Ink dock, a dark floating
pill), an **old dashboard** ("$0", "Finish setup 75%", no chart — now
"THIS MONTH $460" with the moss chart and the NEEDS YOU queue), the row label
**"Support tickets"** (renamed to "Tickets" by Task 11 Sweep B), and the
**pre-cleanup catalogue** (boxing gloves, swim goggles — the store is now 12
curated linen products). The listing sells an app that no longer exists.

Refreshing requires a **release** build, so it sequences *with* the build rather
than before it. The dev-client shots taken during verification are unusable: the
Tools FAB is burned in, and the frames are 6.3"/1206x2622 — App Store Connect
wants 6.9" (1320x2868, iPhone 17 Pro Max).

Tablet screenshots are absent, which is **consistent** with tablet layouts being
explicitly out of scope. Do not add screenshots of layouts that were never
designed or tested.

⚠️ Asymmetry worth remembering: "a prior release proves C2" is valid because Play
*gates* on it; "a prior release proves the service account has permission" was
**not** valid, because Play doesn't care which credential published — a human
upload bypasses the account entirely. Same argument shape, only one holds.

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
