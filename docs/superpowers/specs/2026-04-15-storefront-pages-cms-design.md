# Storefront Pages CMS + Footer

**Date:** 2026-04-15
**Status:** Draft — pending review
**Scope:** marketplace-api, apps/admin, apps/storefront

## Goal

Let merchants author custom content pages (About, Terms of Service, Privacy
Policy, Returns, Contact, etc.) from the admin, and surface them in a
storefront footer they also control. Self-serve, no developer involvement,
per-store scoping.

## Current state

**What exists:**
- `store_branding` table already carries `footer_tagline`, `footer_copyright`,
  and social links (instagram/twitter/facebook/tiktok/youtube) — plain string
  fields, one row per store.
- `BrandingSettingsClient` admin component has a "Footer" tab with those
  plain fields.
- No storefront `Footer` component. Nothing renders branding.footer_* today.
- No Pages CMS, no `/pages/[slug]` route.
- No rich text editor in the codebase (TipTap is in tech stack but unused).

**What's missing:**
- Custom footer link sections (e.g. "Company", "Support", "Legal") each with
  N items.
- A place to author long-form content (pages).
- A way for the footer to link to those pages.
- A storefront route that renders the pages and a storefront footer that
  renders on every page.

## Proposed solution

Three independent surfaces, each shippable on its own:

### 1. Pages (new entity)

Store-scoped, markdown-bodied content pages authored from the admin.

Schema (`pages` table in marketplace-api DB):

| Column | Type | Notes |
|---|---|---|
| id | uuid PK | `gen_random_uuid()` default |
| tenant_id | uuid NOT NULL | indexed |
| store_id | uuid NOT NULL | indexed with slug unique |
| slug | varchar(63) NOT NULL | lowercase, hyphen, unique per store |
| title | varchar(200) NOT NULL | display title |
| body | text NOT NULL | markdown source (stored raw) |
| seo_title | varchar(200) | null → fall back to title |
| seo_description | varchar(300) | meta description |
| published | boolean NOT NULL DEFAULT true | storefront only shows published |
| sort_order | int NOT NULL DEFAULT 0 | for future admin ordering |
| created_at, updated_at | timestamptz | |

**Why markdown, not rich text:** markdown is the smallest surface area
(plain textarea + client-side preview), avoids adding TipTap / sanitization
concerns for v1, and every merchant we care about can paste from Google Docs
or Notion into a markdown editor. We can add TipTap later without a schema
change — the column stays `text`.

### 2. Footer sections (extend branding)

Add one JSON column to `store_branding`:

```sql
ALTER TABLE store_branding ADD COLUMN footer_sections JSONB NOT NULL DEFAULT '[]';
```

Shape:
```json
[
  {
    "label": "Company",
    "items": [
      { "label": "About us", "kind": "page", "page_slug": "about" },
      { "label": "Contact", "kind": "url", "url": "mailto:help@acme.com" }
    ]
  },
  {
    "label": "Legal",
    "items": [
      { "label": "Terms", "kind": "page", "page_slug": "terms" },
      { "label": "Privacy", "kind": "page", "page_slug": "privacy" }
    ]
  }
]
```

**Why JSONB, not a separate table:** sections are a small, ordered,
hierarchical structure that's always read and written as a whole. A
relational design (footer_sections + footer_section_items tables) would add
two migrations, two handlers, and ordering complexity for zero user benefit.
JSONB stays per-store, atomic, and the admin UI round-trips the whole array
on save — matches how the merchant thinks about "my footer."

Each item is either a page reference (`kind: "page"`, with `page_slug`) or
an external URL (`kind: "url"`, with `url`). Storefront resolves page
references at render time against the pages table.

### 3. Storefront surface

- **New route** `apps/storefront/app/pages/[slug]/page.tsx`: fetches the
  page by slug for the current store, 404s if not found or unpublished,
  renders title + markdown body. Uses `remark` + `remark-html` (or
  `react-markdown`) to transform markdown → HTML at request time.
  Metadata comes from seo_title / seo_description.
- **New component** `apps/storefront/components/Footer.tsx`: reads
  `branding.footer_sections` + `footer_tagline` + `footer_copyright` +
  social links, renders them in an editorial footer at the bottom of every
  storefront page. Mounted in `apps/storefront/app/layout.tsx` after
  `CartProvider > children`.
- Footer respects the brand tokens (moss accent for hovered links, paper
  background, hairline rules — no cards).

## API surface

### Admin (platform-scoped, FGA-gated by `can_edit_store_settings`)

```
GET    /api/v1/pages                     list pages for current store
POST   /api/v1/pages                     create
GET    /api/v1/pages/:id                 fetch single
PATCH  /api/v1/pages/:id                 update (all fields optional)
DELETE /api/v1/pages/:id                 delete
```

### Storefront (public, store-scoped)

```
GET /api/v1/storefront/stores/:slug/pages              list published (label + slug only)
GET /api/v1/storefront/stores/:slug/pages/:page-slug   fetch published page body
```

The admin list endpoint doubles as the page picker data source for the
footer editor.

## Admin UX

### New: `/settings/pages`

Lives under Settings → Store group in the sidebar.

- **List view**: simple table of pages with `Title · /pages/slug · Status ·
  Updated` columns, row click → editor, header "+ New page" button. Filter
  by published, empty state with a call to create the first page.
- **Editor view** (modal or full-screen route, whichever matches existing
  admin conventions):
  - Title input (auto-populates slug with slugify, until slug is touched).
  - Slug input with `.mark8ly.com/pages/` prefix label.
  - Body: textarea (split view — editor left, markdown preview right).
  - SEO accordion: seo_title + seo_description fields.
  - Published toggle.
  - Save / delete actions, toast on save.

### Extended: Footer tab in BrandingSettingsClient

Existing fields (tagline, copyright, social links) stay. Add a new section
"Footer navigation" above them:

- "+ Add section" button at the bottom.
- Each section row: label input + items list.
- Each item row: label input + kind picker (Page | URL) + value (page
  `<select>` sourced from `GET /api/v1/pages` OR url text input).
- Drag-reorder sections and items (defer to a follow-up if not trivial —
  can ship with up/down arrows initially).
- Save posts the whole `footer_sections` array atomically.

## Phased rollout

Each phase is independently deployable and reversible.

### Phase A — Pages backend
- Migration: `pages` table
- marketplace-api: `internal/page/` package (model, repository, service,
  admin handler, storefront handler)
- Wire into router
- Unit + integration tests

### Phase B — Storefront page rendering
- Storefront client: `fetchPage(storeSlug, pageSlug)`, `listPages(storeSlug)`
- New route `/pages/[slug]/page.tsx` with markdown rendering
- Metadata generator

### Phase C — Admin Pages CMS
- Server actions: `createPage`, `updatePage`, `deletePage`, `listPages`
- `/settings/pages` list route
- `/settings/pages/[id]` editor route (or modal), with markdown textarea
  + live preview via `react-markdown`

### Phase D — Footer schema + admin editor
- Migration: add `footer_sections JSONB` to `store_branding`
- Backend: extend branding model + service + handler to accept
  `footer_sections` on PATCH (passthrough validation — structure checked
  client-side, server enforces array + string field types)
- Admin: upgrade FooterTab with the sections editor described above

### Phase E — Storefront Footer component
- `apps/storefront/components/Footer.tsx`: render branding footer
- Mount in `app/layout.tsx`
- Every storefront layout inherits it without per-layout changes

### Phase F — Polish
- Delete confirmations, page slug collision handling, markdown safety
  (sanitize during render), SEO fallbacks, empty-footer graceful degradation

## Scope cuts for v1

- No versioning / drafts beyond the boolean `published` flag.
- No scheduled publishing.
- No media upload (merchants link to already-uploaded URLs).
- No rich text editor — markdown only (TipTap is a future upgrade).
- No redirect rules when a slug changes (we just let old URL 404).
- No page hierarchy / nesting (flat list).
- Drag-reorder in footer editor is nice-to-have, up/down arrows ok for v1.
- No audit log for page edits (site uses audit_log for other entities; add
  if someone asks).

## Risks

- **Markdown safety**: rendering user-authored markdown can leak XSS if we
  pass raw HTML through. Use `remark-html` with `sanitize: true` OR
  `react-markdown` with `skipHtml: true` so raw `<script>` tags are stripped.
- **Slug collisions**: two pages in one store can't share a slug. Enforced
  by DB unique constraint; admin shows friendly error on the rare collision.
- **Footer emptiness**: merchant saves with no sections → storefront should
  still render a minimal footer (copyright + social only). Component must
  handle `footer_sections: []`.
- **Branding PATCH size**: `footer_sections` could grow to ~20 items. Set a
  soft cap (e.g. max 6 sections × 10 items) validated client-side with
  server enforcing 64KB JSONB size ceiling.

## Success criteria

- A merchant can create a page in admin and it renders at
  `https://{slug}.mark8ly.com/pages/{page-slug}` within seconds.
- Unpublished pages 404 on the storefront.
- The merchant can add footer sections referencing pages, save, and the
  links appear in the storefront footer on every page, navigating to the
  correct content.
- No developer deploys required for content changes.
