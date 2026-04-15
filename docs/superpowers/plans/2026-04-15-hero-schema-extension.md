# Hero Schema Extension — Task List

**Spec:** `docs/superpowers/specs/2026-04-15-hero-schema-extension-design.md`
**Goal:** add 5 optional fields to `HomepageHero` (eyebrow, secondary CTA, aside image) and wire them through validator → admin editor → per-theme storefront rendering → all 8 default recipes.

## Shared types (backend Go + both frontends TS)

- `eyebrow: string` — max 80 chars
- `cta_secondary_label: string` — max 60 chars
- `cta_secondary_url: string` — max 500 chars, must pass `IsSafeURL`
- `aside_image_url: string` — max 500 chars, must pass `IsSafeURL`
- `aside_image_alt: string` — max 200 chars, required when `aside_image_url` is set

## Tasks

### Phase A — Backend validator

- [ ] **Task 1**: Extend `heroInput` in `services/marketplace-api/internal/branding/homepage_content.go` with the 5 new optional fields.
- [ ] **Task 2**: Add caps to the const block (`maxHeroEyebrow = 80`, `maxHeroSecondaryCtaLabel = 60`, `maxHeroSecondaryCtaURL = 500`, `maxHeroAsideImageURL = 500`, `maxHeroAsideImageAlt = 200`).
- [ ] **Task 3**: Extend `validateHero()` to check:
  - each new field's length cap
  - `cta_secondary_url` via `IsSafeURL` when non-empty
  - `aside_image_url` via `IsSafeURL` when non-empty
  - `cta_secondary_label` and `cta_secondary_url` set together or both empty (paired rule)
  - `aside_image_alt` **required** when `aside_image_url` set (return `hero.aside_image_alt: required when aside_image_url is set`)
- [ ] **Task 4**: Update `services/marketplace-api/internal/branding/homepage_content_test.go` with 6 new test cases covering: each new field alone, paired CTA rule, alt-required rule, oversize caps, `javascript:` URL rejection on secondary CTA, `javascript:` rejection on aside image.
- [ ] **Task 5**: Commit — `feat(marketplace-api): extend hero validator with eyebrow + secondary CTA + aside image`.

### Phase B — Storefront types + shared renderer

- [ ] **Task 6**: Update `HomepageHero` type in `apps/storefront/lib/api/marketplace-api.ts` with the 5 new fields (all optional string | null).
- [ ] **Task 7**: Update `HeroSection.tsx` in `apps/storefront/components/homepage/HeroSection.tsx`:
  - Accept the new fields from `hero` prop
  - Call a new `heroStylesFor(theme.layout)` helper from `apps/storefront/lib/themeBlockStyles.ts` that returns `{ eyebrow, container, titleSize, asideSlot, secondaryCta }` classes per layout
  - Render `eyebrow` above heading when set
  - Render `aside_image_url` as a framed `<Image>` in the aside slot (with `aside_image_alt`) when set AND `heroStylesFor` returns a non-null `asideSlot`
  - Render secondary CTA (label + url) when set — use `safeUrl()` wrapper on the href
- [ ] **Task 8**: Add `heroStylesFor()` to `apps/storefront/lib/themeBlockStyles.ts` with a `switch` on `StorefrontLayout`. Include per-layout styles for editorial / bold_promo / split_hero / story_led / minimal / classic_shop / catalog_first / compact. Default case covers any future layout.
- [ ] **Task 9**: Commit — `feat(storefront): hero renderer supports eyebrow + secondary CTA + aside image per theme`.

### Phase C — Per-theme placeholder images

- [ ] **Task 10**: Add small placeholder images to `apps/storefront/public/layout-placeholders/` (one per layout that uses an aside slot: editorial, split_hero, story_led — 3 files). Use warm/neutral-toned generic studio imagery or plain color with brand wordmark.
- [ ] **Task 11**: Commit — `chore(storefront): add layout-placeholder images`.

### Phase D — Default recipes

Update each `*.defaults.ts` file's `defaultHero()` to include new fields matching the pre-refactor hardcoded content. Files live in `apps/storefront/components/layouts/`.

- [ ] **Task 12**: `EditorialLayout.defaults.ts` — add `eyebrow: "Issue 01 · Now open"`, `cta_secondary_label: "Read the story"`, `cta_secondary_url: "/pages/about"`, `aside_image_url: "/layout-placeholders/editorial-cover.jpg"`, `aside_image_alt: "{store.name} cover story"`.
- [ ] **Task 13**: `BoldPromoLayout.defaults.ts` — add `eyebrow: "The drop · Live now"`, `cta_secondary_label: "Lookbook"`, `cta_secondary_url: "/pages/lookbook"`.
- [ ] **Task 14**: `SplitHeroLayout.defaults.ts` — add `eyebrow: "Featured"`, `aside_image_url: "/layout-placeholders/split-aside.jpg"`, `aside_image_alt: "{store.name} featured"`.
- [ ] **Task 15**: `StoryLedLayout.defaults.ts` — add `eyebrow: "From the studio"`, `aside_image_url: "/layout-placeholders/story-aside.jpg"`, `aside_image_alt: "Studio workshop"`.
- [ ] **Task 16**: `MinimalLayout.defaults.ts`, `ClassicShopLayout.defaults.ts`, `CatalogFirstLayout.defaults.ts`, `CompactLayout.defaults.ts` — add `eyebrow` only (appropriate short phrase per theme), leave other new fields null/undefined.
- [ ] **Task 17**: Commit — `feat(storefront): recipes populate new hero fields per theme`.

### Phase E — Admin HeroEditor form

- [ ] **Task 18**: Update `HomepageHero` type in `apps/admin/lib/api/marketplace-api.ts` with the 5 new fields (mirror storefront).
- [ ] **Task 19**: Update `apps/admin/components/settings/HeroEditor.tsx`:
  - Add "Eyebrow" text input (maxLength 80) below Heading field
  - Add "Secondary CTA" collapsible/section with label + URL inputs (URL reuses page-picker + freeform pattern from CTA URL)
  - Add "Aside image" collapsible/section with URL + alt inputs; alt gets `required` attr and a red asterisk when URL is non-empty
  - Client-side validation: if `aside_image_url` set and `aside_image_alt` empty, disable Save and show inline error
  - Below the form, add a theme hint: fetch `currentLayout` from the Theme tab's branding state and display "Your theme (`{layout}`) uses: eyebrow ✓, secondary CTA ✓, aside image ✗" — map `heroStylesFor()`-equivalent truth table client-side as a hardcoded record (keeps admin and storefront aligned)
- [ ] **Task 20**: Update `HomepageTab.helpers.ts` starter recipes to include new hero fields (mirror storefront recipes from Phase D).
- [ ] **Task 21**: Commit — `feat(admin): HeroEditor supports eyebrow + secondary CTA + aside image with a11y validation`.

### Phase F — Tests + smoke

- [ ] **Task 22**: Extend `apps/admin/tests/` (unit or e2e as applicable) with test asserting the HeroEditor enforces alt-required when aside URL is set.
- [ ] **Task 23**: Extend `apps/storefront/tests/visual-diff/` (or equivalent) with a snapshot of each of the 8 layouts using its default recipe — compare against a pre-committed golden image baseline.
- [ ] **Task 24**: Manual QA walkthrough: (a) open /settings/themes → Homepage → verify all new HeroEditor fields appear, (b) save eyebrow + secondary CTA + aside image + alt on a test tenant, (c) check storefront renders them correctly for current theme, (d) switch theme to one that ignores the aside image and verify it's still stored (switch back and the image returns).
- [ ] **Task 25**: Commit — `test: hero schema extension tests + visual baselines`.

### Phase G — Deploy

- [ ] **Task 26**: `go build ./...` (marketplace-api) + `npx tsc --noEmit` (both admin + storefront) — expect clean.
- [ ] **Task 27**: Push to main, watch CI (flip repo public first if billing cache triggers).
- [ ] **Task 28**: Merge auto-opened tesserix-k8s PR.
- [ ] **Task 29**: Flip repo back to private.
- [ ] **Task 30**: Trigger ArgoCD sync on `mark8ly-admin`, `mark8ly-storefront`, `mark8ly-marketplace-api-admin`, `mark8ly-marketplace-api-storefront`.
- [ ] **Task 31**: Watch rollouts until all 4 deployments report `successfully rolled out`.

## Done criteria

- All 8 layouts with empty `homepage_content` render visually identical to pre-refactor (spec success criterion #2 fully met).
- A merchant can edit all 5 new hero fields in admin, see them render per theme, and switch themes without losing any field.
- XSS tests still pass (`javascript:` URLs rejected on both new URL fields; alt attribute always present when aside image URL is set).
- Zero regression in any of the prior Homepage Content / Pages CMS tests.
