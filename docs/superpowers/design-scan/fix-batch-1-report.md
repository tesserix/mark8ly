# Design-quality fix batch 1 — tokens + primitives

Scope: `lib/theme.ts`, `tailwind.config.js`, `components/ui/StatusBadge.tsx`,
`components/ui/SegmentedControl.tsx`, `components/ui/SearchField.tsx`.

## 1. `lib/theme.ts` — contrast + reconciliation

| Token | Old | New | Rationale |
|---|---|---|---|
| `textTertiary` | `rgba(14, 14, 12, 0.5)` | `#5C5953` (canonical `--ink-500`) | 🔴 P0 — old value fails AA (see contrast below) |
| `inkSoft` (→ `textSecondary`) | `#3A3A36` | `#45433E` (canonical `--ink-600` / tailwind `ink.soft`) | drift reconciliation |
| `bone` (→ `border`) | `#E5E4DF` | `#E2DFD6` (canonical `--ink-100` / tailwind `border`) | drift reconciliation |
| `mossSoft` (→ `accentSoft`) | `#3F6A3D` | `#3D5F38` (canonical `--moss-600` / tailwind `moss.soft`) | drift reconciliation |
| `amber` (→ `warning`) | `#B08A30` | `#B5751F` (tailwind `warning.DEFAULT`) | drift reconciliation |
| `crimson` (→ `danger`) | `#8B2020` | `#8B2E20` (tailwind `danger.DEFAULT`) | drift reconciliation |
| `paperWarm` (→ `surfaceAlt`) | `#FAFAF6` | `#FAF8F2` (canonical `--paper-100`) | drift reconciliation |
| `accentTint` (new) | — | `#E8EEE2` (canonical `--moss-100` / tailwind `moss.tint`) | added for StatusBadge success tint |

`textMuted` (`rgba(14, 14, 12, 0.35)`): grepped every usage of `textMuted` /
`ink-faint` / `theme.colors.textMuted` in the app — **it is not consumed
anywhere** (no screen passes `color="textMuted"`, nothing reads
`theme.colors.textMuted` directly). It's a reserved/unwired token, currently
decorative-by-absence rather than decorative-by-design. Left unchanged per
instructions since it isn't rendering as real text today; flagged here so a
future consumer doesn't wire it in without re-checking AA first.

### Contrast computations (WCAG relative-luminance formula, verified with a script, not eyeballed)

- `textTertiary` **#5C5953** on paper **#F7F6F2** → **6.45:1** (passes AA; old
  rgba value blended to ~`#82827F` on paper → **3.56:1**, failed).
- `textSecondary` / `inkSoft` **#45433E** on paper **#F7F6F2** → **9.14:1**
  (still comfortably passes after the value change).

## 2. NativeWind `ink.muted` (`tailwind.config.js`)

Changed `ink.muted: '#7A766E'` → `'#5C5953'` so the `Text` component's
`eyebrow` preset (`text-ink-muted`) and the `textTertiary`/`textMuted` color
maps (`text-ink-muted`) clear AA, and so the two token sources (theme.ts and
tailwind.config.js) agree on the same muted value.

Grepped `ink-muted` / `text-ink-muted` usages: only `components/ui/Text.tsx`
(the `eyebrow` preset and the `textTertiary` color-token map) reference the
class. Contrast: old **#7A766E** on paper **#F7F6F2** → **4.18:1** (fails AA);
new **#5C5953** → **6.45:1** (passes).

**Found but out of scope for this batch:** `app/login.tsx` (lines 112, 122)
and `components/auth/LinkAccountPrompt.tsx` (line 112) hardcode
`placeholderTextColor="#7A766E"` directly (not through a token). These are
screen files, explicitly out of scope for Batch 1 ("later batches handle
screens"). They now read as literally the *old*, now-inconsistent value.
Flagging for whichever batch touches those screens/forms — should become
`theme.colors.textTertiary` (`#5C5953`) or NativeWind `placeholder:text-ink-muted`.

## 3. `components/ui/StatusBadge.tsx`

- **`warning`** tone: `fg` changed from `theme.colors.inverse` (white/paper) to
  `theme.colors.text` (ink) on the amber `bg`. Old pairing (paper `#F7F6F2` on
  amber `#B08A30`) → **2.98:1** (fails AA). New pairing (ink `#0E0E0C` on the
  *new* amber `#B5751F`) → **5.09:1** (passes).
- **`success`** tone: changed from a solid moss fill (`bg: accent`,
  `fg: inverse`) to a tint (`bg: theme.colors.accentTint` `#E8EEE2`,
  `fg: theme.colors.accent` moss `#2D4A2B`). Contrast: moss `#2D4A2B` on tint
  `#E8EEE2` → **8.35:1**. This stops every success chip from spending the
  single loud moss accent decoratively — moss is now reserved for primary
  actions/press states, matching the "one accent per view" design rule.
- Verified every remaining tone:
  - `neutral`: inverse (paper `#F7F6F2`) on `text` (ink `#0E0E0C`) → **17.87:1**.
  - `info`: text (ink `#0E0E0C`) on `surfaceAlt` (new `#FAF8F2`) → **18.19:1**.
  - `danger`: inverse (paper `#F7F6F2`) on `danger` (new `#8B2E20`) → **7.74:1**.
  - `muted`: `textSecondary` (new `#45433E`, ~9.14:1 on either paper or
    elevated-white background since bg is transparent) on hairline border —
    passes comfortably; no fix needed.

All tones now ≥4.5:1.

### Test check

`__tests__/product-detail-sections.test.tsx` has a source-text assertion
(`"never spends moss on the header status badge — success IS moss"`, line 31)
that only checks the header screen doesn't literally use `tone="success"`. It
does not assert the `TONE.success` color values, so it is unaffected by the
tint change and still passes.

No dedicated `StatusBadge.test.tsx` exists, so no assertion needed updating.

## 4. Touch targets → 44pt

- `components/ui/SegmentedControl.tsx`: `tab.minHeight` `36` → `theme.touchTarget`
  (44). Also swapped `accessibilityRole="button"` → `accessibilityRole="tab"`
  (kept `accessibilityState={{ selected: active }}`, which is valid for the
  `tab` role) since this is a single-choice filter control. No
  `SegmentedControl.test.tsx` / usage test asserts the old role or height, so
  nothing needed updating.
- `components/ui/SearchField.tsx`: `wrapper.height` `40` → `theme.touchTarget`
  (44). No test asserts the old height.

## 5. Radii naming collision — resolved, not just documented

`theme.ts` `radii.md` = 6 vs. tailwind `rounded-md` = 10px was a real
name/value collision. Grepped every `rounded-*` usage in the app
(`app/**`, `components/**`): **zero usages of bare `rounded-md`** exist
anywhere in the codebase (only `rounded`, `rounded-t-xl`, etc. are used). Since
there is no usage to regress, aligned tailwind `md` → `6px` to match
`theme.ts` and the canonical "6px default" rule, rather than leaving a
divergent unused value. Left a `// NOTE:`-style comment in
`tailwind.config.js` documenting why.

## Gates

- `npx jest --forceExit` tail:
  ```
  Test Suites: 47 passed, 47 total
  Tests:       346 passed, 346 total
  Snapshots:   0 total
  Time:        13.07 s
  ```
  (matches baseline 346/346; the single "worker process failed to exit
  gracefully" warning is pre-existing Jest teardown noise, unrelated to this
  change — same warning class the task description anticipated.)
- `apps/mobile-admin`: `npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` → **0**
- `packages/mobile-shared`: same command → **0**

## Files changed

- `apps/mobile-admin/lib/theme.ts`
- `apps/mobile-admin/tailwind.config.js`
- `apps/mobile-admin/components/ui/StatusBadge.tsx`
- `apps/mobile-admin/components/ui/SegmentedControl.tsx`
- `apps/mobile-admin/components/ui/SearchField.tsx`
