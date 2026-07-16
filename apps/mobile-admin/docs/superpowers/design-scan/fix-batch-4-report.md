# Fix Batch 4 (final) — motion polish

Scope: apply the four refinements from `docs/superpowers/design-scan/slice-5-motion.md` (mark8ly root
copy). All four items shipped — none were skipped. Every animated path is gated on
`useReducedMotion()`; every property animated is `opacity`/`transform` only (never height/width/top/
left). One easing throughout: `DISCLOSURE_EASING = Easing.bezier(0.22, 1, 0.36, 1)`, imported from
`components/products/disclosure-motion.ts` everywhere — no curve was redefined.

## 1. Disclosure reveal — directional transform + asymmetric collapse

**Files:** `components/products/disclosure-motion.ts`, `components/products/SectionDisclosure.tsx`,
`components/products/VariantRow.tsx`.

- Added `export const DISCLOSURE_EXIT_DURATION = 160;` (collapse is a system response to a tap, not a
  deliberate reveal — it should snap back faster than the 220ms open).
- Added two custom worklet builders, `disclosureEntering()` / `disclosureExiting()`, returning a
  `LayoutAnimation` (`{ initialValues, animations }`) directly, instead of
  `FadeIn.withInitialValues({ transform: [...] })` as the audit doc's literal suggestion had it.
  **Why not the literal suggestion:** reanimated only interpolates a style key when it also appears
  under the returned `animations` map (confirmed by reading `Fade.ts`/`FadeInUp` source in
  `node_modules/react-native-reanimated/src/layoutReanimation/defaultAnimations/Fade.ts`). Plain
  `FadeIn`'s `animations` only contains `opacity`; seeding `transform` solely via `withInitialValues`
  would apply it once and leave the view permanently offset — not a settling motion. The custom
  builders define `transform.translateY` under both `initialValues` and `animations`, exactly the "or a
  small custom builder" alternative the task allowed.
  - `disclosureEntering`: opacity 0→1 and `translateY` −6px→0, 220ms, `DISCLOSURE_EASING` — content
    settles down into place.
  - `disclosureExiting`: opacity 1→0 and `translateY` 0→−4px, 160ms (`DISCLOSURE_EXIT_DURATION`) —
    lifts up and out, snappier than the open.
- `SectionDisclosure.tsx` / `VariantRow.tsx`: swapped `FadeIn.duration(...).easing(...)` /
  `FadeOut.duration(...).easing(...)` for `disclosureEntering` / `disclosureExiting`, still gated
  `reduceMotion ? undefined : disclosureEntering` / `...disclosureExiting`.
- Chevron rotation (`useChevronRotationStyle`) untouched — out of this item's scope, still 220ms
  both directions.
- **Reduced motion:** unchanged contract — `entering`/`exiting` collapse to `undefined` (instant) when
  `useReducedMotion()` is true.
- **Tests:** `__tests__/section-disclosure.test.tsx` / `__tests__/variant-row.test.tsx` needed no
  changes — they assert `.props.entering` is `undefined`/defined, which holds for a function reference
  the same as a chained builder instance. Both files pass unmodified (18/18).

## 2. `components/products/OptionBuilderSheet.tsx` — chip add/remove animation

- Imported `Animated`, `FadeIn`, `FadeOut`, `LinearTransition`, `useReducedMotion` from
  `react-native-reanimated`, plus `DISCLOSURE_EASING` / `DISCLOSURE_EXIT_DURATION` from
  `disclosure-motion.ts`.
- Each value chip (`values.map(...)`) is now an `Animated.View` (was a plain `View`):
  - `entering={reduceMotion ? undefined : FadeIn.duration(DISCLOSURE_EXIT_DURATION).easing(DISCLOSURE_EASING)}`
  - `exiting={reduceMotion ? undefined : FadeOut.duration(DISCLOSURE_EXIT_DURATION).easing(DISCLOSURE_EASING)}`
  - `layout={reduceMotion ? undefined : LinearTransition.duration(DISCLOSURE_EXIT_DURATION).easing(DISCLOSURE_EASING)}`
    so sibling chips animate their reflow instead of snapping.
  - All three at 160ms (`DISCLOSURE_EXIT_DURATION`) — a chip add/remove is a quick, repeated
    micro-interaction, not a deliberate reveal.
  - Added `testID={\`option-chip-${value}\`}` for test targeting.
- **Reduced motion:** all three props (`entering`/`exiting`/`layout`) collapse to `undefined`.
- **Tests:**
  - `__tests__/option-builder-sheet.test.tsx`: added a `react-native-reanimated` jest mock (the file
    now transitively imports the real module at load time) with `FadeIn`/`FadeOut`/`LinearTransition`
    chainables. Changed the `@gorhom/bottom-sheet` mock's `BottomSheetModal` to render `children`
    directly (was returning `null`) so the sheet's body — and its chips — can actually mount; this only
    affects this file (each test file's mock is local). Added a new describe block,
    `OptionBuilderSheet — chip reduced motion`, that renders the real sheet, adds one chip via the
    "Add a value" field + `submitEditing`, and asserts `entering`/`exiting`/`layout` are
    `undefined` (reduced) vs. defined (not reduced) on the resulting `option-chip-Small` node — 2 new,
    non-vacuous tests.
  - `__tests__/options-editor.test.tsx` and `__tests__/use-add-option-handler.test.tsx` both
    transitively import `OptionBuilderSheet.tsx` (via `OptionsEditor`/`toOptionRequestBodies`) and did
    not previously mock `react-native-reanimated` — their `BottomSheetModal` mocks return `null`
    unconditionally, so the chips never render there, but the *module* import still needed a mock to
    avoid the real reanimated module throwing under jest. Added the same minimal virtual mock to both.
  - `__tests__/product-detail-created-banner-offsets.test.tsx` already had a full reanimated mock
    (covers `VariantRow`) — no change needed; reran to confirm still green.

## 3. List spinner → content crossfade

**Files:** `app/(tabs)/products/index.tsx`, `app/(tabs)/orders/index.tsx`,
`app/(tabs)/customers/index.tsx`.

- Wrapped the loaded `FlatList` branch in each screen with an `Animated.View`
  (`style={styles.listWrap}` → `{ flex: 1 }`, so the wrapper preserves the `FlatList`'s existing
  flex-fill/scroll behaviour) and
  `entering={reduceMotion ? undefined : FadeIn.duration(180).easing(DISCLOSURE_EASING)}`. The spinner
  itself is untouched — only the list's mount crossfades in.
  - `testID`s added for test targeting: `products-list-wrap`, `orders-list-wrap`,
    `customers-list-wrap`.
  - `DISCLOSURE_EASING` imported from `@/components/products/disclosure-motion` in all three screens,
    per the task's explicit instruction to reuse the token even across domains (orders/customers
    importing from a `products/` file is unusual but intentional, per the task brief).
- **Reduced motion:** `useReducedMotion()` added to each screen; `entering` collapses to `undefined`.
- **Tests:** none of the three screens had any test coverage before this batch. Added
  `__tests__/products-index-reduced-motion.test.tsx` (representative — all three screens follow the
  identical pattern) with mocks for `expo-router`, `@/lib/hooks/use-products`,
  `react-native-safe-area-context`, `lucide-react-native`, and `react-native-reanimated`. Two
  non-vacuous tests: `products-list-wrap`'s `entering` is `undefined` when reduced motion is on, and
  defined when off. `orders/index.tsx` and `customers/index.tsx` were not given their own duplicate
  test files (same pattern, same risk profile) — flagging this as the one place coverage is
  representative rather than exhaustive, per instructions to avoid forcing brittle/redundant work.

## 4. `components/products/CreateNextStepsBanner.tsx` — exit transition on dismiss

- Wrapped the returned `Card` in an `Animated.View` (`testID="create-next-steps-banner"`):
  - `entering={reduceMotion ? undefined : FadeIn.duration(DISCLOSURE_DURATION).easing(DISCLOSURE_EASING)}`
    (220ms — matches the app's open-reveal rhythm; in practice this rarely plays since the banner is
    present at first paint on hand-off, but the task asked for "a matching subtle entering").
  - `exiting={reduceMotion ? undefined : FadeOut.duration(DISCLOSURE_EXIT_DURATION).easing(DISCLOSURE_EASING)}`
    (160ms) — so the dismiss (X button) fades the card out instead of yanking the scroll content
    upward instantly.
- Dismiss behaviour unchanged: `onDismiss` still fully removes the banner from the tree (via
  `dismissCreatedBanner` in `use-created-banner.ts`) once the exit animation completes; the parent's
  `{showCreatedBanner ? <CreateNextStepsBanner .../> : null}` conditional at
  `app/(tabs)/products/[id].tsx:262` is unchanged.
- **Tests:**
  - `__tests__/create-next-steps-banner.test.tsx`: added the same minimal `react-native-reanimated`
    jest mock (file previously had none — the component now imports the real module). Added a new
    describe block, `CreateNextStepsBanner — reduced motion`, asserting `entering`/`exiting` are
    `undefined` (reduced) vs. defined (not reduced) — 2 new, non-vacuous tests. All 7 tests in the
    file pass (5 pre-existing + 2 new).
  - `__tests__/product-detail-created-banner-offsets.test.tsx` renders the banner through the full
    product-detail screen and already had a full reanimated mock — reran to confirm still green
    (unaffected by the new `entering`/`exiting` props, since its mocked `Animated.View` is a plain RN
    `View`).

## Skipped

None. All four items shipped as specified; the one item flagged above (list-screen test coverage is
representative, not exhaustive across all three screens) is a coverage-depth note, not a skip.

## Gates

- `npx jest --forceExit` (full suite): **352/352 passed**, 48 suites (baseline was 346/346, 47 suites;
  +6 new tests across `option-builder-sheet.test.tsx` (+2), `products-index-reduced-motion.test.tsx`
  (new file, +2), `create-next-steps-banner.test.tsx` (+2)). Tail:
  ```
  Test Suites: 48 passed, 48 total
  Tests:       352 passed, 352 total
  Snapshots:   0 total
  Time:        5.536 s
  Ran all test suites.
  Force exiting Jest: Have you considered using `--detectOpenHandles` to detect async operations that
  kept running after all tests finished?
  ```
  (Same pre-existing "worker process failed to exit gracefully" / force-exit note as batch 3 — unrelated
  to these changes, a leaked timer/handle elsewhere in the suite.)
- `apps/mobile-admin`: `npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` → **0**
- `packages/mobile-shared`: `npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` → **0**

No dependencies added. No token values changed — only `DISCLOSURE_EASING` / `DISCLOSURE_DURATION` /
the newly-added `DISCLOSURE_EXIT_DURATION`, all from `disclosure-motion.ts`, used everywhere. `Dock.tsx`'s
duplicate `ENTRANCE_EASING` (noted in the audit as a P3 token-drift risk) was left untouched — out of
this batch's four-item scope.
