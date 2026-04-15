# Storefront Layout Blocks Refactor

**Date:** 2026-04-15
**Status:** Draft — pending review
**Scope:** apps/storefront, apps/admin, marketplace-api (HomepageContent validator only)
**Depends on:**
- `2026-04-15-storefront-pages-cms-design.md` (shipped)
- `2026-04-15-storefront-homepage-content-design.md` (shipped)

## Goal

Remove every hardcoded content element from storefront layouts so the
merchant owns 100% of what renders on their homepage. Keep the out-of-box
experience identical to today for merchants who haven't authored
anything yet, via per-theme default content recipes.

## Problem

After the Homepage Content v1 ship, the merchant can edit the hero and
append sections, but each layout still carries a block of hardcoded
editorial chrome that ignores merchant input. Specifically in
`EditorialLayout.tsx` today:

- Marquee terms `"Hand picked · Small batch · Ships worldwide · SLUG · Est. 2026"`
- Pull quote `"We chose fewer things and we chose them well."` with
  `"Editor's note · {store.name}"` attribution
- "The edit" section label + `"Three pieces we love right now"` + N° 01-03
  placeholder cards
- "Letter from the studio" block with hardcoded eyebrow / title / body / CTA

All six other opinionated layouts (`CatalogFirst`, `SplitHero`, `StoryLed`,
`BoldPromo`, `Compact`, `ClassicShop`) carry similar baked-in decorations.
Merchants have no way to change, remove, or reorder them.

The current architecture puts *content* in layout React components. We
want layouts to hold only *styling* — content always lives in
`homepage_content.sections`.

## Proposed solution

### Pure blocks, soft landing

Every layout becomes:

```tsx
export function EditorialLayout({ store, theme, content }: LayoutProps) {
  const sections = content?.sections?.length
    ? content.sections
    : defaultEditorialSections(store);

  return (
    <article className="space-y-20">
      <HeroSection hero={content?.hero} theme={theme} fallbackHeading={store.name} />
      <SectionsRenderer sections={sections} theme={theme} storeSlug={store.slug} />
    </article>
  );
}
```

No other content inside. Layout's job is spacing, asymmetric grids,
marquee animation CSS, theme-specific block styling — none of the
merchant-facing copy.

**Soft landing:** when `content?.sections` is empty, render the theme's
idiomatic default recipe. The recipe produces the *same content as
today's hardcoded chrome* — merchants on a fresh store see no visual
difference. The moment they add even one section, the defaults stop
applying entirely. Merchants never see a mixed state.

### Admin: "Start from theme defaults" action

When a merchant opens the Homepage tab and `sections` is empty, show:

- A badge near the sections list: `Using theme defaults`
- A button: `Start from theme defaults` — clicking copies the recipe
  into the form state (not saved yet). The merchant now edits real
  content, can delete sections they don't want, reorder, etc.
- A "Reset to theme defaults" action in the overflow menu (after
  authoring) that clears sections and returns to the default state.

## New section types

Three new block types to cover today's hardcoded content without losing
expressiveness:

| Type | Fields | Used by | Notes |
|---|---|---|---|
| `marquee` | `items: string[]` (max 8), `speed?: "slow"/"normal"/"fast"` | Editorial, SplitHero | Scrolling ticker, theme styles animation |
| `pull_quote` | `text: string`, `attribution?: string` | Editorial, StoryLed | Large quote, theme decides typography |
| `letter` | `eyebrow?: string`, `title: string`, `body: string` (markdown), `cta_label?: string`, `cta_url?: string` | Editorial, StoryLed | "Letter from the studio"–style block |

Each renders through `SectionsRenderer` like existing blocks. Per-theme
styling lives inside each block's renderer component (branches on
`theme.layout` for positioning and type treatment).

## Extended `featured_products`

Current shape supports only `collection_slug + limit`. Add:

- `product_slugs?: string[]` — explicit hand-picked products (max 6).
  When present, `collection_slug` is ignored.

Storefront resolves: if `product_slugs` has items, fetch those specific
products by slug. If fewer than the requested number resolve (deleted
products, renamed slugs), render only what resolves without padding.
Fallback order: `product_slugs` → `collection_slug` → empty section.

Covers editorial layout's "N° 01/02/03" curated picks without requiring
merchants to create a throwaway collection.

## Per-theme default recipes

Each layout ships a `defaults.ts` module co-located with the layout
component:

```ts
// apps/storefront/components/layouts/EditorialLayout.defaults.ts
import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

export function defaultEditorialSections(store: PublicStore): HomepageSection[] {
  return [
    {
      type: "marquee",
      items: ["Hand picked", "Small batch", "Ships worldwide", store.slug.toUpperCase(), "Est. 2026"],
    },
    {
      type: "pull_quote",
      text: "We chose fewer things and we chose them well.",
      attribution: `Editor's note · ${store.name}`,
    },
    {
      type: "featured_products",
      heading: "Three pieces we love right now",
      limit: 3,
      // no product_slugs → falls back to first-3-products-in-catalog
    },
    {
      type: "letter",
      eyebrow: "Letter from the studio",
      title: "Built for the long shelf life.",
      body: `Every piece in ${store.name} is chosen to outlast a season. Paper packaging, repairable construction, and a quiet attitude toward trend cycles.`,
      cta_label: "About the studio",
      cta_url: "/pages/about",
    },
  ];
}
```

**Why co-located:** when someone updates a layout's visual design, the
recipe is right there so they update both together. Prevents drift
between the layout's styling and its default content.

**Recipes are pure functions** of the `store` — same input → same
output. No DB reads, no randomness, no branching on time. Safe to render
server-side on every request, always produces the same markup.

## Theme-aware block styling

Each section renderer branches on `theme.layout` to apply theme-specific
styling:

```tsx
// apps/storefront/components/homepage/sections/PullQuoteSection.tsx
export function PullQuoteSection({ section, theme }: Props) {
  const styles = pullQuoteStylesFor(theme.layout);
  return (
    <blockquote className={styles.container}>
      <p className={styles.text}>{section.text}</p>
      {section.attribution && <cite className={styles.attribution}>{section.attribution}</cite>}
    </blockquote>
  );
}

function pullQuoteStylesFor(layout: StorefrontLayout): PullQuoteStyles {
  switch (layout) {
    case "editorial":
      return { container: "col-span-4", text: "font-serif text-3xl leading-tight", attribution: "mt-4 text-sm uppercase tracking-[0.16em]" };
    case "minimal":
      return { container: "mx-auto max-w-2xl text-center", text: "text-xl leading-relaxed", attribution: "mt-3 text-xs text-foreground-secondary" };
    // ...
    default:
      return { container: "", text: "text-lg", attribution: "text-sm" };
  }
}
```

**Default case is always present** — covers any layout we haven't
opinionated for yet. Ensures a block always renders somehow, even on
a new theme that hasn't been style-profiled.

## Admin additions

### 3 new per-section forms
- `MarqueeSectionForm` — items editor (chips-style input, max 8 items,
  drag reorder within the section)
- `PullQuoteSectionForm` — text textarea + attribution input
- `LetterSectionForm` — eyebrow + title + body (markdown with preview) +
  CTA label + CTA URL (page picker + freeform)

### Product picker for `featured_products`
Existing form gains a "Picks" mode toggle:
- **By collection** (current): collection_slug dropdown + limit
- **Hand-picked**: product_slugs multi-select search (max 6, shows product title + thumbnail)

Both modes write the same section type; storefront resolves based on
which field is populated.

### Recipe action in HomepageTab
- When `sections` is empty: show "Start from theme defaults" button
- After authoring: overflow menu includes "Reset to theme defaults"
  (confirmation dialog: "This will discard your authored sections and
  revert to the {layout} theme defaults. Continue?")

## Migration path

Zero data migration required. Existing stores are in one of two states:

1. **`homepage_content = {}` or sections empty** → defaults kick in
   automatically on first render. No merchant-visible change.
2. **`homepage_content.sections` populated** (unlikely in early adopters) →
   merchant's content renders as-is, including any of the new block
   types they happened to add via API.

Stores migrated today keep working. Merchants who customize after this
ships get the new block types; merchants who don't see no change.

## Phased rollout

### Phase A — Backend: extend validator
- `homepage_content.go` validator accepts new section types: `marquee`,
  `pull_quote`, `letter`, and `product_slugs` on `featured_products`.
- Unit tests cover each new type + caps.

### Phase B — Storefront section renderers
- New components: `MarqueeSection`, `PullQuoteSection`, `LetterSection`.
- Extend `FeaturedProductsSection` to honour `product_slugs`.
- Add per-theme styling helpers for each block.
- `SectionsRenderer` dispatcher gains the 3 new cases.

### Phase C — Default recipes
- `EditorialLayout.defaults.ts` + co-located recipes for every other
  opinionated layout that currently has hardcoded content
  (`CatalogFirst`, `SplitHero`, `StoryLed`, `BoldPromo`, `Compact`,
  `ClassicShop`).
- Layouts removed of all hardcoded chrome, consuming recipes when
  `sections` is empty.

### Phase D — Admin forms
- 3 new per-section forms.
- `featured_products` picker mode toggle.
- "Start from theme defaults" button + "Reset to theme defaults"
  overflow action.
- Toasts on all state transitions.

### Phase E — Polish
- Product slug resolution: when a `product_slugs` entry is missing
  (deleted/renamed), log a warn on the admin side (small badge on the
  section card) and drop from render.
- URL scheme validation on `letter.cta_url` (reuse `IsSafeURL`).
- XSS parity: `letter.body` markdown renders with `skipHtml`; marquee
  items render as text only (no HTML).
- Smoke test: walk every layout with (a) empty content, (b) custom
  content, (c) mixed defaults then merchant edits — verify no visual
  regressions.

## Scope cuts for v1

- **Only 3 new block types** (marquee, pull_quote, letter). If we later
  discover themes need others (carousel, testimonial grid, video embed),
  add them one at a time.
- **No block-level theme override** — a merchant can't say "I want this
  pull quote to use Minimal's centered style in my Editorial layout."
  Blocks follow the current theme, always.
- **No drag-reorder** — up/down arrows only (matches Homepage Content v1).
- **No recipe versioning / history** — changing a recipe changes what
  new merchants see; existing merchants with authored content are
  unaffected.
- **Product-picker search scope** — current store only (no cross-store
  product references).

## Risks

- **Recipe drift**: updating a layout's visual design without updating
  its recipe. Mitigation: co-locate recipe file next to layout; require
  PR reviewers to verify both change together when visual design
  changes.
- **New-theme onboarding**: adding a new theme requires writing a
  recipe. Default fallback (empty recipe → `[]` sections) means a
  merchant without authored content sees only the hero on a new
  theme. Acceptable because new themes should always ship with a
  recipe.
- **Product slug drift**: a merchant hand-picks products, later renames
  one, homepage silently drops that card. Admin badge surfaces the
  issue so they can re-pick. No merchant-data loss.
- **Theme switch aesthetic mismatch**: a merchant authors sections in
  Editorial, switches to Minimal. Their marquee renders using Minimal's
  interpretation of marquee (still a marquee, but styled quieter).
  Expected per the "zero data loss" goal; if merchants complain, we
  revisit with optional per-block theme hints later.
- **Admin UX weight**: 7 block types × their forms is a lot. Mitigation:
  dropdown on "+ Add section" groups related types (Content: text,
  image, quote; Featured: featured_products; Editorial: marquee, pull_quote,
  letter). Makes choice less overwhelming.
- **Performance**: featured_products with `product_slugs` makes N
  individual product fetches vs one collection fetch. Batch them with a
  new `fetchProductsBySlugs(storeSlug, slugs[])` endpoint on
  marketplace-api; avoid N+1.

## Success criteria

- Zero hardcoded merchant-facing content remains in any layout component
  under `apps/storefront/components/layouts/`.
- A merchant with empty `homepage_content` sees visually identical output
  to today (theme recipes produce matching rendering).
- A merchant can "Start from theme defaults" and get an editable form
  populated with the recipe; edits/saves work end-to-end.
- A merchant can add any of the 7 block types (4 existing + 3 new),
  reorder, remove, save, and see results on the storefront within
  seconds.
- Switching themes never loses content; each block renders in the new
  theme's style.
- Hand-picked `featured_products` resolve without N+1 (single batched
  call).
- XSS tests still pass: `<script>` in `letter.body` and `pull_quote.text`
  renders inert; `letter.cta_url = "javascript:…"` rejected at API.

## Implementation size estimate

- Backend: validator extension — ~0.5 day
- Storefront renderers + theme styling helpers — ~1 day
- 7 default recipes (one per opinionated layout) — ~0.5 day
- Admin forms + picker mode toggle + defaults action — ~1 day
- Polish + tests + smoke test walk-through — ~0.5 day

**Total: ~3-4 engineering days.** Reasonable as a follow-up milestone
once the current Pages CMS + Homepage Content v1 lands in prod.
