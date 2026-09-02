---
type: quick
slug: homepage-lead-with-offer
branch: feat/homepage-lead-with-offer
---

# Quick Plan: Homepage leads with the ninety-day offer

## Objective

The ninety-day free trial (vs Shopify's 3 days, Wix/Squarespace's 14) is
Mark8ly's strongest differentiator, but on the homepage it is tertiary fine
print *below* the CTAs, and the page `<title>` is brand-led with no offer at
all. Promote the offer to a hero deck line and front-load it in the metadata,
without touching the `<h1>` or the design language.

## Context

- `apps/onboarding/app/page.tsx` — `Hero()` (~:305-343), page `metadata` (~:35-47)
- `mark8ly/.impeccable.md` — paper/ink/moss, serif for display, calm voice,
  no urgency or scarcity, asymmetric/left-aligned.

## Tasks

### Task 1 — Hero deck line (type: auto)

- Keep the serif `<h1>` byte-identical ("A storefront / worth opening.").
- Insert between `<h1>` and the descriptive `<p>`:
  "Ninety days free, no card. And we never take a cut of what you sell."
- Style with classes already used in the file only: `mt-6 max-w-xl text-xl
  leading-[1.2] text-foreground`. Sans, not serif — the serif stays reserved
  for the `<h1>` so the deck supports rather than competes.
- Trim the tertiary line at :335 to pricing detail only:
  "Three clear plans after that, from $15 a month, billed yearly."
- Update the Hero header comment, which claims "Headline carries the offer" —
  it no longer will; the deck does.

Verify: offer appears exactly once above the fold, no new tokens introduced.

### Task 2 — Homepage metadata (type: auto)

- `title.absolute`: offer-led, brand present, ≤ 60 chars.
- `description`: ninety days + 0% fees first, ≤ 155 chars.
- Every claim must already be committed elsewhere in the repo
  (`app/terms` ninety days free/no card, homepage FAQ 0% transaction fees,
  Pricing $15/mo billed yearly). No migration claims — the importer is CSV.

Verify: character counts measured, not estimated.

### Task 3 — Typecheck + commit (type: auto)

`npm run check-types -w @mark8ly/onboarding` (i.e. `tsc --noEmit`). No
`npm install`. Atomic single-line conventional commits.

## Out of scope

- `SITE_JSON_LD` / `SITE_DESCRIPTION` in `lib/seo/site-json-ld.ts` — the CSP
  hash is content-derived so an edit is safe, but the site-wide description is
  not the homepage description and changing it would widen the blast radius
  past this task. Leave it.
- The `<h1>`, the SEO landing pages, `SeoLanding.tsx`, `Pricing.tsx`.

## Success criteria

- [ ] Offer stated once, prominently, above the CTAs
- [ ] Fine print keeps only the price detail
- [ ] Title ≤ 60 chars, description ≤ 155 chars, both offer-led
- [ ] `tsc --noEmit` clean
- [ ] Existing e2e heading assertion (`/a storefront worth opening/i`) still passes
