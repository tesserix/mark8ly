# mobile-admin — API contract foundation (sub-project A) — Design

**Date:** 2026-07-15
**Status:** Approved
**Scope:** `packages/mobile-shared/api` + `apps/mobile-admin` dashboard/stores only

## Goal

Make the mobile app talk to prod successfully for the first time: sign in → store picker →
**a working dashboard**. Establish the contract machinery (wire-truthful zod schemas, inferred
types, loud failures) that makes sub-projects B–D cheap instead of archaeology.

## Background — what we found

An audit of all five domains found **31 verified contract mismatches**. The mobile API type layer
was written speculatively ~2 months ago and **has never been run against prod**.

**The backend is not at fault.** `services/marketplace-api/internal/handlers/admin/mobile_routes.go:21`
states it plainly: *"Same handlers, same authz, different auth."* The web admin drives these same
handlers in production daily. All mobile routes are deployed and live (probed prod: every
`/api/v1/mobile/admin/*` path returns **401**, not 404; a negative control 404s).

**Why 31 mismatches stayed invisible for two months:**
`packages/mobile-shared/api/client.ts:169` supports validation —
`if (options?.schema) return options.schema.parse(data)` — but **no api module passes a schema**.
Every call is `client.get<T>(path)`, and `<T>` is erased at runtime (`client.ts:170-171`:
`const data = await res.json(); return data as T;` — no unwrapping, no validation). A mismatch
yields silent `undefined`, not an error.

### The blocking defect (verified end-to-end)

- `mobile_routes.go:42-44` binds `GET /stores` → `stores.go:74` returns `{"data": resp}`
- `use-store.ts:19-21`: `if (Array.isArray(data)) return data; return data.items ?? []`
- Response is an object, has no `items` → **`useStores()` always returns `[]`**
- `use-tenant-resolver.ts:99` → `showOnboarding = stores.length === 0` → **every merchant sees
  "No store yet"; the dashboard is unreachable**

Nobody has ever got past the store picker. That is why none of this was caught.

### Locked decisions (user)

1. **App adapts to the backend.** Backend is touched ONLY where data genuinely does not exist
   (deferred to sub-project E).
2. **Products go variant-aware**, like the web admin (sub-project D).
3. **Types are inferred from zod schemas** (`z.infer`) — types cannot drift from validation again.
4. **Money is transformed string→number at the schema boundary** — screens keep working unchanged.
5. **Contract breaks fail loudly**, naming the field path.

### Ground truth

`apps/admin/lib/api/marketplace-api.ts` (battle-tested in prod) — lists are
`{ data: T[], meta: { page, page_size, total, total_pages } }`, and money is a **quoted string**
(`AdminOrder.grand_total: string`).

⚠️ **`GET /stores` is the exception.** The web admin resolves its store from the **subdomain**
(`{slug}-admin.mark8ly.com` + session `tenant_id`; `apps/admin/middleware.ts:204,236`), so it never
lists stores. `StoresHandler.List` is bound on both route trees (`routes.go:156`,
`mobile_routes.go:44`) but is **mobile-only in practice and therefore NOT battle-tested**. Its shape
was still verified by reading the handler directly. Treat it as unproven until observed live.

## Decomposition (this spec is A only)

| | Sub-project | Rationale |
|---|---|---|
| **A** | **Contract foundation** — envelope, page pagination, money, zod, `/stores`, dashboard | Unblocks the dashboard so B–D can be verified empirically |
| B | Customers + dashboard field renames | Cheap once A lands |
| C | Orders alignment | `line_items`, refund/cancel bodies, address fields |
| D | Products variant-aware rework | Largest |
| E | Backend gaps (Go + deploy) | `recent_orders`, `average_order_value`, tracking persistence, `POST /variants`, `low_stock` |

**The key leverage:** wire-truthful schemas + inferred types turn every remaining mismatch into a
**compile error**. B–D become "run tsc, fix what it names" rather than debugging blank screens.

## Design

### C1 — `packages/mobile-shared/api/schema-helpers.ts` (new)

```ts
/** Money crosses the wire in TWO shapes. */
export const money = z.union([z.number(), z.string()]).transform(Number);

export const pageMeta = z.object({
  page: z.number(),
  page_size: z.number(),
  total: z.number(),
  total_pages: z.number(),
});

export const paginated = <T extends z.ZodTypeAny>(item: T) =>
  z.object({ data: z.array(item), meta: pageMeta });

/** `/stores` returns `{data}` with NO meta (stores.go:74). */
export const dataOnly = <T extends z.ZodTypeAny>(item: T) =>
  z.object({ data: z.array(item) });
```

**Why `money` is a union, not `z.coerce.number()`:** the wire genuinely carries both forms —
`dashboard.go:23` emits `float64` (JSON number) while orders/products emit `decimal.Decimal`
(quoted string; `shopspring/decimal@v1.4.0/decimal.go:1783-1791` quotes unless
`MarshalJSONWithoutQuotes` is set, and repo-wide grep confirms it never is). `z.coerce.number()`
would silently turn `null` and `""` into `0` — a wrong price is worse than a loud failure.

**`dataOnly` is separate from `paginated` deliberately** — forcing `/stores` into `paginated` would
require inventing a `meta` the endpoint does not send.

### C2 — `packages/mobile-shared/api/schemas/stores.ts` (new)

Wire-truthful, from `stores.go:28-35` (`AdminStoreResponse`):

```ts
export const storeSchema = z.object({
  id: z.string(),
  name: z.string(),
  slug: z.string(),
  country_code: z.string(),
  currency_code: z.string(),
  status: z.string(),
});
export type Store = z.infer<typeof storeSchema>;
export const storesResponseSchema = dataOnly(storeSchema);
```

### C3 — `packages/mobile-shared/api/schemas/dashboard.ts` (new)

Wire-truthful, from `dashboard.go:22-86`. **Field names come from the Go json tags, not from the
current TS types** — that is the whole point:

- `dashboardStatsSchema` — 12 fields, all matching today (`revenue_today` … `pending_reviews`);
  `revenue_trend: z.array(z.number())`; money fields use `money`.
- `recentOrderSchema` — `{id, order_number, customer_email, grand_total (money), status, created_at}`.
- `topProductSchema` — **`{id, title, revenue (money), units_sold, image_url: z.string().nullable()}`**.
  Note `title`/`units_sold`, NOT the current TS `name`/`total_sold`.
- `lowStockItemSchema` — **`{id, title, variant_title, quantity, low_stock_threshold}`**.
  Note `title`/`quantity`, NOT `name`/`stock`. There is **no `thumbnail_url`** field.
- `setupChecklistSchema` — **8** fields (`has_store`, `has_brand_assets`, `has_product`,
  `has_storefront_theme`, `has_payment_provider`, `has_shipping_carrier`, `has_return_policy`,
  `has_custom_domain`). The current TS type declares 5 different ones; no screen reads it today.
- `dashboardResponseSchema` — `{stats, recent_orders, top_products, low_stock, setup_checklist}`.

Types via `z.infer` for each.

### C4 — `types.ts` slims

Delete `Store`, `DashboardStats`, `RecentOrder`, `TopProduct`, `LowStockItem`, `SetupChecklist`,
`DashboardResponse`; re-export the inferred equivalents from the schema modules so existing imports
keep resolving. **Everything else in `types.ts` stays untouched** — B/C/D own their own.

`PaginatedResponse` stays as-is in this sub-project (still referenced by orders/products/customers/
notifications). B/C/D each replace their usage with `paginated(...)`; the last one deletes it.

### C5 — Call sites pass schemas

- `api/dashboard.ts`: `client.get("/dashboard", undefined, dashboardResponseSchema)`
- `use-store.ts`: `client.getTenant("/stores", undefined, storesResponseSchema)` then
  **`return res.data`** — deleting the `Array.isArray(data) … data.items ?? []` hedge, which exists
  only because the author did not know the shape.

### C6 — `app/(tabs)/index.tsx` follows the compile errors

`top_products` → `title`/`units_sold`; `low_stock` → `title`/`quantity`. The inferred types make
these hard compile errors rather than "undefined sold" on screen.

### C7 — `client.ts` fails loudly with the field path

Replace the bare `options.schema.parse(data)` at `client.ts:169` with:

```ts
    if (options?.schema) {
      const parsed = options.schema.safeParse(data);
      if (!parsed.success) {
        // A contract break must name the field. Silent `undefined` is what
        // let 31 mismatches hide for two months.
        const i = parsed.error.issues[0];
        const path = i.path.join(".") || "(root)";
        throw new ApiError(500, "contract_mismatch", `${path}: ${i.message}`);
      }
      return parsed.data;
    }
```

**Verified against the installed zod 4.4.3**, not assumed: `safeParse` failure yields
`issues[0] = { code: "invalid_type", expected: "number", path: ["top_products", 0, "units_sold"],
message: "Invalid input: expected number, received undefined" }`. So the thrown message reads
`top_products.0.units_sold: Invalid input: expected number, received undefined`.

`z.prettifyError` also exists in 4.4.3 and could format the whole error, but the first issue's path
is the actionable part and keeps the message to one line for the metro log. Reporting only
`issues[0]` is a deliberate trade: one clear cause beats a wall of cascading errors.

`status: 500` is chosen because a contract mismatch is our bug, not the user's, and must never be
mistaken for the 401/403 paths that drive sign-out and access copy. Screens keep rendering their
existing error state; the diagnosis lands in the log.

### C8 — `packages/mobile-shared/package.json` stops lying about zod

It declares `zod ^3.23.0` but has **no local `node_modules`**, so it resolves the monorepo root's
**4.4.3** — the same dual-resolution class as the react/zustand jest trap. `client.ts` already
imports zod, so it has always been zod 4 at runtime. Correct the declaration to `^4.4.3`.
Declaration only — the root already satisfies it, so **no install is required** (and none is
permitted; see landmines).

## Testing

Tests live in `apps/mobile-admin/__tests__/` ONLY.

- **`schema-helpers.test.tsx`** — `money` accepts `84` → `84` and `"84.00"` → `84`; **rejects
  `null`, `undefined`, `""`** (the `z.coerce` trap); `paginated`/`dataOnly` accept their real
  shapes.
- **`schemas-stores.test.tsx`** — parses a `{data:[…]}` fixture built from `stores.go:28-35`.
  **Negative control:** the OLD `{items:[…]}` shape **fails** to parse — proving the schema is
  wire-truthful rather than permissive. Without this, a sloppy schema could pass vacuously.
- **`schemas-dashboard.test.tsx`** — parses a fixture built from `dashboard.go:22-86`. **Negative
  control:** a fixture using the OLD names (`name`/`total_sold`) **fails**.
- **`use-store.test.tsx`** — `useStores` returns the array from `{data:[…]}` (the regression that
  made the dashboard unreachable).
- **`client` contract-mismatch test** — a schema failure throws `ApiError` with `contract_mismatch`
  and a message naming the field path.

**Fixtures are hand-built from the Go DTO definitions, not captured from a live response.** No
authenticated response has ever been observed. Live capture requires a test merchant credential and
is the natural follow-up; until then a Go-side change could still drift from these fixtures
undetected.

## Out of scope

- Orders / products / customers / notifications schemas and screens (B, C, D).
- Backend gaps (E): customer `recent_orders` + `average_order_value`; order `tracking_number`
  persistence (`MarkFulfilled` never binds a body); `POST /products/:id/variants` (route absent);
  `low_stock` filter; orders multi-value status filter (`"pending,confirmed"` exact-matches nothing).
- Capturing real prod responses (needs a test merchant credential).
- Apple sign-in device verification (parked).

## Landmines

- **NEVER** run `npm ci` / `npm install` / `npm install --package-lock-only` / `rm -rf node_modules`
  — a metro dev server runs against this tree, and `--package-lock-only` triggers a ~4871-line mass
  re-resolve.
- **`packages/mobile-shared` has NO `node_modules`** — every import resolves the monorepo root. This
  is the root cause of the zod-3-vs-4 lie (C8) and of the react/zustand jest trap.
- Tests live in `apps/mobile-admin/__tests__/` ONLY — expo-router bundles anything under `app/`.
- jest.mock factories must build their mock fns INSIDE the factory (babel hoists imports above outer
  `const`).
- Zod is **v4** at runtime — schemas must be v4-valid.
- Do **not** touch: `metro.config.js`, `tsconfig.json`, `jest.config.js`, `babel.config.js`,
  tailwind/nativewind wiring, `app.config.js`, `eas.json`.
- Commits: single-line conventional, no signatures, direct to `main`.
