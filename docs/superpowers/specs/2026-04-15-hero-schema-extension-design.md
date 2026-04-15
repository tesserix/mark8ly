# Hero Schema Extension

**Date:** 2026-04-15
**Status:** Draft — pending review
**Scope:** marketplace-api, apps/admin, apps/storefront
**Depends on:** `2026-04-15-storefront-layout-blocks-refactor-design.md` (shipped)

## Goal

Restore per-theme hero chrome (eyebrow label, secondary CTA, aside image)
that was simplified during the pure-blocks refactor, without re-introducing
hardcoded content. All additions remain merchant-authored via the
`homepage_content.hero` object.

## Problem

The blocks refactor flattened every layout's hero into `HeroSection`, which
renders just `heading / subheading / primary CTA / background image`. The
previous hardcoded layouts had richer chrome:

- **Editorial**: `"Issue 01 · Now open"` eyebrow + asymmetric 7/5 grid with a
  cover-story `ProductSlot` aside + a `"Read the story"` secondary link
- **BoldPromo**: `"The drop · Live now"` eyebrow + a `"Lookbook"` secondary CTA
- **StoryLed**: tagline-style eyebrow + paired image on the right
- **SplitHero**: split-column hero that needs a right-side image slot
- Other layouts: used the eyebrow slot implicitly via `<Eyebrow>` primitive

With only 5 fields on `hero`, every merchant ends up with the same basic
treatment regardless of theme. Spec success criterion #2 ("empty content
renders visually identical to today") is only partially met.

## Proposed solution — 5 new optional hero fields

Extend `HomepageHero` with optional fields; existing fields stay unchanged.
Themes consume what they understand, ignore what they don't. Merchants who
don't use the new fields see no change.

### New fields

| Field | Type | Purpose | Cap |
|---|---|---|---|
| `eyebrow` | `string?` | Small uppercase label above the heading | 80 chars |
| `cta_secondary_label` | `string?` | Label for a secondary CTA link | 60 chars |
| `cta_secondary_url` | `string?` | URL for the secondary CTA (IsSafeURL) | 500 chars |
| `aside_image_url` | `string?` | Optional right-side / adjacent image URL | 500 chars |
| `aside_image_alt` | `string?` | Alt text (required when `aside_image_url` set) | 200 chars |

### Validation rules
- `eyebrow`: ≤ 80 chars, optional
- `cta_secondary_label` + `cta_secondary_url` must be set together or both empty
- `cta_secondary_url` must pass `IsSafeURL` (same policy as `cta_url`)
- `aside_image_url` must pass `IsSafeURL`
- `aside_image_alt` is **required** when `aside_image_url` is set (a11y)
- Size cap on the hero object: no change (already enforced by overall JSONB cap)

### Why not a `variant` field?

Each theme's `HeroSection` already branches on `theme.layout` for positioning
and typography. Adding a `variant` field would move that decision to the
merchant, who shouldn't need to know "full-bleed" vs "split" vs "compact" —
that's theme-designer territory. Keep layout decisions on the theme side.

### Why not `stats[]` for BoldPromo drop stats?

Deferred. Only one layout (BoldPromo) had stats. Merchants can emulate with
a `text` section below the hero containing a small grid. If merchants
complain, we add `stats` as a dedicated field or a new `stat_grid` block
type later.

## Per-theme rendering

Each layout's hero rendering consumes the new fields in its own way:

| Layout | eyebrow | cta_secondary | aside_image | Notes |
|---|---|---|---|---|
| `editorial` | small caps eyebrow above heading | quieter text-link style to right of primary | right column in asymmetric grid | The classic editorial treatment |
| `bold_promo` | small bright eyebrow | outline-button beside primary | NOT used (full-bleed hero) | Drop-style |
| `split_hero` | uppercase eyebrow | text link below primary | right column of split grid | Required for the split aesthetic |
| `story_led` | tagline above title | inline "Read the story" link | right-side framed image | Editorial-adjacent |
| `minimal` | optional small eyebrow | small secondary link | ignored (minimal has no aside slot) | Keeps the restrained look |
| `classic_shop` | small eyebrow | single secondary link | ignored | Simple shop header |
| `catalog_first` | minor eyebrow | not prominent | ignored (catalog is the focus) | Rail-first treatment |
| `compact` | eyebrow used as sidebar label | ignored (no room) | ignored | Sidebar treatment |

Each theme's `HeroSection` variant — or the shared `HeroSection` with a
`theme.layout` branch — renders only the fields it uses. Unused fields are
preserved in `homepage_content` so switching themes doesn't lose them.

## API surface

No new endpoints. `homepage_content.hero` passes through the existing
branding PATCH with shape validation.

## Admin UX

Extend `HeroEditor` in `apps/admin/components/settings/HeroEditor.tsx` with:

- New "Eyebrow" text field below Heading
- New "Secondary CTA" group (label + URL with page-picker) below primary CTA
- New "Aside image" group (image URL + alt) below CTA group
- Alt field becomes required (red asterisk) when URL field is non-empty;
  client-side validation prevents save until alt is filled in
- Per-theme hint under the form: "Your current theme (`{layout}`) uses:
  eyebrow ✓, secondary CTA ✓, aside image ✗" — tells merchants which fields
  affect their current rendering

## Recipes

All 8 `*.defaults.ts` recipe files update their `defaultHero()` returns to
include the new fields matching the pre-refactor hardcoded content:

- `Editorial`: `eyebrow = "Issue 01 · Now open"`, `cta_secondary = { label: "Read the story", url: "/pages/about" }`, `aside_image_url = "/placeholder-cover.jpg"` (or null — use first featured product image at render time?)
- `BoldPromo`: `eyebrow = "The drop · Live now"`, `cta_secondary = { label: "Lookbook", url: "/pages/lookbook" }`
- `SplitHero`: `eyebrow = "Featured"`, `aside_image_url = "/placeholder-split.jpg"` (or null)
- `StoryLed`: `eyebrow = "From the studio"`, `aside_image_url = "/placeholder-story.jpg"`
- Other layouts keep their simpler hero recipes unchanged

Note: placeholder image URLs need to resolve. Options:
1. Ship a small set of placeholder images in the storefront's `public/` dir,
   reference by relative path in recipes
2. Recipes return `aside_image_url = null` and the storefront renderer
   substitutes `store.featured_products[0]?.image_url` at render time
3. Pull from the store's first published product image server-side in the
   layout

**Recommendation:** ship placeholder images in `public/layout-placeholders/`
(recipe returns the relative path). Merchants replace via the admin when
they save their first real image. This keeps recipes pure functions.

## Migration

Zero migration. New fields are optional additive. Existing stores see no
change.

## Success criteria

- Empty `homepage_content` renders visually identical to pre-refactor for
  all 8 layouts (spec criterion #2 now fully met).
- A merchant can set eyebrow, secondary CTA, and aside image in the admin
  and see them render appropriately per theme.
- Alt text is enforced when aside image URL is set (a11y).
- Switching themes preserves all 5 new fields silently, even if the new
  theme ignores some of them.
- `cta_secondary_url` and `aside_image_url` both enforce `IsSafeURL`.

## Scope cuts for v1

- **No `stats[]` field** (BoldPromo's drop stats) — merchants use a `text`
  section below hero if they want callouts.
- **No `variant` field** — theme decides layout, merchant decides content.
- **No image upload UI** — merchants paste URLs for v1 (matches existing hero
  image flow). Upload is a separate initiative.
- **No per-field hint badges** beyond the single "fields your theme uses"
  hint — don't litter the form.

## Risks

- **a11y regression** if alt enforcement isn't client-side too — merchant
  could paste URL, skip alt, get a 400 from backend on save. Fix with
  client-side required validation.
- **Placeholder image 404** if images aren't shipped — verify in tests.
- **Schema drift from admin form vs backend validator** — same risk as
  every other field. Keep caps in one place (`internal/branding/homepage_content.go`
  constants) and mirror in admin form maxLength attrs; document "update
  both" comment (already the pattern).

## Size estimate

- Backend validator: +5 fields + 3 URL checks + alt-required rule = ~0.5 day
- Admin HeroEditor form: +3 field groups + a11y validation + theme hint = ~0.5 day
- Storefront HeroSection per-theme styling: ~1 day (8 layouts each need a
  small branch)
- Recipe updates + placeholder images: ~0.5 day
- Tests + smoke test walk all 8 layouts: ~0.5 day

**Total: ~3 days.** Fits as a small follow-up to the pure-blocks refactor.
