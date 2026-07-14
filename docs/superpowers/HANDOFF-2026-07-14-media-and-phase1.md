# Session handoff — web-admin image fix, then mobile Phase 1a/1b

Paste everything below the line into a new session as the opening prompt.

---

You are taking over work in `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly` (Tesserix "Mark8ly" merchant marketplace — Go microservices + Next.js web apps + an Expo mobile-admin app). **Commit directly to `main`, single-line conventional messages, no signatures, no PRs** (project convention).

**First, read these memory files for full context** (`/Users/Mahesh.Sangawar/.claude/projects/-Users-Mahesh-Sangawar-personal-tesserix-new-mark8ly/memory/`):
- `project_mobile_admin_modernise.md` — the mobile-admin modernise program: phases, locked decisions, current state (Phase 0 done + demo + polish).
- `mobile_admin_nativewind_metro_landmines.md` — **CRITICAL before touching the Expo app**: 9 build/runtime traps (metro `unstable_enablePackageExports` MUST stay `false`, `disableHierarchicalLookup` on, postinstall tailwindcss@3 symlink, no `.test` files under `app/`, GoogleService-Info.plist gating, static frameworks, expo-camera dropped, etc.).
- `MEMORY.md` — index of everything else.

Do the three tasks below in order. Use the superpowers workflow (brainstorming → writing-plans → subagent-driven-development) for Task 1; Tasks 2/3 already have a plan.

## TASK 1 (do first) — Web-admin product image resolution loss

**Problem:** In the web admin (`apps/admin`), Product → add image **destroys the merchant's high-resolution original**. Store owners upload professional photoshoot images and the tool forces a small, lossy square.

**Confirmed root cause (this session):**
- `apps/admin/components/products/form/MediaTab.tsx:254-255` — the fresh-add flow uploads **only the cropped blob** (`new File([blob], …)`), the original file is **never uploaded**; `gcs_path_original` is set to `""` (line 101).
- `apps/admin/components/products/media/MediaCropDialog.tsx` — invoked from MediaTab **without an `aspect` prop**, so it defaults to `aspect = 1` (forced 1:1 **square**). Zoom slider `min=1 max=3`, **no minimum-output-size floor**.
- `apps/admin/components/products/media/cropImage.ts` — `cropToBlob` outputs JPEG **0.92** at the crop region's source pixels (fine for the kept region, but the region is a forced small square, re-encoded lossily).
- Backend `services/marketplace-api`: media upload is a **signed-URL flow** — `POST /media/upload-url {content_hash, filename, content_type oneof image/png|jpeg|webp}` → PUT to GCS → `POST /products/:id/media {storage_key, url, …}`. Allowed formats **PNG/JPEG/WebP only**. **No** server-side size/dimension/aspect enforcement. The recrop service (`internal/product/service_media_recrop.go`) is explicitly designed to keep `gcs_path_original` pristine for future recrops — **the add flow just never populates it.**

**Fix goal — preserve the pristine original + make cropping non-destructive/optional:**
1. **Upload the full original** to `gcs_path_original` (untouched, full resolution) on add — this is the key fix.
2. **Make crop optional and non-square**: pass an `aspect` from MediaTab; offer "use original / no crop" plus aspect choices (default **4:5**, options 1:1 / 3:4 / original). Don't force a square.
3. Store the crop as **parameters** (crop_box + rotation) against the original where feasible, and/or upload a **high-quality derived rendition**, but **never discard the original**.
4. Serve storefront **display variants** from the original (quality becomes a display concern, not a destructive upload-time one).
5. Raise JPEG quality (~0.95) or keep WebP; add a **min-resolution guard** (warn/block under ~1000px short edge).

**Before coding, confirm with the user:** whether to also change the backend media contract (populate `gcs_path_original` in `POST /products/:id/media`, add a variant/rendition path) or keep the first cut frontend-only. Then brainstorm → write a short spec + plan under `docs/superpowers/` → execute subagent-driven. There are Playwright e2e tests at `apps/admin/tests/e2e/products-media-flow.spec.ts` and `seed-product-images.spec.ts` — keep them green.

## TASK 2 — mobile-admin Phase 1a (real auth + social login, credential-free)

**Plan already written:** `docs/superpowers/plans/2026-07-14-mobile-admin-phase-1-real-auth.md`. Execute **Phase 1a** (5 tasks) with `superpowers:subagent-driven-development`. It adds `signInWithGoogle`/`signInWithApple` to `@repo/mobile-shared/auth` (mirror Home-Chef `../Home-Chef-App/packages/mobile-shared/src/auth/sign-in.ts`), Google + Apple buttons on `apps/mobile-admin/app/login.tsx`, native wrappers (`@react-native-google-signin/google-signin` + `expo-apple-authentication`), and env-driven native config. All of 1a is buildable/testable **without real credentials** (native modules mocked in jest; demo backend covers the sim). Tests: `cd apps/mobile-admin && npx jest`.

mark8ly auth is **GIP-direct (no BFF)** — after `signInWithCredential`, `onAuthStateChanged` fires and the existing `AuthGate` routes. Do NOT add Home-Chef's `completeBFFLogin`/`setAuthResponse`.

## TASK 3 — Phase 1b (needs credentials, likely blocked)

Blocked until the user provides: **`GoogleService-Info.plist`** (mark8ly GIP iOS app, bundle `com.mark8ly.admin`) + **Google OAuth client IDs** (web + iOS) + reversed-client-id `iosUrlScheme`, and enables **Google + Apple** providers in the GIP/Identity Platform console. Then: real native build (NO `EXPO_PUBLIC_AUTH_BACKEND=demo`) → verify email/Google/Apple → `TenantGate` → `StorePicker` → **live dashboard**. Apple-nonce fallback is documented in the plan (Task 1b.2).

## Running / verifying the mobile app

- Run on sim: from `apps/mobile-admin`, `EXPO_PUBLIC_AUTH_BACKEND=demo npx expo run:ios --device "iPhone 17 Pro"`. Demo login = **any email/password**. Demo data comes from `apps/mobile-admin/lib/demo-api-client.ts` (store "Bondi Beach Co.", canned orders/products/customers).
- metro dev server: `EXPO_PUBLIC_AUTH_BACKEND=demo npx expo start --dev-client --clear`. watchman is installed.
- Can't tap headlessly. To screenshot the tabs, temporarily set `createDemoBackend`'s initial `active` user in `packages/mobile-shared/auth/provider.tsx` (there's a `let active: AuthUser | null = null;` — set it to a demo user), reload, screenshot, then **revert**. To skip the dev-launcher explainer for a clean shot: `xcrun simctl openurl <udid> "mark8ly-admin://expo-development-client/?url=http%3A%2F%2Flocalhost%3A8081"`.

## Deferred nice-to-haves (mention to user; don't auto-do)

- Dashboard **notification bell** (hairline bell top-right, moss unread dot → `/(tabs)/more/notifications`; `useNotifications()` gives unread count).
- Products **"add" FAB** peeks behind the floating dock — needs `useDockClearance()` bottom offset.
- **Phase 0 final whole-branch review** — package at `.superpowers/sdd/review-d449b4db..HEAD.diff`, never run.
- Mobile product upload uses **multipart** to `/products/{id}/media` but the backend only supports the **signed-URL flow** — mobile image upload is broken against the real backend (Phase 2 product-wizard fix); mobile picker also uses `allowsEditing` (iOS square-crop, small output) and no resize.
