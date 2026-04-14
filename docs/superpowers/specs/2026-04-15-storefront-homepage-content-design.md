# Storefront Homepage Content

**Date:** 2026-04-15
**Status:** Draft — pending review
**Scope:** marketplace-api, apps/admin, apps/storefront
**Depends on:** `2026-04-15-storefront-pages-cms-design.md` (footer_sections JSONB pattern, Pages table for link targets)

## Goal

Let merchants author their storefront **homepage content** — hero, text
sections, featured products, images, quotes — from the admin, independent
of which layout theme they've chosen. Switching themes should restyle
the content, not destroy it.

## Problem

Each of the 5 storefront themes (`editorial`, `minimal`, `classic-shop`,
`compact`, `hero-focus`) has different visual treatments but they all
need **the same kinds of content**: a hero, some introductory text,
featured products, maybe a quote or image.

Today the storefront home renders a hardcoded demo layout (the
`StorefrontLayoutRenderer` component). Merchants can't edit it. When
they switch themes, they see *the same placeholder content styled
differently*.

The Pages CMS (sibling spec) covers long-form static pages. Homepage
content is a different shape — structured, composable, per-theme styled.

## Proposed solution — fixed hero + ordered sections

Homepage content lives in a single JSONB column on `store_branding`:

```sql
ALTER TABLE store_branding
  ADD COLUMN homepage_content JSONB NOT NULL DEFAULT '{}'::jsonb;
```

Shape:

```json
{
  "hero": {
    "enabled": true,
    "image_url": "https://.../hero.jpg",
    "heading": "Handcrafted goods from Jaipur",
    "subheading": "Block-printed textiles and brass homeware, sourced directly from the workshop.",
    "cta_label": "Shop new arrivals",
    "cta_url": "/collections/new"
  },
  "sections": [
    { "type": "text", "markdown": "## Our story\n\nAcme was founded..." },
    { "type": "image", "url": "https://.../studio.jpg", "alt": "The studio in Jaipur", "caption": "Jaipur, 2026" },
    { "type": "featured_products", "collection_slug": "bestsellers", "limit": 8, "heading": "Customer favourites" },
    { "type": "quote", "text": "Beautiful craft, beautifully shipped.", "attribution": "Vogue India, 2026" }
  ]
}
```

**Why fixed hero + sections, not pure blocks:** every theme has a top
area that functions as a hero. Making it a first-class field keeps the
common case simple. Everything beneath the hero is variable → that's
what sections are for.

**Why v1 ships only 4 section types** (`text`, `image`, `featured_products`,
`quote`): covers 90% of what merchants author on day one. New types
(image_grid, video_embed, collection_spotlight, testimonials carousel)
are additive — each new type = one React component + one admin form
partial, no schema change.

**Why JSONB on `store_branding` rather than a new table:** sections are
always read and written as a whole. A relational design would force joins
on every homepage render. Matches the footer_sections precedent exactly.

## Theme-agnostic rendering

Each theme (layout component) consumes the **same content shape** and
styles it in its own way:

| Theme | Hero treatment | Section rhythm | Products rail |
|---|---|---|---|
| `editorial` | Large cover + serif overlay | Airy, magazine-style, asymmetric | Editorial grid with article-style intro |
| `minimal` | Smaller hero, centered | Tight, symmetric, generous whitespace | Dense grid, minimal chrome |
| `classic-shop` | Slim header strip | Products-first, sections beneath | Prominent product grid top |
| `compact` | Sidebar hero panel | Sections in main column, sidebar nav | 6-col grid, compact cards |
| `hero-focus` | Full-viewport hero with CTA | Sections scroll beneath | Standard grid after hero |

The theme component contract becomes:

```ts
interface LayoutProps {
  store: PublicStore;
  theme: StorefrontTheme;
  content: HomepageContent | null;  // null → render sensible defaults
}
```

Each layout component reads `content.hero` and `content.sections` and
renders its own styled version. No new theme renderer is "blocks-aware"
in the OS 2.0 Sections sense — themes always know the block vocabulary
at compile time (4 types today). Adding a new type means adding a
`switch` case in each theme that cares.

**Default fallback** when `content` is `{}` or missing: show a minimal
hero using `store.name` as the heading + a product rail pulling from the
first published collection. Every new store has a working homepage on
day one without the merchant touching the admin.

## API surface

No new endpoints. `homepage_content` is a passthrough field on the
existing `PATCH /api/v1/branding` admin endpoint and the existing
`GET /api/v1/storefront/stores/:slug/branding` public endpoint.

Validation on the backend enforces the JSON shape:
- `hero.enabled: boolean`, all other hero fields optional strings ≤ reasonable caps
- `sections` is an array, each element has a known `type`, max 12 sections
- Per-type validation lives in a small Go dispatcher

## Admin UX

New "Homepage" tab in `BrandingSettingsClient` (next to Identity / Colors
/ Typography / Layout / Footer / Advanced).

### Hero editor (top of the tab)

- Toggle: show hero Y/N
- Image upload (reuses the existing logo/favicon upload path if the
  branding module already has file upload; else link to an already-hosted
  URL for v1)
- Heading + subheading text fields
- CTA label + CTA URL (URL field includes a page-picker dropdown from
  `listPages` like the footer editor, plus a free-text URL fallback)

### Sections editor (beneath)

- "+ Add section" button → dropdown: `Text`, `Image`, `Featured products`,
  `Quote`
- Each section renders as a card:
  - Type-specific inline form (text → markdown textarea with preview,
    image → URL + alt + caption, featured_products → collection picker +
    limit + heading, quote → text + attribution)
  - Up/down arrows (drag-reorder deferred to a follow-up)
  - Remove button
- Save posts the whole `homepage_content` object atomically.

### Live preview (v2, not v1)

Not in scope for v1. Admin saves → merchant opens storefront in a new
tab to verify. A proper split-view preview using the storefront's own
components is a future enhancement.

## Interaction with other systems

- **Pages CMS**: CTA URL + hero links can use the same `listPages` picker
  the footer editor uses. Reuses the Pages admin API.
- **Storefront theme**: `normalizeStorefrontTheme` is already applied at
  the layout root; layout components get both `theme` and `content`
  props. Existing theme tokens drive visual style; `content` drives what
  the merchant wrote.
- **Collections**: `featured_products` references collection by slug —
  existing collection APIs resolve products at render time. If the
  collection is deleted the storefront shows an empty state for that
  section; admin shows a friendly "Collection missing" warning.

## Phased rollout

### Phase A — Schema + passthrough backend
- Migration: `store_branding.homepage_content` JSONB column
- Backend validator for the hero + sections shape
- No UI yet — branding PATCH accepts the new field

### Phase B — Storefront default rendering
- Update `StorefrontLayoutRenderer` to accept `content` prop
- Each layout component (`EditorialLayout`, `MinimalLayout`,
  `ClassicShopLayout`, `CompactLayout`, `HeroFocusLayout`) reads
  `content.hero` + `content.sections` and renders them in its own style
- Shared section renderers in `packages/ui` or `apps/storefront/components/sections/`
- Fallback default when content is empty

### Phase C — Admin Homepage tab
- Hero editor (toggle + image + text fields + CTA)
- Sections editor (4 types, up/down reorder, add/remove)
- Wire into `BrandingSettingsClient` as a new tab

### Phase D — Polish
- Collection picker uses existing collection API
- Markdown safety parity with Pages CMS (`skipHtml`)
- Empty-state copy in admin
- Smoke test all 5 themes with the same content

## Scope cuts for v1

- **Only 4 section types** (`text`, `image`, `featured_products`, `quote`).
  Image grid, video embed, collection spotlight, testimonial carousel →
  v2.
- **No drag-reorder.** Up/down arrows only.
- **No in-admin preview pane.** Save, open storefront in new tab.
- **No A/B content versions**, **no scheduled publish**, **no per-locale
  variants**.
- **Image upload reuses Pages/branding media flow** — no new upload UI
  unless the existing path can't be reused as-is.
- **CTA and section heading fields are plain text only.** No markdown
  inside heading copy.
- **Default fallback content is baked in, not admin-configurable.** A
  new store shows the store name + first collection until the merchant
  edits.

## Risks

- **Theme switch data loss**: zero. Content is theme-agnostic; switching
  theme just changes rendering. Explicit goal.
- **Section type drift**: if we add new types in v2, old theme components
  will fall through their `switch` to a default. Keep the dispatcher
  forgiving — unknown types render nothing (logged, not thrown).
- **XSS via text sections**: markdown rendered with `skipHtml` like the
  Pages CMS. Reuse the `Markdown` helper from Phase B of the Pages plan.
- **Collection delete orphan**: `featured_products` referencing a deleted
  collection renders an empty section; admin surfaces a warning on the
  section card.
- **JSONB size ceiling**: 12-section cap keeps the row small (rough
  bound: 64KB). Server enforces; admin shows a friendly error.

## Success criteria

- A merchant opens the Homepage tab, enables the hero, uploads an image,
  writes a heading + CTA, adds a Text section and a Featured products
  section, hits Save.
- The live storefront renders exactly those elements in the merchant's
  chosen theme.
- Switching from `editorial` → `minimal` in the Themes tab re-styles the
  same content without losing a single field.
- A new store with no homepage_content still has a working homepage
  (default hero + a collection rail).
