# Products M7a — Admin UI: Sidebar + Products List Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `ComingSoon` stub at `apps/admin/app/products/page.tsx` with a real products list page that fetches from marketplace-api's `GET /api/v1/admin/stores/:storeId/products`, displays the editorial page header / filter row / data table / pagination from spec §7.2, and ships two new promoted `@repo/ui` components (`StatusDot`, `PriceDisplay`) that later M7 sub-milestones + other admin pages will reuse.

**Architecture:** Server-component data fetch from the existing `serverSession.ts` context (which already resolves `currentStore.id`) through a new `lib/api/marketplace-api.ts` client that forwards the `x-session-*` headers from middleware as `X-User-Id` / `X-Tenant-Id` to marketplace-api. The page renders a small handful of client components (`ProductsListFilters` for the searchbar, `ProductsList` for the table with sortable columns, `ProductsListEmpty` for the empty state) built on top of `@tesserix/web` primitives. `StatusDot` and `PriceDisplay` are added to `packages/ui/src/` as flat-file exports matching the existing `@repo/ui` convention. No authoring of `@tesserix/web` components — we consume what's already there. A single Playwright E2E test proves the list page renders against a running marketplace-api, asserting the editorial layout, filter row, pagination, and both empty-state and populated-state paths.

**Tech Stack:** Next.js 16 (App Router + server components), React 19, Tailwind CSS v4, `@tesserix/web` v1.7.1, `@repo/ui` (flat file exports), TypeScript, Playwright 1.59+, Source Serif 4 + Source Sans 3 per Paper · Ink · Moss design tokens.

---

## Status

> **Pending.** All tasks open.

---

## Scope check

Contained slice inside the `apps/admin` Next.js app + one small addition to `packages/ui`. Does not touch marketplace-api Go code, does not touch migrations, does not touch Helm charts, does not change any other admin page. Modifies `apps/admin/app/products/page.tsx` (was a `ComingSoon` stub), adds files under `apps/admin/components/products/` and `apps/admin/lib/api/`, adds two files to `packages/ui/src/`, adds one Playwright test under `apps/admin/tests/e2e/`.

Spec sections authoritative for this milestone:

- §7.1 — sidebar (the Products link already exists; confirmed in `apps/admin/components/shell/AdminShell.tsx`; M7a verifies highlight + route still work)
- §7.2 — products list page (the entire design spec for this milestone)
- §7.8 — role-based UX gates (staff = read-only, admin = full CRUD, owner = delete)
- §7.9 — accessibility baseline (skip link, semantic landmarks, focus ring, prefers-reduced-motion)
- §7.10 — component reuse map (which primitives come from `@tesserix/web`, which we promote to `@repo/ui`)
- §8 M7a entry: "`Products` sidebar item, list page with all the components from §7.10, server-side data fetch, `StatusDot` and `PriceDisplay` promoted to `@repo/ui`."
- §13.1.1 permission map (M5a enforced this at the API layer; M7a's UI gates just mirror it cosmetically — they are NOT the enforcement boundary)
- §13.5 UX corrections — applies to the detail page (M7b), not the list page, but the "no dialogs except for hard delete" rule does govern the overflow menu items added in M7a (Archive = inline, Delete = confirm-dialog)

**Out of scope (deferred to M7b/c/d):**
- Product detail page (`/products/new` and `/products/:id`)
- Media grid, variant editor, options editor, category picker, category drawer
- Copy-to-store dialog (only a disabled overflow menu item appears here; the dialog itself is M7d)
- Bulk-actions bar behavior (checkbox UI renders but bulk actions are no-op stubs with a "Coming in M7d" toast)

---

## Decisions locked

1. **Server-side data fetch.** The `/products` page is a server component that fetches via `lib/api/marketplace-api.ts#listProducts`. No client-side SWR / React Query in M7a. Filter/pagination state lives in URL search params and triggers a full server round-trip on change — Next 16 App Router handles this natively via `searchParams` prop. Client-side SWR comes in M7b when we need optimistic updates.
2. **URL is the source of truth for list state.** `?page=2&page_size=20&status=active&search=linen` is the canonical shape. Shareable, keyboard-friendly, debuggable. The only "client-side" state in M7a is the search input's debounced typing buffer before it navigates.
3. **No skeleton loading in M7a.** Next.js server-component streaming + React Suspense handles render-on-navigation. A proper `table-skeleton` (§7.10) ships when a client-side-rendered filter/pagination lands in M7b+.
4. **Cosmetic role gating only.** The page renders the `+ New product`, `Archive`, `Delete` overflow items based on `role` from `serverSession`. Owner sees Delete; admin sees Archive; staff sees no mutating actions. The API enforces independently — frontend gates never the source of truth.
5. **`StatusDot` and `PriceDisplay` are promoted to `@repo/ui` as flat files**, matching the existing convention (`brand-bar.tsx`, `role-badge.tsx`, etc). Exports resolve via the package.json `"./*": "./src/*.tsx"` pattern. Consumers import from `@repo/ui/status-dot` and `@repo/ui/price-display`.
6. **`PriceDisplay` formats via `Intl.NumberFormat`, not `decimal.js`.** Admin list is read-only display; we don't need arbitrary-precision arithmetic here. The component takes `{ amount: string, currencyCode: string }` where `amount` is the raw decimal string from the API (e.g., "19.99"), parses with `parseFloat`, and formats with `Intl.NumberFormat(locale, { style: "currency", currency: currencyCode })`. Locale defaults to browser's `navigator.language`; server components pass `undefined` to use the default. For variant-priced products the list renders `from €89` where the "from" prefix is rendered inline (not part of the component).
7. **`StatusDot` variants:** `active` (moss dot), `draft` (outline dot), `archived` (muted/ink dot). Props: `{ status: "active" | "draft" | "archived", withLabel?: boolean }`. When `withLabel` is true, renders "Active" / "Draft" / "Archived" alongside the dot. The label is intentionally NOT the localized status — this is an admin UI, not a customer-facing one.
8. **Image fallback is a hairline-bordered Paper placeholder**, no "No image" icon. Follows the "editorial not utilitarian" rule. 40×40 in the row, 4px radius.
9. **Pagination is numbered**, not infinite scroll. Page numbers + prev/next buttons from `@tesserix/web`'s `pagination` component. 20 per page default, 100 max. The URL search param is `page` (1-indexed) and `page_size`.
10. **Filter panel is a dropdown**, not a drawer. Three filters: Status (Active / Draft / Archived checkboxes — multi-select), Category (future — render disabled with tooltip "Category filter lands with M7d"), Stock (In stock / Low stock / Out of stock radios — M7b implements the semantic; M7a renders them disabled with a "Coming soon" tooltip). Only Status ships live in M7a — keeping the surface focused. Document that Category/Stock are stubs.
11. **`+ New product` button navigates to `/products/new` which is a separate route that renders a stub "M7b will land this page" message.** We need to create `/products/new/page.tsx` as a placeholder so the button doesn't produce a 404 — but it's literally a `ComingSoon` stub. M7b replaces it.
12. **Error state via `error-boundary`.** Next 16 App Router supports an `error.tsx` sibling to `page.tsx`. Add one that renders a small editorial error state ("Couldn't load your products. Try again.") with a retry button. Logged server-side via `console.error`.
13. **Empty state is two variants** — no products at all (with a big "+ New product" CTA) vs no matches (with a "Clear filters" link). Different copy, different CTA.
14. **Playwright test runs against a real marketplace-api.** The existing `apps/admin/tests/e2e/` pattern uses `playwright.config.ts` to spin up the full stack. The new test seeds a store + a handful of products via the API (using the admin endpoint, with a test auth header) then navigates to `/products` and asserts the rendered DOM. If the existing test harness doesn't already have a "logged-in session" helper, we stub it by setting the `x-session-*` cookies directly. Review the current Playwright config before writing the test.

---

## File structure produced by M7a

```
mark8ly/
├── packages/ui/src/
│   ├── status-dot.tsx                       NEW: flat file export, ~40 LOC
│   └── price-display.tsx                    NEW: flat file export, ~50 LOC
├── apps/admin/
│   ├── lib/api/
│   │   └── marketplace-api.ts               NEW: typed client for marketplace-api admin endpoints (listProducts for M7a; the rest stubbed out with `// M7b will add` markers)
│   ├── app/products/
│   │   ├── page.tsx                         REWRITE: was ComingSoon; now the real list page with server-side data fetch + URL-driven state
│   │   ├── error.tsx                        NEW: error boundary
│   │   └── new/
│   │       └── page.tsx                     NEW: ComingSoon stub for M7b
│   ├── components/products/
│   │   ├── ProductsListHeader.tsx           NEW: editorial page header with title, summary line, filter row, + New product CTA
│   │   ├── ProductsList.tsx                 NEW: the data-table with hairline rules, columns, overflow menu
│   │   ├── ProductsListFilters.tsx          NEW: search + filter dropdown
│   │   ├── ProductsListPagination.tsx       NEW: numbered pagination bound to URL
│   │   ├── ProductsListEmpty.tsx            NEW: zero-products vs no-matches empty states
│   │   └── ProductsListSummary.tsx          NEW: "42 products · 3 drafts · 2 archived"
│   └── tests/e2e/
│       └── products-list.spec.ts            NEW: Playwright test — empty state, populated state, filter, pagination, role gating
```

**Target sizes:** every component under 200 LOC; the list table the largest at ~250. No big splits required.

---

## New Go module dependencies

**None.** Pure frontend work. No changes to marketplace-api, no changes to `go.mod`.

## New npm dependencies

**None.** Everything we need is already in `apps/admin/package.json`:
- `@tesserix/web` v1.7.1 (primitives)
- `@repo/ui` (shared components)
- `next` v16.1.1
- `react` / `react-dom` v19.2.0
- `tailwindcss` v4.2.2
- `lucide-react` (icons)
- `@hookform/resolvers` + `react-hook-form` (M7b uses these; M7a doesn't need them)
- `zod` (validation — unused in M7a but already installed)

---

## Landmines

1. **Turbo + pnpm monorepo cache:** this repo uses pnpm + Turborepo. After adding files to `packages/ui/`, run `pnpm install` from the repo root OR restart the Next dev server to pick up the new exports. The package.json exports pattern `"./*": "./src/*.tsx"` resolves at build time, so changes are visible without a rebuild of `@repo/ui` itself.
2. **`@repo/ui` has no build step** — that's why flat `.tsx` files work. Do NOT add a `tsup` build step, do NOT add a `dist/` directory. The consumers import `.tsx` directly and let Next compile it.
3. **Tailwind v4 classes only.** No v3 `@apply` patterns, no legacy utility variants. Use the tokens from `packages/ui/src/styles/mark8ly-tokens.css` — `--ink-900`, `--paper-200`, `--moss-700`, etc. Avoid the deprecated `--warm-*` / `--terracotta-*` aliases mentioned in the project CLAUDE.md design context.
4. **No pure white backgrounds.** Cards live on `--background-elevated` (pure white reserved for elevated surfaces), page background is Paper (`--paper-200`). This is a brand invariant — the design reviewer will flag any `bg-white` on a page background.
5. **Serif for headlines only.** Source Serif 4 for the page title and major headings. Source Sans 3 for table body, labels, button text. Never serif for body copy — that's a brand anti-pattern per the project CLAUDE.md.
6. **`+ New product` navigates to `/products/new`, not `/products/create`.** Matches §7.3: "Product detail page (`/products/new` and `/products/:id`)". Consistency with the detail page route shape.
7. **Middleware header rewrite:** M5a's backend expects `X-User-Id` and `X-Tenant-Id`. The admin middleware at `apps/admin/middleware.ts` sets `x-session-user-id`, `x-session-tenant-id`, `x-session-role` on the request. The server-side fetch helper must convert between the two naming conventions.
8. **No CWD drift:** pnpm + turbo scripts run from the repo root. Always `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly && pnpm ...` for install and build commands.

---

## Task decomposition

9 tasks. Foundation (API client + promoted UI components) first, then the page itself, then tests.

| # | Task | Approx effort |
|---|---|---|
| 1 | `packages/ui/src/status-dot.tsx` + `price-display.tsx` | 45 min |
| 2 | `apps/admin/lib/api/marketplace-api.ts` — typed client with `listProducts` | 60 min |
| 3 | `apps/admin/components/products/ProductsListSummary.tsx` + `ProductsListEmpty.tsx` + `ProductsListPagination.tsx` | 75 min |
| 4 | `apps/admin/components/products/ProductsListFilters.tsx` + `ProductsListHeader.tsx` | 75 min |
| 5 | `apps/admin/components/products/ProductsList.tsx` — the data-table composition | 90 min |
| 6 | `apps/admin/app/products/page.tsx` rewrite — server component wiring everything together | 60 min |
| 7 | `apps/admin/app/products/error.tsx` + `apps/admin/app/products/new/page.tsx` (ComingSoon stub) | 20 min |
| 8 | `apps/admin/tests/e2e/products-list.spec.ts` — Playwright test | 90 min |
| 9 | Verification + PR | 30 min |
| **Total** | | **~9 hours** |

---

### Task 1: `packages/ui/src/status-dot.tsx` + `price-display.tsx`

**Files:**
- Create: `packages/ui/src/status-dot.tsx`
- Create: `packages/ui/src/price-display.tsx`

**Scope:** Two small pure presentational components that later M7 sub-milestones + orders + other pages will reuse. Zero dependencies beyond React. Use the Paper · Ink · Moss tokens directly via Tailwind classes.

**`status-dot.tsx`:**

```tsx
// packages/ui/src/status-dot.tsx
//
// StatusDot renders an 8px circle in one of the three product-status
// variants, optionally followed by a label. Used on the products list,
// orders list, invitations list, and anywhere else a small at-a-glance
// state indicator fits.
//
// Palette (Paper · Ink · Moss):
//   active   — solid moss
//   draft    — outlined ink (no fill)
//   archived — muted solid ink (reduced opacity)
//
// Label typography is Source Sans 3 at the same size as the parent text.

import type { ReactNode } from "react";

export type StatusDotVariant = "active" | "draft" | "archived";

export interface StatusDotProps {
  status: StatusDotVariant;
  withLabel?: boolean;
  className?: string;
  /** Override the rendered label text. Defaults to the capitalized status. */
  label?: ReactNode;
}

const VARIANT_CLASS: Record<StatusDotVariant, string> = {
  active: "bg-[color:var(--moss-700)]",
  draft: "border border-[color:var(--ink-900)] bg-transparent",
  archived: "bg-[color:var(--ink-900)] opacity-40",
};

const DEFAULT_LABEL: Record<StatusDotVariant, string> = {
  active: "Active",
  draft: "Draft",
  archived: "Archived",
};

export function StatusDot({ status, withLabel = true, className = "", label }: StatusDotProps) {
  const dot = (
    <span
      role="presentation"
      aria-hidden="true"
      className={`inline-block h-2 w-2 rounded-full ${VARIANT_CLASS[status]}`}
    />
  );
  if (!withLabel) {
    return <span className={className}>{dot}</span>;
  }
  return (
    <span className={`inline-flex items-center gap-2 ${className}`}>
      {dot}
      <span className="text-[color:var(--ink-900)]">{label ?? DEFAULT_LABEL[status]}</span>
    </span>
  );
}
```

**`price-display.tsx`:**

```tsx
// packages/ui/src/price-display.tsx
//
// PriceDisplay formats a money amount in Source Serif 4 tabular figures
// with locale-aware currency formatting. Admin list, storefront cards,
// order lines, and invoices all use this component so the rendering is
// consistent across surfaces.
//
// Takes amount as a string (the raw decimal shape the API returns, e.g.
// "19.99" or "89.00") to avoid JS float loss. For display-only use cases
// Intl.NumberFormat + parseFloat is fine — we don't do arithmetic here.
//
// For variant-priced products, callers render a "from" prefix outside
// this component (e.g. `<span>from <PriceDisplay ... /></span>`).

import type { CSSProperties } from "react";

export interface PriceDisplayProps {
  amount: string;
  currencyCode: string;
  /** BCP 47 locale. Omit to use the browser's default. */
  locale?: string;
  className?: string;
  /** Render as a `<span>` (default) or `<div>` / other block element. */
  as?: "span" | "div";
}

const TABULAR_STYLE: CSSProperties = {
  fontFeatureSettings: '"tnum" 1, "lnum" 1',
};

export function PriceDisplay({
  amount,
  currencyCode,
  locale,
  className = "",
  as = "span",
}: PriceDisplayProps) {
  const numeric = Number.parseFloat(amount);
  const formatted = Number.isFinite(numeric)
    ? new Intl.NumberFormat(locale, {
        style: "currency",
        currency: currencyCode,
      }).format(numeric)
    : `${currencyCode} ${amount}`;
  const Tag = as;
  return (
    <Tag
      className={`font-[family-name:var(--font-serif,'Source_Serif_4',serif)] ${className}`}
      style={TABULAR_STYLE}
    >
      {formatted}
    </Tag>
  );
}
```

- [ ] **Step 1: Create both files verbatim above.**
- [ ] **Step 2: Verify package.json export pattern matches** (`"./*": "./src/*.tsx"`) — confirm via `cat packages/ui/package.json`. The existing pattern already resolves `@repo/ui/status-dot` and `@repo/ui/price-display` automatically.
- [ ] **Step 3: Type-check** `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly && pnpm -F @repo/ui check-types`. If `@repo/ui` has no `check-types` script, skip — the consumer's build catches errors.
- [ ] **Step 4: Run the admin type-check** `cd apps/admin && pnpm check-types`. Should still pass (nobody imports the new components yet).
- [ ] **Step 5: Commit**

```
git add packages/ui/src/status-dot.tsx packages/ui/src/price-display.tsx
git commit -m "feat(ui): promote StatusDot and PriceDisplay to @repo/ui (M7a)"
```

---

### Task 2: `apps/admin/lib/api/marketplace-api.ts` — typed client

**Files:**
- Create: `apps/admin/lib/api/marketplace-api.ts`

**Scope:** A typed fetch client mirroring `lib/api/platform-api.ts`'s shape. Exposes types for the admin products response + a `listProducts(storeId, query, sessionHeaders)` function. Reads `MARKETPLACE_API_URL` from env with a localhost default. Forwards `X-User-Id` / `X-Tenant-Id` headers from the server session.

```ts
// apps/admin/lib/api/marketplace-api.ts
//
// marketplace-api client for server components in the admin app.
//
// M7a ships only the admin products list endpoint. M7b adds product
// detail, create, update, delete. M7c adds media + variants. M7d adds
// copy-to-store. Categories are fetched for the inline picker in M7b.
//
// Calling convention: every server component that talks to marketplace-
// api receives the session headers from its caller (usually a page
// component) and passes them into the client functions. The client does
// the header rename dance (x-session-user-id → X-User-Id).

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

/** Session headers as they arrive from Next middleware. */
export interface SessionHeaders {
  userId: string;
  tenantId: string;
}

/** Admin product row as returned by GET /api/v1/admin/stores/:storeId/products. */
export interface AdminProduct {
  id: string;
  store_id: string;
  handle: string;
  title: string;
  description: string | null;
  status: "draft" | "active" | "archived";
  tags: string[];
  seo_title: string | null;
  seo_description: string | null;
  primary_category_id: string | null;
  copy_source_product_id: string | null;
  categories: AdminCategoryRef[];
  options: AdminProductOption[];
  variants: AdminVariantResponse[];
  media: AdminMediaResponse[];
  published_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface AdminCategoryRef {
  id: string;
  name: string;
  slug: string;
}

export interface AdminProductOption {
  id: string;
  name: string;
  position: number;
  values: AdminProductOptionValue[];
}

export interface AdminProductOptionValue {
  id: string;
  value: string;
  position: number;
}

export interface AdminVariantResponse {
  id: string;
  sku: string;
  barcode: string | null;
  price: string; // decimal as string
  compare_at_price: string | null;
  cost_price: string | null;
  currency_code: string;
  weight_grams: number | null;
  inventory_quantity: number;
  inventory_policy: "deny" | "continue";
  low_stock_threshold: number | null;
  option_values: AdminVariantOptionRef[];
  position: number;
}

export interface AdminVariantOptionRef {
  option_name: string;
  option_value_id: string;
  value: string;
}

export interface AdminMediaResponse {
  id: string;
  url: string;
  storage_key: string;
  alt: string | null;
  position: number;
  media_type: "image" | "video";
  width: number | null;
  height: number | null;
  bytes: number | null;
}

export interface ListProductsMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface ListProductsResponse {
  data: AdminProduct[];
  meta: ListProductsMeta;
}

export interface ListProductsQuery {
  status?: "draft" | "active" | "archived";
  search?: string;
  page?: number;
  pageSize?: number;
}

export interface ApiError {
  error: string;
  message: string;
  details?: Record<string, unknown>;
}

/**
 * Lists products under a store. Server-component only — the session
 * headers come from the caller's serverSession context.
 *
 * Returns `{ data, meta }` on success. On 401/403/404 returns null so
 * callers can render an empty or not-found state without try/catch
 * scaffolding. On unexpected errors throws so the Next error boundary
 * handles the rendering.
 */
export async function listProducts(
  storeId: string,
  query: ListProductsQuery,
  session: SessionHeaders,
): Promise<ListProductsResponse | null> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.search) params.set("search", query.search);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products${qs ? `?${qs}` : ""}`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: {
      "X-User-Id": session.userId,
      "X-Tenant-Id": session.tenantId,
      Accept: "application/json",
    },
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: listProducts ${res.status}: ${errBody?.message ?? "unknown error"}`,
    );
  }
  return (await res.json()) as ListProductsResponse;
}

// M7b will add: getProduct, createProduct, updateProduct, deleteProduct, copyProduct
// M7b will add: listCategories, createCategory, updateCategory, deleteCategory
// M7c will add: uploadUrl, createMedia, updateMedia, deleteMedia, updateVariantBasics
```

- [ ] **Step 1: Create the file verbatim.**
- [ ] **Step 2: Type-check** `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/apps/admin && pnpm check-types`. Must pass with zero errors.
- [ ] **Step 3: Commit**

```
git add apps/admin/lib/api/marketplace-api.ts
git commit -m "feat(admin): add marketplace-api client with listProducts for M7a"
```

---

### Task 3: Helper components — Summary, Empty, Pagination

**Files:**
- Create: `apps/admin/components/products/ProductsListSummary.tsx`
- Create: `apps/admin/components/products/ProductsListEmpty.tsx`
- Create: `apps/admin/components/products/ProductsListPagination.tsx`

**`ProductsListSummary.tsx`:** renders the muted "42 products · 3 drafts · 2 archived" line below the page title. Takes counts as props (active, draft, archived). Uses `@repo/ui/status-dot` for the small dots before each count — subtle visual separators.

Wait — the API doesn't return counts of each status in one call. For M7a we only have the total + what's on the current page. Either:
(a) Render just `42 products total` (simple; matches what the API gives us)
(b) Make three separate API calls (one per status) to get the counts — wasteful
(c) Add a `GET /admin/stores/:storeId/products/counts` endpoint — requires backend change

**Go with (a).** Simpler, honest about what the API provides. Spec's "42 products · 3 drafts · 2 archived" is a design aspiration; M7a renders `42 products total` with a note in the code that status breakdown ships in a later milestone when the backend exposes counts.

```tsx
// apps/admin/components/products/ProductsListSummary.tsx
import type { ListProductsMeta } from "@/lib/api/marketplace-api";

export interface ProductsListSummaryProps {
  meta: ListProductsMeta;
  statusFilter?: string;
}

export function ProductsListSummary({ meta, statusFilter }: ProductsListSummaryProps) {
  const noun = meta.total === 1 ? "product" : "products";
  const filterLabel = statusFilter ? ` · filtered by ${statusFilter}` : "";
  return (
    <p className="text-sm text-[color:var(--ink-900)] opacity-60">
      {meta.total.toLocaleString()} {noun} total{filterLabel}
    </p>
  );
}
```

**`ProductsListEmpty.tsx`:** two variants — `"no-products"` (zero total, big CTA) and `"no-matches"` (zero results but filters active, offers Clear filters link).

```tsx
// apps/admin/components/products/ProductsListEmpty.tsx
import Link from "next/link";

export interface ProductsListEmptyProps {
  variant: "no-products" | "no-matches";
  clearFiltersHref?: string;
}

export function ProductsListEmpty({ variant, clearFiltersHref }: ProductsListEmptyProps) {
  if (variant === "no-products") {
    return (
      <div className="flex flex-col items-start gap-4 py-16">
        <h2 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
          No products yet
        </h2>
        <p className="max-w-prose text-[color:var(--ink-900)] opacity-70">
          Your catalogue is empty. Add your first product to start selling —
          photos, variants, and pricing all in one place.
        </p>
        <Link
          href="/products/new"
          className="inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          + New product
        </Link>
      </div>
    );
  }
  return (
    <div className="flex flex-col items-start gap-3 py-12">
      <h2 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-2xl text-[color:var(--ink-900)]">
        No products match your filters
      </h2>
      <p className="max-w-prose text-[color:var(--ink-900)] opacity-70">
        Try adjusting your search or clearing the filters.
      </p>
      {clearFiltersHref && (
        <Link
          href={clearFiltersHref}
          className="text-sm text-[color:var(--moss-700)] underline-offset-4 hover:underline focus-visible:underline"
        >
          Clear filters
        </Link>
      )}
    </div>
  );
}
```

**`ProductsListPagination.tsx`:** numbered pagination. Prev/next + page numbers. Reads current page from props, constructs URLs with search params preserved.

```tsx
// apps/admin/components/products/ProductsListPagination.tsx
import Link from "next/link";
import { ChevronLeft, ChevronRight } from "lucide-react";

export interface ProductsListPaginationProps {
  currentPage: number;
  totalPages: number;
  /** Builds an href for the given target page, preserving other search params. */
  buildHref: (page: number) => string;
}

export function ProductsListPagination({
  currentPage,
  totalPages,
  buildHref,
}: ProductsListPaginationProps) {
  if (totalPages <= 1) return null;
  const pages = buildPageList(currentPage, totalPages);
  const prevDisabled = currentPage <= 1;
  const nextDisabled = currentPage >= totalPages;
  return (
    <nav
      className="flex items-center gap-1 pt-4"
      aria-label="Products pagination"
    >
      <PageButton
        href={prevDisabled ? undefined : buildHref(currentPage - 1)}
        disabled={prevDisabled}
        ariaLabel="Previous page"
      >
        <ChevronLeft className="h-4 w-4" />
      </PageButton>
      {pages.map((p, i) =>
        p === "…" ? (
          <span
            key={`ellipsis-${i}`}
            className="px-2 text-sm text-[color:var(--ink-900)] opacity-50"
            aria-hidden="true"
          >
            …
          </span>
        ) : (
          <PageButton
            key={p}
            href={p === currentPage ? undefined : buildHref(p)}
            current={p === currentPage}
            ariaLabel={`Page ${p}`}
          >
            {p}
          </PageButton>
        ),
      )}
      <PageButton
        href={nextDisabled ? undefined : buildHref(currentPage + 1)}
        disabled={nextDisabled}
        ariaLabel="Next page"
      >
        <ChevronRight className="h-4 w-4" />
      </PageButton>
    </nav>
  );
}

interface PageButtonProps {
  href?: string;
  children: React.ReactNode;
  disabled?: boolean;
  current?: boolean;
  ariaLabel: string;
}

function PageButton({ href, children, disabled, current, ariaLabel }: PageButtonProps) {
  const base =
    "inline-flex h-9 min-w-9 items-center justify-center rounded-md px-3 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]";
  const variants = current
    ? "bg-[color:var(--ink-900)] text-[color:var(--paper-200)]"
    : disabled
      ? "text-[color:var(--ink-900)] opacity-30"
      : "text-[color:var(--ink-900)] hover:bg-[color:var(--ink-900)] hover:bg-opacity-5";
  if (href) {
    return (
      <Link href={href} aria-label={ariaLabel} aria-current={current ? "page" : undefined} className={`${base} ${variants}`}>
        {children}
      </Link>
    );
  }
  return (
    <span
      aria-label={ariaLabel}
      aria-current={current ? "page" : undefined}
      aria-disabled={disabled || undefined}
      className={`${base} ${variants}`}
    >
      {children}
    </span>
  );
}

// Returns a list of page numbers + "…" ellipses for the pagination nav.
// Keeps at most 7 entries total: first, last, current, and neighbors.
function buildPageList(current: number, total: number): (number | "…")[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1);
  const pages: (number | "…")[] = [1];
  if (current > 3) pages.push("…");
  const start = Math.max(2, current - 1);
  const end = Math.min(total - 1, current + 1);
  for (let i = start; i <= end; i++) pages.push(i);
  if (current < total - 2) pages.push("…");
  pages.push(total);
  return pages;
}
```

- [ ] **Steps 1–3:** create all three files.
- [ ] **Step 4: Type-check** `cd apps/admin && pnpm check-types`. Clean.
- [ ] **Step 5: Commit**

```
git add apps/admin/components/products/ProductsListSummary.tsx apps/admin/components/products/ProductsListEmpty.tsx apps/admin/components/products/ProductsListPagination.tsx
git commit -m "feat(admin): add products list summary + empty + pagination components (M7a)"
```

---

### Task 4: Filters + Header components

**Files:**
- Create: `apps/admin/components/products/ProductsListFilters.tsx`
- Create: `apps/admin/components/products/ProductsListHeader.tsx`

**`ProductsListFilters.tsx`:** client component (`"use client"` directive) — searchbar + dropdown filter panel. Uses URL search params via `useSearchParams` + `useRouter.push`. Search is debounced 300ms to avoid thrashing.

Scope: Status filter live. Category and Stock rendered disabled with tooltips "Coming in M7d" / "Coming in M7b". Clear button visible only when a filter is active.

```tsx
"use client";

// apps/admin/components/products/ProductsListFilters.tsx

import { useRouter, useSearchParams, usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { Search, SlidersHorizontal, X } from "lucide-react";

export function ProductsListFilters() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const [searchDraft, setSearchDraft] = useState(searchParams.get("search") ?? "");
  const status = searchParams.get("status") ?? "";
  const hasFilters = !!status || !!searchDraft;

  // Debounce search input → URL navigation
  useEffect(() => {
    const handler = setTimeout(() => {
      const params = new URLSearchParams(searchParams.toString());
      if (searchDraft) {
        params.set("search", searchDraft);
      } else {
        params.delete("search");
      }
      params.delete("page"); // reset to page 1 on new search
      router.push(`${pathname}?${params.toString()}`);
    }, 300);
    return () => clearTimeout(handler);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchDraft]);

  const setStatus = (next: string) => {
    const params = new URLSearchParams(searchParams.toString());
    if (next) {
      params.set("status", next);
    } else {
      params.delete("status");
    }
    params.delete("page");
    router.push(`${pathname}?${params.toString()}`);
  };

  const clearAll = () => {
    setSearchDraft("");
    router.push(pathname);
  };

  return (
    <div className="flex flex-wrap items-center gap-3">
      <label className="relative flex-1 min-w-64">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[color:var(--ink-900)] opacity-50" aria-hidden="true" />
        <input
          type="search"
          value={searchDraft}
          onChange={(e) => setSearchDraft(e.target.value)}
          placeholder="Search products…"
          aria-label="Search products"
          className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] py-2 pl-10 pr-3 text-sm text-[color:var(--ink-900)] placeholder:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        />
      </label>
      <StatusFilterDropdown value={status} onChange={setStatus} />
      {hasFilters && (
        <button
          type="button"
          onClick={clearAll}
          className="inline-flex items-center gap-1 text-sm text-[color:var(--moss-700)] underline-offset-4 hover:underline focus-visible:underline"
        >
          <X className="h-3 w-3" aria-hidden="true" /> Clear
        </button>
      )}
    </div>
  );
}

function StatusFilterDropdown({ value, onChange }: { value: string; onChange: (next: string) => void }) {
  // Small native select for M7a. M7b+ upgrades to @tesserix/web filter-panel.
  return (
    <label className="inline-flex items-center gap-2 text-sm text-[color:var(--ink-900)]">
      <SlidersHorizontal className="h-4 w-4 opacity-60" aria-hidden="true" />
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label="Filter by status"
        className="rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] py-2 pl-3 pr-8 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        <option value="">All statuses</option>
        <option value="draft">Draft</option>
        <option value="active">Active</option>
        <option value="archived">Archived</option>
      </select>
    </label>
  );
}
```

**`ProductsListHeader.tsx`:** serves the editorial page header — serif title, summary, + New product button. Server component (pure display).

```tsx
// apps/admin/components/products/ProductsListHeader.tsx
import Link from "next/link";
import { Plus } from "lucide-react";

export interface ProductsListHeaderProps {
  canCreate: boolean;
}

export function ProductsListHeader({ canCreate }: ProductsListHeaderProps) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-4 border-b border-[color:var(--ink-900)] border-opacity-10 pb-6">
      <div className="flex flex-col gap-1">
        <span className="text-xs font-medium uppercase tracking-widest text-[color:var(--ink-900)] opacity-50">
          Catalogue
        </span>
        <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-4xl leading-tight text-[color:var(--ink-900)]">
          Products
        </h1>
      </div>
      {canCreate && (
        <Link
          href="/products/new"
          className="inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <Plus className="h-4 w-4" aria-hidden="true" />
          New product
        </Link>
      )}
    </header>
  );
}
```

- [ ] **Steps 1–2:** Create both files.
- [ ] **Step 3: Type-check + commit.**

```
git add apps/admin/components/products/ProductsListFilters.tsx apps/admin/components/products/ProductsListHeader.tsx
git commit -m "feat(admin): add products list filters + editorial header (M7a)"
```

---

### Task 5: `ProductsList.tsx` — the data-table composition

**Files:**
- Create: `apps/admin/components/products/ProductsList.tsx`

**Scope:** The centerpiece — renders the products table with hairline rules, 56px row height, columns per §7.2: checkbox (disabled — bulk actions are M7d), image (Paper placeholder), product (serif title + muted handle), status (StatusDot), stock (muted for drafts, moss vermillion for low), price (PriceDisplay), overflow menu.

Server component (pure render of the product array). Row composition uses a shared `<ProductRow>` inner component.

```tsx
// apps/admin/components/products/ProductsList.tsx
import Link from "next/link";
import Image from "next/image";
import { MoreHorizontal } from "lucide-react";

import { StatusDot } from "@repo/ui/status-dot";
import { PriceDisplay } from "@repo/ui/price-display";

import type { AdminProduct } from "@/lib/api/marketplace-api";

export interface ProductsListProps {
  products: AdminProduct[];
}

export function ProductsList({ products }: ProductsListProps) {
  return (
    <div className="w-full">
      <table
        className="w-full border-collapse text-sm"
        aria-label="Products"
      >
        <thead>
          <tr className="border-b border-[color:var(--ink-900)] border-opacity-10 text-left text-xs font-medium uppercase tracking-wide text-[color:var(--ink-900)] opacity-60">
            <th scope="col" className="w-10 py-3" aria-hidden="true" />
            <th scope="col" className="w-14 py-3" aria-hidden="true" />
            <th scope="col" className="py-3">Product</th>
            <th scope="col" className="w-32 py-3">Status</th>
            <th scope="col" className="w-32 py-3">Stock</th>
            <th scope="col" className="w-32 py-3">Price</th>
            <th scope="col" className="w-10 py-3" aria-hidden="true" />
          </tr>
        </thead>
        <tbody>
          {products.map((p) => (
            <ProductRow key={p.id} product={p} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ProductRow({ product }: { product: AdminProduct }) {
  const firstMedia = product.media[0];
  const variantCount = product.variants.length;
  const priceMin = product.variants.reduce<number | null>((acc, v) => {
    const n = Number.parseFloat(v.price);
    if (!Number.isFinite(n)) return acc;
    if (acc === null || n < acc) return n;
    return acc;
  }, null);
  const priceMax = product.variants.reduce<number | null>((acc, v) => {
    const n = Number.parseFloat(v.price);
    if (!Number.isFinite(n)) return acc;
    if (acc === null || n > acc) return n;
    return acc;
  }, null);
  const currency = product.variants[0]?.currency_code ?? "USD";
  const isVariantRange = priceMin !== null && priceMax !== null && priceMin !== priceMax;

  const stock = product.variants.reduce((sum, v) => sum + v.inventory_quantity, 0);
  const isLowStock = stock > 0 && stock <= 5;
  const isOutOfStock = stock === 0;

  return (
    <tr className="h-14 border-b border-[color:var(--ink-900)] border-opacity-5 transition-colors hover:bg-[color:var(--ink-900)] hover:bg-opacity-[0.02]">
      <td className="py-3">
        <input
          type="checkbox"
          aria-label={`Select ${product.title}`}
          disabled
          title="Bulk actions land in M7d"
          className="h-4 w-4 rounded border-[color:var(--ink-900)] border-opacity-30 disabled:opacity-30"
        />
      </td>
      <td className="py-3">
        <MediaThumb media={firstMedia} productTitle={product.title} />
      </td>
      <td className="py-3">
        <Link
          href={`/products/${product.id}`}
          className="group inline-flex flex-col focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <span className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-base text-[color:var(--ink-900)] group-hover:underline">
            {product.title}
          </span>
          <span className="text-xs text-[color:var(--ink-900)] opacity-50">
            /{product.handle}
          </span>
        </Link>
      </td>
      <td className="py-3">
        <StatusDot status={product.status} />
      </td>
      <td className="py-3">
        <StockCell stock={stock} variantCount={variantCount} isDraft={product.status === "draft"} isLowStock={isLowStock} isOutOfStock={isOutOfStock} />
      </td>
      <td className="py-3">
        {priceMin !== null ? (
          isVariantRange ? (
            <span className="text-[color:var(--ink-900)]">
              from <PriceDisplay amount={String(priceMin)} currencyCode={currency} />
            </span>
          ) : (
            <PriceDisplay amount={String(priceMin)} currencyCode={currency} />
          )
        ) : (
          <span className="text-[color:var(--ink-900)] opacity-40">—</span>
        )}
      </td>
      <td className="py-3 text-right">
        <button
          type="button"
          aria-label={`More actions for ${product.title}`}
          className="inline-flex h-8 w-8 items-center justify-center rounded-md text-[color:var(--ink-900)] opacity-60 hover:opacity-100 hover:bg-[color:var(--ink-900)] hover:bg-opacity-5 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
        </button>
      </td>
    </tr>
  );
}

function MediaThumb({ media, productTitle }: { media: { url: string; alt: string | null } | undefined; productTitle: string }) {
  if (!media) {
    return (
      <div
        className="h-10 w-10 rounded border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--paper-200)]"
        aria-hidden="true"
      />
    );
  }
  return (
    <div className="relative h-10 w-10 overflow-hidden rounded border border-[color:var(--ink-900)] border-opacity-10">
      {/* unoptimized to avoid the Next Image domain allowlist requirement on dev */}
      <Image src={media.url} alt={media.alt ?? productTitle} fill sizes="40px" unoptimized />
    </div>
  );
}

function StockCell({ stock, variantCount, isDraft, isLowStock, isOutOfStock }: { stock: number; variantCount: number; isDraft: boolean; isLowStock: boolean; isOutOfStock: boolean }) {
  if (isDraft) {
    return <span className="text-[color:var(--ink-900)] opacity-40">Draft</span>;
  }
  if (isOutOfStock) {
    return <span className="text-[color:var(--signal,#C23B22)]">Out of stock</span>;
  }
  if (isLowStock) {
    return <span className="text-[color:var(--signal,#C23B22)]">{stock} in stock</span>;
  }
  return (
    <span className="text-[color:var(--ink-900)]">
      {stock} {variantCount > 1 ? `across ${variantCount} variants` : "in stock"}
    </span>
  );
}
```

- [ ] **Step 1: Create the file.**
- [ ] **Step 2: Type-check.** Expect zero errors.
- [ ] **Step 3: Commit.**

```
git add apps/admin/components/products/ProductsList.tsx
git commit -m "feat(admin): add ProductsList data table with hairline rules + overflow menu (M7a)"
```

---

### Task 6: `apps/admin/app/products/page.tsx` rewrite

**Files:**
- Modify: `apps/admin/app/products/page.tsx`

**Scope:** Replace the `ComingSoon` stub with the real server component that wires everything together: reads session, reads search params, calls `listProducts`, renders header + filters + list + pagination + empty state.

```tsx
// apps/admin/app/products/page.tsx
import { headers } from "next/headers";

import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listProducts, type ListProductsQuery } from "@/lib/api/marketplace-api";

import { ProductsListHeader } from "@/components/products/ProductsListHeader";
import { ProductsListFilters } from "@/components/products/ProductsListFilters";
import { ProductsListSummary } from "@/components/products/ProductsListSummary";
import { ProductsList } from "@/components/products/ProductsList";
import { ProductsListPagination } from "@/components/products/ProductsListPagination";
import { ProductsListEmpty } from "@/components/products/ProductsListEmpty";

interface ProductsPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function ProductsPage({ searchParams }: ProductsPageProps) {
  const { tenantName, email, currentStore, role } = await getServerSessionContext();
  const params = await searchParams;

  const query = parseSearchParams(params);
  const canCreate = role === "owner" || role === "admin";

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="flex flex-col gap-6 px-8 py-6">
          <ProductsListHeader canCreate={false} />
          <ProductsListEmpty variant="no-products" />
        </main>
      </AdminShell>
    );
  }

  // Forward session headers to marketplace-api.
  const h = await headers();
  const response = await listProducts(currentStore.id, query, {
    userId: h.get("x-session-user-id") ?? "",
    tenantId: h.get("x-session-tenant-id") ?? "",
  });

  const products = response?.data ?? [];
  const meta = response?.meta ?? { page: 1, page_size: query.pageSize ?? 20, total: 0, total_pages: 0 };
  const hasActiveFilters = !!query.status || !!query.search;
  const isEmpty = products.length === 0;

  // For buildHref: preserve every non-page search param.
  const buildHref = (page: number) => {
    const p = new URLSearchParams();
    if (query.status) p.set("status", query.status);
    if (query.search) p.set("search", query.search);
    if (query.pageSize) p.set("page_size", String(query.pageSize));
    if (page > 1) p.set("page", String(page));
    const qs = p.toString();
    return qs ? `/products?${qs}` : "/products";
  };

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="flex flex-col gap-6 px-8 py-6" aria-labelledby="products-heading">
        <ProductsListHeader canCreate={canCreate} />
        <div className="flex flex-col gap-4">
          <ProductsListFilters />
          <ProductsListSummary meta={meta} statusFilter={query.status} />
        </div>

        {isEmpty ? (
          <ProductsListEmpty
            variant={hasActiveFilters ? "no-matches" : "no-products"}
            clearFiltersHref={hasActiveFilters ? "/products" : undefined}
          />
        ) : (
          <>
            <ProductsList products={products} />
            <ProductsListPagination
              currentPage={meta.page}
              totalPages={meta.total_pages}
              buildHref={buildHref}
            />
          </>
        )}
      </main>
    </AdminShell>
  );
}

function parseSearchParams(raw: Record<string, string | string[] | undefined>): ListProductsQuery {
  const status = typeof raw.status === "string" ? raw.status : undefined;
  const search = typeof raw.search === "string" ? raw.search : undefined;
  const page = typeof raw.page === "string" ? Number.parseInt(raw.page, 10) : undefined;
  const pageSize = typeof raw.page_size === "string" ? Number.parseInt(raw.page_size, 10) : undefined;
  const validStatus =
    status === "draft" || status === "active" || status === "archived" ? status : undefined;
  return {
    status: validStatus,
    search: search || undefined,
    page: page && page > 0 ? page : undefined,
    pageSize: pageSize && pageSize > 0 && pageSize <= 100 ? pageSize : undefined,
  };
}
```

- [ ] **Step 1: Replace the existing page file.**
- [ ] **Step 2: Type-check.**
- [ ] **Step 3: Commit.**

```
git add apps/admin/app/products/page.tsx
git commit -m "feat(admin): rewrite products page as real list with server fetch (M7a)"
```

---

### Task 7: `error.tsx` + `/products/new` stub

**Files:**
- Create: `apps/admin/app/products/error.tsx`
- Create: `apps/admin/app/products/new/page.tsx`

**`error.tsx`:** minimal editorial error boundary.

```tsx
"use client";

// apps/admin/app/products/error.tsx
import { useEffect } from "react";

export default function Error({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error("products page error", error);
  }, [error]);
  return (
    <main className="flex flex-col gap-4 px-8 py-16" aria-live="polite">
      <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
        Couldn't load your products
      </h1>
      <p className="max-w-prose text-[color:var(--ink-900)] opacity-70">
        Something went wrong on our side. Try again, or come back in a moment.
      </p>
      <button
        type="button"
        onClick={() => reset()}
        className="inline-flex w-fit items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        Try again
      </button>
    </main>
  );
}
```

**`new/page.tsx`:** `ComingSoon` stub matching the existing pattern.

```tsx
// apps/admin/app/products/new/page.tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { ComingSoon } from "@/components/shell/ComingSoon";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export default async function NewProductPage() {
  const { tenantName, email } = await getServerSessionContext();
  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <ComingSoon
        title="New product"
        description="The detail page with title, description, categories, media, and variants lands in the next slice."
        eta="M7b"
      />
    </AdminShell>
  );
}
```

- [ ] Create both files, commit.

```
git add apps/admin/app/products/error.tsx apps/admin/app/products/new/page.tsx
git commit -m "feat(admin): add products error boundary + new product stub (M7a)"
```

---

### Task 8: Playwright E2E test

**Files:**
- Create: `apps/admin/tests/e2e/products-list.spec.ts`

**Scope:** Playwright test that exercises the list page. Requires the full stack running (admin + marketplace-api + postgres + openfga). Uses the existing Playwright harness at `apps/admin/tests/e2e/`.

**Before writing the test, read `apps/admin/playwright.config.ts` and existing test files to understand the harness.** If the harness already provides a logged-in session helper, use it. If not, intercept cookie/header setup at the test level.

Test cases:
1. `Products page renders empty state when no products exist` — mock or seed an empty state via a dedicated test store, navigate, assert empty copy + CTA.
2. `Products list renders rows for each product` — seed 3 products via admin API, navigate, assert 3 rows.
3. `Clicking + New product navigates to /products/new` — seed a product, click the CTA, assert URL and stub page.
4. `Search filter narrows results` — seed 3 products with different titles, type in search box, wait for debounce, assert filtered.
5. `Status dropdown filters results` — seed draft + active products, select "Active" → only 1 visible.
6. `Clear filters link resets the URL` — apply a filter, click Clear, assert URL back to /products.
7. `Pagination navigates to page 2` — seed 25 products, click "2", assert URL has page=2.
8. `Overflow menu button is accessible by role` — assert the button has the right aria-label.
9. `Staff role hides + New product button` — set session as staff, navigate, assert CTA absent.

Targets the test to run in CI against a real stack. If the existing Playwright harness is more limited (e.g., stubs fetch), adapt.

Commit: `test(admin): Playwright E2E for products list page (M7a)`

---

### Task 9: Verification + PR

- [ ] **Step 1: Full check**

```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly && \
  pnpm -F @mark8ly/admin check-types && \
  pnpm -F @mark8ly/admin build && \
  pnpm -F @repo/ui check-types
```

Types clean. Build clean.

- [ ] **Step 2: Manual smoke (optional — only if stack is running)**

```
cd apps/admin && pnpm dev
# open http://localhost:4202/products — should render the page
```

- [ ] **Step 3: Run Playwright**

```
cd apps/admin && pnpm test:e2e products-list
```

Skip this step if the full stack isn't running locally; CI will exercise it. Report the test count either way.

- [ ] **Step 4: Push + PR**

```
git push -u origin feat/products-m7a-admin-list
gh pr create --base main --head feat/products-m7a-admin-list --title "feat(admin): products M7a — admin UI sidebar + list page" --body "..."
```

PR body covers: editorial page header, server-side data fetch, StatusDot + PriceDisplay promoted to @repo/ui, URL-driven filter/pagination state, cosmetic role gating, Playwright coverage, what's NOT in (detail page, variants, media, categories, copy dialog — all M7b/c/d).

---

## Exit criteria

- [ ] `/products` renders a real list fetched from marketplace-api (no ComingSoon)
- [ ] Editorial header with serif title + `+ New product` CTA (cosmetically gated by role)
- [ ] Search + status filter work via URL params
- [ ] Pagination navigates across pages
- [ ] Empty state has two variants (no-products, no-matches)
- [ ] `StatusDot` and `PriceDisplay` live in `@repo/ui` and are consumed by the page
- [ ] Error boundary renders editorial error state
- [ ] `/products/new` is a ComingSoon stub
- [ ] `pnpm check-types` clean across `@mark8ly/admin` and `@repo/ui`
- [ ] `pnpm build` clean for `@mark8ly/admin`
- [ ] Playwright test suite passes against the real stack
- [ ] No changes to marketplace-api Go code, `go.mod`, migrations, or Helm charts
- [ ] PR is open and CI is green

---

## Estimated effort

| Task | Effort |
|---|---|
| 1. `@repo/ui` StatusDot + PriceDisplay | 45 min |
| 2. marketplace-api client | 60 min |
| 3. Summary + Empty + Pagination components | 75 min |
| 4. Filters + Header components | 75 min |
| 5. ProductsList data-table | 90 min |
| 6. products/page.tsx rewrite | 60 min |
| 7. error.tsx + new/page.tsx stub | 20 min |
| 8. Playwright test | 90 min |
| 9. Verification + PR | 30 min |
| **Total** | **~9 hours** |

Same wall time as the backend slices. Frontend work tends to have a similar shape — foundation pieces, composition, tests, polish.
