# Handoff — mobile-admin media follow-ups (2026-07-16, later session)

Work in `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`. Commit directly to `main`, single-line
conventional messages, **no signatures, no PRs**. Same for `tesserix-k8s` (one level up).

## Read first
- `~/.claude/projects/-Users-Mahesh-Sangawar-personal-tesserix-new-mark8ly/memory/mobile_admin_contract_mismatches.md`
  — the live wire truth, the lockfile landmine, the gate blind spots. **Do not re-derive any of it.**
- Ledger (gitignored scratch, rich): `.superpowers/sdd/progress.md`
- Spec/plan: `docs/superpowers/{specs,plans}/2026-07-16-mobile-admin-lists-bcd*.md`

## State: B/C/D + notifications + CI gates are SHIPPED and CI is GREEN

`6b7b6648..51d2e80b` on main. **The product catalog renders** — verified on a simulator with real prod
data (Anti-Fog Swim Goggles $21.00 etc., thumbnails, Draft badges, All/Active/Draft tabs). The
`PaginatedResponse` `{items}` fiction that hid 161 products for two months is **deleted**.

Gates (from `apps/mobile-admin` unless noted): **jest 206/206 · tsc 0** · `packages/mobile-shared` **tsc 0**
+ **83 vitest** · CI now gates BOTH mobile packages · `npm ci` works again.

## 🔴 YOUR JOB — one unverified fix, then the follow-ups

### 1. Add-photo: fix applied, NOT yet verified on device (start here)

The user reported add-photo left them **stuck in a photo grid with an X and no confirm button**.
Diagnosis: that is **iOS's limited-photo-library management sheet**, not the picker. Cause: the previous
session's brief told the implementer to `requestMediaLibraryPermissionsAsync()` before
`launchImageLibraryAsync`. On modern iOS that is **unnecessary and harmful** — `launchImageLibraryAsync`
uses PHPicker, which runs out-of-process and needs **no** library permission. Asking opts into the legacy
flow; picking "Limited Access" strands you in that sheet and the real picker never opens.
**The pre-existing `components/ProductMediaPicker.tsx` has always called the picker directly — that was
the reference and it was ignored.**

**Fixed in `51d2e80b`** (removed the permission request + a comment explaining why). **Nobody has confirmed
it works.** Do that first:
- Metro is running real-mode on :8081 (`npx expo start --dev-client --port 8081`, **NO demo flag** — a
  stale demo metro silently serves the demo bundle). Sim `AD109A46-2F99-43C3-8AAA-FEE68DC8499E` is booted.
- The sim's photo library has 2 photos added via `xcrun simctl addmedia`, and full photo access was
  granted via `xcrun simctl privacy <UDID> grant photos com.mark8ly.admin`. **Note: that grant may mask
  the bug** — a truly clean test wants `xcrun simctl privacy <UDID> reset photos com.mark8ly.admin` first.
- 🔑 **You CAN navigate without tapping:** `xcrun simctl openurl <UDID> "mark8ly-admin://products"`
  (scheme `mark8ly-admin`, `app.config.js:24`). **You still CANNOT tap** — `idb` absent, AppleScript
  blocked (`-1719`). The human taps; you screenshot (`xcrun simctl io <UDID> screenshot out.png`).
  ⚠️ **Deep-link to the LIST (`://products`), never straight to `://products/<id>`** — the latter pushes
  the detail with no list beneath it, so Back exits to the dashboard and the tab restores the detail.
  The previous session reported that as a navigation bug; **it was an artifact of its own deep-linking**.
  The stack config (`app/(tabs)/products/_layout.tsx`) is correct.

**The 3-step upload flow itself is PROVEN against prod by hand** (curl, 2026-07-16): `POST /media/upload-url`
→ **200** `{url, storage_key, expires_at}` · `PUT` bytes to the signed GCS url → **200** · `POST /media` →
**201** · CDN served the bytes (200, exact byte count) · `DELETE /media/{id}` → **204**. So if add-photo
still fails, it is client-side, not the API.

🔴 **The load-bearing invariant — do not "fix" this:** step 3 sends **`url: storage_key`**, NOT a CDN URL.
The backend builds the public URL itself and ignores what you send
(`services/marketplace-api/internal/product/service_single_media.go:91-97`); the web admin
(`apps/admin/components/products/media/mediaUploadClient.ts`) does the same. A test pins this
(`__tests__/add-product-media.test.tsx`). **Never hardcode `https://cdn.mark8ly.com` in the app.**
`content_hash` is only a path segment (never verified against bytes; `min=16,max=128`) — there is no
crypto in this app and **you must not add a dependency**.

### 2. Also fixed this session, also unverified on device
- **Save acknowledgement** (`Save → Saving… → Saved` ~2s). Was invisible: only the error path was wired.
- **Header right slot wrapping** — `BackHeader` had `right: { width: 44 }`, a hard 44pt, so "Saving…"/
  "Saved" wrapped to `Save`/`d`. Now `minWidth: 44` + `numberOfLines={1}`. **Also fixes "Mark all" on the
  notifications screen**, which had the same latent bug. Worth eyeballing both.
- **Tap-to-view image** — photos were `TouchableOpacity` with `accessibilityRole="button"` and **no
  `onPress`**; now opens a full-screen Modal. Long-press still deletes.

### 3. Open follow-ups (none blocking)
- 🔴 **Real pagination** — `page_size=100` is an interim, so **100 of 161 products reachable**.
  `useCustomers`/`useOrders` send no page_size (server defaults 50/50 — invisible at 1 customer/0 orders).
- **Order detail is deliberately un-migrated** — 6 of its 12 fields don't exist on the wire
  (`timeline`/`payment_method`/`payment_transaction_id` exist nowhere). `get(id)` intentionally passes
  **no schema**. Needs a seeded order to fix honestly.
- **Create-wizard image upload** — `new.tsx` collects images before a product id exists; needs a different
  flow. `ProductMediaPicker.tsx` is orphaned and shaped for exactly that.
- **mobile-admin has NO ESLint setup** — `lint` is now the repo's stub pattern (`@mark8ly/admin` and
  `@mark8ly/onboarding` use the same). `@repo/mobile-storefront` + `@repo/storefront-mobile` still declare
  a broken `eslint .` (hidden — excluded from CI).
- Backend (E): customer `recent_orders`/`average_order_value`, real `low_stock`, orders multi-status.
  Customer money renders **USD** (no currency field on that wire shape). `extra.eas.projectId` is still
  `'your-eas-project-id'` → **`eas build` blocked**.
- Minor, recorded: `ProductRow` mixes semantics (price = primary variant, stock = SUM across variants);
  `deriveSku` degrades on non-Latin titles; `[id].tsx` is now 571 lines (`ImageViewer` would extract cleanly).

## Landmines — do not rediscover these
- 🔴 **`--pretty false` is MANDATORY**: `npx tsc --noEmit 2>&1 | grep -c "error TS"` returns **0 while
  errors exist** (ANSI colour). **Count; never grep by filename.** jest summary lines are coloured too —
  **read the TAIL**, `grep "^Tests:"` matches nothing. (I fell into this again this session.)
- 🔴 **Run BOTH tsc gates.** `packages/mobile-shared` has its own tsconfig extending the ROOT (strict,
  `noUncheckedIndexedAccess`); `apps/mobile-admin` uses the laxer Expo base. The mobile-admin gate is
  **blind** to real mobile-shared errors.
- 🔴 **A file nothing imports is invisible to tsc.** A green gate on a new unimported file proves nothing.
- 🔴 **NEVER** `npm ci`/`npm install`/`rm -rf node_modules` — metro runs against this tree. **Never touch
  anything inside any `node_modules/`.** If the lockfile ever needs work: **a plain `npm install` is NOT
  zod-scoped** — it mass-bumps expo and re-creates the nested zod 3. Edit the lock **surgically**
  (see the memory file).
- Single-file `npx jest <file>` **hangs forever** without `--forceExit`.
- Don't touch `metro.config.js`/`tsconfig.json`/`jest.config.js`/`babel.config.js`/tailwind/
  `app.config.js`/`eas.json`.
- `gh` is authed as the WORK account and cannot see `tesserix/mark8ly` (404). Run
  `gh auth switch --user mahesh-sangawar` first; switch back after. Git push over SSH works regardless.
- **There is NO prod deploy for this app** — no Dockerfile, no CI image, not in `tesserix-k8s`. It ships
  via `eas build` → TestFlight. Nothing in this work touches the backend.

## The lesson that cost this session the most — please inherit it

**Nine planning errors, every single one the same species: a claim asserted without running the command
that would falsify it, each falsifiable in under 60 seconds.** Examples: `productStock===1` (contradicted
its own implementation); `options.values: string[]` (that is the **request** shape — the response sends
`[{id,value,position}]`; it would have blanked all 161 products the day a merchant added one option);
"vitest is not installed" (it is — 83 green tests were excluded from CI on that false premise);
`lint: eslint .` asserted safe without ever running lint (it had never worked — no eslint config);
"media upload is infeasible" (it works — proven end-to-end); and the permission request above.

Every one was caught by a reviewer, by CI, or by the user actually using the app — **never by me**.
**Verify with the EXACT command, not a subset** (CI runs `lint check-types build`, not just check-types).
And **verify reviewer *remedies* too** — the whole-branch review's suggested fix (drop the turbo filter
entirely) would have newly gated `@mark8ly/admin`/`storefront` + the Go services.

## Suggested approach
`superpowers:systematic-debugging` for the add-photo verification (root cause before fixes), then
`superpowers:brainstorming` → spec → `writing-plans` → `subagent-driven-development` for anything larger,
and **do not skip the opus whole-branch review** — it caught the options bomb that all six per-task
reviews missed. **Verify its claims yourself.**
