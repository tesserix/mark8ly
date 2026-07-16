# Handoff — mobile-admin product editor redesign (2026-07-16, late)

Work in `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`. Commit directly to `main`,
single-line conventional messages, **no signatures, no PRs**.

## Read first (do not re-derive any of it)
- **Ledger (live state):** `.superpowers/sdd/progress.md` — the redesign SDD ledger. Trust it + `git log`.
- **Spec:** `docs/superpowers/specs/2026-07-16-mobile-admin-product-editor-redesign.md`
- **Plan:** `docs/superpowers/plans/2026-07-16-mobile-admin-product-editor-redesign.md`
- **Memory:** `~/.claude/projects/-Users-Mahesh-Sangawar-personal-tesserix-new-mark8ly/memory/mobile_admin_contract_mismatches.md`
  (wire truth, landmines, the "assert vs run" lesson) and `feedback_ci_billing_workaround.md`
  (the ACTIVE public-repo override, below).

## 🔴 STANDING USER INSTRUCTIONS (honor these)
- **The repo is PUBLIC right now, and MUST STAY PUBLIC until the user explicitly says to revert.**
  The user said "don't change back to private until we resolve all issues." The auto-flip-back step
  of the CI-billing workaround is SUSPENDED. Do NOT flip to private. (Public = unlimited Actions
  minutes, which this repo's Free plan needs — minutes are exhausted for private.) `gh` for this repo
  needs `gh auth switch --user mahesh-sangawar`; switch back to `Mahesh-Sangawar_civica` after.
- User-approved working defaults for `buildOptionMatrix` (correctable): new option combinations inherit
  the first existing variant's PRICE + inventory_quantity 0; ambiguous multi-axis re-expansions FAIL
  LOUD (OptionMatrixError → Alert), never guess.
- Two locked design decisions: Categories → tappable field + @gorhom/bottom-sheet picker; Create →
  streamlined single screen, hand off to edit via `router.replace(?created=1)`.

## Process (continue exactly this)
`superpowers:subagent-driven-development`: fresh implementer per task → generate review package →
task reviewer → fix loop → mark complete. Ledger `red-` prefixed artifacts (shared scratch has
clobbered before). Gates after EVERY task: `npx jest` (read the TAIL; coloured), `npx tsc --noEmit
--pretty false 2>&1 | grep -c "error TS"` = 0 in BOTH `apps/mobile-admin` AND `packages/mobile-shared`.
Component tests need the `jest.mock("lucide-react-native", ...)` stub. `noUnusedLocals` OFF — eyeball
orphaned imports. **NEVER `npm install`.**

## DONE this session
- **#3 padding fix** — `8a1f7a3c` (variant fields no longer edge-to-edge).
- **Task 1** — `FieldInput`/`FieldLabel` shared primitives — `53b617d3`.
- **Task 2** — `buildOptionMatrix` (`lib/option-matrix.ts`) + `lib/sku.ts` (deriveSku moved from
  new.tsx + deriveVariantSku) + `VariantMatrixInput`/`variants?` on `UpdateProductBody` —
  `9d7fe677` + fix `0fe6e8dc`. **OPUS ADVERSARIAL REVIEW PROVED the soft-delete guarantee safe.**
  🔑 **`buildOptionMatrix` is the ONLY safe producer of a `variants` PATCH body — never hand-build one.**
  Fix added duplicate-value/name guards. 277/277.
- **Task 3** — Options empty-state + `OptionBuilderSheet` + `lib/hooks/use-add-option-handler.ts`,
  wired through buildOptionMatrix — `9d172da6`. Review APPROVED; safety rule PROVEN (handler sends
  only matrix output, catches OptionMatrixError). 290/290, both tsc 0.

## 🔴 Task 3 FOLLOW-UPS (do these first — review found them, not yet fixed)
1. **The OptionBuilderSheet has NO scroll container** — content (title + name + chips + consequence
   note + footer) can clip / be keyboard-obscured on small screens (iPhone SE). Wrap the sheet body in
   `BottomSheetScrollView` (NOT plain View — note the implementer used plain `View` for the body to
   dodge a dual-`@types/react` TS2786 landmine on `BottomSheetView`; that swap is fine, but there's
   still no scroll wrapper). Important — the feature may be unusable on small devices until fixed.
2. **The `variants:` source-guard is now vacuous** — `product-detail-sections.test.tsx:36` greps
   `[id].tsx`, but the `variants`-bearing `mutate` moved to `use-add-option-handler.ts`. Add a sibling
   assertion grepping `use-add-option-handler.ts` for a hand-built `variants:` literal.
3. **Missing `Haptics.notificationAsync(Success)` on add-option confirm** — spec Area 1 line, dropped.
   `expo-haptics` is installed. Minor.

## TODO — Tasks 4–7 (per the plan)
- **Task 4** — `CategoryField` + `CategoryPickerSheet` (search + tree sheet; reuse `sortCategoryTree`;
  commit selection once on Done). Resolves issue #2 (long category list).
- **Task 5** — streamlined `new.tsx` (single screen, no wizard) + `router.replace('/[id]?created=1')`
  hand-off + `CreateNextStepsBanner`. `useCreateProduct` returns the full product incl. `id` (verified).
  Resolves issue #4.
- **Task 6** — `VariantRow` (collapsible) + `SectionDisclosure` (reduced-motion-aware). Tames the dense
  variant wall. 🔑 **RUN the `review-animations` skill on this task's motion** (the disclosure
  expand/collapse) — it's the one place the redesign writes real custom animation. Standards: sub-300ms,
  ease-out/custom curve, honor `prefers-reduced-motion` (instant, not zero), interruptible, GPU props.
- **Task 7** — editorial rhythm pass (header block, two movements, ghost cards, one-accent, migrate
  inline inputs to `FieldInput`). Resolves issue #5.

## Sim / device state — ⚠️ needs a clean metro restart
- Sim `AD109A46-2F99-43C3-8AAA-FEE68DC8499E` booted, app installed. **Metro on :8081 is STALE/
  unresponsive** after this long session — the dev client shows "Failed to load app from
  localhost:8081" and `curl localhost:8081/index.bundle` returns empty. **Restart metro clean:**
  `cd apps/mobile-admin && npx expo start --dev-client --port 8081 --clear` (NO demo flag — a stale
  demo metro silently serves the demo bundle). Then load the bundle into the dev client:
  `xcrun simctl openurl <UDID> "mark8ly-admin://expo-development-client/?url=http%3A%2F%2Flocalhost%3A8081"`,
  wait ~40s for the first bundle, then `xcrun simctl openurl <UDID> "mark8ly-admin://products"`.
  Deep-link to the LIST, never `://products/<id>`. You CANNOT tap (idb absent) — the human taps; you
  `xcrun simctl io <UDID> screenshot`.
- **Device tap-through of the new Options add-flow is UNVERIFIED** — the human must exercise: empty
  Options → "＋ Add option" → sheet → add "Size: S,M,L" → confirm it creates 3 variants with the
  existing one preserved. This is the acceptance test for Task 2+3.

## Skills added this session
`emilkowalski/skills` cloned + installed into `~/.claude/skills/`: `review-animations`,
`improve-animations`, `find-animation-opportunities`, `apple-design`, `emil-design-eng`,
`animation-vocabulary`. Use `review-animations` on Task 6's motion.

## The lessons that keep paying off (inherit them)
- **A test that passes with the bug present is not a test.** This session, FOUR tasks needed a fix
  for exactly that (vacuous sort/order tests, a regex anchored on a token not in the file). Every
  sort/order/guard test needs an OUT-OF-ORDER fixture or a real counterexample.
- **The whole-branch review (opus) earns its cost** — last sub-project it found a shipped-broken
  feature (reorder wrote duplicate positions) that 11 green per-task reviews missed. Do the opus
  whole-branch review at the end of THIS redesign too, and verify its claims yourself.
- **Read the whole function; run the falsifying command.** A `category_ids` "bug" and a `buildOptionMatrix`
  algorithm both came from partial reads / untested assumptions; both were caught by running the code.
