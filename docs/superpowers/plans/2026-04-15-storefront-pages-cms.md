# Storefront Pages CMS + Footer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a self-serve Pages CMS + storefront footer so merchants can author About/Terms/Privacy/etc. and surface them in a footer they control.

**Architecture:** New `pages` table in marketplace-api (markdown-bodied, store-scoped). Extend `store_branding` with a `footer_sections` JSONB column. Admin gets a `/settings/pages` list + editor and a Footer-tab sections editor. Storefront gets a `/pages/[slug]` route and a `<Footer>` component mounted in root layout.

**Tech Stack:** Go 1.26 + Gin + GORM (marketplace-api), Next.js 16 + React 19 + Tailwind (admin, storefront), react-markdown for rendering, simple textarea + preview for authoring (no TipTap in v1).

## Spec reference

`docs/superpowers/specs/2026-04-15-storefront-pages-cms-design.md` — this plan implements phases A through E.

---

## File structure

### marketplace-api (new)
- `services/marketplace-api/migrations/000029_pages.up.sql`
- `services/marketplace-api/migrations/000029_pages.down.sql`
- `services/marketplace-api/migrations/000030_branding_footer_sections.up.sql`
- `services/marketplace-api/migrations/000030_branding_footer_sections.down.sql`
- `services/marketplace-api/internal/page/models.go`
- `services/marketplace-api/internal/page/repository.go`
- `services/marketplace-api/internal/page/repository_integration_test.go`
- `services/marketplace-api/internal/page/service.go`
- `services/marketplace-api/internal/page/service_integration_test.go`
- `services/marketplace-api/internal/page/handler_admin.go`
- `services/marketplace-api/internal/page/handler_storefront.go`
- `services/marketplace-api/internal/page/handler_integration_test.go`

### marketplace-api (modify)
- `services/marketplace-api/migrations.go` — register new migrations
- `services/marketplace-api/cmd/marketplace-api/main.go` — wire page module
- `services/marketplace-api/internal/branding/models.go` — add `FooterSections`
- `services/marketplace-api/internal/branding/service.go` — accept `FooterSections` on update
- `services/marketplace-api/internal/branding/handler.go` — pass through

### apps/admin (new)
- `apps/admin/app/(admin)/settings/pages/page.tsx` — list
- `apps/admin/app/(admin)/settings/pages/[id]/page.tsx` — editor (or modal pattern if admin uses modals elsewhere; see Task 9)
- `apps/admin/app/(admin)/settings/pages/actions.ts` — server actions (CRUD)
- `apps/admin/components/settings/PagesList.tsx`
- `apps/admin/components/settings/PageEditor.tsx`
- `apps/admin/components/settings/FooterSectionsEditor.tsx` — embedded in BrandingSettingsClient's FooterTab

### apps/admin (modify)
- `apps/admin/lib/api/marketplace-api.ts` — add Page + FooterSection types, CRUD client fns
- `apps/admin/components/shell/AdminShell.tsx` — add "Pages" nav item under Settings → Store
- `apps/admin/components/settings/BrandingSettingsClient.tsx` — wire new FooterSectionsEditor into FooterTab; send `footer_sections` in PATCH

### apps/storefront (new)
- `apps/storefront/app/pages/[slug]/page.tsx` — markdown renderer + metadata
- `apps/storefront/components/Footer.tsx` — sitewide footer
- `apps/storefront/lib/markdown.ts` — `renderMarkdown(source)` helper

### apps/storefront (modify)
- `apps/storefront/app/layout.tsx` — mount `<Footer branding={...} />` after children
- `apps/storefront/lib/api/marketplace-api.ts` — add `fetchPage(storeSlug, pageSlug)`, `listPages(storeSlug)` (public shape only)

---

## Phase A — Pages backend

### Task 1: pages migration

**Files:**
- Create: `services/marketplace-api/migrations/000029_pages.up.sql`
- Create: `services/marketplace-api/migrations/000029_pages.down.sql`
- Modify: `services/marketplace-api/migrations.go`

- [ ] **Step 1: Write `000029_pages.up.sql`**

```sql
CREATE TABLE IF NOT EXISTS pages (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL,
    store_id        UUID         NOT NULL,
    slug            VARCHAR(63)  NOT NULL,
    title           VARCHAR(200) NOT NULL,
    body            TEXT         NOT NULL DEFAULT '',
    seo_title       VARCHAR(200),
    seo_description VARCHAR(300),
    published       BOOLEAN      NOT NULL DEFAULT true,
    sort_order      INT          NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS pages_store_slug_idx ON pages (store_id, slug);
CREATE INDEX IF NOT EXISTS pages_tenant_id_idx        ON pages (tenant_id);
CREATE INDEX IF NOT EXISTS pages_store_published_idx  ON pages (store_id, published);
```

- [ ] **Step 2: Write `000029_pages.down.sql`**

```sql
DROP INDEX IF EXISTS pages_store_published_idx;
DROP INDEX IF EXISTS pages_tenant_id_idx;
DROP INDEX IF EXISTS pages_store_slug_idx;
DROP TABLE IF EXISTS pages;
```

- [ ] **Step 3: Register in `migrations.go`** following the existing pattern.

- [ ] **Step 4: Run `go run ./cmd/migrate -direction up` against local DB** and confirm `migrated to 29`.

- [ ] **Step 5: Commit** — `feat(marketplace-api): add pages table`

---

### Task 2: Page model

**Files:**
- Create: `services/marketplace-api/internal/page/models.go`

- [ ] **Step 1: Write the model**

```go
package page

import (
	"time"

	"github.com/google/uuid"
)

type Page struct {
	ID             uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID        uuid.UUID `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	Slug           string    `gorm:"column:slug;type:varchar(63);not null"                    json:"slug"`
	Title          string    `gorm:"column:title;type:varchar(200);not null"                  json:"title"`
	Body           string    `gorm:"column:body;type:text;not null;default:''"                json:"body"`
	SEOTitle       *string   `gorm:"column:seo_title;type:varchar(200)"                       json:"seo_title,omitempty"`
	SEODescription *string   `gorm:"column:seo_description;type:varchar(300)"                 json:"seo_description,omitempty"`
	Published      bool      `gorm:"column:published;not null;default:true"                   json:"published"`
	SortOrder      int       `gorm:"column:sort_order;not null;default:0"                     json:"sort_order"`
	CreatedAt      time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
}

func (Page) TableName() string { return "pages" }
```

- [ ] **Step 2: Commit** — `feat(marketplace-api): add Page model`

---

### Task 3: Page repository + integration tests

**Files:**
- Create: `services/marketplace-api/internal/page/repository.go`
- Create: `services/marketplace-api/internal/page/repository_integration_test.go`

- [ ] **Step 1: Write failing integration tests** (behind `//go:build integration`). Mirror the pattern of `services/marketplace-api/internal/vendor/repository_integration_test.go`:
  - `TestRepository_Create_GetByID`
  - `TestRepository_GetBySlug_PublishedOnly`
  - `TestRepository_ListByStore`
  - `TestRepository_Update`
  - `TestRepository_Delete`
  - `TestRepository_Create_DuplicateSlug_Errors`

- [ ] **Step 2: Run** — expect fail.

- [ ] **Step 3: Write repository**

```go
package page

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, p *Page) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Page, error) {
	var p Page
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetBySlug returns the page for a given store + slug. The
// publishedOnly flag filters unpublished pages from the storefront
// read path; admin calls with publishedOnly=false to preview drafts.
func (r *Repository) GetBySlug(ctx context.Context, storeID uuid.UUID, slug string, publishedOnly bool) (*Page, error) {
	var p Page
	q := r.db.WithContext(ctx).Where("store_id = ? AND slug = ?", storeID, slug)
	if publishedOnly {
		q = q.Where("published = ?", true)
	}
	err := q.First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) ListByStore(ctx context.Context, storeID uuid.UUID, publishedOnly bool) ([]Page, error) {
	var pages []Page
	q := r.db.WithContext(ctx).Where("store_id = ?", storeID)
	if publishedOnly {
		q = q.Where("published = ?", true)
	}
	if err := q.Order("sort_order ASC, title ASC").Find(&pages).Error; err != nil {
		return nil, err
	}
	return pages, nil
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, patch map[string]any) error {
	return r.db.WithContext(ctx).Model(&Page{}).Where("id = ?", id).Updates(patch).Error
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&Page{}).Error
}
```

- [ ] **Step 4: Run** — expect pass with `-tags=integration`.

- [ ] **Step 5: Commit** — `feat(marketplace-api): add Page repository`

---

### Task 4: Page service (validation + slug rules)

**Files:**
- Create: `services/marketplace-api/internal/page/service.go`
- Create: `services/marketplace-api/internal/page/service_integration_test.go`

- [ ] **Step 1: Write failing tests**
  - `TestService_Create_RejectsInvalidSlug` (slug `Foo Bar` rejected; valid `about-us` allowed)
  - `TestService_Create_RejectsOversizeTitle` (201 chars → error)
  - `TestService_Create_DefaultsPublishedToTrue`
  - `TestService_Update_PartialPatch` (patching only title leaves body intact)
  - `TestService_GetBySlug_FiltersUnpublishedOnStorefrontRead`

- [ ] **Step 2: Implement `Service`** with:
  - `slugPattern = /^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$/`
  - `Create(ctx, in CreateInput) (*Page, error)`
  - `Update(ctx, id, in UpdateInput) (*Page, error)` — patch-style, all fields optional
  - `Delete(ctx, id) error`
  - `ListByStore(ctx, storeID, publishedOnly) ([]Page, error)`
  - `GetByID(ctx, id) (*Page, error)`
  - `GetBySlugForStorefront(ctx, storeID, slug) (*Page, error)` — publishedOnly=true

- [ ] **Step 3: Commit** — `feat(marketplace-api): add Page service with validation`

---

### Task 5: Admin + storefront handlers

**Files:**
- Create: `services/marketplace-api/internal/page/handler_admin.go`
- Create: `services/marketplace-api/internal/page/handler_storefront.go`
- Create: `services/marketplace-api/internal/page/handler_integration_test.go`

- [ ] **Step 1: Admin handler** — mirror `internal/branding/handler.go` patterns, FGA-gated by `can_edit_store_settings` on the current store. Routes:
  - `GET    /api/v1/pages`
  - `POST   /api/v1/pages`
  - `GET    /api/v1/pages/:id`
  - `PATCH  /api/v1/pages/:id`
  - `DELETE /api/v1/pages/:id`

  All infer `tenant_id` + `store_id` from the request context (header) — never trust the body for those.

- [ ] **Step 2: Storefront handler** — public routes, no auth, store resolved from URL slug:
  - `GET /api/v1/storefront/stores/:slug/pages`                     (list published, label+slug only)
  - `GET /api/v1/storefront/stores/:slug/pages/:page-slug`          (fetch published full)

- [ ] **Step 3: Integration tests** cover happy path + 404 unpublished + 404 wrong store.

- [ ] **Step 4: Commit** — `feat(marketplace-api): add Page admin + storefront HTTP handlers`

---

### Task 6: Wire page module into main.go

- [ ] **Step 1:** Construct repo → service → handlers following the `vendor` module wiring. Mount admin routes on the authenticated `/api/v1` group (behind tenant middleware), storefront routes on the public group.
- [ ] **Step 2:** `go build ./...`, expect clean.
- [ ] **Step 3: Commit** — `feat(marketplace-api): wire Page module into router`

---

## Phase B — Storefront page rendering

### Task 7: Storefront API client + markdown helper

**Files:**
- Create: `apps/storefront/lib/markdown.ts`
- Modify: `apps/storefront/lib/api/marketplace-api.ts`

- [ ] **Step 1: Add `fetchPage` + `listPages` to `marketplace-api.ts`**

```ts
export interface StorefrontPage {
  slug: string;
  title: string;
  body: string;
  seo_title: string | null;
  seo_description: string | null;
}

export interface StorefrontPageSummary {
  slug: string;
  title: string;
}

export async function listPages(storeSlug: string): Promise<StorefrontPageSummary[]> { /* ... */ }
export async function fetchPage(storeSlug: string, pageSlug: string): Promise<StorefrontPage | null> { /* ... */ }
```

- [ ] **Step 2: Add `react-markdown` to `apps/storefront/package.json`**

```bash
cd apps/storefront && npm install react-markdown remark-gfm
```

- [ ] **Step 3: Write markdown helper**

```ts
// apps/storefront/lib/markdown.ts
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { ComponentProps } from "react";

export type MarkdownProps = {
  children: string;
  className?: string;
};

export function Markdown({ children, className }: MarkdownProps) {
  return (
    <div className={className}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>
        {children}
      </ReactMarkdown>
    </div>
  );
}
```

  Note: `skipHtml` is the XSS guard — user-authored markdown can include
  `<script>` tags, and we strip them here.

- [ ] **Step 4: Commit** — `feat(storefront): add markdown renderer + page API client`

---

### Task 8: `/pages/[slug]` route

**Files:**
- Create: `apps/storefront/app/pages/[slug]/page.tsx`

- [ ] **Step 1: Write the route**

```tsx
// apps/storefront/app/pages/[slug]/page.tsx
import { notFound } from "next/navigation";
import { headers } from "next/headers";
import type { Metadata } from "next";

import { slugFromHost } from "@/lib/slug";
import { fetchPage } from "@/lib/api/marketplace-api";
import { Markdown } from "@/lib/markdown";

interface Props {
  params: Promise<{ slug: string }>;
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params;
  const h = await headers();
  const storeSlug = slugFromHost(h.get("host")) ?? process.env.DEFAULT_STORE_SLUG ?? "";
  const page = storeSlug ? await fetchPage(storeSlug, slug) : null;
  if (!page) return { title: "Page not found" };
  return {
    title: page.seo_title ?? page.title,
    description: page.seo_description ?? undefined,
  };
}

export default async function PageView({ params }: Props) {
  const { slug } = await params;
  const h = await headers();
  const storeSlug = slugFromHost(h.get("host")) ?? process.env.DEFAULT_STORE_SLUG ?? "";
  const page = storeSlug ? await fetchPage(storeSlug, slug) : null;

  if (!page) notFound();

  return (
    <main id="main" className="mx-auto max-w-3xl px-6 py-16 sm:py-20">
      <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground">
        {page.title}
      </h1>
      <Markdown className="prose mt-8 max-w-none prose-headings:font-serif prose-headings:text-foreground prose-a:text-moss-700">
        {page.body}
      </Markdown>
    </main>
  );
}
```

- [ ] **Step 2: Test manually** — create a page via raw API curl against local marketplace-api, hit `/pages/{slug}` on storefront localhost.

- [ ] **Step 3: Commit** — `feat(storefront): add /pages/[slug] route with markdown rendering`

---

## Phase C — Admin Pages CMS

### Task 9: Decide routing pattern

- [ ] **Step 1:** Check existing admin surfaces (e.g. `/products`, `/customers`) — do they use modals or dedicated routes for editors?
- [ ] **Step 2:** Match the existing pattern. If products uses `/products/[id]`, pages uses `/settings/pages/[id]`. If modals, do modals. Document the choice in this file before coding.

### Task 10: Admin API client + server actions

**Files:**
- Modify: `apps/admin/lib/api/marketplace-api.ts`
- Create: `apps/admin/app/(admin)/settings/pages/actions.ts`

- [ ] Types for `Page`, `CreatePageInput`, `UpdatePageInput`.
- [ ] Admin fetch helpers: `listPages`, `getPage`, `createPage`, `updatePage`, `deletePage`.
- [ ] Server actions wrap those and include FGA gating via the existing
  `canEditSettings` helper.
- [ ] Commit — `feat(admin): add Page API client + server actions`

### Task 11: `/settings/pages` list route

**Files:**
- Create: `apps/admin/app/(admin)/settings/pages/page.tsx`
- Create: `apps/admin/components/settings/PagesList.tsx`

- [ ] Table view with title, `/pages/slug` preview link, status pill, updated_at.
- [ ] "+ New page" button creates a draft row and navigates to editor.
- [ ] Commit — `feat(admin): add /settings/pages list`

### Task 12: Editor route + markdown preview

**Files:**
- Create: `apps/admin/app/(admin)/settings/pages/[id]/page.tsx`
- Create: `apps/admin/components/settings/PageEditor.tsx`

- [ ] Form fields: title, slug (with auto-slugify until touched), body textarea, live preview right column via `react-markdown`, SEO accordion, published toggle.
- [ ] Toast on save + delete.
- [ ] Commit — `feat(admin): add Pages CMS editor`

### Task 13: Add "Pages" to admin nav

**Files:**
- Modify: `apps/admin/components/shell/AdminShell.tsx`

- [ ] Add `{ group: "Store", label: "Pages", href: "/settings/pages" }` to the settings section's `children`.
- [ ] Commit — `chore(admin): add Pages to settings nav`

---

## Phase D — Footer sections schema + admin editor

### Task 14: `footer_sections` migration

**Files:**
- Create: `services/marketplace-api/migrations/000030_branding_footer_sections.up.sql`
- Create: `services/marketplace-api/migrations/000030_branding_footer_sections.down.sql`

```sql
-- up
ALTER TABLE store_branding ADD COLUMN IF NOT EXISTS footer_sections JSONB NOT NULL DEFAULT '[]'::jsonb;

-- down
ALTER TABLE store_branding DROP COLUMN IF EXISTS footer_sections;
```

- [ ] Register, run, commit — `feat(marketplace-api): store_branding.footer_sections column`

### Task 15: Extend branding model + service + handler

**Files:**
- Modify: `services/marketplace-api/internal/branding/models.go`
- Modify: `services/marketplace-api/internal/branding/service.go`
- Modify: `services/marketplace-api/internal/branding/handler.go`

- [ ] Add `FooterSections datatypes.JSON` to the model (use `gorm.io/datatypes` or `pgtype.JSONB`).
- [ ] Validate shape on update: must be a JSON array, each element `{label string, items [{label, kind in ["page","url"], page_slug? string, url? string}]}`. Cap at 6 sections × 10 items. Reject anything else with `apperrors.ValidationFailed`.
- [ ] Accept `footer_sections` in the update handler body.
- [ ] Unit test the validator.
- [ ] Commit — `feat(marketplace-api): branding accepts footer_sections`

### Task 16: FooterSectionsEditor admin component

**Files:**
- Create: `apps/admin/components/settings/FooterSectionsEditor.tsx`
- Modify: `apps/admin/components/settings/BrandingSettingsClient.tsx`

- [ ] Sections list with + Add section / Remove section.
- [ ] Per section: label input + items list.
- [ ] Per item: label input, kind radio/select (Page | URL), value (page `<select>` from `listPages` OR url input).
- [ ] Up/down arrows for ordering; defer drag-reorder.
- [ ] Wire `footer_sections` into the branding PATCH payload.
- [ ] Commit — `feat(admin): footer sections editor`

---

## Phase E — Storefront Footer component

### Task 17: `<Footer>` component

**Files:**
- Create: `apps/storefront/components/Footer.tsx`

- [ ] Read branding: `footer_tagline`, `footer_copyright`, `social_*`, `footer_sections`.
- [ ] Render editorial footer: hairline rule, three-column grid on desktop (tagline + social on left, link sections in middle, copyright on right). Stacks on mobile.
- [ ] Link items: if `kind === "page"` → `href={/pages/${page_slug}}`; if `kind === "url"` → external link, new tab if absolute URL.
- [ ] Empty-state: if `footer_sections` is empty, still show tagline + copyright + social so the footer always has content.
- [ ] Commit — `feat(storefront): add Footer component`

### Task 18: Mount Footer in RootLayout

**Files:**
- Modify: `apps/storefront/app/layout.tsx`

- [ ] Mount `<Footer branding={brandingData?.branding ?? null} />` after `<CustomerAuthProvider>` children.
- [ ] Smoke test locally — every storefront route now renders the footer.
- [ ] Commit — `feat(storefront): mount Footer in RootLayout`

---

## Phase F — Polish

### Task 19: Page slug collision UX

- [ ] On create/update, catch PG `23505 unique_violation` on `pages_store_slug_idx` and return `apperrors.Conflict("slug_taken", ...)`; admin editor shows inline "This slug is already used".

### Task 20: Markdown safety + styling review

- [ ] Confirm `skipHtml` is set on every `ReactMarkdown` render (storefront + admin preview).
- [ ] Confirm XSS test: create a page with body `<script>alert(1)</script>` → storefront renders it as plain text (no script executes).

### Task 21: Deploy + verify

- [ ] Standard CI+deploy flow (flip public if billing, merge k8s PR, private, ArgoCD sync, rollout).
- [ ] Create an "About" page in prod, add to footer under a "Company" section, verify it renders at `/pages/about` and the footer link works across multiple storefront routes.

---

## Done criteria

- [ ] `pages` table + `store_branding.footer_sections` exist in prod DB.
- [ ] A merchant creates a page in admin, publishes it, and it renders at `https://{slug}.mark8ly.com/pages/{page-slug}`.
- [ ] The merchant adds a footer section referencing that page; the link appears on every storefront page and navigates correctly.
- [ ] Unpublished pages return 404 on storefront.
- [ ] Markdown XSS test (`<script>` in body) renders inert.
- [ ] No user-visible impact on stores that have no pages / no footer sections — they see a minimal footer with just tagline + copyright + social.
- [ ] Admin tests still pass; new backend + admin UI unit tests pass.
