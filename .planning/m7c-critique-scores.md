# M7c Critique Scores (Task 14)

Generated: 2026-04-10

## Summary

| # | Surface | Initial | Final | Pass? |
|---|---|---|---|---|
| 1 | ProductFormTabs | 8.5/10 | 8.5/10 | PASS |
| 2 | MediaTab | 8.0/10 | 8.0/10 | PASS |
| 3 | OptionsTab | 7.0/10 | 8.0/10 | PASS |
| 4 | VariantsTab | 7.0/10 | 8.0/10 | PASS |
| 5 | MediaCropDialog | 8.5/10 | 8.5/10 | PASS |
| 6 | MediaGrid + MediaCard | 7.5/10 | 7.5/10 | PASS |
| 7 | MediaUploader | 8.5/10 | 8.5/10 | PASS |
| 8 | OptionsEditor + OptionRow | 8.0/10 | 8.0/10 | PASS |
| 9 | VariantMatrixTable + VariantRow | 8.0/10 | 8.0/10 | PASS |
| 10 | VariantBulkBar | 7.5/10 | 8.0/10 | PASS |

All surfaces >= 7.5 after polish.

## Detail

### 1. ProductFormTabs — 8.5/10
**Strengths:** Strong keyboard nav (Arrow/Home/End), clear aria-selected/disabled, moss underline on active, focus-visible ring, tokens only, left-aligned tab bar with hairline border.
**Deductions:** -0.5 `border-opacity-10` pattern slightly muddier than using `--ink-100`; -1.0 non-active tabs rely on opacity for hierarchy rather than weight/color contrast (acceptable trade-off).
**Polish fixes applied:** none.

### 2. MediaTab — 8.0/10
**Strengths:** Container-only component, clean gap rhythm, tokens in descendant components, no rogue styling.
**Deductions:** -2.0 minimal visual surface of its own (container).
**Polish fixes applied:** none.

### 3. OptionsTab — 7.0/10 -> 8.0/10
**Strengths:** Hairline-free error alert with signal color, serif-free utilitarian stack.
**Deductions (initial):** (line 17) hardcoded hex fallback `#C23B22` violates "no new hex values" rule. Tokens-only rule means fallback hex must go.
**Polish fixes applied:**
- line 17: replaced `var(--signal,#C23B22)` (twice) with `var(--signal)` — tokens only.

### 4. VariantsTab — 7.0/10 -> 8.0/10
**Strengths:** Serif headline in empty state, token-based colors.
**Deductions (initial):** (line 30-31) empty state was a centered, dashed-border card with `text-center` and `p-12` — violates asymmetric/left-aligned rule and "no bordered cards".
**Polish fixes applied:**
- line 30: replaced dashed-bordered centered card with left-aligned hairline top rule (`border-t border-[color:var(--ink-100)] py-12 pl-1`), removed `text-center`.

### 5. MediaCropDialog — 8.5/10
**Strengths:** Full-panel editorial workbench (not a modal card), serif "Crop image" headline, "Editorial workbench" uppercase eyebrow, ink-on-paper footer chrome, moss accent only on the primary CTA, Escape/Enter keyboard, role=dialog + aria-modal, moss focus rings on every control.
**Deductions:** -1.5 zoom + rotate controls presented as two bordered buttons (border hairlines acceptable since they are form controls, not cards).
**Polish fixes applied:** none.

### 6. MediaGrid + MediaCard — 7.5/10
**Strengths:** Sortable list semantics (`<ol role="list">`), drag handle via attributes, serif primary badge, token-only palette, shadow scale from tokens, focus ring on menu trigger, group-focus-within reveals menu button (keyboard-discoverable).
**Deductions:** -1.0 MediaCard uses a bordered thumbnail (line 29 `border border-[var(--ink-100)]`) which flirts with the "no bordered cards" rule (defensible for image tiles); -0.5 menu popover is also bordered (ditto); -1.0 shadow on hover is a micro lift (kept within token scale so allowed).
**Polish fixes applied:** none — above 7.5.

### 7. MediaUploader — 8.5/10
**Strengths:** Serif headline call, uppercase eyebrow on file types, moss focus-within + hover, progress rows use hairline-bottom rule (not bordered cards), tokenized danger/moss bars, accessible label linkage.
**Deductions:** -1.5 dashed border on dropzone (defensible convention for drop targets).
**Polish fixes applied:** none.

### 8. OptionsEditor + OptionRow — 8.0/10
**Strengths:** Rows separated by hairline bottom rules (no bordered cards), chip-based values with individual remove affordances + aria-labels, moss focus rings, uppercase eyebrow "Add option" CTA.
**Deductions:** -1.0 chip pill uses a border (acceptable micro-component chrome); -1.0 name input has no visible placeholder hint beyond placeholder text (minor typography beat).
**Polish fixes applied:** none.

### 9. VariantMatrixTable + VariantRow — 8.0/10
**Strengths:** Table rows with hairline bottoms, uppercase eyebrow headers, tabular-nums on numeric columns, per-input aria-labels, moss focus rings, currency-aware price header, controlled-buffer inputs with commit-on-blur.
**Deductions:** -1.0 numeric column headers could use serif for editorial rhythm but sans uppercase micro-type reads fine; -1.0 image thumbnail button is bordered (micro-component convention).
**Polish fixes applied:** none.

### 10. VariantBulkBar — 7.5/10 -> 8.0/10
**Strengths:** Hairline top rule container (not bordered card), serif variant count, moss accent on Apply verb, uppercase eyebrow tracking-widest actions.
**Deductions (initial):** (line 60) redundant inline `style={{ fontFamily: ... }}` duplicating the `font-[var(--font-serif)]` class — violates "trust the system" / single source of truth.
**Polish fixes applied:**
- line 60: removed redundant inline style, switched class to `font-[family-name:var(--font-serif)]` for consistency with VariantsTab empty state.

## Notes

- All fixes are surgical visual/token cleanups. No behavioral changes, no new deps, no dialogs added, no new hex values.
- `go.work.sum` untouched.
- Touched only files in `apps/admin/components/products/form/` + `apps/admin/components/products/variants/`.
