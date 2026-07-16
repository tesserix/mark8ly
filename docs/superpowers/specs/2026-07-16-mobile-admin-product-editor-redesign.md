# Spec — mobile-admin product editor redesign (2026-07-16)

Redesign of the product **edit** and **create** screens after real-device feedback exposed five
problems. Grounded in a design proposal from the mobile design specialist and in backend behaviour
verified against `services/marketplace-api`. Scope approved by the user: **everything, including
add-option-with-variants.**

## The five problems (observed on device)

1. **Options renders an empty box.** Single-variant products have `options: []`; `OptionsEditor`
   only edits existing values — no empty state, no way to add an axis. Blank card reads as broken.
2. **Categories is an unbounded inline checkbox list** that pushes the rest of the form off-screen.
3. **Variant fields run edge-to-edge** — no horizontal padding.
4. **The create screen lacks all this depth** — it's a shallow 3-step wizard.
5. **Overall layout is five identical cards** — "assembled, not composed."

## Locked decisions (user, 2026-07-16)

- **Categories:** a tappable field showing selected chips → opens a **bottom-sheet picker** with
  search + the category tree. Multi-select, commit on close.
- **Create:** **streamlined** — quick create (title, primary variant price/SKU/stock, status,
  optional category), then hand off to the edit screen where options/extra variants/media are added.
- **Full scope** including add-option-that-generates-variants.

## Enablers (verified present — no `npm install`)

- 🔑 **`@gorhom/bottom-sheet` v5.2.8 is installed and `BottomSheetModalProvider` is already mounted**
  in `app/_layout.tsx` (with `GestureHandlerRootView`). Use `BottomSheetModal` + `BottomSheetFlatList`.
- `expo-haptics` installed (currently unused) — use for consequential actions.
- `react-native-gesture-handler`, `react-native-reanimated` installed.
- Design tokens: `apps/mobile-admin/lib/theme.ts` (paper/ink/moss, spacing, radii, hairline, shadows).
- Primitives: `apps/mobile-admin/components/ui/` (`Screen, Card, Text, Eyebrow, Hairline,
  SegmentedControl, SearchField, EmptyState, StatusBadge, PageHeader, BackHeader`). `Card` supports
  `variant="ghost"` and `padding={0}`.

## Design system constraints (non-negotiable)

Paper · Ink · Moss, editorial luxury, light mode only. One accent (moss) per view, spent on the
single primary action. Hairline rules over bordered cards where it fits. Source Serif for editorial
moments. Only `lib/theme.ts` tokens — no hardcoded colours/radii/spacing. WCAG 2.1 AA (44pt targets,
visible moss focus, SR labels, honor reduced motion). The floating Dock overlaps scroll bottoms —
clear it with `useDockClearance()`; sheets/CTAs must clear the home indicator.

---

## 🔴 Area 1 — Options: add an axis that actually creates variants

### The backend truth (verified 2026-07-16 — do NOT re-derive)

`UpdateAggregate` runs `applyOptionsDiff` (creates option/value rows) and, **separately**,
`applyVariantsDiff` — the latter **only runs when `req.Variants != nil || req.RemovedVariantIDs != nil`**
(`service_aggregate.go`). So:

- `PATCH {options: [{name:"Size", values:["S","M"]}]}` **alone** creates the Size option with S/M
  values but **creates NO variants and leaves the existing variant unlinked** → an incoherent
  product: an option axis exists, but there are zero sellable S/M variants. **This is the trap the
  designer's proposal fell into.**
- To add an axis meaningfully, the client MUST send `options` **and** a `variants` matrix in the
  same PATCH, where each variant carries its `option_values` tuple.
- 🔴 `variants` is a **FULL DESIRED MATRIX** — `applyVariantsDiff` **soft-deletes any existing
  variant not present**. So the generated matrix must carry the **existing variant's `id`** on the
  combination it maps to, or that variant (its price/stock/sales history) is destroyed.

### The matrix-generation design (the load-bearing, dangerous part)

A **pure, exhaustively-tested helper** `buildOptionMatrix` owns this. Given the current product
(existing options + variants) and the desired option set (with the new axis), it returns the full
`variants` array to send. Rules:

1. The desired variant set is the **Cartesian product** of all option values across all axes.
2. **Preserve existing variants by identity.** For each existing variant, compute its option tuple
   (its `option_values`). If that tuple still exists in the new product space, the generated variant
   for that tuple **reuses the existing variant's `id`, sku, price, stock, dimensions** — untouched.
3. **New combinations** (tuples with no existing variant) are generated with: empty `id`, a derived
   SKU (`deriveSku(title)` + value suffixes, the existing helper), the **first existing variant's
   price** as a sensible default, `inventory_quantity: 0`, and `option_values` set to the tuple.
4. **The common case** (single-variant product, 0 existing options, adding one axis "Size: S,M,L"):
   the sole existing variant maps to the **first** value (S) — keeps its id/price/stock/sku; M and L
   are new. Result: 3 variants, one carrying the original data, two to be filled in.
5. **Removing an axis / value** flows through the same helper (Cartesian shrinks; orphaned
   combinations drop). Backend rejects removing a value still referenced by a surviving variant
   (`OptionValueInUse`) — surface that error.
6. Never emit a variant without its `option_values` tuple, and never omit an existing variant's id
   when its tuple survives — those are the two ways to corrupt or destroy data. Both get pinning
   tests.

### The UI

- **Empty state** (no options): inline copy inside the Options card (NOT the full-screen
  `EmptyState` primitive) — "This product has one variant." / "Add an option like Size or Colour to
  sell variations." + a full-width **"＋ Add option"** row (44pt, hairline, moss `Plus` + label).
- **`OptionBuilderSheet`** (`BottomSheetModal`, `snapPoints={['60%']}`): serif title "New option";
  Name field; Values chip-input (type + return → chip; ≥1 required); a **consequence note** in a
  `surfaceAlt` ghost block with a `warning` `AlertTriangle`: *"Adding an option creates a variation
  for each value. Your current price and stock stay on the first one; the new variations start empty
  — fill them in below."* (This copy matches the verified behaviour above.) Sticky footer: ghost
  Cancel + moss **"Add option"**. On confirm: `buildOptionMatrix` → `updateProduct({id, body:{
  options, variants }})` → `Haptics.notificationAsync(Success)`.
- Existing options keep the current name→chips→add-value editing, each on its own hairline block,
  with the "＋ Add option" row to add a second axis.
- **Multi-axis on multi-variant** (e.g. product with Size, adding Colour): `buildOptionMatrix`
  produces the full Cartesian product, preserving all existing variants by tuple. This is the
  hardest path — it gets its own test matrix. If any existing tuple is ambiguous or unresolvable,
  the helper **fails loudly** (throws a named error surfaced as an alert) rather than guessing.
- States: applying (sheet Apply → spinner, disabled); error (existing `alertOnError`).
- A11y: add row `role=button`; sheet fields labeled; consequence note is real text; focus into Name.

---

## Area 2 — Categories: field → bottom-sheet picker

- **`CategoryField`** replaces the inline picker in the Categories card. Empty → "Add categories" +
  chevron. Selected → selected categories as chips (reuse the `OptionsEditor` chip visual, no X;
  >4 → show 4 + "+N"). Whole field is one `Pressable` (`role=button`, "Categories, N selected, edit").
- **`CategoryPickerSheet`** (`BottomSheetModal`, `snapPoints={['92%']}`, `enablePanDownToClose`):
  serif "Categories" + moss "Done"; `SearchField` pinned (filters `sortCategoryTree` by name,
  case-insensitive — matches-only while a query is present, tree restored when cleared);
  `BottomSheetFlatList` of 44pt tree rows (reuse `sortCategoryTree` verbatim — orphan/self-ref safe),
  `paddingLeft: spacing.md * depth`, name + trailing moss `Check` when selected. Selection **staged
  in the sheet, committed once on Done** (one PATCH, not per-toggle — kinder to the 60/min limiter).
  Preserve store ordering (`nodes.map(...).filter(...)`). List `contentContainerStyle` bottom padding
  clears the home indicator.
- States: loading (field disabled "Loading categories…"; sheet spinner); empty (no categories in
  store → sheet `EmptyState` pointing to web admin — category creation isn't in this app);
  no-results (inline "No categories match '…'"); error (field "Couldn't load, tap to retry").
- A11y: rows `role=checkbox` + `accessibilityState={{checked}}` (carry over); `Haptics.selectionAsync`
  on toggle.

---

## Area 3 — Variants: padding fix + tame density

- **Padding fix (do immediately, one line):** add `paddingHorizontal: theme.spacing.lg` to
  `VariantEditor`'s `root`. The Variants `Card` stays `padding={0}` (full-bleed hairlines); the
  content insets. Fixes the edge-to-edge bug.
- **`VariantRow` (collapsible):** a summary row (44–56pt) — `variantLabel` in `bodyEmphasis`, a
  caption "A$34.00 · 12 in stock · SKU-123", a stock `StatusBadge` (`success`/`warning`/`muted`) and
  a rotating `ChevronDown`. Tap expands into today's `VariantEditor` body. **Shipping (weight + 4
  dimensions) collapses one level deeper** behind its own "Shipping & dimensions" disclosure —
  hides 5 of 7 fields until asked. Single-variant products auto-expand the sole row ("Default
  variant" heading, since it has no option tuple). Keep the blur/NaN/rate-limit commit logic exactly.
- `SectionDisclosure` — a generic reduced-motion-aware expand/collapse used by `VariantRow` and the
  Shipping subsection.
- A11y: summary `role=button` + `accessibilityState={{expanded}}`, full label; instant show under
  reduce-motion.

---

## Area 4 — Create: streamlined single screen + hand-off

- Collapse `new.tsx` to **one screen, no steps** (delete `StepDots`/`TOTAL_STEPS`/step state/footer).
  Under `BackHeader eyebrow="NEW PRODUCT"`: **Essentials** (Title* autofocus, Price(AUD)* + Stock
  2-col, SKU with `deriveSku` placeholder, Description optional), **Status** (`SegmentedControl`
  Draft/Active), optional **Category** (the same `CategoryField` from Area 2). Tags deferred to edit.
- **Sticky bottom CTA** (clears the dock): full-width moss **"Create product"** + ghost "Save as
  draft". Keep `deriveSku` safety net + AUD default.
- **Hand-off:** `create` returns the full product incl. `id` (verified). On success,
  `router.replace('/(tabs)/products/[id]?created=1')` — land on edit, not `router.back()`.
- **`CreateNextStepsBanner`** on edit when `?created=1`: dismissible `Card variant="ghost"` on
  `surfaceAlt` — serif "Nice — '{title}' is live." + caption + three ghost chips that scroll-to
  Photos / Options / Variants (ScrollView ref + section `onLayout` offsets). Normal edit visits never
  show it. `Haptics.notificationAsync(Success)` on landing.
- States: submitting (spinner); validation inline under the field (title + non-negative price),
  not an Alert; create error surfaces `ApiError.message` (like edit's `getErrorMessage`).

---

## Area 5 — Overall editor rhythm

- **Editorial header block** at the top of the scroll: main image thumb (72×72, `radii.md`) left,
  serif `h2` title + `StatusBadge` right, on a `surfaceAlt` band with a bottom hairline. `BackHeader`
  keeps chevron + Save.
- **Two movements** with more air between (`spacing.xxl`) than within (`spacing.lg`): *Presentation*
  (Photos, Details) and *Commerce* (Options, Categories, Variants).
- **Ghost cards + hairlines** for Details/Options/Categories (`Card variant="ghost"` + top
  `Hairline`); reserve bordered `elevated` `Card` for the Photos strip and Variants list.
- **One accent, spent once:** moss on header Save only at rest; photo "Add" and option/category adds
  are ink, turning moss on press/focus.
- **`FieldInput` / `FieldLabel` shared primitive** — kill the input-styling drift (`[id].tsx`,
  `new.tsx`, `VariantEditor`, `OptionsEditor` each redefine it, two different backgrounds). Standard
  on `surfaceAlt`.

---

## New components

| Component | Responsibility |
|---|---|
| `buildOptionMatrix` (pure helper) | Given current product + desired options, return the full `variants` matrix, preserving existing variants by id/tuple. The dangerous, heavily-tested core. |
| `OptionBuilderSheet` | Bottom sheet to define a new option axis (name + values) with the matrix-consequence confirm. |
| `CategoryField` | Tappable field: selected chips or "Add categories"; opens the sheet. |
| `CategoryPickerSheet` | Full-height sheet: search + depth-indented tree, multi-select, commit on Done. |
| `VariantRow` | Collapsible per-variant summary that expands into `VariantEditor`. |
| `SectionDisclosure` | Generic reduced-motion-aware expand/collapse. |
| `CreateNextStepsBanner` | Dismissible post-create nudge that scroll-jumps to sections. |
| `FieldInput` / `FieldLabel` | Shared input + label primitive to stop create/edit drift. |

## Build order (per the designer, re-costed)

1. Variant padding fix (trivial). 2. `FieldInput`/`FieldLabel` consolidation (unblocks the rest).
3. `buildOptionMatrix` + tests (the risky core, isolated first). 4. Options empty-state +
`OptionBuilderSheet` (consumes #3). 5. `CategoryField` + `CategoryPickerSheet`. 6. Streamlined
create + hand-off + `CreateNextStepsBanner`. 7. `VariantRow`/`SectionDisclosure` density. 8. Rhythm
pass (editorial header, ghost cards, two movements, one-accent).

## Testing & gates

- `buildOptionMatrix` gets an exhaustive pure-function test matrix: single-variant + one axis;
  multi-variant + new axis (Cartesian); value removal; existing-variant preservation by id (the
  soft-delete guard); the fail-loud path. **A test must prove no existing variant id is ever
  dropped when its tuple survives.**
- Component tests (render + fireEvent) for each new component; sheets tested for open/select/commit.
- Reuse the `jest.mock("lucide-react-native")` stub for any component pulling `@/components/ui`.
- Gates unchanged: `npx jest` (grows from 263), `npx tsc --noEmit --pretty false | grep -c "error TS"`
  = 0 in **both** `apps/mobile-admin` and `packages/mobile-shared`. Never `npm install`.

## Out of scope

- Category creation on mobile (web admin only — the empty state points there).
- Backend changes (all client-side; the `variants` full-matrix contract already exists).
- A transactional batch reorder endpoint (separate follow-up from the prior sub-project).

## Risks

- **`buildOptionMatrix` is the highest-risk code in the app** — a wrong tuple mapping soft-deletes
  real variants. It is a pure helper, tested exhaustively, and fails loud on ambiguity rather than
  guessing. The `OptionBuilderSheet` never sends `variants` except via this helper.
- Multi-axis-on-multi-variant re-expansion is genuinely complex; the helper supports it but the
  spec accepts fail-loud over silent-wrong for any ambiguous case.
