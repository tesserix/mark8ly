# Handoff — mobile-admin design pass + create-category (2026-07-17)

Work in `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`. Commit directly to `main`, single-line
conventional messages, **no signatures, no PRs**. Everything below is already pushed to `origin/main` (HEAD `a1c5cccb`).

## Read first (don't re-derive)
- **Memory:** `~/.claude/projects/-Users-Mahesh-Sangawar-personal-tesserix-new-mark8ly/memory/`
  — `mobile_admin_design_pass.md` (this session: design tokens, one-accent, un-centered layouts, the
  **modal-sheet-provider landmine**, and the **create-category** feature+deploy),
  `mobile_admin_contract_mismatches.md` (API/wire truth, sim-driving, test-mock landmines),
  `mark8ly_kargo_image_promotion.md` + `feedback_marketplace_api_landmines.md` (backend deploy),
  `feedback_ci_billing_workaround.md` (public-repo override).
- Design bar: `mark8ly/.impeccable.md` (Paper·Ink·Moss, one accent, asymmetric-never-centered, serif).
- Design audit artifacts: `docs/superpowers/design-scan/` (5 slice audits + 4 fix-batch reports).

## 🔴 STANDING INSTRUCTIONS
- **Repo is PUBLIC** (CI-billing workaround) — **stays public until the user says revert.** Do NOT flip to private.
- `gh` for this repo needs `gh auth switch --user mahesh-sangawar` (currently active; the other account is
  `Mahesh-Sangawar_civica`). `git push` uses SSH, works regardless.
- git identity: **Mahesh Sangawar / mayu.b14@gmail.com** — never override.
- Gates (from `apps/mobile-admin`): `npx jest` (read TAIL; single-file hangs without `--forceExit`) +
  `npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` = **0** in BOTH `apps/mobile-admin` AND
  `packages/mobile-shared`; `packages/mobile-shared` also has vitest (`npx vitest run`). NEVER `npm install`.
- Current green baseline: **jest 354/354, both tsc 0, vitest 83.**

## ✅ DONE THIS SESSION (all pushed)
1. **Product-editor redesign** Tasks 4–7 + Task-3 follow-ups (SDD, per-task reviews). Opus whole-branch
   review found+fixed a real banner-scroll bug (`e8258d73`, test proven RED→GREEN). Ledger:
   `.superpowers/sdd/progress.md` (gitignored). Commits `d7654ea0..85660413`.
2. **Whole-app design pass** (`e4c5c98b..a4f67b2d` + StatTile `b6894515`): AA-contrast tokens
   (textTertiary/ink.muted → `#5C5953`; warning ink-on-amber; success → moss-tint), 44pt controls,
   decorative moss stripped (one-accent), un-centered dashboard/login/customer layouts, keyboard-safe
   secure login, motion polish. **Device-verified** dashboard/catalog/orders/customers/more (screenshots).
3. **Create-modal fix** (`ad7bf6a5`): `new.tsx` is `presentation:"modal"` → needs its OWN
   `BottomSheetModalProvider` or its sheets render behind the modal; footer uses safe-area not
   `useDockClearance()`; `Screen topInset={false}`. **Device-verified** (category sheet now opens).
4. **Create-category from mobile** — backend `8e2be5ce` (`POST /mobile/admin/stores/:storeId/categories`,
   RoleAdmin, reuses `CategoryHandler.Create`) + client `a1c5cccb` (picker "＋ New category" composer,
   auto-select, drop-safe `handleDone`). **DEPLOYED to prod** (CI 29544884252 green → image `main-a1c5ccc`;
   Kargo prod pod rolled 1/1 Running). kubectl context = `tesseract-prod-in-gke`.

## 🔴 PENDING / OPEN
- **Device end-to-end verification** (idb absent — human taps, you `xcrun simctl io <UDID> screenshot`;
  sim `AD109A46-2F99-43C3-8AAA-FEE68DC8499E`, booted):
  - **create-category flow** — backend live + client loaded; user was about to test (create screen →
    Add categories → ＋ New category → name → Create → should create+select). NOT yet confirmed by eye.
  - **product editor** (Tasks 4–7), **customer detail** — tap-only screens, never eyeballed.
  - **Create-screen top whitespace** — user flagged "too much space top and bottom"; bottom fixed
    (safe-area), top may still feel spacious (native modal card gap) — confirm on device; if still off,
    consider dropping `presentation:"modal"` (would also simplify the sheet provider, but changes UX).
- **Metro is DOWN** (killed). Restart: `cd apps/mobile-admin && npx expo start --dev-client --port 8081`
  (add `--clear` only if stale). Reconnect: `xcrun simctl openurl <UDID>
  "mark8ly-admin://expo-development-client/?url=http%3A%2F%2Flocalhost%3A8081"`; deep-link to tab roots
  only (`mark8ly-admin://products|orders|customers|more`, dashboard) — nested routes (`/products/new`,
  `/products/<id>`) do NOT push via deep link.
- **Dependabot: 2 vulns (1 critical, 1 moderate)** on the repo — flagged at push, unaddressed.
- Minor design residuals (low priority, in `mobile_admin_design_pass.md`): radii naming collision
  (theme `md`=6 vs tailwind `rounded-md`=10); DashboardStats section break reads fine on device.

## Deploy mechanics (if you touch marketplace-api again)
CI (`ci.yml`, `go test ./...`) builds `mark8ly-marketplace-api` image → Kargo Warehouse `services`
(kargo-mark8ly) → **prod auto-promotes**. GAR-remote lazy-mirror LAG bites marketplace-api: if the new
Freight has the old tag, `gcloud artifacts docker images describe
asia-south1-docker.pkg.dev/tesseracthub-480811/ghcr-remote/tesserix/mark8ly-marketplace-api:main-<sha7>`
to force-resolve, then `kubectl -n kargo-mark8ly annotate warehouse services
kargo.akuity.io/refresh=$(date +%s) --overwrite`. Verify: pod image ==`main-<sha7>`. distroless container
(`server`, no sh/wget) — can't shell-probe; verify via device.
