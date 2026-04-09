# Products M7b — Admin UI: Product Detail Page (Simple Products) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `/products/new` ComingSoon stub with the real create form, add `/products/[id]` for the edit form, and ship the **simple product** create/update/delete lifecycle end-to-end through the admin UI. Variants and media belong to M7c; copy-to-store and the category drawer belong to M7d. M7b's exit is "a merchant can create, edit, publish, and delete a simple product with at least one category link via the admin UI, and the change round-trips to marketplace-api."

**Architecture:** Two new server-component pages (`/products/new`, `/products/[id]`) that render a shared `ProductForm` client component. The form uses `react-hook-form` (already in `apps/admin/package.json`) + Zod schema validation and submits via a Next.js 16 server action that calls `marketplace-api` through the extended `lib/api/marketplace-api.ts` client. The category picker is a client-side `@tesserix/web` `combobox` bound to the category list fetched server-side on page load. Create, update, and delete all go through thin server-action wrappers that forward the session headers. Cosmetic role gating matches M7a — staff see read-only, admin sees full CRUD, owner additionally sees the Delete option.

**Tech Stack:** Next.js 16 (server actions), React 19, `react-hook-form` v7.72, `zod` v4.3, Tailwind CSS v4, `@tesserix/web` v1.7.1 primitives, `@repo/ui` shared components, TypeScript, Playwright.

---

## Status

> **Pending.** All tasks open.

---

## Scope check

Single contained slice in `apps/admin`. Extends `lib/api/marketplace-api.ts` with five new endpoints (`getProduct`, `createProduct`, `updateProduct`, `deleteProduct`, `listCategories`). Adds two server-action files, one new client component (`ProductForm`), one new server component (`ProductCategoriesPicker`), two page rewrites (`/products/new`, `/products/[id]/page.tsx`), and one Playwright spec. No changes to marketplace-api Go code, no new npm deps, no Helm chart changes.

Spec sections authoritative for this milestone:
- §7.3 — product detail page (M7b delivers the simple-product subset; variants deferred to M7c)
- §7.4 — inline category picker (M7b ships the combobox against existing categories; **inline `+ Create` deferred to a follow-up** to keep this PR focused)
- §7.8 — role-based UX gates
- §7.9 — accessibility baseline
- §8 M7b entry: "Title, description, status, categories with inline picker, SEO, sticky action bar, unsaved guard. Create and simple-update flows working."
- §13.5 UX corrections (no dialogs except hard delete; delete gets a confirm-dialog; everything else resolves inline via banners + sticky bars)

---

## Deliberate cuts from §7.3

To ship a clean M7b in one session-sized slice, these items are **explicitly deferred** and documented in the PR description:

1. **Rich text editor** — spec calls for `rich-text-editor`, M7b ships a plain `<textarea>` with a `max-length: 5000` limit and a "Plain text only for now; rich text lands in a follow-up" helper line. The backend sanitizer (bluemonday, M3) handles any HTML if the client ever sends it.
2. **Inline `+ Create "<text>"` on the category picker** — the combobox filters existing categories but cannot create a new one. Merchants bounce to M7d's category drawer for that. Document as a follow-up.
3. **SEO section** — the API supports `seo_title` / `seo_description`, and the DTO maps them, but the form UI doesn't render a SEO section in M7b. The fields are carried through unchanged on edit (the server action preserves whatever was already there). Deferred to a polish PR between M7c and M7d.
4. **Sticky action bar** — M7b renders a plain action row at the bottom of the form: moss `Save` button + ghost `Discard` link back to `/products`. Spec's sticky-on-scroll behavior is a 30-min follow-up once the core flow works.
5. **Unsaved-changes guard** — the `confirm-dialog` that blocks navigation when the form is dirty. M7b relies on `react-hook-form`'s `formState.isDirty` but does not block navigation. Deferred.
6. **Handle live-preview of `mark8ly.com/<handle>`** — shown as plain helper text under the input, not a real preview box. Stretch goal for a polish PR.
7. **Media grid** — M7c.
8. **Options + variant matrix editor** — M7c. M7b enforces "simple product only" by sending `options: [], variants: [one row]` on create.
9. **Copy-to-store overflow menu item** — M7d. The form's overflow menu shows only Archive (admin) / Delete (owner).

The scope split keeps M7b focused on the **simple-product round-trip** and lets M7c + M7d ship their own clean PRs without rework.

---

## Decisions locked

1. **Server actions, not client-side fetch.** Create / update / delete all go through Next.js server actions defined in `apps/admin/app/products/actions.ts`. The actions read the session, call `marketplace-api`, then `redirect` on success or `return { error }` on typed failures. Matches the existing pattern in `apps/admin/app/settings/general/actions.ts`.
2. **`ProductForm` is a single shared client component** for both create and edit. Takes an optional `initialProduct?: AdminProduct` prop — when absent it renders the create form; when present it pre-fills fields and flips the action target to `updateProduct`. Same component, two pages.
3. **Zod schema defined once** in `apps/admin/lib/validation/product-form.ts`, used by the client form (`react-hook-form`'s `zodResolver`) AND by the server action for defense-in-depth validation. Single source of truth for field constraints.
4. **Simple-product invariant**: the form has no Options/Variants UI in M7b, and the create action constructs a `variants: [{ sku, price, inventoryQuantity, ... }]` array with one entry from the form's Price + Stock fields. On update, if the existing product already has >1 variant (e.g., created via API or via a future M7c edit), the form renders a banner saying "This product has variants. Open the variant editor" with a disabled price/stock field pair. Banner copy includes a link to `/products/:id` with `?view=variants` (M7c feature — for M7b it just redirects back to the list).
5. **Category picker uses `@tesserix/web`'s `combobox` primitive** if it exists, otherwise a native `<select multiple>` as a fallback. Check `node_modules/@tesserix/web` for the export first; degrade gracefully if absent (document the fallback in the plan's Task 4).
6. **Price input is a plain `<input type="number" step="0.01">`** for M7b. The `MoneyInput` promoted component lands in M7c where multi-currency actually matters.
7. **Delete flow is a confirm-dialog** (§13.5: only hard delete gets a dialog). Uses `@tesserix/web`'s `confirm-dialog` or a lightweight inline variant. Delete is only visible to owner role; admin sees only Archive.
8. **Archive = status update, not delete.** "Archive" in the overflow menu PATCHes `status: archived`. No dialog.
9. **Publish = status update to `active`**, done via the form's status select — not a separate button.
10. **Server action return shape**: `{ ok: true, productId } | { ok: false, error: { code, message, field? } }`. The client form catches the error and displays it inline via react-hook-form's `setError` on the named field (or a root-level form error for unknown codes).
11. **`listCategories` returns all categories for the store** (active + inactive, excluding soft-deleted). The picker filters to active categories only at render time. Inactive categories remain visible in `ProductCategoriesPicker` if the product is already linked to them (so editing an existing product with an inactive category doesn't silently drop it).
12. **No optimistic updates.** Create/update redirect on success; delete redirects to `/products`. Skeleton/loading states come from Next's `loading.tsx` sibling (ship a small one alongside the page).
13. **URL segments**: create page is `/products/new`, edit page is `/products/[id]`. The `[id]` is the product UUID, matching the spec §6.1 route shape.

---

## File structure produced by M7b

```
apps/admin/
├── lib/api/marketplace-api.ts                         MODIFIED: add getProduct, createProduct, updateProduct, deleteProduct, listCategories
├── lib/validation/
│   └── product-form.ts                                NEW: Zod schema for the form; shared between client + server action
├── app/products/
│   ├── actions.ts                                     NEW: createProductAction, updateProductAction, deleteProductAction
│   ├── loading.tsx                                    NEW: small editorial skeleton for the list + detail pages
│   ├── new/
│   │   ├── page.tsx                                   REWRITE: fetch categories, render ProductForm in "create" mode
│   │   └── error.tsx                                  NEW: route-local error boundary using EditorialError
│   └── [id]/
│       ├── page.tsx                                   NEW: fetch product + categories, render ProductForm in "edit" mode
│       ├── not-found.tsx                              NEW: editorial not-found screen
│       └── error.tsx                                  NEW: route-local error boundary
├── components/products/
│   ├── ProductForm.tsx                                NEW: client component with react-hook-form + zod; title, handle, description, status, price, stock, categories
│   ├── ProductFormActions.tsx                         NEW: the bottom action row (Save, Discard, overflow menu with Archive/Delete/Copy-TBD)
│   ├── ProductCategoriesPicker.tsx                    NEW: category combobox + chip row
│   └── ProductNotFound.tsx                            NEW: shared not-found state
└── tests/e2e/
    └── products-detail.spec.ts                        NEW: Playwright — create, edit, archive, delete flows end-to-end
```

Target sizes: `ProductForm.tsx` ~350 lines (the largest), everything else under 200.

---

## New Go/npm dependencies

**None.** `react-hook-form`, `zod`, `@hookform/resolvers`, `@tesserix/web`, `@repo/ui`, `lucide-react` are all already in `apps/admin/package.json`.

---

## Landmines

- **Landmine #1 (go.work)** — no new Go module, confirm empty `git diff go.work`.
- **CWD drift** — absolute `cd` on every bash command.
- **(NEW) server action session context** — server actions run outside the request/response cycle of a given page component, but `getServerSessionContext()` reads from `headers()` which Next 16 exposes inside server actions. Verify the `headers()` call works from within `app/products/actions.ts` before committing to the pattern. The existing `apps/admin/app/settings/general/actions.ts` already uses this pattern, so cross-reference before writing new code.
- **(NEW) Next 16 server action `redirect()`** — calling `redirect()` from a server action throws a special `NEXT_REDIRECT` error that must NOT be caught by a generic `try/catch`. Either don't wrap, or re-throw errors matching `isRedirectError`.
- **(NEW) Category fetch shape** — the M5b `GET /admin/stores/:storeId/categories` endpoint returns `{"data": [...]}` per the `CategoryHandler.List` M5b implementation. Do NOT assume a `meta` envelope; categories are a flat list in M5b.
- **(NEW) Simple-product variant creation** — M3's `product.Service.Create` requires `Variants: []VariantInput` with at least 1 entry. For a simple product M7b synthesizes ONE variant with a default SKU (`<handle>-default`), the form's price, stock, and currency from the store. The service then runs the matrix validator in "no options" mode which the M3 matrix validator already handles correctly.

---

## Task decomposition

10 tasks. Tasks 1–4 build the foundations (API client, schema, server actions, picker). Tasks 5–7 compose the form + pages. Tasks 8–9 ship tests + error boundaries. Task 10 closes out.

| # | Task | Approx effort |
|---|---|---|
| 1 | Extend `lib/api/marketplace-api.ts` with 5 new endpoints | 60 min |
| 2 | `lib/validation/product-form.ts` — Zod schema | 30 min |
| 3 | `app/products/actions.ts` — server actions | 90 min |
| 4 | `components/products/ProductCategoriesPicker.tsx` | 60 min |
| 5 | `components/products/ProductForm.tsx` + `ProductFormActions.tsx` | 2 hours |
| 6 | `app/products/new/page.tsx` rewrite | 45 min |
| 7 | `app/products/[id]/page.tsx` + `not-found.tsx` + `loading.tsx` | 60 min |
| 8 | Route-local `error.tsx` boundaries | 15 min |
| 9 | Playwright spec: create + edit + archive + delete flows | 90 min |
| 10 | Verification + PR | 30 min |
| **Total** | | **~9 hours** |

---

### Task 1: Extend `marketplace-api.ts` with product + category endpoints

**Files:**
- Modify: `apps/admin/lib/api/marketplace-api.ts`

**Scope:** Add 5 new functions matching the marketplace-api admin surface from M5a/M5b. All take the `SessionHeaders` struct and forward `X-User-Id` / `X-Tenant-Id`. Return `null` on 4xx-user-error, throw on unexpected errors.

```ts
// New types:
export interface CreateProductRequest {
  handle?: string;
  title: string;
  description?: string;
  status?: "draft" | "active" | "archived";
  tags?: string[];
  seo_title?: string;
  seo_description?: string;
  primary_category_id?: string;
  options?: Array<{ name: string; values: string[] }>;
  variants: Array<{
    sku: string;
    barcode?: string;
    price: string;
    compare_at_price?: string;
    cost_price?: string;
    currency_code: string;
    weight_grams?: number;
    inventory_quantity: number;
    inventory_policy?: "deny" | "continue";
    low_stock_threshold?: number;
    option_values?: Array<{ option_name: string; value: string }>;
    position?: number;
  }>;
  media?: Array<{
    storage_key: string;
    url: string;
    alt?: string;
    position?: number;
    media_type?: "image" | "video";
  }>;
  category_ids?: string[];
}

export type UpdateProductRequest = Partial<CreateProductRequest> & {
  /** Identifier passed in the URL, not the body. Keep at the call site. */
};

// New functions — all receive (storeId, body | id, session) and return the
// parsed response or throw ApiError with the typed code preserved.

export async function getProduct(
  storeId: string,
  productId: string,
  session: SessionHeaders,
): Promise<AdminProduct | null>

export async function createProduct(
  storeId: string,
  body: CreateProductRequest,
  session: SessionHeaders,
): Promise<AdminProduct | { error: ApiError }>

export async function updateProduct(
  storeId: string,
  productId: string,
  body: Partial<CreateProductRequest>,
  session: SessionHeaders,
): Promise<AdminProduct | { error: ApiError }>

export async function deleteProduct(
  storeId: string,
  productId: string,
  session: SessionHeaders,
): Promise<true | { error: ApiError }>

export interface AdminCategory {
  id: string;
  parent_id: string | null;
  name: string;
  slug: string;
  description: string | null;
  image_url: string | null;
  position: number;
  is_active: boolean;
}

export interface ListCategoriesResponse {
  data: AdminCategory[];
}

export async function listCategories(
  storeId: string,
  session: SessionHeaders,
): Promise<AdminCategory[]>
```

**Important:** for create/update/delete, **preserve the typed error from the API** so the server action and form can surface `handle_taken`, `sku_taken`, `validation_failed`, `not_found`, `forbidden`, etc. with field-level detail. The existing M7a `listProducts` returns `null` on 4xx which is fine for read paths, but mutations need the error code.

**Steps:**
- [ ] Read the current `marketplace-api.ts` to understand the existing error handling pattern.
- [ ] Add the new types + 5 functions appended to the bottom of the file.
- [ ] Type-check: `cd apps/admin && pnpm check-types`.
- [ ] Commit: `feat(admin): extend marketplace-api client with product + category CRUD (M7b)`

---

### Task 2: `lib/validation/product-form.ts` — Zod schema

**Files:**
- Create: `apps/admin/lib/validation/product-form.ts`

```ts
import { z } from "zod";

// Shared between the client form (zodResolver) and the server action
// (second-pass validation). Keep every constraint here; the form pulls
// error messages from Zod's issue format.

export const productFormSchema = z.object({
  title: z.string().trim().min(1, "Title is required").max(300, "Title is too long"),
  handle: z
    .string()
    .trim()
    .max(200, "Handle is too long")
    .regex(/^[a-z0-9-]*$/, "Handle may only contain lowercase letters, numbers, and dashes")
    .optional()
    .or(z.literal("")),
  description: z.string().max(5000, "Description is too long").optional().or(z.literal("")),
  status: z.enum(["draft", "active", "archived"]),
  price: z
    .string()
    .regex(/^\d+(\.\d{1,2})?$/, "Enter a valid price, e.g. 19.99"),
  inventoryQuantity: z
    .string()
    .regex(/^\d+$/, "Enter a whole number")
    .transform((v) => Number.parseInt(v, 10)),
  sku: z.string().trim().max(100, "SKU is too long").optional().or(z.literal("")),
  categoryIds: z.array(z.string().uuid()).default([]),
});

export type ProductFormValues = z.infer<typeof productFormSchema>;

/**
 * The shape of form input BEFORE Zod transforms apply (inventoryQuantity
 * is still a string). Used by the client form before submission.
 */
export type ProductFormInput = {
  title: string;
  handle?: string;
  description?: string;
  status: "draft" | "active" | "archived";
  price: string;
  inventoryQuantity: string;
  sku?: string;
  categoryIds: string[];
};
```

Commit: `feat(admin): add product form Zod schema (M7b)`

---

### Task 3: `app/products/actions.ts` — server actions

**Files:**
- Create: `apps/admin/app/products/actions.ts`

**Scope:** Three server actions: `createProductAction`, `updateProductAction`, `deleteProductAction`. Each:
1. `"use server"` directive at the top of the file
2. Read session via `getServerSessionContext()`
3. Parse + validate the form payload with `productFormSchema`
4. Build the marketplace-api request body (synthesize the single-variant for simple products)
5. Call the appropriate marketplace-api client function
6. On typed API error: return `{ ok: false, error: { code, message, field? } }`
7. On success: `redirect()` to the target page (create → `/products/<newId>`; update → `/products/<id>`; delete → `/products`)

**Key pattern — synthesizing the variant for a simple product:**

```ts
const currency = session.currentStore?.currency_code ?? "USD";
const body: CreateProductRequest = {
  handle: values.handle || undefined,
  title: values.title,
  description: values.description || undefined,
  status: values.status,
  category_ids: values.categoryIds,
  options: [],
  variants: [
    {
      sku: values.sku || `${slugFromTitle(values.title)}-default`,
      price: values.price,
      currency_code: currency,
      inventory_quantity: values.inventoryQuantity,
      inventory_policy: "deny",
      position: 0,
    },
  ],
};
```

`slugFromTitle` is a pure helper that lowercases and replaces spaces/punct with dashes — keep the implementation minimal.

**Redirect handling:** call `redirect()` OUTSIDE any try/catch that might swallow it (Next's `isRedirectError` helper from `next/navigation` is the safe way to re-throw if you do catch).

Commit: `feat(admin): add product create/update/delete server actions (M7b)`

---

### Task 4: `ProductCategoriesPicker.tsx`

**Files:**
- Create: `apps/admin/components/products/ProductCategoriesPicker.tsx`

**Scope:** Client component that takes the full category list + the currently-selected ids + an `onChange` callback. Renders:
- Chip row showing selected categories with an X button to remove
- A combobox (or `<select>` fallback) to search and add

**Implementation choice — combobox or fallback:**

First check what `@tesserix/web` exports. If a `Combobox` component exists, use it. Otherwise ship a small custom implementation: a text input with a dropdown list that filters as the user types, keyboard-navigable with arrows + enter.

For M7b the simpler fallback (filterable dropdown) is acceptable. Full accessible combobox with aria-combobox is a polish pass.

**Accessibility:**
- `aria-label="Add category"` on the input
- Remove buttons on chips have `aria-label="Remove {categoryName}"`
- Enter key on dropdown selects
- Escape closes the dropdown

Commit: `feat(admin): add product categories picker (M7b)`

---

### Task 5: `ProductForm.tsx` + `ProductFormActions.tsx`

**Files:**
- Create: `apps/admin/components/products/ProductForm.tsx`
- Create: `apps/admin/components/products/ProductFormActions.tsx`

**`ProductForm.tsx`** — the big one. Client component using `react-hook-form` with `zodResolver(productFormSchema)`. Props:

```ts
interface ProductFormProps {
  mode: "create" | "edit";
  storeId: string;
  initialProduct?: AdminProduct;
  categories: AdminCategory[];
  currencyCode: string;
  canDelete: boolean;
  canArchive: boolean;
}
```

Sections (in order, single column):
1. **Header strip:** back link to `/products`, serif title (product title on edit; "New product" on create), muted handle
2. **Title + Handle + Description** — stacked inputs
3. **Status** — select with Draft / Active / Archived
4. **Categories** — the `ProductCategoriesPicker`
5. **Pricing / Inventory** — price + stock + sku row
6. **Actions** — the `ProductFormActions` row

On submit, calls the appropriate server action (create or update) and handles `{ ok: false, error }` returns by calling `form.setError` on the right field.

**Multi-variant warning:** if `initialProduct?.variants.length > 1`, render a banner above the Pricing section: "This product has variants. Price and stock live in the variant editor." Disable the Pricing inputs.

**`ProductFormActions.tsx`** — the action row: Save primary button, Discard link, overflow menu (DropdownMenu from @tesserix/web) with Archive (admin+) and Delete (owner) items.

Commit: `feat(admin): add product form + actions (M7b)`

---

### Task 6: `/products/new/page.tsx` rewrite

**Files:**
- Modify: `apps/admin/app/products/new/page.tsx`

**Scope:** Server component that reads session, fetches categories, renders `<ProductForm mode="create" ... />`. No `ComingSoon` anymore.

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listCategories } from "@/lib/api/marketplace-api";
import { ProductForm } from "@/components/products/ProductForm";
import { ProductNotFound } from "@/components/products/ProductNotFound";

export default async function NewProductPage() {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <ProductNotFound title="No store selected" />
      </AdminShell>
    );
  }

  const categories = await listCategories(currentStore.id, { userId, tenantId });

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-3xl px-8 py-8">
        <ProductForm
          mode="create"
          storeId={currentStore.id}
          categories={categories}
          currencyCode={currentStore.currency_code}
          canDelete={false}
          canArchive={role === "owner" || role === "admin"}
        />
      </main>
    </AdminShell>
  );
}
```

Commit: `feat(admin): wire new product page to ProductForm (M7b)`

---

### Task 7: `/products/[id]/page.tsx` + `not-found.tsx` + `loading.tsx`

**Files:**
- Create: `apps/admin/app/products/[id]/page.tsx`
- Create: `apps/admin/app/products/[id]/not-found.tsx`
- Create: `apps/admin/app/products/loading.tsx` (applies to both list + detail)
- Create: `apps/admin/components/products/ProductNotFound.tsx`

**`[id]/page.tsx`:**

```tsx
import { notFound } from "next/navigation";
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getProduct, listCategories } from "@/lib/api/marketplace-api";
import { ProductForm } from "@/components/products/ProductForm";

interface PageProps {
  params: Promise<{ id: string }>;
}

export default async function ProductDetailPage({ params }: PageProps) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;
  if (!currentStore) notFound();

  const [product, categories] = await Promise.all([
    getProduct(currentStore.id, id, { userId, tenantId }),
    listCategories(currentStore.id, { userId, tenantId }),
  ]);
  if (!product) notFound();

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-3xl px-8 py-8">
        <ProductForm
          mode="edit"
          storeId={currentStore.id}
          initialProduct={product}
          categories={categories}
          currencyCode={currentStore.currency_code}
          canDelete={role === "owner"}
          canArchive={role === "owner" || role === "admin"}
        />
      </main>
    </AdminShell>
  );
}
```

**`not-found.tsx`** calls `<ProductNotFound />` inside the shell.

**`loading.tsx`** renders a small skeleton (a few hairline-bordered placeholder rows) so navigation feels instant.

Commit: `feat(admin): add product detail page + not-found + loading (M7b)`

---

### Task 8: Route-local error boundaries

**Files:**
- Create: `apps/admin/app/products/new/error.tsx`
- Create: `apps/admin/app/products/[id]/error.tsx`

Both just render `<EditorialError title="Couldn't load this product" onRetry={reset} />` from `@repo/ui/editorial-error` (shipped in M7a).

Commit: `feat(admin): add product detail error boundaries (M7b)`

---

### Task 9: Playwright spec

**Files:**
- Create: `apps/admin/tests/e2e/products-detail.spec.ts`

**Scope:** Exercise the create → edit → archive → delete lifecycle end-to-end against the real stack. Patterns from the M7a `products-list.spec.ts` and the `settings-general.spec.ts` reference.

Cases:
1. `create a simple product and it appears on the list`
2. `edit the title and it round-trips after reload`
3. `archive from the overflow menu updates the status on the list`
4. `delete removes the product from the list and redirects home`
5. `validation error on empty title renders inline`

Seed via the admin form itself — no raw SQL. The test onboards a fresh merchant via the existing `completeOnboarding` helper, signs in, and drives the UI.

Commit: `test(admin): Playwright for create/edit/archive/delete flows (M7b)`

---

### Task 10: Verification + PR

- [ ] `pnpm check-types && pnpm build` from `apps/admin/`
- [ ] Push branch + open PR
- [ ] Merge after CI green

---

## Exit criteria

- [ ] `/products/new` renders the real create form
- [ ] `/products/[id]` renders the real edit form with pre-filled data
- [ ] Create → new product appears on `/products` list
- [ ] Update → changes round-trip after reload
- [ ] Archive → status flips to archived, product still appears in list with archived dot
- [ ] Delete → product removed from list (owner only)
- [ ] Validation errors render inline via react-hook-form + Zod
- [ ] Typed API errors (`handle_taken`, `sku_taken`) render field-level on the form
- [ ] Role gating: staff sees read-only, admin sees Archive, owner sees Delete
- [ ] `pnpm check-types` clean
- [ ] `pnpm build` clean
- [ ] Playwright spec passes against the real stack
- [ ] No changes to marketplace-api Go code
- [ ] PR open, CI green

---

## Estimated effort

| Task | Effort |
|---|---|
| 1. API client extensions | 60 min |
| 2. Zod schema | 30 min |
| 3. Server actions | 90 min |
| 4. Categories picker | 60 min |
| 5. ProductForm + actions | 2 hours |
| 6. /products/new rewrite | 45 min |
| 7. /products/[id] + not-found + loading | 60 min |
| 8. Error boundaries | 15 min |
| 9. Playwright spec | 90 min |
| 10. Verification + PR | 30 min |
| **Total** | **~9 hours** |

Comparable to M7a. Core value is the simple-product create + update + delete round-trip; everything else from §7.3 (variants, media, rich text, SEO, sticky bar, unsaved guard, inline category create) is explicitly deferred and tracked in the PR description.
