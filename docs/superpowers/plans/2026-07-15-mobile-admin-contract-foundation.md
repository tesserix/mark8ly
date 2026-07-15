# mobile-admin API Contract Foundation (sub-project A) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the mobile app reach a working dashboard against prod for the first time, by replacing the fictional API type layer with wire-truthful zod schemas whose types are inferred, so every remaining mismatch becomes a compile error.

**Architecture:** Add zod schemas built from *observed* prod responses (not guesses). `types.ts` stops declaring dashboard/store shapes and re-exports `z.infer` equivalents, so existing imports keep resolving while the shapes become correct. Call sites pass schemas to the client, which now fails loudly naming the offending field path. Each task keeps `tsc` and `jest` green on its own.

**Tech Stack:** TypeScript, zod **4.4.3**, @tanstack/react-query, jest + jest-expo, Expo/React Native.

## Global Constraints

- **Zod is v4 (4.4.3) at runtime** — schemas must be v4-valid. `packages/mobile-shared/package.json` declares `^3.23.0`, which is a lie (Task 1 fixes it).
- **NEVER** run `npm ci` / `npm install` / `npm install --package-lock-only` / `rm -rf node_modules` — a metro dev server runs against this tree, and `--package-lock-only` triggers a ~4871-line mass re-resolve. **No install is needed by this plan.**
- `packages/mobile-shared` has **no `node_modules`** — every import resolves from the monorepo root.
- Tests live in `apps/mobile-admin/__tests__/` **ONLY** — expo-router bundles anything under `app/`.
- `jest.mock` factories must build their mock fns **inside** the factory (babel hoists imports above outer `const`).
- Do **not** touch: `metro.config.js`, `tsconfig.json`, `jest.config.js`, `babel.config.js`, tailwind/nativewind wiring, `app.config.js`, `eas.json`.
- Commits: single-line conventional, no signatures, direct to `main`.

### Gates (run from `apps/mobile-admin`)

```bash
npx jest                                              # baseline: 98/98 passing, 11 suites
npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"   # baseline: 2
```

⚠️ **The `--pretty false` flag is MANDATORY and non-obvious.** The gate documented in the old handoff —
`npx tsc --noEmit 2>&1 | grep -c "error TS"` — **returns 0 even when there are 2 real errors**, because
tsc emits ANSI colour codes (`[91merror[0m[90m TS2322:`) so the literal string `error TS` never appears.
That gate passes vacuously no matter what you break. Verified 2026-07-15: with `--pretty false` the count
is 2 at baseline, 3 with an injected error, back to 2 when removed.

The **2 baseline errors are pre-existing** and both in `app/(tabs)/_layout.tsx` (expo-notifications:
`shouldShowBanner`/`shouldShowList` missing, and `removeNotificationSubscription` gone). They are NOT
yours. **Count them — do not grep by filename** (a per-file grep passed vacuously before and missed 6 real errors).

---

## Verified ground truth (do not re-derive)

**The real prod `GET /mobile/admin/stores/{storeId}/dashboard` response**, captured 2026-07-15 with a real
token from `demo@mark8ly.com` (tenant `MP-Internal-e986p`, store `8b69eea9-2537-4d36-9d99-bafcbad02dbc`):

```json
{"stats":{"revenue_today":0,"revenue_week":0,"revenue_month":0,"revenue_change_pct":0,
"revenue_trend":[0,0,0,0,0,0,0],"orders_today":0,"orders_pending":0,"orders_fulfilled":0,
"orders_cancelled":0,"customers_total":1,"customers_new_this_week":0,"pending_reviews":0},
"recent_orders":[],"top_products":[],"low_stock":[],
"setup_checklist":{"has_store":true,"has_brand_assets":true,"has_product":true,
"has_storefront_theme":true,"has_payment_provider":false,"has_shipping_carrier":false,
"has_return_policy":true,"has_custom_domain":true}}
```

**The real `GET /mobile/admin/stores` response:**

```json
{"data":[{"id":"8b69eea9-2537-4d36-9d99-bafcbad02dbc","name":"The Bondi Store",
"slug":"the-bondi-store","country_code":"AU","currency_code":"AUD","status":"active"}]}
```

This **live-verifies** `stats` (12 fields), `setup_checklist` (8 fields), the top-level envelope, the
`{data:[...]}` store shape, and that dashboard money arrives as a JSON **number**.

⚠️ **`recent_orders`, `top_products` and `low_stock` came back `[]`** — the Bondi store has no orders or
sales, so those three item shapes could **not** be observed live. They are taken from the Go DTOs in
`services/marketplace-api/internal/handlers/admin/dashboard.go:36-63`, re-read and confirmed 2026-07-15:

| Go DTO | Fields (json tags) |
|---|---|
| `RecentOrder` | `id`, `order_number`, `customer_email`, `grand_total` (float64), `status`, `created_at` |
| `TopProduct` | `id`, `title`, `revenue` (float64), `units_sold` (int64), `image_url` (`*string` → nullable) |
| `LowStockItem` | `id`, `title`, `variant_title`, `quantity`, `low_stock_threshold` |

If a later task's fixture disagrees with that table, **the table wins** — it is the marshalled truth.

---

## File Structure

| File | Responsibility |
|---|---|
| `packages/mobile-shared/api/schema-helpers.ts` (**new**) | `money`, `pageMeta`, `paginated`, `dataOnly` — the shared contract primitives |
| `packages/mobile-shared/api/schemas/stores.ts` (**new**) | `storeSchema`, `storesResponseSchema`, `Store` |
| `packages/mobile-shared/api/schemas/dashboard.ts` (**new**) | the 6 dashboard schemas + inferred types |
| `packages/mobile-shared/api/client.ts` (modify `:168-170`) | loud `contract_mismatch` failure naming the field path |
| `packages/mobile-shared/api/types.ts` (modify `:1-60`, `:174-178`) | drop 7 fictional types; re-export inferred ones |
| `packages/mobile-shared/api/dashboard.ts` (modify) | pass `dashboardResponseSchema` |
| `packages/mobile-shared/package.json` (modify `:28`) | stop declaring zod `^3.23.0` |
| `apps/mobile-admin/lib/hooks/use-store.ts` (modify `:19-21`) | **the dashboard unblock** — read `.data` |
| `apps/mobile-admin/app/(tabs)/index.tsx` (modify `:21`, `:191-195`, `:238-242`) | follow the compile errors |

**Nested subpath resolution is verified** — `@repo/mobile-shared/api/schemas/stores` resolves under both
jest and tsc via the `"./api/*": "./api/*.ts"` exports pattern (`*` matches across `/`). Probed live
2026-07-15; no config change needed.

---

### Task 1: Schema helpers + honest zod declaration

**Files:**
- Create: `packages/mobile-shared/api/schema-helpers.ts`
- Modify: `packages/mobile-shared/package.json:28`
- Test: `apps/mobile-admin/__tests__/schema-helpers.test.tsx`

**Interfaces:**
- Consumes: nothing.
- Produces: `money: z.ZodType<number>` (accepts `number | numeric-string`, outputs `number`);
  `pageMeta` (`{page, page_size, total, total_pages}` all numbers);
  `paginated<T>(item)` → `z.object({data: T[], meta: pageMeta})`;
  `dataOnly<T>(item)` → `z.object({data: T[]})`.

> **DEVIATION FROM SPEC — READ THIS.** The spec's design section defines
> `money = z.union([z.number(), z.string()]).transform(Number)` and its testing section claims that
> "rejects `null`, `undefined`, `""`". **Verified against the installed zod 4.4.3, that is false:**
> `""` → **0**, `"   "` → **0**, and `"abc"` → **NaN** (silently, not rejected). Those are exactly the
> `z.coerce` traps the spec set out to avoid — its own stated rule is "a wrong price is worse than a loud
> failure", and `$NaN` on screen is the bug the audit already found on the customers screen.
> The implementation below keeps locked decision #4 (a `number | string` union transformed to `number`,
> **not** `z.coerce`) and actually delivers the promised rejections. Verified behaviour:
> accepts `84`, `0`, `12.5`, `"84.00"`, `"0"`, `" 84.00 "` (trims); rejects `""`, `"   "`, `"abc"`,
> `null`, `undefined`, `true`, `[]`, `{}`.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/schema-helpers.test.tsx`:

```tsx
import { z } from "zod";
import { money, pageMeta, paginated, dataOnly } from "@repo/mobile-shared/api/schema-helpers";

describe("money", () => {
  it.each([
    ["number", 84, 84],
    ["zero", 0, 0],
    ["float", 12.5, 12.5],
    ["decimal string", "84.00", 84],
    ["zero string", "0", 0],
    ["padded string", " 84.00 ", 84],
  ])("accepts %s", (_label, input, expected) => {
    expect(money.parse(input)).toBe(expected);
  });

  // The whole reason money is not z.coerce.number(): these must FAIL, not
  // silently become 0 or NaN. A wrong price is worse than a loud failure.
  // Each row is [label, value] so jest never has to pretty-print a bare
  // undefined/[]/{} as the test name.
  it.each([
    ["empty string", ""],
    ["whitespace only", "   "],
    ["non-numeric string", "abc"],
    ["null", null],
    ["undefined", undefined],
    ["boolean", true],
    ["array", []],
    ["object", {}],
  ])("rejects %s", (_label, input) => {
    expect(money.safeParse(input).success).toBe(false);
  });
});

describe("pageMeta", () => {
  it("accepts the real meta shape", () => {
    expect(
      pageMeta.parse({ page: 1, page_size: 20, total: 3, total_pages: 1 }),
    ).toEqual({ page: 1, page_size: 20, total: 3, total_pages: 1 });
  });
});

describe("paginated", () => {
  it("accepts {data, meta}", () => {
    const schema = paginated(z.object({ id: z.string() }));
    const parsed = schema.parse({
      data: [{ id: "a" }],
      meta: { page: 1, page_size: 20, total: 1, total_pages: 1 },
    });
    expect(parsed.data).toEqual([{ id: "a" }]);
  });

  it("rejects the fictional {items} shape", () => {
    const schema = paginated(z.object({ id: z.string() }));
    expect(schema.safeParse({ items: [{ id: "a" }], total: 1 }).success).toBe(false);
  });
});

describe("dataOnly", () => {
  it("accepts {data} with no meta", () => {
    const schema = dataOnly(z.object({ id: z.string() }));
    expect(schema.parse({ data: [{ id: "a" }] }).data).toEqual([{ id: "a" }]);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/schema-helpers.test.tsx`
Expected: FAIL — `Cannot find module '@repo/mobile-shared/api/schema-helpers'`

- [ ] **Step 3: Write the implementation**

Create `packages/mobile-shared/api/schema-helpers.ts`:

```ts
import { z } from "zod";

/**
 * Money crosses the wire in TWO real shapes: a JSON number (dashboard.go
 * uses float64) and a quoted string (orders/products marshal
 * shopspring/decimal, which quotes unless MarshalJSONWithoutQuotes is set —
 * repo-wide grep confirms it never is).
 *
 * NOT z.coerce.number(): coerce turns null -> 0, "" -> 0 and true -> 1.
 * A silently wrong price is worse than a loud failure. The .min(1) rejects
 * empty/whitespace strings (a bare union would let "" through as 0) and the
 * .pipe(finite) rejects "abc" (a bare union would yield NaN).
 */
export const money = z
  .union([z.number(), z.string().trim().min(1)])
  .transform(Number)
  .pipe(z.number().finite());

/** Every paginated list endpoint returns this meta block. */
export const pageMeta = z.object({
  page: z.number(),
  page_size: z.number(),
  total: z.number(),
  total_pages: z.number(),
});

/** The standard list envelope: `{data, meta}`. */
export const paginated = <T extends z.ZodTypeAny>(item: T) =>
  z.object({ data: z.array(item), meta: pageMeta });

/**
 * `/stores` returns `{data}` with NO meta (stores.go:74) — kept separate from
 * `paginated` deliberately, so we never invent a meta the endpoint doesn't send.
 */
export const dataOnly = <T extends z.ZodTypeAny>(item: T) =>
  z.object({ data: z.array(item) });
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/mobile-admin && npx jest __tests__/schema-helpers.test.tsx`
Expected: PASS — 18 tests

- [ ] **Step 5: Fix the zod declaration**

In `packages/mobile-shared/package.json`, change line 28 from `"zod": "^3.23.0"` to `"zod": "^4.4.3"`.

The package has no `node_modules`, so this has always resolved the root's 4.4.3 — `client.ts` has been
importing zod 4 at runtime all along. This is a **declaration-only** correction; the root already
satisfies it. **Do not run any install.**

- [ ] **Step 6: Run the gates**

```bash
cd apps/mobile-admin
npx jest                                                     # expect 116 passed (98 + 18)
npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"    # expect 2
```

- [ ] **Step 7: Commit**

```bash
git add packages/mobile-shared/api/schema-helpers.ts packages/mobile-shared/package.json apps/mobile-admin/__tests__/schema-helpers.test.tsx
git commit -m "feat(mobile-shared): add wire-truthful schema helpers and correct the zod declaration"
```

---

### Task 2: Stores schema

**Files:**
- Create: `packages/mobile-shared/api/schemas/stores.ts`
- Test: `apps/mobile-admin/__tests__/schemas-stores.test.tsx`

**Interfaces:**
- Consumes: `dataOnly` from `../schema-helpers` (Task 1).
- Produces: `storeSchema`; `storesResponseSchema` (= `z.object({data: Store[]})`);
  `type Store = {id: string; name: string; slug: string; country_code: string; currency_code: string; status: string}`.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/schemas-stores.test.tsx`. The fixture is the **real captured prod
response**, not a guess:

```tsx
import { storeSchema, storesResponseSchema } from "@repo/mobile-shared/api/schemas/stores";

// Captured from prod 2026-07-15: GET /api/v1/mobile/admin/stores
const REAL_RESPONSE = {
  data: [
    {
      id: "8b69eea9-2537-4d36-9d99-bafcbad02dbc",
      name: "The Bondi Store",
      slug: "the-bondi-store",
      country_code: "AU",
      currency_code: "AUD",
      status: "active",
    },
  ],
};

describe("storesResponseSchema", () => {
  it("parses the real prod {data:[...]} response", () => {
    const parsed = storesResponseSchema.parse(REAL_RESPONSE);
    expect(parsed.data).toHaveLength(1);
    expect(parsed.data[0].name).toBe("The Bondi Store");
    expect(parsed.data[0].currency_code).toBe("AUD");
  });

  // Negative control. The old TS type claimed {items}. If this passed, the
  // schema would be permissive rather than wire-truthful, and the bug that
  // made the dashboard unreachable could come back unnoticed.
  it("REJECTS the fictional {items:[...]} shape", () => {
    expect(
      storesResponseSchema.safeParse({ items: REAL_RESPONSE.data }).success,
    ).toBe(false);
  });

  it("rejects a store missing the fields the old type omitted", () => {
    // The old `Store` interface was only {id,name,slug}.
    expect(
      storeSchema.safeParse({ id: "a", name: "b", slug: "c" }).success,
    ).toBe(false);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-stores.test.tsx`
Expected: FAIL — `Cannot find module '@repo/mobile-shared/api/schemas/stores'`

- [ ] **Step 3: Write the implementation**

Create `packages/mobile-shared/api/schemas/stores.ts`:

```ts
import { z } from "zod";
import { dataOnly } from "../schema-helpers";

/**
 * Wire-truthful, from admin/stores.go AdminStoreResponse. Verified against a
 * real prod response 2026-07-15.
 *
 * NOTE: unlike the list endpoints, GET /stores returns `{data}` with NO meta,
 * so this uses dataOnly rather than paginated.
 */
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

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-stores.test.tsx`
Expected: PASS — 3 tests

- [ ] **Step 5: Run the gates**

```bash
cd apps/mobile-admin
npx jest                                                     # expect 119 passed
npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"    # expect 2
```

- [ ] **Step 6: Commit**

```bash
git add packages/mobile-shared/api/schemas/stores.ts apps/mobile-admin/__tests__/schemas-stores.test.tsx
git commit -m "feat(mobile-shared): add wire-truthful stores schema verified against prod"
```

---

### Task 3: Dashboard schemas

**Files:**
- Create: `packages/mobile-shared/api/schemas/dashboard.ts`
- Test: `apps/mobile-admin/__tests__/schemas-dashboard.test.tsx`

**Interfaces:**
- Consumes: `money` from `../schema-helpers` (Task 1).
- Produces: `dashboardStatsSchema`, `recentOrderSchema`, `topProductSchema`, `lowStockItemSchema`,
  `setupChecklistSchema`, `dashboardResponseSchema`, and the inferred types
  `DashboardStats`, `RecentOrder`, `TopProduct`, `LowStockItem`, `SetupChecklist`, `DashboardResponse`.
  Field names later tasks depend on: `TopProduct.title`, `TopProduct.units_sold`,
  `TopProduct.image_url` (nullable), `LowStockItem.title`, `LowStockItem.quantity`,
  `LowStockItem.variant_title`, `LowStockItem.low_stock_threshold`.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/schemas-dashboard.test.tsx`:

```tsx
import {
  dashboardResponseSchema,
  topProductSchema,
  lowStockItemSchema,
  setupChecklistSchema,
} from "@repo/mobile-shared/api/schemas/dashboard";

// Captured verbatim from prod 2026-07-15:
// GET /api/v1/mobile/admin/stores/8b69eea9-.../dashboard
const REAL_RESPONSE = {
  stats: {
    revenue_today: 0,
    revenue_week: 0,
    revenue_month: 0,
    revenue_change_pct: 0,
    revenue_trend: [0, 0, 0, 0, 0, 0, 0],
    orders_today: 0,
    orders_pending: 0,
    orders_fulfilled: 0,
    orders_cancelled: 0,
    customers_total: 1,
    customers_new_this_week: 0,
    pending_reviews: 0,
  },
  recent_orders: [],
  top_products: [],
  low_stock: [],
  setup_checklist: {
    has_store: true,
    has_brand_assets: true,
    has_product: true,
    has_storefront_theme: true,
    has_payment_provider: false,
    has_shipping_carrier: false,
    has_return_policy: true,
    has_custom_domain: true,
  },
};

describe("dashboardResponseSchema", () => {
  it("parses the real prod response", () => {
    const parsed = dashboardResponseSchema.parse(REAL_RESPONSE);
    expect(parsed.stats.customers_total).toBe(1);
    expect(parsed.stats.revenue_trend).toHaveLength(7);
    expect(parsed.setup_checklist.has_payment_provider).toBe(false);
  });

  it("parses a populated response (arrays were empty in prod)", () => {
    const parsed = dashboardResponseSchema.parse({
      ...REAL_RESPONSE,
      recent_orders: [
        {
          id: "o1",
          order_number: "1001",
          customer_email: "a@b.com",
          grand_total: 84,
          status: "pending",
          created_at: "2026-07-15T00:00:00Z",
        },
      ],
      top_products: [
        { id: "p1", title: "Tee", revenue: 84, units_sold: 2, image_url: null },
      ],
      low_stock: [
        {
          id: "v1",
          title: "Tee",
          variant_title: "M / Black",
          quantity: 2,
          low_stock_threshold: 5,
        },
      ],
    });
    expect(parsed.top_products[0].title).toBe("Tee");
    expect(parsed.top_products[0].units_sold).toBe(2);
    expect(parsed.low_stock[0].quantity).toBe(2);
  });
});

describe("negative controls — the OLD fictional field names must fail", () => {
  it("rejects TopProduct {name,total_sold}", () => {
    expect(
      topProductSchema.safeParse({ id: "p1", name: "Tee", total_sold: 2, revenue: 84 }).success,
    ).toBe(false);
  });

  it("rejects LowStockItem {name,stock,thumbnail_url}", () => {
    expect(
      lowStockItemSchema.safeParse({ id: "v1", name: "Tee", stock: 2, thumbnail_url: null })
        .success,
    ).toBe(false);
  });

  it("rejects the old 5-field SetupChecklist", () => {
    expect(
      setupChecklistSchema.safeParse({
        has_products: true,
        has_payment: true,
        has_shipping: true,
        has_domain: true,
        has_branding: true,
      }).success,
    ).toBe(false);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-dashboard.test.tsx`
Expected: FAIL — `Cannot find module '@repo/mobile-shared/api/schemas/dashboard'`

- [ ] **Step 3: Write the implementation**

Create `packages/mobile-shared/api/schemas/dashboard.ts`:

```ts
import { z } from "zod";
import { money } from "../schema-helpers";

/**
 * Wire-truthful, from admin/dashboard.go:22-86. Field names come from the Go
 * json tags, NOT from the TS types they replace — the old types invented
 * `name`/`total_sold`/`stock` and a 5-field checklist that never existed.
 *
 * `stats`, `setup_checklist` and the envelope were verified against a real
 * prod response 2026-07-15. `recent_orders`/`top_products`/`low_stock` came
 * back empty (the store has no orders), so those three item shapes are taken
 * from the Go DTOs.
 */
export const dashboardStatsSchema = z.object({
  revenue_today: money,
  revenue_week: money,
  revenue_month: money,
  revenue_change_pct: z.number(),
  revenue_trend: z.array(z.number()),
  orders_today: z.number(),
  orders_pending: z.number(),
  orders_fulfilled: z.number(),
  orders_cancelled: z.number(),
  customers_total: z.number(),
  customers_new_this_week: z.number(),
  pending_reviews: z.number(),
});

export const recentOrderSchema = z.object({
  id: z.string(),
  order_number: z.string(),
  customer_email: z.string(),
  grand_total: money,
  status: z.string(),
  created_at: z.string(),
});

export const topProductSchema = z.object({
  id: z.string(),
  title: z.string(),
  revenue: money,
  units_sold: z.number(),
  image_url: z.string().nullable(),
});

export const lowStockItemSchema = z.object({
  id: z.string(),
  title: z.string(),
  variant_title: z.string(),
  quantity: z.number(),
  low_stock_threshold: z.number(),
});

export const setupChecklistSchema = z.object({
  has_store: z.boolean(),
  has_brand_assets: z.boolean(),
  has_product: z.boolean(),
  has_storefront_theme: z.boolean(),
  has_payment_provider: z.boolean(),
  has_shipping_carrier: z.boolean(),
  has_return_policy: z.boolean(),
  has_custom_domain: z.boolean(),
});

export const dashboardResponseSchema = z.object({
  stats: dashboardStatsSchema,
  recent_orders: z.array(recentOrderSchema),
  top_products: z.array(topProductSchema),
  low_stock: z.array(lowStockItemSchema),
  setup_checklist: setupChecklistSchema,
});

export type DashboardStats = z.infer<typeof dashboardStatsSchema>;
export type RecentOrder = z.infer<typeof recentOrderSchema>;
export type TopProduct = z.infer<typeof topProductSchema>;
export type LowStockItem = z.infer<typeof lowStockItemSchema>;
export type SetupChecklist = z.infer<typeof setupChecklistSchema>;
export type DashboardResponse = z.infer<typeof dashboardResponseSchema>;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-dashboard.test.tsx`
Expected: PASS — 5 tests

- [ ] **Step 5: Run the gates**

```bash
cd apps/mobile-admin
npx jest                                                     # expect 124 passed
npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"    # expect 2
```

- [ ] **Step 6: Commit**

```bash
git add packages/mobile-shared/api/schemas/dashboard.ts apps/mobile-admin/__tests__/schemas-dashboard.test.tsx
git commit -m "feat(mobile-shared): add wire-truthful dashboard schemas from the Go DTOs"
```

---

### Task 4: Client fails loudly on a contract break

**Files:**
- Modify: `packages/mobile-shared/api/client.ts:168-170`
- Test: `apps/mobile-admin/__tests__/client-contract-mismatch.test.tsx`

**Interfaces:**
- Consumes: `ApiError` (already exported from `client.ts:37`).
- Produces: on schema failure, `request` throws `ApiError` with `status: 500`,
  `code: "contract_mismatch"`, and `message` = `` `${path}: ${issue.message}` ``.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/client-contract-mismatch.test.tsx`. Follow the existing
`api-client-unauthorized.test.tsx` pattern (fake `globalThis.fetch`, no jest.mock needed):

```tsx
import { z } from "zod";
import { createApiClient, ApiError } from "@repo/mobile-shared/api/client";

function jsonResponse(body: unknown): Response {
  return { status: 200, ok: true, json: async () => body, text: async () => "" } as Response;
}

describe("contract mismatch", () => {
  const realFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  function clientReturning(body: unknown) {
    globalThis.fetch = jest.fn().mockResolvedValue(jsonResponse(body)) as unknown as typeof fetch;
    return createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => "t",
      getStoreId: () => null,
    });
  }

  const schema = z.object({
    top_products: z.array(z.object({ units_sold: z.number() })),
  });

  it("throws ApiError with code contract_mismatch", async () => {
    const client = clientReturning({ top_products: [{}] });
    await expect(client.get("/dashboard", undefined, schema)).rejects.toBeInstanceOf(ApiError);
  });

  it("names the exact field path in the message", async () => {
    const client = clientReturning({ top_products: [{}] });
    // Silent `undefined` is what let 31 mismatches hide for two months. The
    // message must point at the field, not just say "invalid".
    await expect(client.get("/dashboard", undefined, schema)).rejects.toMatchObject({
      status: 500,
      code: "contract_mismatch",
      message: "top_products.0.units_sold: Invalid input: expected number, received undefined",
    });
  });

  it("returns parsed data when the schema matches", async () => {
    const client = clientReturning({ top_products: [{ units_sold: 2 }] });
    await expect(client.get("/dashboard", undefined, schema)).resolves.toEqual({
      top_products: [{ units_sold: 2 }],
    });
  });
});
```

> The exact message string was verified against the installed zod 4.4.3:
> `safeParse` yields `issues[0].path = ["top_products", 0, "units_sold"]` and
> `issues[0].message = "Invalid input: expected number, received undefined"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/client-contract-mismatch.test.tsx`
Expected: FAIL — the first two tests fail. `schema.parse()` currently throws a raw `ZodError`, not an
`ApiError`, so `rejects.toBeInstanceOf(ApiError)` and the `toMatchObject` both fail. (The third test
passes already.)

- [ ] **Step 3: Write the implementation**

In `packages/mobile-shared/api/client.ts`, replace lines 168-170:

```ts
    const data = await res.json();
    if (options?.schema) return options.schema.parse(data);
    return data as T;
```

with:

```ts
    const data = await res.json();
    if (options?.schema) {
      const parsed = options.schema.safeParse(data);
      if (!parsed.success) {
        // A contract break must name the field. Silent `undefined` is what let
        // 31 mismatches hide for two months. 500 (not 401/403) because this is
        // our bug, not the user's — it must never reach the sign-out paths.
        const issue = parsed.error.issues[0];
        const path = issue.path.join(".") || "(root)";
        throw new ApiError(500, "contract_mismatch", `${path}: ${issue.message}`);
      }
      return parsed.data;
    }
    return data as T;
```

Only `issues[0]` is reported: one clear cause beats a wall of cascading errors in the metro log.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/mobile-admin && npx jest __tests__/client-contract-mismatch.test.tsx`
Expected: PASS — 3 tests

- [ ] **Step 5: Run the gates**

```bash
cd apps/mobile-admin
npx jest                                                     # expect 127 passed
npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"    # expect 2
```

- [ ] **Step 6: Commit**

```bash
git add packages/mobile-shared/api/client.ts apps/mobile-admin/__tests__/client-contract-mismatch.test.tsx
git commit -m "feat(mobile-shared): fail loudly on contract mismatch naming the field path"
```

---

### Task 5: Wire up `/stores` — the dashboard unblock

This is the task that makes the dashboard reachable for the first time.

**Files:**
- Modify: `packages/mobile-shared/api/types.ts:174-178` (replace the `Store` interface with a re-export)
- Modify: `apps/mobile-admin/lib/hooks/use-store.ts:1-24`
- Test: `apps/mobile-admin/__tests__/use-store.test.tsx`

**Interfaces:**
- Consumes: `storesResponseSchema`, `Store` (Task 2); `client.getTenant` (unchanged).
- Produces: `useStores()` → `UseQueryResult<Store[]>`. `Store` remains importable from
  `@repo/mobile-shared/api/types` (now 6 fields, was 3).

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/use-store.test.tsx`. The `jest.mock` factory builds its mock fn
**inside** the factory — babel hoists `jest.mock` above outer `const` declarations:

```tsx
import { renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useStores } from "@/lib/hooks/use-store";

const REAL_RESPONSE = {
  data: [
    {
      id: "8b69eea9-2537-4d36-9d99-bafcbad02dbc",
      name: "The Bondi Store",
      slug: "the-bondi-store",
      country_code: "AU",
      currency_code: "AUD",
      status: "active",
    },
  ],
};

jest.mock("@/lib/api-client", () => {
  const getTenant = jest.fn();
  return {
    useApiClient: () => ({ getTenant }),
    __getTenant: getTenant,
  };
});

// eslint-disable-next-line @typescript-eslint/no-var-requires
const { __getTenant } = require("@/lib/api-client") as { __getTenant: jest.Mock };

function wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe("useStores", () => {
  beforeEach(() => {
    __getTenant.mockReset();
  });

  it("returns the array from the {data:[...]} envelope", async () => {
    // The regression that made the dashboard unreachable: the hook read
    // `.items`, the endpoint sends `.data`, so it always returned [] and every
    // merchant saw "No store yet".
    __getTenant.mockResolvedValue(REAL_RESPONSE);
    const { result } = renderHook(() => useStores(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].name).toBe("The Bondi Store");
  });

  it("passes the stores schema to the client", async () => {
    __getTenant.mockResolvedValue(REAL_RESPONSE);
    const { result } = renderHook(() => useStores(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(__getTenant).toHaveBeenCalledWith("/stores", undefined, expect.anything());
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/use-store.test.tsx --forceExit`

🔴 **`--forceExit` is REQUIRED for single-file runs of THIS test — without it jest HANGS FOREVER.**
This test renders a react-query `QueryClient`, which leaves an open handle. When jest runs a single
suite it runs it in-band, so the process never exits once the tests finish (you'd see the tests pass or
fail, then nothing). Running the **full** suite (`npx jest`) is unaffected — it uses workers, which get
torn down — so the gate in Step 6 needs no flag. Verified 2026-07-15: the single-file run hangs; the
full suite exits in <10s.

Expected: FAIL — **both** tests fail. The first fails with `expect(received).toHaveLength(1)` /
received length `0`, because the hook reads `data.items` which is `undefined` and falls back to `[]` —
this is the live production bug, reproduced. The second fails with
`Expected: "/stores", undefined, Anything` / `Received: "/stores"`, because the hook currently passes
only one argument and will pass three after your change.

- [ ] **Step 3: Swap the `Store` type to the inferred one**

In `packages/mobile-shared/api/types.ts`, delete the `Store` interface at lines 174-178:

```ts
export interface Store {
  id: string;
  name: string;
  slug: string;
}
```

and replace it with a re-export so every existing `import type { Store } from ".../api/types"` keeps resolving:

```ts
// Re-exported from the schema module so the type can never drift from the
// validation again. Was a hand-written 3-field interface; the wire has 6.
export type { Store } from "./schemas/stores";
```

- [ ] **Step 4: Rewrite the hook**

Replace the whole of `apps/mobile-admin/lib/hooks/use-store.ts` with:

```ts
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { storesResponseSchema } from "@repo/mobile-shared/api/schemas/stores";
import type { Store } from "@repo/mobile-shared/api/types";
import { useApiClient } from "@/lib/api-client";

/**
 * Lists every store the signed-in user has any role on. This is the
 * mobile equivalent of the admin web's `/api/v1/users/me/tenants` —
 * the membership probe used by the post-login resolver and by the
 * in-app store switcher. Routed through the shared api-client so 401
 * (token expired) and tenant-invalid responses self-correct.
 */
export function useStores() {
  const client = useApiClient();
  return useQuery<Store[]>({
    queryKey: ["stores"],
    queryFn: async () => {
      const res = await client.getTenant("/stores", undefined, storesResponseSchema);
      return res.data;
    },
  });
}

export function useSwitchStore() {
  const setActiveStore = useTenantStore((s) => s.setActiveStore);
  const queryClient = useQueryClient();

  return useCallback(
    (store: Store) => {
      setActiveStore(store);
      queryClient.invalidateQueries();
    },
    [setActiveStore, queryClient],
  );
}
```

The `if (Array.isArray(data)) return data; return data.items ?? []` hedge is deleted — it existed only
because the author did not know the shape. The schema now knows it.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd apps/mobile-admin && npx jest __tests__/use-store.test.tsx --forceExit`
(the `--forceExit` is required here for the same reason as Step 2 — without it jest hangs after the
tests finish)
Expected: PASS — 2 tests

- [ ] **Step 6: Run the gates**

```bash
cd apps/mobile-admin
npx jest                                                     # expect 129 passed
npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"    # expect 2
```

If tsc reports a NEW error in `use-store.ts` about `res.data`, the schema generic did not infer —
report it rather than casting with `as`.

- [ ] **Step 7: Commit**

```bash
git add packages/mobile-shared/api/types.ts apps/mobile-admin/lib/hooks/use-store.ts apps/mobile-admin/__tests__/use-store.test.tsx
git commit -m "fix(mobile-admin): read stores from the data envelope so the dashboard is reachable"
```

---

### Task 6: Wire up the dashboard and follow the compile errors

`types.ts` and `index.tsx` must change **together** — swapping the types alone leaves `tsc` red, so this
is one task, not two.

**Files:**
- Modify: `packages/mobile-shared/api/types.ts:8-60` (delete 6 interfaces, re-export)
- Modify: `packages/mobile-shared/api/dashboard.ts`
- Modify: `apps/mobile-admin/app/(tabs)/index.tsx:191-195`, `:238-242`

**Interfaces:**
- Consumes: `dashboardResponseSchema` and the inferred types (Task 3).
- Produces: `createDashboardApi(client).get()` → validated `DashboardResponse`.

**Who imports the types you are swapping** (checked 2026-07-15) — only the first should break:

| Importer | Type | Expected |
|---|---|---|
| `app/(tabs)/index.tsx:21` | `RecentOrder`, `LowStockItem` | **BREAKS** on `LowStockItem` — you fix it in Steps 4-5 |
| `app/(tabs)/customers/[id].tsx:24` | `RecentOrder` | no break — shape is unchanged |
| `components/DashboardStats.tsx:5` | `DashboardStats` | no break — shape is unchanged |
| `lib/hooks/use-dashboard.ts:3` | `DashboardResponse` | no break — envelope is unchanged |

`DashboardStats`, `RecentOrder`, `SetupChecklist` and `DashboardResponse` keep **identical** shapes after
the swap (the `money` fields still infer to `number`), so those three importers stay green. Only
`TopProduct` (`name`→`title`, `total_sold`→`units_sold`) and `LowStockItem` (`name`→`title`,
`stock`→`quantity`) actually change. If any file other than `index.tsx` goes red, stop and report it —
that means a shape changed that this plan did not predict.

> Note: `customers/[id].tsx` importing the **dashboard's** `RecentOrder` for customer data is a real smell,
> and customer `recent_orders` is a known backend gap (sub-project E). Out of scope here — the type is
> shape-identical, so it compiles. Do not "fix" it in this task.

- [ ] **Step 1: Swap the dashboard types to the inferred ones**

In `packages/mobile-shared/api/types.ts`, delete lines 8-60 — the `DashboardStats`, `RecentOrder`,
`TopProduct`, `LowStockItem`, `SetupChecklist` and `DashboardResponse` interfaces — and replace them with:

```ts
// Re-exported from the schema module so types can never drift from validation
// again. The hand-written versions invented `name`/`total_sold`/`stock` and a
// 5-field setup checklist; the wire has `title`/`units_sold`/`quantity` and 8.
export type {
  DashboardStats,
  RecentOrder,
  TopProduct,
  LowStockItem,
  SetupChecklist,
  DashboardResponse,
} from "./schemas/dashboard";
```

**Leave `PaginatedResponse` (lines 1-6) exactly as it is.** It is still referenced by
orders/products/customers/notifications; B/C/D each replace their own usage and the last one deletes it.
Everything below line 60 stays untouched.

- [ ] **Step 2: Pass the schema at the call site**

Replace the whole of `packages/mobile-shared/api/dashboard.ts` with:

```ts
import type { createApiClient } from "./client";
import { dashboardResponseSchema } from "./schemas/dashboard";

export function createDashboardApi(client: ReturnType<typeof createApiClient>) {
  return {
    get: () => client.get("/dashboard", undefined, dashboardResponseSchema),
  };
}
```

- [ ] **Step 3: Run tsc to see the compile errors you are meant to fix**

```bash
cd apps/mobile-admin
npx tsc --noEmit --pretty false 2>&1 | grep "error TS"
```

Expected: the 2 pre-existing `_layout.tsx` errors **plus** new errors in `app/(tabs)/index.tsx` naming
`name`, `total_sold` and `stock`. These are the point of the exercise — the inferred types turn what used
to be "undefined sold" on screen into a hard compile error.

- [ ] **Step 4: Fix the top-products rows**

In `apps/mobile-admin/app/(tabs)/index.tsx`, replace lines 191-195:

```tsx
                      primary={p.name}
                      secondary={`${p.total_sold} sold`}
                      trailing={formatCurrency(p.revenue)}
                      onPress={() => router.push(`/(tabs)/products/${p.id}`)}
                      accessibilityLabel={`${p.name}, ${p.total_sold} sold, ${formatCurrency(p.revenue)} revenue`}
```

with:

```tsx
                      primary={p.title}
                      secondary={`${p.units_sold} sold`}
                      trailing={formatCurrency(p.revenue)}
                      onPress={() => router.push(`/(tabs)/products/${p.id}`)}
                      accessibilityLabel={`${p.title}, ${p.units_sold} sold, ${formatCurrency(p.revenue)} revenue`}
```

- [ ] **Step 5: Fix the low-stock row**

In the same file, replace lines 238-242:

```tsx
      primary={item.name}
      trailing={`${item.stock} left`}
      trailingTone="danger"
      onPress={onPress}
      accessibilityLabel={`${item.name}, ${item.stock} left in stock`}
```

with:

```tsx
      primary={item.title}
      trailing={`${item.quantity} left`}
      trailingTone="danger"
      onPress={onPress}
      accessibilityLabel={`${item.title}, ${item.quantity} left in stock`}
```

`item.title` is the product title and `item.variant_title` is the variant ("M / Black"). Showing the
product title matches the previous intent; wiring `variant_title` into the UI is a design change and is
**not** in scope here.

- [ ] **Step 6: Run the gates**

```bash
cd apps/mobile-admin
npx jest                                                     # expect 129 passed
npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"    # expect 2 (back to baseline)
```

The count returning to **2** is the proof the swap is complete: every fictional field name has been
chased out of the screen.

- [ ] **Step 7: Commit**

```bash
git add packages/mobile-shared/api/types.ts packages/mobile-shared/api/dashboard.ts "apps/mobile-admin/app/(tabs)/index.tsx"
git commit -m "fix(mobile-admin): align dashboard types and screen with the real wire shape"
```

---

## Final verification — against prod, not just tests

Tests and tsc do not prove the app works. The whole reason this sub-project exists is that the type layer
was never run against prod.

- [ ] **Step 1: Confirm the endpoints still answer**

```bash
cd apps/mobile-admin
KEY=$(plutil -extract API_KEY raw GoogleService-Info.plist)
T=$(curl -s -X POST "https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=$KEY" \
  -H "Content-Type: application/json" \
  -d '{"email":"demo@mark8ly.com","password":"Admin@123","tenantId":"MP-Internal-e986p","returnSecureToken":true}' \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['idToken'])")
curl -s -H "Authorization: Bearer $T" https://api.mark8ly.com/api/v1/mobile/admin/stores
curl -s -H "Authorization: Bearer $T" \
  https://api.mark8ly.com/api/v1/mobile/admin/stores/8b69eea9-2537-4d36-9d99-bafcbad02dbc/dashboard
```

Both must return 200. If either 401s, stop — see `mobile_admin_gip_tenant_id_claim.md` in memory.

- [ ] **Step 2: Drive the real app**

Start metro from `apps/mobile-admin` (`npx expo start --dev-client --port 8081`, **no demo flag** — a
stale demo metro silently serves the demo bundle). Sign in as `demo@mark8ly.com` / `Admin@123` and
confirm: the store picker lists **The Bondi Store** (not "No store yet"), and the dashboard renders with
`customers_total: 1` and the setup checklist. **This is the first time the dashboard has ever been
reachable** — it is the deliverable, so look at it.

- [ ] **Step 3: Whole-branch review**

Run the opus whole-branch review (`scripts/review-package BASE HEAD`). **Do not skip it** — it caught a
Critical the per-task reviews missed on all three features of the previous session. Verify its claims
yourself; one earlier finding was a false positive.

---

## Known limitation to carry into B–E

`recent_orders`, `top_products` and `low_stock` are **schema-checked but never observed live** — the Bondi
store has no orders, so prod returns `[]` for all three. Their fixtures come from the Go DTOs. If a Go-side
change drifted those three shapes, these schemas would now fail **loudly** at runtime (which is the
improvement), but no test would have caught it first. Seeding an order in the Bondi store would close this
gap and is the natural follow-up for C (orders).
