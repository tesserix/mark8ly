# Mobile-admin Product Form Depth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the mobile product editor the depth the web admin has — options, full variants, categories, SKU, weight/dimensions, media alt text + reorder, and pick-time crop — so it can honestly represent a multi-variant product.

**Architecture:** Almost all client work against the existing mobile route surface
(`handlers/admin/mobile_routes.go`), plus **one** backend fix: `category_ids` is currently
discarded by the PATCH handler's branch. Variant-level edits (SKU, weight, dims) extend the
existing variant quick-PATCH; only **options** need the aggregate path. Schemas are the source of
truth — types come from `z.infer`, so they cannot drift from validation.

**Tech Stack:** Expo 56 / React Native, TypeScript, zod 4, @tanstack/react-query, jest (mobile-admin),
Go 1.26 + Gin + GORM (marketplace-api).

**Spec:** `docs/superpowers/specs/2026-07-16-mobile-admin-product-form-depth-design.md`

## Global Constraints

- **NEVER** run `npm ci` / `npm install` / `rm -rf node_modules`. Metro runs against this tree. Every
  dependency this plan needs is already installed. If you believe you need a new one, **stop and ask**.
- **Never touch anything inside any `node_modules/`.**
- **Do not modify** `metro.config.js`, `tsconfig.json`, `jest.config.js`, `babel.config.js`, tailwind
  config, `app.config.js`, or `eas.json`.
- **tsc gate is `npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` and must print `0`.**
  `--pretty false` is MANDATORY — without it the count is `0` while errors exist (ANSI colour).
  **Count; never grep by filename.**
- **Run the tsc gate in BOTH `apps/mobile-admin` AND `packages/mobile-shared`.** mobile-shared has its
  own tsconfig extending the ROOT (strict, `noUncheckedIndexedAccess`); mobile-admin uses the laxer
  Expo base and is **blind** to real mobile-shared errors.
- **jest summary lines are coloured — read the TAIL of the output.** `grep "^Tests:"` matches nothing.
- Single-file `npx jest <file>` **hangs forever** without `--forceExit`.
- Money and ALL decimals (`price`, `cost_price`, `compare_at_price`, `length_cm`, `width_cm`,
  `height_cm`) cross the wire **quoted** — use the `number|string` union, **never** `z.coerce.number()`
  (it turns `null`/`""` into `0`) and never `z.number()`.
- Go `omitempty` pointers arrive **absent, not null** → `.optional()`, **never** `.nullable()`.
- **Variants come back UNSORTED** (a real product returns positions `2,3,4,0,1`). `variants[0]` is the
  WRONG variant. Sort by `position`.
- Commit messages: **single-line**, conventional (`feat:`/`fix:`/`test:`/`docs:`), **no signatures**,
  no multi-line body. Commit directly to `main`. No PRs.
- Never hardcode `https://cdn.mark8ly.com`. `url` on media create carries the **storage key**; the
  backend builds the public URL itself (`service_single_media.go:91-97`).
- **Deliberate, do not "fix":** Tasks 10 and 11 assert on **source text**
  (`fs.readFileSync` + `toContain`). This is a conscious decision (user, 2026-07-16), not an
  oversight. They guard two things that are painful to express behaviourally and that have each
  already cost this project real money: "never reintroduce `requestMediaLibraryPermissionsAsync`"
  (it stranded users in iOS's limited-library sheet) and "never send `variants` on a product PATCH"
  (the full-desired-matrix soft-deletes real variants). A behavioural test cannot express "this
  string must never appear". Reviewers: flag if you disagree, but this was chosen with the
  trade-off understood.

---

### Task 1: Fix the `category_ids` silent-discard bug (backend)

`PATCH {category_ids: [...]}` currently returns **200 OK and does nothing**. The aggregate applies
category links (`service_aggregate.go:266`), but the handler only routes there when
`options`/`variants`/`removed_variant_ids` are present. `UpdateBasicsRequest` has no `CategoryIDs`
field at all, so the basics path physically cannot set them.

Safe because `UpdateAggregate` is nil-safe: `Options == nil` → `optionSpecsFromExisting(existing.Options)`;
options diff guarded by `if req.Options != nil`; variants diff by
`if req.Variants != nil || req.RemovedVariantIDs != nil`.

The test hits the **web** admin route; the handler is shared, so it proves the fix for mobile too.

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/products.go:172`
- Test: `services/marketplace-api/internal/handlers/admin/products_integration_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces: `PATCH /products/:id` with body `{"category_ids": ["<uuid>"]}` persists category links
  and returns 200. No signature changes.

- [ ] **Step 1: Write the failing test**

Append to `services/marketplace-api/internal/handlers/admin/products_integration_test.go`:

```go
// A categories-only PATCH must persist. Before the branch fix this returned
// 200 while silently discarding the edit — the handler routed to neither the
// aggregate (which owns category links) nor basics (which has no CategoryIDs
// field at all).
func TestAPI_AdminProducts_Patch_CategoryIDsOnly_Persists(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)
	catID := seedCategoryViaRepo(t, env, storeID, tenantID, "Swimwear", "swimwear-"+uuid.NewString()[:6])

	w := request(t, env.router, http.MethodPatch,
		"/api/v1/admin/stores/"+storeID+"/products/"+pid,
		map[string]any{"category_ids": []string{catID}},
		authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("patch category_ids: want 200, got %d: %s", w.Code, w.Body.String())
	}

	// Read it back — a 200 alone proves nothing here; that was the bug.
	w2 := request(t, env.router, http.MethodGet,
		"/api/v1/admin/stores/"+storeID+"/products/"+pid, nil,
		authHeaders(userID, tenantID))
	if w2.Code != http.StatusOK {
		t.Fatalf("get product: want 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var got struct {
		Categories []struct {
			ID string `json:"id"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Categories) != 1 || got.Categories[0].ID != catID {
		t.Fatalf("category link not persisted: got %+v, want [%s]", got.Categories, catID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/handlers/admin/ -run TestAPI_AdminProducts_Patch_CategoryIDsOnly_Persists -v`
Expected: **FAIL** — `category link not persisted: got [], want [<uuid>]`. The PATCH returns 200; the link is absent. That 200-with-no-effect IS the bug.

If it fails because `json` is not imported in the test file, add `"encoding/json"` to its imports.

- [ ] **Step 3: Write minimal implementation**

In `services/marketplace-api/internal/handlers/admin/products.go`, change the branch condition at
line 172 from:

```go
	if req.Options != nil || req.Variants != nil || req.RemovedVariantIDs != nil {
```

to:

```go
	// CategoryIDs must be here, not in the basics branch below: UpdateBasicsRequest
	// has no CategoryIDs field, so a categories-only PATCH previously matched
	// neither branch and returned 200 having done nothing. UpdateAggregate is
	// nil-safe — nil Options/Variants mean "leave alone" — so routing a
	// categories-only patch through it applies links and touches nothing else.
	if req.Options != nil || req.Variants != nil || req.RemovedVariantIDs != nil ||
		req.CategoryIDs != nil {
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/marketplace-api && go test ./internal/handlers/admin/ -run TestAPI_AdminProducts_Patch_CategoryIDsOnly_Persists -v`
Expected: **PASS**

Then run the whole admin package to prove nothing regressed — especially the existing aggregate and
basics tests:

Run: `cd services/marketplace-api && go test ./internal/handlers/admin/ 2>&1 | tail -5`
Expected: `ok` — no failures.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/products.go services/marketplace-api/internal/handlers/admin/products_integration_test.go
git commit -m "fix(marketplace-api): route category_ids patches through the aggregate so they persist"
```

---

### Task 2: Complete the variant response schema

`AdminVariantResponse` (`dto.go:127`) returns 7 fields the schema doesn't model: `barcode`,
`cost_price`, `weight_grams`, `length_cm`, `width_cm`, `height_cm`, `low_stock_threshold`.
`weight_grams` is `*int` → a plain number. The rest of the decimals are `*decimal.Decimal` → they
arrive **quoted**, exactly like `price`.

**Files:**
- Modify: `packages/mobile-shared/api/schema-helpers.ts`
- Modify: `packages/mobile-shared/api/schemas/products.ts:22-37`
- Test: `apps/mobile-admin/__tests__/schemas-products.test.tsx` (append)

**Interfaces:**
- Consumes: `money` from `../schema-helpers`.
- Produces: `decimalNumber` exported from `schema-helpers`; `productVariantSchema` gains
  `barcode?: string`, `cost_price?: number`, `weight_grams?: number`, `length_cm?: number`,
  `width_cm?: number`, `height_cm?: number`, `low_stock_threshold?: number`. `ProductVariant`
  (`z.infer`) picks these up automatically.

- [ ] **Step 1: Write the failing test**

Append to `apps/mobile-admin/__tests__/schemas-products.test.tsx`:

```tsx
describe("productVariantSchema — shipping fields", () => {
  const BASE = {
    id: "3eabedcb",
    sku: "TBS-PBLR-XS-S",
    price: "199",
    currency_code: "AUD",
    inventory_quantity: 0,
    inventory_policy: "deny",
    option_values: [],
    position: 0,
  };

  it("parses quoted decimal dimensions into numbers", () => {
    const parsed = productVariantSchema.parse({
      ...BASE,
      weight_grams: 450,
      length_cm: "30.5",
      width_cm: "20",
      height_cm: "10.25",
    });
    expect(parsed.weight_grams).toBe(450);
    expect(parsed.length_cm).toBe(30.5);
    expect(parsed.width_cm).toBe(20);
    expect(parsed.height_cm).toBe(10.25);
  });

  it("treats omitted shipping fields as absent, not null (Go omitempty)", () => {
    const parsed = productVariantSchema.parse(BASE);
    expect(parsed.weight_grams).toBeUndefined();
    expect(parsed.length_cm).toBeUndefined();
    expect(parsed.barcode).toBeUndefined();
    expect(parsed.cost_price).toBeUndefined();
    expect(parsed.low_stock_threshold).toBeUndefined();
  });

  it("parses barcode, cost_price and low_stock_threshold", () => {
    const parsed = productVariantSchema.parse({
      ...BASE,
      barcode: "9310779300005",
      cost_price: "88.40",
      low_stock_threshold: 5,
    });
    expect(parsed.barcode).toBe("9310779300005");
    expect(parsed.cost_price).toBe(88.4);
    expect(parsed.low_stock_threshold).toBe(5);
  });

  it("rejects an empty-string dimension rather than silently reading it as 0", () => {
    expect(() => productVariantSchema.parse({ ...BASE, length_cm: "" })).toThrow();
  });
});
```

Add `productVariantSchema` to the file's existing import from
`@repo/mobile-shared/api/schemas/products`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-products.test.tsx --forceExit 2>&1 | tail -15`
Expected: FAIL — `expect(parsed.weight_grams).toBe(450)` receives `undefined`, because the schema
strips unmodelled keys.

- [ ] **Step 3: Write minimal implementation**

In `packages/mobile-shared/api/schema-helpers.ts`, append after the `money` export:

```ts
/**
 * Non-money decimals (variant dimensions) have the SAME wire reality as money:
 * shopspring/decimal quotes them, so they arrive as "30.5" not 30. Aliased
 * rather than duplicated — the parsing rules are identical, only the meaning
 * differs, and `money.length_cm` would read as nonsense.
 */
export const decimalNumber = money;
```

In `packages/mobile-shared/api/schemas/products.ts`, change the import on line 2 to:

```ts
import { decimalNumber, money, paginated } from "../schema-helpers";
```

and replace the `productVariantSchema` body (lines 22-37) with:

```ts
export const productVariantSchema = z.object({
  id: z.string(),
  sku: z.string(),
  barcode: z.string().optional(),
  price: money,
  compare_at_price: money.optional(),
  cost_price: money.optional(),
  currency_code: z.string(),
  // Shipping fields (AdminVariantResponse, dto.go:135-138). weight_grams is a
  // Go *int -> a plain number; the *_cm fields are *decimal.Decimal and arrive
  // QUOTED like price. All are omitempty -> absent, never null.
  weight_grams: z.number().optional(),
  length_cm: decimalNumber.optional(),
  width_cm: decimalNumber.optional(),
  height_cm: decimalNumber.optional(),
  inventory_quantity: z.number(),
  inventory_policy: z.string(),
  low_stock_threshold: z.number().optional(),
  option_values: z.array(variantOptionValueSchema),
  /**
   * The wire does NOT sort variants by position — a real product came back
   * as 2,3,4,0,1. Anything picking a "primary" variant must sort by this
   * field; variants[0] is not it. See lib/product-display.ts.
   */
  position: z.number(),
});
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-products.test.tsx --forceExit 2>&1 | tail -8`
Expected: PASS, all green.

Run both tsc gates:
```bash
cd apps/mobile-admin && npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"
cd ../../packages/mobile-shared && npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"
```
Expected: `0` and `0`.

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/api/schema-helpers.ts packages/mobile-shared/api/schemas/products.ts apps/mobile-admin/__tests__/schemas-products.test.tsx
git commit -m "feat(mobile-shared): model variant barcode, cost price and shipping dimensions"
```

---

### Task 3: Real category schemas

Two different shapes share the word "category" — do not conflate them:
- `product.categories[]` is `AdminCategoryRef` (`dto.go:165`) — lean `{id, name, slug}`.
- `GET /categories` returns `AdminCategoryResponse` (`dto.go:14`) — the full record, including
  `parent_id`, because **categories are a tree**.

`productSchema.categories` is currently `z.array(z.unknown())` — a punt from the contract work.

**Files:**
- Create: `packages/mobile-shared/api/schemas/categories.ts`
- Modify: `packages/mobile-shared/api/schemas/products.ts:88`
- Test: `apps/mobile-admin/__tests__/schemas-categories.test.tsx`

**Interfaces:**
- Consumes: `dataOnly` from `../schema-helpers`.
- Produces: `categoryRefSchema` / `CategoryRef` (`{id, name, slug}`); `categorySchema` / `Category`
  (full record); `categoryListSchema` / `CategoryListResponse` (`{data: Category[]}`).
  `productSchema.categories` becomes `z.array(categoryRefSchema)` → `Product["categories"]` is
  `CategoryRef[]`.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/schemas-categories.test.tsx`:

```tsx
import {
  categorySchema,
  categoryListSchema,
  categoryRefSchema,
} from "@repo/mobile-shared/api/schemas/categories";
import { productSchema } from "@repo/mobile-shared/api/schemas/products";

// Shape per AdminCategoryResponse (dto.go:14-27).
const REAL_CATEGORY = {
  id: "bdd640fb-0667-4ad1-9c80-317fa3b1799d",
  store_id: "8b69eea9-2537-4d36-9d99-bafcbad02dbc",
  name: "Swimwear",
  slug: "swimwear",
  position: 0,
  is_active: true,
  featured: false,
  created_at: "2026-05-04T23:48:01.08461Z",
  updated_at: "2026-05-04T23:48:01.08461Z",
};

describe("categorySchema", () => {
  it("parses a category with no parent (a root of the tree)", () => {
    const parsed = categorySchema.parse(REAL_CATEGORY);
    expect(parsed.name).toBe("Swimwear");
    expect(parsed.parent_id).toBeUndefined();
  });

  it("parses a child category carrying parent_id", () => {
    const parsed = categorySchema.parse({ ...REAL_CATEGORY, parent_id: "parent-uuid" });
    expect(parsed.parent_id).toBe("parent-uuid");
  });

  it("treats omitted optionals as absent, not null (Go omitempty)", () => {
    const parsed = categorySchema.parse(REAL_CATEGORY);
    expect(parsed.description).toBeUndefined();
    expect(parsed.image_url).toBeUndefined();
  });

  it("parses the {data} envelope — categories send NO meta", () => {
    const parsed = categoryListSchema.parse({ data: [REAL_CATEGORY] });
    expect(parsed.data[0]!.slug).toBe("swimwear");
  });

  it("fails loudly on a malformed category rather than passing it through", () => {
    // `position` is required; a category missing it must throw, not arrive as
    // undefined and sort unpredictably in the picker.
    const { position, ...noPosition } = REAL_CATEGORY;
    expect(() => categoryListSchema.parse({ data: [noPosition] })).toThrow();
  });
});

describe("categoryRefSchema", () => {
  it("parses the lean ref embedded on a product", () => {
    const parsed = categoryRefSchema.parse({ id: "c1", name: "Swimwear", slug: "swimwear" });
    expect(parsed.name).toBe("Swimwear");
  });

  it("product.categories parses refs, not full categories", () => {
    const product = {
      id: "p1",
      store_id: "s1",
      handle: "h",
      title: "T",
      status: "active",
      tags: [],
      categories: [{ id: "c1", name: "Swimwear", slug: "swimwear" }],
      options: [],
      variants: [],
      media: [],
      created_at: "2026-05-04T23:48:01.08461Z",
      updated_at: "2026-05-04T23:48:01.08461Z",
    };
    const parsed = productSchema.parse(product);
    expect(parsed.categories[0]!.name).toBe("Swimwear");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-categories.test.tsx --forceExit 2>&1 | tail -10`
Expected: FAIL — `Cannot find module '@repo/mobile-shared/api/schemas/categories'`.

- [ ] **Step 3: Write minimal implementation**

Create `packages/mobile-shared/api/schemas/categories.ts`:

```ts
import { z } from "zod";
import { dataOnly } from "../schema-helpers";

/**
 * Wire truth for the admin category endpoints.
 *
 * TWO shapes share the name "category" — do not conflate them:
 *  - `categoryRefSchema` (AdminCategoryRef, dto.go:165) is what a PRODUCT
 *    embeds under `categories[]`: id/name/slug only.
 *  - `categorySchema` (AdminCategoryResponse, dto.go:14) is what
 *    `GET /categories` returns: the full record, including `parent_id`.
 *
 * Categories are a TREE. `parent_id` is a Go *string with omitempty, so a root
 * category OMITS the key — absent, never null.
 */
export const categoryRefSchema = z.object({
  id: z.string(),
  name: z.string(),
  slug: z.string(),
});
export type CategoryRef = z.infer<typeof categoryRefSchema>;

export const categorySchema = z.object({
  id: z.string(),
  store_id: z.string(),
  parent_id: z.string().optional(),
  name: z.string(),
  slug: z.string(),
  description: z.string().optional(),
  image_url: z.string().optional(),
  position: z.number(),
  is_active: z.boolean(),
  featured: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Category = z.infer<typeof categorySchema>;

/**
 * `GET /categories` returns `{data}` with NO meta (categories.go:44) — the same
 * envelope as /stores, NOT the `{data, meta}` of /products. Using `paginated`
 * here would invent a meta block the endpoint never sends.
 */
export const categoryListSchema = dataOnly(categorySchema);
export type CategoryListResponse = z.infer<typeof categoryListSchema>;
```

In `packages/mobile-shared/api/schemas/products.ts`, add to the imports:

```ts
import { categoryRefSchema } from "./categories";
```

and replace line 88 (`categories: z.array(z.unknown()),`) with:

```ts
  // AdminCategoryRef (dto.go:165) — id/name/slug only, NOT the full category
  // record that GET /categories returns. Was z.array(z.unknown()).
  categories: z.array(categoryRefSchema),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-categories.test.tsx __tests__/schemas-products.test.tsx --forceExit 2>&1 | tail -8`
Expected: PASS.

Run both tsc gates (see Global Constraints). Expected: `0` and `0`.

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/api/schemas/categories.ts packages/mobile-shared/api/schemas/products.ts apps/mobile-admin/__tests__/schemas-categories.test.tsx
git commit -m "feat(mobile-shared): add real category schemas and type product.categories"
```

---

### Task 4: Categories API + `useCategories` hook

**Files:**
- Create: `packages/mobile-shared/api/categories.ts`
- Modify: `apps/mobile-admin/lib/admin-api/product-crud.ts` (append hook)
- Test: `apps/mobile-admin/__tests__/use-categories.test.tsx`

**Interfaces:**
- Consumes: `categoryListSchema`, `CategoryListResponse`, `Category` (Task 3);
  `createApiClient` from `./client`.
- Produces: `createCategoriesApi(client)` → `{ list: () => Promise<CategoryListResponse> }`;
  `useCategories()` → react-query result of `Category[]`.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/use-categories.test.tsx`:

```tsx
import { createCategoriesApi } from "@repo/mobile-shared/api/categories";

describe("createCategoriesApi", () => {
  it("GETs /categories and validates with the {data} schema", async () => {
    const get = jest.fn().mockResolvedValue({ data: [] });
    const api = createCategoriesApi({ get } as never);
    await api.list();
    expect(get).toHaveBeenCalledTimes(1);
    const [path, params, schema] = get.mock.calls[0]!;
    expect(path).toBe("/categories");
    expect(params).toBeUndefined();
    // The schema must be passed — an unvalidated response is how the {items}
    // fiction hid 161 products for two months.
    expect(schema).toBeDefined();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/use-categories.test.tsx --forceExit 2>&1 | tail -10`
Expected: FAIL — `Cannot find module '@repo/mobile-shared/api/categories'`.

- [ ] **Step 3: Write minimal implementation**

Create `packages/mobile-shared/api/categories.ts`:

```ts
import type { createApiClient } from "./client";
import {
  categoryListSchema,
  type Category,
  type CategoryListResponse,
} from "./schemas/categories";

/**
 * `GET /categories` (mobile_routes.go:95) takes no query params and returns
 * every category for the store in one `{data}` payload — there is no pagination
 * on this endpoint, so there is nothing to page through.
 */
export function createCategoriesApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: () =>
      client.get<CategoryListResponse>("/categories", undefined, categoryListSchema),
  };
}

export type { Category, CategoryListResponse };
```

In `apps/mobile-admin/lib/admin-api/product-crud.ts`, change line 1 from:

```ts
import { useMutation, useQueryClient } from "@tanstack/react-query";
```

to:

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
```

and add to the imports:

```ts
import { createCategoriesApi } from "@repo/mobile-shared/api/categories";
```

Then append to the file:

```ts
/**
 * Categories change rarely and the picker needs them on every product edit —
 * a 5-minute staleTime keeps the sheet instant without going stale enough to
 * mislead.
 */
export function useCategories() {
  const client = useApiClient();
  const categoriesApi = createCategoriesApi(client);

  return useQuery({
    queryKey: ["categories"],
    queryFn: async () => {
      const res = await categoriesApi.list();
      return res.data;
    },
    staleTime: 5 * 60 * 1000,
  });
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/mobile-admin && npx jest __tests__/use-categories.test.tsx --forceExit 2>&1 | tail -8`
Expected: PASS.

Run both tsc gates. Expected: `0` and `0`.

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/api/categories.ts apps/mobile-admin/lib/admin-api/product-crud.ts apps/mobile-admin/__tests__/use-categories.test.tsx
git commit -m "feat(mobile-admin): fetch store categories via a validated categories api"
```

---

### Task 5: Request bodies — options, categories, and the extended variant patch

Three separate wire truths land here:
1. `UpdateVariantBody` currently exposes only `{sku, price, inventory_quantity}` but
   `UpdateVariantRequest` (`validation.go:43`) accepts barcode, cost_price, compare_at_price,
   weight_grams, length_cm, width_cm, height_cm, inventory_policy, low_stock_threshold, position.
2. `UpdateProductBody` cannot express options, categories, or removed variants.
3. **The options request/response asymmetry.** On the REQUEST `values` is `string[]`
   (`CreateProductOptionInput`, `validation.go:251`); on the RESPONSE it is
   `[{id, value, position}]`. One field name, two shapes. Modelling the response with the request
   shape would blank every product the moment a merchant adds an option.

**Files:**
- Modify: `packages/mobile-shared/api/products.ts:43-55`
- Test: `apps/mobile-admin/__tests__/product-request-bodies.test.tsx`

**Interfaces:**
- Consumes: `productDetailSchema`, `productOptionSchema` (existing).
- Produces:
  - `UpdateProductOptionBody = { name: string; values: string[] }`
  - `UpdateProductBody` gains `options?: UpdateProductOptionBody[]`,
    `category_ids?: string[]`, `primary_category_id?: string`, `removed_variant_ids?: string[]`
  - `UpdateVariantBody` gains `barcode?`, `compare_at_price?`, `cost_price?`, `weight_grams?`,
    `length_cm?`, `width_cm?`, `height_cm?`, `inventory_policy?`, `low_stock_threshold?`,
    `position?` — all `number` except `barcode`/`inventory_policy` (`string`).

**Note:** `variants` is deliberately NOT added to `UpdateProductBody`. `UpdateAggregateRequest.Variants`
is a **full desired matrix** and `applyVariantsDiff` soft-deletes anything missing from it. Variant
edits go through the quick-PATCH instead. Do not add it.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/product-request-bodies.test.tsx`:

```tsx
import { productOptionSchema } from "@repo/mobile-shared/api/schemas/products";
import type { UpdateProductBody, UpdateVariantBody } from "@repo/mobile-shared/api/products";

// This file pins the request/response option asymmetry. It is a type-level and
// shape-level guard against the single most expensive recurring bug on this
// project: modelling the RESPONSE with the REQUEST's shape.
describe("product option request vs response shapes", () => {
  it("REQUEST options.values is string[]", () => {
    const body: UpdateProductBody = {
      options: [{ name: "Size", values: ["S", "M", "L"] }],
    };
    expect(body.options![0]!.values).toEqual(["S", "M", "L"]);
  });

  it("RESPONSE options.values is [{id, value, position}] — NOT string[]", () => {
    const parsed = productOptionSchema.parse({
      id: "opt-1",
      name: "Size",
      position: 0,
      values: [{ id: "v1", value: "S", position: 0 }],
    });
    expect(parsed.values[0]!.value).toBe("S");
    expect(parsed.values[0]!.id).toBe("v1");
  });

  it("RESPONSE rejects the REQUEST shape — the two must never be swapped", () => {
    expect(() =>
      productOptionSchema.parse({ id: "opt-1", name: "Size", position: 0, values: ["S", "M"] }),
    ).toThrow();
  });
});

describe("UpdateVariantBody", () => {
  it("carries the shipping fields the variant quick-PATCH accepts", () => {
    const body: UpdateVariantBody = {
      sku: "ABC-1",
      weight_grams: 450,
      length_cm: 30.5,
      width_cm: 20,
      height_cm: 10,
    };
    expect(body.weight_grams).toBe(450);
    expect(body.length_cm).toBe(30.5);
  });
});

describe("UpdateProductBody", () => {
  it("carries category_ids", () => {
    const body: UpdateProductBody = { category_ids: ["cat-1", "cat-2"] };
    expect(body.category_ids).toHaveLength(2);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/product-request-bodies.test.tsx --forceExit 2>&1 | tail -12`
Expected: FAIL — jest runs via babel (types are stripped), so the runtime assertions on `options`
and `category_ids` pass trivially, but **`npx tsc --noEmit --pretty false` reports errors**:
`Object literal may only specify known properties, and 'options' does not exist in type 'UpdateProductBody'`.

Run: `cd apps/mobile-admin && npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"`
Expected: a **non-zero** count. That is this task's real red state.

- [ ] **Step 3: Write minimal implementation**

In `packages/mobile-shared/api/products.ts`, replace `UpdateProductBody` and `UpdateVariantBody`
(lines 43-55) with:

```ts
/**
 * REQUEST shape for a product option (CreateProductOptionInput,
 * validation.go:251-254). `values` is `string[]` HERE.
 *
 * The RESPONSE (productOptionSchema) sends `[{id, value, position}]` for the
 * same field name. These are two different shapes and must never be swapped —
 * doing so would blank the product list the moment any product has options.
 */
export interface UpdateProductOptionBody {
  name: string;
  values: string[];
}

/**
 * PATCH /products/:id body (UpdateProductRequest, validation.go:296).
 *
 * `variants` is deliberately absent. UpdateAggregateRequest.Variants is a FULL
 * DESIRED MATRIX — applyVariantsDiff soft-deletes any existing variant missing
 * from it. Variant edits go through updateVariant() instead. Do not add it here.
 *
 * Sending `options`, `removed_variant_ids` or `category_ids` routes the handler
 * through the aggregate path (products.go:172); a body of scalars alone routes
 * through basics. Send only what changed.
 */
export interface UpdateProductBody {
  title?: string;
  description?: string;
  status?: string;
  tags?: string[];
  options?: UpdateProductOptionBody[];
  removed_variant_ids?: string[];
  category_ids?: string[];
  primary_category_id?: string;
}

/**
 * UpdateVariantRequest (validation.go:43-58) — the variant quick-PATCH. There
 * is no `stock` field; it is `inventory_quantity`.
 *
 * This endpoint accepts SKU and all the shipping fields, which is why weight
 * and dimensions need no aggregate call.
 */
export interface UpdateVariantBody {
  sku?: string;
  barcode?: string;
  price?: number;
  compare_at_price?: number;
  cost_price?: number;
  weight_grams?: number;
  length_cm?: number;
  width_cm?: number;
  height_cm?: number;
  inventory_quantity?: number;
  /** deny | continue */
  inventory_policy?: string;
  low_stock_threshold?: number;
  position?: number;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/mobile-admin && npx jest __tests__/product-request-bodies.test.tsx --forceExit 2>&1 | tail -8`
Expected: PASS.

Run both tsc gates. Expected: `0` and `0` — the `0` in mobile-admin is what actually proves this task.

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/api/products.ts apps/mobile-admin/__tests__/product-request-bodies.test.tsx
git commit -m "feat(mobile-shared): model option, category and shipping request bodies"
```

---

### Task 6: VariantEditor — SKU, weight and dimensions

Extends the existing `VariantRow` in `[id].tsx` (currently price + stock only) and moves it into its
own file. `[id].tsx` is 571 lines and must not grow.

Each field commits on blur via the existing `useUpdateVariant` quick-PATCH — no aggregate.

**Files:**
- Create: `apps/mobile-admin/components/products/VariantEditor.tsx`
- Modify: `apps/mobile-admin/app/(tabs)/products/[id].tsx` (remove `VariantRow`, import `VariantEditor`)
- Test: `apps/mobile-admin/__tests__/variant-editor.test.tsx`

**Interfaces:**
- Consumes: `ProductVariant` (Task 2), `UpdateVariantBody` (Task 5), `Text`/`Eyebrow` from
  `@/components/ui`, `theme` from `@/lib/theme`.
- Produces: `<VariantEditor variant={ProductVariant} onUpdate={(variantId: string, body: UpdateVariantBody) => void} />`
  and the exported helper `variantLabel(variant: ProductVariant): string`.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/variant-editor.test.tsx`:

```tsx
import { render, fireEvent } from "@testing-library/react-native";
import { VariantEditor, variantLabel } from "@/components/products/VariantEditor";

const VARIANT = {
  id: "v1",
  sku: "TBS-PBLR-XS-S",
  price: 199,
  currency_code: "AUD",
  inventory_quantity: 4,
  inventory_policy: "deny",
  option_values: [],
  position: 0,
} as const;

describe("variantLabel", () => {
  it("uses option values when present", () => {
    expect(
      variantLabel({
        ...VARIANT,
        option_values: [
          { option_name: "Size", option_value_id: "a", value: "M" },
          { option_name: "Colour", option_value_id: "b", value: "Blue" },
        ],
      } as never),
    ).toBe("M / Blue");
  });

  it("falls back to SKU when the variant has no option values", () => {
    expect(variantLabel(VARIANT as never)).toBe("TBS-PBLR-XS-S");
  });
});

describe("VariantEditor", () => {
  it("commits a weight edit on blur as weight_grams", () => {
    const onUpdate = jest.fn();
    const { getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
    const input = getByLabelText("Weight in grams");
    fireEvent.changeText(input, "450");
    fireEvent(input, "blur");
    expect(onUpdate).toHaveBeenCalledWith("v1", { weight_grams: 450 });
  });

  it("commits an SKU edit on blur", () => {
    const onUpdate = jest.fn();
    const { getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
    const input = getByLabelText("SKU");
    fireEvent.changeText(input, "NEW-SKU-1");
    fireEvent(input, "blur");
    expect(onUpdate).toHaveBeenCalledWith("v1", { sku: "NEW-SKU-1" });
  });

  it("does NOT fire when the value is unchanged", () => {
    const onUpdate = jest.fn();
    const { getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
    const input = getByLabelText("SKU");
    fireEvent(input, "blur");
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("does NOT fire on unparseable input rather than sending NaN", () => {
    const onUpdate = jest.fn();
    const { getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
    const input = getByLabelText("Weight in grams");
    fireEvent.changeText(input, "heavy");
    fireEvent(input, "blur");
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("rejects an empty SKU — the backend requires it", () => {
    const onUpdate = jest.fn();
    const { getByLabelText } = render(
      <VariantEditor variant={VARIANT as never} onUpdate={onUpdate} />,
    );
    const input = getByLabelText("SKU");
    fireEvent.changeText(input, "   ");
    fireEvent(input, "blur");
    expect(onUpdate).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/variant-editor.test.tsx --forceExit 2>&1 | tail -10`
Expected: FAIL — `Cannot find module '@/components/products/VariantEditor'`.

- [ ] **Step 3: Write minimal implementation**

Create `apps/mobile-admin/components/products/VariantEditor.tsx`:

```tsx
import { useState } from "react";
import { View, TextInput, StyleSheet } from "react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { ProductVariant } from "@repo/mobile-shared/api/schemas/products";
import type { UpdateVariantBody } from "@repo/mobile-shared/api/products";

/**
 * The wire has no variant name. A variant is described by its option values
 * ("M / Blue"); the SKU — which every real variant has — is the honest fallback.
 */
export function variantLabel(variant: ProductVariant): string {
  if (variant.option_values.length > 0) {
    return variant.option_values.map((o) => o.value).join(" / ");
  }
  return variant.sku;
}

function FieldLabel({ label }: { label: string }) {
  return (
    <Text preset="caption" color="textTertiary">
      {label}
    </Text>
  );
}

interface NumericFieldProps {
  label: string;
  accessibilityLabel: string;
  initial: number | undefined;
  integer?: boolean;
  onCommit: (value: number) => void;
}

/**
 * Commits on blur, never on change — a PATCH per keystroke would hammer the
 * 60 req/min per-user rate limiter on the mobile routes. Silently does nothing
 * for unparseable or unchanged input: sending NaN would be a silent data loss,
 * which is the bug class this project exists to kill.
 */
function NumericField({ label, accessibilityLabel, initial, integer, onCommit }: NumericFieldProps) {
  const [text, setText] = useState(initial === undefined ? "" : String(initial));

  const handleBlur = () => {
    const trimmed = text.trim();
    if (trimmed === "") return;
    const parsed = integer ? parseInt(trimmed, 10) : parseFloat(trimmed);
    if (Number.isNaN(parsed)) return;
    if (parsed === initial) return;
    onCommit(parsed);
  };

  return (
    <View style={styles.field}>
      <FieldLabel label={label} />
      <TextInput
        style={styles.input}
        value={text}
        onChangeText={setText}
        onBlur={handleBlur}
        keyboardType="decimal-pad"
        accessibilityLabel={accessibilityLabel}
      />
    </View>
  );
}

interface VariantEditorProps {
  variant: ProductVariant;
  onUpdate: (variantId: string, body: UpdateVariantBody) => void;
}

export function VariantEditor({ variant, onUpdate }: VariantEditorProps) {
  const [sku, setSku] = useState(variant.sku);

  const handleSkuBlur = () => {
    const trimmed = sku.trim();
    // SKU is `binding:"required,max=100"` on the wire — an empty one is a 400.
    if (trimmed === "" || trimmed === variant.sku) return;
    onUpdate(variant.id, { sku: trimmed });
  };

  return (
    <View style={styles.root}>
      <Text preset="bodyEmphasis" color="text">
        {variantLabel(variant)}
      </Text>

      <View style={styles.field}>
        <FieldLabel label="SKU" />
        <TextInput
          style={styles.input}
          value={sku}
          onChangeText={setSku}
          onBlur={handleSkuBlur}
          autoCapitalize="characters"
          accessibilityLabel="SKU"
        />
      </View>

      <View style={styles.row}>
        <NumericField
          label={`Price (${variant.currency_code})`}
          accessibilityLabel="Price"
          initial={variant.price}
          onCommit={(price) => onUpdate(variant.id, { price })}
        />
        <NumericField
          label="Stock"
          accessibilityLabel="Stock"
          initial={variant.inventory_quantity}
          integer
          onCommit={(inventory_quantity) => onUpdate(variant.id, { inventory_quantity })}
        />
      </View>

      <Text preset="caption" color="textTertiary">
        Shipping
      </Text>
      <View style={styles.row}>
        <NumericField
          label="Weight (g)"
          accessibilityLabel="Weight in grams"
          initial={variant.weight_grams}
          integer
          onCommit={(weight_grams) => onUpdate(variant.id, { weight_grams })}
        />
        <NumericField
          label="Length (cm)"
          accessibilityLabel="Length in centimetres"
          initial={variant.length_cm}
          onCommit={(length_cm) => onUpdate(variant.id, { length_cm })}
        />
      </View>
      <View style={styles.row}>
        <NumericField
          label="Width (cm)"
          accessibilityLabel="Width in centimetres"
          initial={variant.width_cm}
          onCommit={(width_cm) => onUpdate(variant.id, { width_cm })}
        />
        <NumericField
          label="Height (cm)"
          accessibilityLabel="Height in centimetres"
          initial={variant.height_cm}
          onCommit={(height_cm) => onUpdate(variant.id, { height_cm })}
        />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { gap: theme.spacing.sm, paddingVertical: theme.spacing.md },
  row: { flexDirection: "row", gap: theme.spacing.md },
  field: { flex: 1, gap: theme.spacing.xs },
  input: {
    height: 44,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    paddingHorizontal: theme.spacing.sm,
    color: theme.colors.text,
    backgroundColor: theme.colors.elevated,
  },
});
```

Then in `apps/mobile-admin/app/(tabs)/products/[id].tsx`: delete the local `variantLabel` function,
the `VariantRowProps` interface and the `VariantRow` component, add
`import { VariantEditor } from "@/components/products/VariantEditor";`, and replace the `<VariantRow`
usage in the Variants section with `<VariantEditor`. Remove any now-unused imports (`FieldLabel` stays
if the Details section still uses it) — tsc will name them.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/mobile-admin && npx jest __tests__/variant-editor.test.tsx --forceExit 2>&1 | tail -8`
Expected: PASS.

Run the full suite — `[id].tsx` changed, so its existing tests must still pass:
Run: `cd apps/mobile-admin && npx jest 2>&1 | tail -6`
Expected: all suites pass.

Run both tsc gates. Expected: `0` and `0`.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/components/products/VariantEditor.tsx "apps/mobile-admin/app/(tabs)/products/[id].tsx" apps/mobile-admin/__tests__/variant-editor.test.tsx
git commit -m "feat(mobile-admin): edit variant sku, weight and dimensions"
```

---

### Task 7: CategoryPicker

Categories are a **tree** (`parent_id`). Render them indented by depth; a flat list would misrepresent
the store's structure.

Selection commits `category_ids` (the full desired set — `ReplaceCategoryLinksInTx` replaces, it does
not merge). Requires Task 1's backend fix to persist.

**Files:**
- Create: `apps/mobile-admin/components/products/CategoryPicker.tsx`
- Test: `apps/mobile-admin/__tests__/category-picker.test.tsx`

**Interfaces:**
- Consumes: `Category`, `CategoryRef` (Task 3); `useCategories` (Task 4).
- Produces: `<CategoryPicker categories={Category[]} selected={CategoryRef[]} onChange={(ids: string[]) => void} />`
  and the exported pure helper `sortCategoryTree(categories: Category[]): Array<{ category: Category; depth: number }>`.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/category-picker.test.tsx`:

```tsx
import { render, fireEvent } from "@testing-library/react-native";
import { CategoryPicker, sortCategoryTree } from "@/components/products/CategoryPicker";

const cat = (id: string, name: string, parent_id?: string) => ({
  id,
  store_id: "s1",
  name,
  slug: name.toLowerCase(),
  position: 0,
  is_active: true,
  featured: false,
  created_at: "2026-05-04T23:48:01Z",
  updated_at: "2026-05-04T23:48:01Z",
  ...(parent_id ? { parent_id } : {}),
});

describe("sortCategoryTree", () => {
  it("nests children under their parent with a depth", () => {
    const flat = [cat("c2", "Bikinis", "c1"), cat("c1", "Swimwear"), cat("c3", "Towels")];
    const tree = sortCategoryTree(flat as never);
    expect(tree.map((n) => [n.category.name, n.depth])).toEqual([
      ["Swimwear", 0],
      ["Bikinis", 1],
      ["Towels", 0],
    ]);
  });

  it("keeps an orphan (parent_id pointing nowhere) visible at root rather than dropping it", () => {
    const flat = [cat("c9", "Orphan", "missing-parent")];
    const tree = sortCategoryTree(flat as never);
    expect(tree).toHaveLength(1);
    expect(tree[0]!.depth).toBe(0);
  });

  it("does not infinitely recurse on a self-referencing parent", () => {
    const flat = [cat("c1", "Loop", "c1")];
    expect(sortCategoryTree(flat as never)).toHaveLength(1);
  });
});

describe("CategoryPicker", () => {
  const CATS = [cat("c1", "Swimwear"), cat("c2", "Bikinis", "c1")];

  it("emits the full desired id set when one is added", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <CategoryPicker
        categories={CATS as never}
        selected={[{ id: "c1", name: "Swimwear", slug: "swimwear" }]}
        onChange={onChange}
      />,
    );
    fireEvent.press(getByLabelText("Bikinis"));
    // Replace semantics: send the whole set, not a delta.
    expect(onChange).toHaveBeenCalledWith(["c1", "c2"]);
  });

  it("emits the remaining set when one is removed", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <CategoryPicker
        categories={CATS as never}
        selected={[
          { id: "c1", name: "Swimwear", slug: "swimwear" },
          { id: "c2", name: "Bikinis", slug: "bikinis" },
        ]}
        onChange={onChange}
      />,
    );
    fireEvent.press(getByLabelText("Bikinis"));
    expect(onChange).toHaveBeenCalledWith(["c1"]);
  });

  it("emits an empty array when the last category is removed", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <CategoryPicker
        categories={CATS as never}
        selected={[{ id: "c1", name: "Swimwear", slug: "swimwear" }]}
        onChange={onChange}
      />,
    );
    fireEvent.press(getByLabelText("Swimwear"));
    expect(onChange).toHaveBeenCalledWith([]);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/category-picker.test.tsx --forceExit 2>&1 | tail -10`
Expected: FAIL — `Cannot find module '@/components/products/CategoryPicker'`.

- [ ] **Step 3: Write minimal implementation**

Create `apps/mobile-admin/components/products/CategoryPicker.tsx`:

```tsx
import { View, Pressable, StyleSheet } from "react-native";
import { Check } from "lucide-react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Category, CategoryRef } from "@repo/mobile-shared/api/schemas/categories";

export interface CategoryNode {
  category: Category;
  depth: number;
}

/**
 * Flattens the category tree into a render order, depth-tagged.
 *
 * Robust to bad data on purpose: a category whose parent_id points at a
 * category that doesn't exist (or at itself) is surfaced at root rather than
 * silently dropped — an invisible category is worse than an oddly-placed one.
 */
export function sortCategoryTree(categories: Category[]): CategoryNode[] {
  const byId = new Map(categories.map((c) => [c.id, c]));
  const childrenOf = new Map<string, Category[]>();
  const roots: Category[] = [];

  for (const c of categories) {
    const parentId = c.parent_id;
    const hasRealParent = parentId !== undefined && parentId !== c.id && byId.has(parentId);
    if (hasRealParent) {
      const siblings = childrenOf.get(parentId!) ?? [];
      childrenOf.set(parentId!, [...siblings, c]);
    } else {
      roots.push(c);
    }
  }

  const byPosition = (a: Category, b: Category) => a.position - b.position;
  const out: CategoryNode[] = [];
  const walk = (nodes: Category[], depth: number) => {
    for (const c of [...nodes].sort(byPosition)) {
      out.push({ category: c, depth });
      walk(childrenOf.get(c.id) ?? [], depth + 1);
    }
  };
  walk(roots, 0);
  return out;
}

interface CategoryPickerProps {
  categories: Category[];
  /** What the product currently links to — the lean refs the product embeds. */
  selected: CategoryRef[];
  /** Receives the FULL desired id set. The backend REPLACES links, not merges. */
  onChange: (ids: string[]) => void;
}

export function CategoryPicker({ categories, selected, onChange }: CategoryPickerProps) {
  const selectedIds = new Set(selected.map((c) => c.id));
  const nodes = sortCategoryTree(categories);

  const toggle = (id: string) => {
    const next = new Set(selectedIds);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    // Preserve the store's own ordering rather than Set insertion order.
    onChange(nodes.map((n) => n.category.id).filter((cid) => next.has(cid)));
  };

  return (
    <View style={styles.root}>
      {nodes.map(({ category, depth }) => {
        const isSelected = selectedIds.has(category.id);
        return (
          <Pressable
            key={category.id}
            style={[styles.row, { paddingLeft: theme.spacing.md * depth }]}
            onPress={() => toggle(category.id)}
            accessibilityRole="checkbox"
            accessibilityState={{ checked: isSelected }}
            accessibilityLabel={category.name}
          >
            <View style={[styles.box, isSelected && styles.boxChecked]}>
              {isSelected ? <Check size={12} color={theme.colors.inverse} strokeWidth={3} /> : null}
            </View>
            <Text preset="body" color={isSelected ? "text" : "textSecondary"}>
              {category.name}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { gap: theme.spacing.xs },
  row: { flexDirection: "row", alignItems: "center", gap: theme.spacing.sm, height: 44 },
  box: {
    width: 20,
    height: 20,
    borderRadius: theme.radii.sm,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    alignItems: "center",
    justifyContent: "center",
  },
  boxChecked: { backgroundColor: theme.colors.accent, borderColor: theme.colors.accent },
});
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/mobile-admin && npx jest __tests__/category-picker.test.tsx --forceExit 2>&1 | tail -8`
Expected: PASS.

Run both tsc gates. Expected: `0` and `0`.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/components/products/CategoryPicker.tsx apps/mobile-admin/__tests__/category-picker.test.tsx
git commit -m "feat(mobile-admin): add a tree-aware category picker"
```

---

### Task 8: OptionsEditor

The only feature needing the **aggregate** path. Emits the REQUEST shape: `{name, values: string[]}`.

⚠️ Sending `options` triggers `applyOptionsDiff`, which reconciles variants against the new option
matrix. This is the highest-risk write in the plan. The editor therefore emits the **complete**
desired option set, always — derived from the response shape, converted to the request shape.

**Files:**
- Create: `apps/mobile-admin/components/products/OptionsEditor.tsx`
- Test: `apps/mobile-admin/__tests__/options-editor.test.tsx`

**Interfaces:**
- Consumes: `ProductOption` (response shape, existing), `UpdateProductOptionBody` (Task 5).
- Produces: `<OptionsEditor options={ProductOption[]} onChange={(options: UpdateProductOptionBody[]) => void} />`
  and the exported pure helper
  `toOptionRequestBodies(options: ProductOption[]): UpdateProductOptionBody[]`.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/options-editor.test.tsx`:

```tsx
import { render, fireEvent } from "@testing-library/react-native";
import { OptionsEditor, toOptionRequestBodies } from "@/components/products/OptionsEditor";

const RESPONSE_OPTIONS = [
  {
    id: "opt-1",
    name: "Size",
    position: 0,
    values: [
      { id: "v2", value: "M", position: 1 },
      { id: "v1", value: "S", position: 0 },
    ],
  },
];

describe("toOptionRequestBodies", () => {
  it("converts the RESPONSE shape [{id,value,position}] to the REQUEST shape string[]", () => {
    expect(toOptionRequestBodies(RESPONSE_OPTIONS as never)).toEqual([
      { name: "Size", values: ["S", "M"] },
    ]);
  });

  it("orders values by position — the wire does not guarantee order", () => {
    const [first] = toOptionRequestBodies(RESPONSE_OPTIONS as never);
    expect(first!.values).toEqual(["S", "M"]);
  });

  it("returns [] for a product with no options", () => {
    expect(toOptionRequestBodies([])).toEqual([]);
  });
});

describe("OptionsEditor", () => {
  it("emits the COMPLETE desired option set when a value is added", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <OptionsEditor options={RESPONSE_OPTIONS as never} onChange={onChange} />,
    );
    const input = getByLabelText("Add a value to Size");
    fireEvent.changeText(input, "L");
    fireEvent(input, "submitEditing");
    expect(onChange).toHaveBeenCalledWith([{ name: "Size", values: ["S", "M", "L"] }]);
  });

  it("emits the set without the removed value", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <OptionsEditor options={RESPONSE_OPTIONS as never} onChange={onChange} />,
    );
    fireEvent.press(getByLabelText("Remove M from Size"));
    expect(onChange).toHaveBeenCalledWith([{ name: "Size", values: ["S"] }]);
  });

  it("ignores a blank value rather than sending an empty string", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <OptionsEditor options={RESPONSE_OPTIONS as never} onChange={onChange} />,
    );
    const input = getByLabelText("Add a value to Size");
    fireEvent.changeText(input, "   ");
    fireEvent(input, "submitEditing");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("ignores a duplicate value — the backend keys variants off the tuple", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <OptionsEditor options={RESPONSE_OPTIONS as never} onChange={onChange} />,
    );
    const input = getByLabelText("Add a value to Size");
    fireEvent.changeText(input, "S");
    fireEvent(input, "submitEditing");
    expect(onChange).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/options-editor.test.tsx --forceExit 2>&1 | tail -10`
Expected: FAIL — `Cannot find module '@/components/products/OptionsEditor'`.

- [ ] **Step 3: Write minimal implementation**

Create `apps/mobile-admin/components/products/OptionsEditor.tsx`:

```tsx
import { useState } from "react";
import { View, TextInput, Pressable, StyleSheet } from "react-native";
import { X } from "lucide-react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { ProductOption } from "@repo/mobile-shared/api/schemas/products";
import type { UpdateProductOptionBody } from "@repo/mobile-shared/api/products";

/**
 * Converts the RESPONSE option shape into the REQUEST one.
 *
 * 🔴 The response sends `values: [{id, value, position}]`; the request wants
 * `values: string[]`. Same field name, two shapes — the single most expensive
 * recurring bug on this project. This function is the ONLY place that bridges
 * them, and __tests__/product-request-bodies.test.tsx pins both sides.
 *
 * Values are ordered by `position`: the wire does not guarantee array order
 * (variants demonstrably come back 2,3,4,0,1).
 */
export function toOptionRequestBodies(options: ProductOption[]): UpdateProductOptionBody[] {
  return [...options]
    .sort((a, b) => a.position - b.position)
    .map((o) => ({
      name: o.name,
      values: [...o.values].sort((a, b) => a.position - b.position).map((v) => v.value),
    }));
}

interface OptionsEditorProps {
  options: ProductOption[];
  /**
   * Receives the COMPLETE desired option set, never a delta — sending `options`
   * routes the PATCH through UpdateAggregate, whose applyOptionsDiff reconciles
   * the whole variant matrix against what it is given.
   */
  onChange: (options: UpdateProductOptionBody[]) => void;
}

export function OptionsEditor({ options, onChange }: OptionsEditorProps) {
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const current = toOptionRequestBodies(options);

  const addValue = (optionName: string) => {
    const raw = drafts[optionName] ?? "";
    const value = raw.trim();
    if (value === "") return;
    const target = current.find((o) => o.name === optionName);
    if (!target || target.values.includes(value)) return;
    setDrafts((d) => ({ ...d, [optionName]: "" }));
    onChange(
      current.map((o) => (o.name === optionName ? { ...o, values: [...o.values, value] } : o)),
    );
  };

  const removeValue = (optionName: string, value: string) => {
    onChange(
      current.map((o) =>
        o.name === optionName ? { ...o, values: o.values.filter((v) => v !== value) } : o,
      ),
    );
  };

  return (
    <View style={styles.root}>
      {current.map((option) => (
        <View key={option.name} style={styles.option}>
          <Text preset="bodyEmphasis" color="text">
            {option.name}
          </Text>
          <View style={styles.chips}>
            {option.values.map((value) => (
              <View key={value} style={styles.chip}>
                <Text preset="caption" color="text">
                  {value}
                </Text>
                <Pressable
                  onPress={() => removeValue(option.name, value)}
                  accessibilityRole="button"
                  accessibilityLabel={`Remove ${value} from ${option.name}`}
                  hitSlop={8}
                >
                  <X size={12} color={theme.colors.textTertiary} strokeWidth={2.5} />
                </Pressable>
              </View>
            ))}
          </View>
          <TextInput
            style={styles.input}
            value={drafts[option.name] ?? ""}
            onChangeText={(t) => setDrafts((d) => ({ ...d, [option.name]: t }))}
            onSubmitEditing={() => addValue(option.name)}
            placeholder={`Add a ${option.name.toLowerCase()}…`}
            placeholderTextColor={theme.colors.textTertiary}
            accessibilityLabel={`Add a value to ${option.name}`}
            returnKeyType="done"
          />
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { gap: theme.spacing.md },
  option: { gap: theme.spacing.sm },
  chips: { flexDirection: "row", flexWrap: "wrap", gap: theme.spacing.xs },
  chip: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.xs,
    paddingHorizontal: theme.spacing.sm,
    height: 32,
    borderRadius: theme.radii.sm,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    backgroundColor: theme.colors.elevated,
  },
  input: {
    height: 44,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    paddingHorizontal: theme.spacing.sm,
    color: theme.colors.text,
    backgroundColor: theme.colors.elevated,
  },
});
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/mobile-admin && npx jest __tests__/options-editor.test.tsx --forceExit 2>&1 | tail -8`
Expected: PASS.

Run both tsc gates. Expected: `0` and `0`.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/components/products/OptionsEditor.tsx apps/mobile-admin/__tests__/options-editor.test.tsx
git commit -m "feat(mobile-admin): edit product options"
```

---

### Task 9: MediaGrid — reorder and alt text

`PATCH /media/:mediaId` accepts `{alt, position}` → **204 No Content**. Reorder is a position PATCH.
Position 0 is the hero image.

No drag-and-drop: `@dnd-kit` is web-only and a new RN drag library is forbidden (no `npm install`).
Move-left / move-right buttons are honest, accessible, and need nothing new.

**Files:**
- Create: `apps/mobile-admin/components/products/MediaGrid.tsx`
- Modify: `packages/mobile-shared/api/products.ts` (add `updateMedia`)
- Modify: `apps/mobile-admin/lib/admin-api/product-crud.ts` (add `useUpdateMedia`)
- Test: `apps/mobile-admin/__tests__/media-grid.test.tsx`

**Interfaces:**
- Consumes: `ProductMedia` (existing).
- Produces:
  - `UpdateMediaBody = { alt?: string; position?: number }`
  - `productsApi.updateMedia(productId, mediaId, body) => Promise<void>`
  - `useUpdateMedia()` react-query mutation taking `{productId, mediaId, body}`
  - `<MediaGrid media={ProductMedia[]} onReorder={(mediaId: string, position: number) => void} onAltChange={(mediaId: string, alt: string) => void} onPress={(m: ProductMedia) => void} onLongPress={(mediaId: string) => void} />`
  - exported helper `sortMedia(media: ProductMedia[]): ProductMedia[]`

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/media-grid.test.tsx`:

```tsx
import { render, fireEvent } from "@testing-library/react-native";
import { MediaGrid, sortMedia } from "@/components/products/MediaGrid";

const media = (id: string, position: number, alt?: string) => ({
  id,
  url: `https://cdn.mark8ly.com/${id}.png`,
  storage_key: `tenants/x/${id}.png`,
  position,
  media_type: "image",
  ...(alt ? { alt } : {}),
});

const UNSORTED = [media("c", 2), media("a", 0), media("b", 1)];

describe("sortMedia", () => {
  it("orders by position — the wire does not guarantee array order", () => {
    expect(sortMedia(UNSORTED as never).map((m) => m.id)).toEqual(["a", "b", "c"]);
  });

  it("does not mutate its input", () => {
    const input = [...UNSORTED];
    sortMedia(input as never);
    expect(input.map((m) => m.id)).toEqual(["c", "a", "b"]);
  });
});

describe("MediaGrid", () => {
  const noop = () => {};

  it("marks the position-0 photo as the hero", () => {
    const { getByLabelText } = render(
      <MediaGrid
        media={UNSORTED as never}
        onReorder={noop}
        onAltChange={noop}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    expect(getByLabelText("Photo 1, main image")).toBeTruthy();
  });

  it("moving a photo left emits its new position", () => {
    const onReorder = jest.fn();
    const { getByLabelText } = render(
      <MediaGrid
        media={UNSORTED as never}
        onReorder={onReorder}
        onAltChange={noop}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    fireEvent.press(getByLabelText("Move photo 2 earlier"));
    expect(onReorder).toHaveBeenCalledWith("b", 0);
  });

  it("the first photo cannot move earlier", () => {
    const { queryByLabelText } = render(
      <MediaGrid
        media={UNSORTED as never}
        onReorder={noop}
        onAltChange={noop}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    expect(queryByLabelText("Move photo 1 earlier")).toBeNull();
  });

  it("the last photo cannot move later", () => {
    const { queryByLabelText } = render(
      <MediaGrid
        media={UNSORTED as never}
        onReorder={noop}
        onAltChange={noop}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    expect(queryByLabelText("Move photo 3 later")).toBeNull();
  });

  it("commits alt text on blur", () => {
    const onAltChange = jest.fn();
    const { getByLabelText } = render(
      <MediaGrid
        media={[media("a", 0)] as never}
        onReorder={noop}
        onAltChange={onAltChange}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    const input = getByLabelText("Alt text for photo 1");
    fireEvent.changeText(input, "Linen robe, front");
    fireEvent(input, "blur");
    expect(onAltChange).toHaveBeenCalledWith("a", "Linen robe, front");
  });

  it("does not re-commit unchanged alt text", () => {
    const onAltChange = jest.fn();
    const { getByLabelText } = render(
      <MediaGrid
        media={[media("a", 0, "Existing")] as never}
        onReorder={noop}
        onAltChange={onAltChange}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    fireEvent(getByLabelText("Alt text for photo 1"), "blur");
    expect(onAltChange).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/media-grid.test.tsx --forceExit 2>&1 | tail -10`
Expected: FAIL — `Cannot find module '@/components/products/MediaGrid'`.

- [ ] **Step 3: Write minimal implementation**

In `packages/mobile-shared/api/products.ts`, add the body type near `CreateMediaBody`:

```ts
/**
 * PATCH /products/{id}/media/{mediaId} (UpdateMediaWireRequest,
 * validation.go:85). Returns 204 No Content — there is no body to parse, so
 * no schema is passed. Reorder is a `position` patch; position 0 is the hero.
 */
export interface UpdateMediaBody {
  alt?: string;
  position?: number;
}
```

and add to the object returned by `createProductsApi`:

```ts
    updateMedia: (productId: string, mediaId: string, body: UpdateMediaBody) =>
      client.patch(`/products/${productId}/media/${mediaId}`, body),
```

Append to `apps/mobile-admin/lib/admin-api/product-crud.ts`, mirroring `useUpdateVariant`
(`product-crud.ts:66-85`) exactly — same `productsApi` const, same invalidation key:

```ts
export function useUpdateMedia() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      productId,
      mediaId,
      body,
    }: {
      productId: string;
      mediaId: string;
      body: UpdateMediaBody;
    }) => productsApi.updateMedia(productId, mediaId, body),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ["product", variables.productId] });
    },
  });
}
```

Add `type UpdateMediaBody` to that file's existing type import from
`@repo/mobile-shared/api/products` (which already imports `CreateProductBody`, `UpdateProductBody`,
`UpdateVariantBody`, `ProductMedia`).

Create `apps/mobile-admin/components/products/MediaGrid.tsx`:

```tsx
import { useState } from "react";
import { View, Image, TextInput, Pressable, ScrollView, StyleSheet } from "react-native";
import { ChevronLeft, ChevronRight } from "lucide-react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { ProductMedia } from "@repo/mobile-shared/api/schemas/products";

/** The wire does not guarantee array order — order by position. Pure; no mutation. */
export function sortMedia(media: ProductMedia[]): ProductMedia[] {
  return [...media].sort((a, b) => a.position - b.position);
}

interface MediaGridProps {
  media: ProductMedia[];
  onReorder: (mediaId: string, position: number) => void;
  onAltChange: (mediaId: string, alt: string) => void;
  onPress: (media: ProductMedia) => void;
  onLongPress: (mediaId: string) => void;
}

interface AltInputProps {
  media: ProductMedia;
  index: number;
  onAltChange: (mediaId: string, alt: string) => void;
}

function AltInput({ media, index, onAltChange }: AltInputProps) {
  const [alt, setAlt] = useState(media.alt ?? "");
  const handleBlur = () => {
    const trimmed = alt.trim();
    if (trimmed === (media.alt ?? "")) return;
    onAltChange(media.id, trimmed);
  };
  return (
    <TextInput
      style={styles.alt}
      value={alt}
      onChangeText={setAlt}
      onBlur={handleBlur}
      placeholder="Alt text"
      placeholderTextColor={theme.colors.textTertiary}
      accessibilityLabel={`Alt text for photo ${index + 1}`}
    />
  );
}

/**
 * Reorder is move-earlier/move-later, not drag-and-drop: @dnd-kit (what the web
 * admin uses) is web-only, and adding an RN drag library would mean an npm
 * install, which this repo forbids. Buttons are also more accessible.
 */
export function MediaGrid({ media, onReorder, onAltChange, onPress, onLongPress }: MediaGridProps) {
  const ordered = sortMedia(media);

  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.root}>
      {ordered.map((m, i) => (
        <View key={m.id} style={styles.item}>
          <Pressable
            onPress={() => onPress(m)}
            onLongPress={() => onLongPress(m.id)}
            accessibilityRole="imagebutton"
            accessibilityLabel={i === 0 ? `Photo 1, main image` : `Photo ${i + 1}`}
          >
            <Image source={{ uri: m.url }} style={styles.thumb} />
          </Pressable>

          <View style={styles.controls}>
            {i > 0 ? (
              <Pressable
                onPress={() => onReorder(m.id, i - 1)}
                accessibilityRole="button"
                accessibilityLabel={`Move photo ${i + 1} earlier`}
                hitSlop={8}
              >
                <ChevronLeft size={16} color={theme.colors.text} strokeWidth={2} />
              </Pressable>
            ) : null}
            {i === 0 ? (
              <Text preset="caption" color="textTertiary">
                Main
              </Text>
            ) : null}
            {i < ordered.length - 1 ? (
              <Pressable
                onPress={() => onReorder(m.id, i + 1)}
                accessibilityRole="button"
                accessibilityLabel={`Move photo ${i + 1} later`}
                hitSlop={8}
              >
                <ChevronRight size={16} color={theme.colors.text} strokeWidth={2} />
              </Pressable>
            ) : null}
          </View>

          <AltInput media={m} index={i} onAltChange={onAltChange} />
        </View>
      ))}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  root: { gap: theme.spacing.sm, paddingVertical: theme.spacing.xs },
  item: { gap: theme.spacing.xs, width: 120 },
  thumb: {
    width: 120,
    height: 120,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
  },
  controls: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    height: 24,
  },
  alt: {
    height: 36,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.sm,
    paddingHorizontal: theme.spacing.xs,
    fontSize: 12,
    color: theme.colors.text,
    backgroundColor: theme.colors.elevated,
  },
});
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/mobile-admin && npx jest __tests__/media-grid.test.tsx --forceExit 2>&1 | tail -8`
Expected: PASS.

Run both tsc gates. Expected: `0` and `0`.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/components/products/MediaGrid.tsx packages/mobile-shared/api/products.ts apps/mobile-admin/lib/admin-api/product-crud.ts apps/mobile-admin/__tests__/media-grid.test.tsx
git commit -m "feat(mobile-admin): reorder product photos and edit alt text"
```

---

### Task 10: Extract ImageViewer and add pick-time crop

Two small changes to `[id].tsx`, together because both touch the media path and each alone is too
small for its own review gate.

🔴 **Do NOT reintroduce `requestMediaLibraryPermissionsAsync()`.** `51d2e80b` removed it deliberately
and the fix is verified on device: `launchImageLibraryAsync` uses PHPicker, which runs
out-of-process and needs **no** library permission. Asking opts into the legacy flow, where "Limited
Access" strands the user in iOS's limited-library management sheet and the picker never opens.
`allowsEditing` is orthogonal to permission and safe.

**Files:**
- Create: `apps/mobile-admin/components/products/ImageViewer.tsx`
- Modify: `apps/mobile-admin/app/(tabs)/products/[id].tsx`
- Test: `apps/mobile-admin/__tests__/add-product-media.test.tsx` (append)

**Interfaces:**
- Produces: `<ImageViewer image={{ uri: string; alt?: string } | null} onClose={() => void} />`.

- [ ] **Step 1: Write the failing test**

Append to `apps/mobile-admin/__tests__/add-product-media.test.tsx`:

```tsx
describe("image picker options", () => {
  it("requests the system cropper and never asks for library permission", () => {
    // Guard for 51d2e80b: requestMediaLibraryPermissionsAsync opts into the
    // legacy flow, where "Limited Access" strands the user in iOS's
    // limited-library management sheet and the real picker never opens.
    const source = require("fs").readFileSync(
      require("path").join(__dirname, "../app/(tabs)/products/[id].tsx"),
      "utf8",
    );
    expect(source).toContain("allowsEditing: true");
    expect(source).not.toContain("requestMediaLibraryPermissionsAsync");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/add-product-media.test.tsx --forceExit 2>&1 | tail -10`
Expected: FAIL — `expect(source).toContain("allowsEditing: true")` — the option isn't set yet.
(The `not.toContain` half already passes; that is the regression guard.)

- [ ] **Step 3: Write minimal implementation**

Create `apps/mobile-admin/components/products/ImageViewer.tsx`, moving the full-screen Modal that
`ab27c6ff` added to `[id].tsx` into it verbatim, with this signature:

```tsx
import { Modal, View, Image, Pressable, StyleSheet } from "react-native";
import { X } from "lucide-react-native";
import { theme } from "@/lib/theme";

export interface ViewerImage {
  uri: string;
  alt?: string;
}

interface ImageViewerProps {
  image: ViewerImage | null;
  onClose: () => void;
}

/** Full-screen photo viewer. Extracted from [id].tsx, which was 571 lines. */
export function ImageViewer({ image, onClose }: ImageViewerProps) {
  return (
    <Modal
      visible={image !== null}
      transparent
      animationType="fade"
      onRequestClose={onClose}
    >
      <View style={styles.backdrop}>
        <Pressable
          style={styles.close}
          onPress={onClose}
          accessibilityRole="button"
          accessibilityLabel="Close photo"
          hitSlop={12}
        >
          <X size={24} color={theme.colors.inverse} strokeWidth={2} />
        </Pressable>
        {image ? (
          <Image
            source={{ uri: image.uri }}
            style={styles.image}
            resizeMode="contain"
            accessibilityLabel={image.alt ?? "Product photo"}
          />
        ) : null}
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: "rgba(0,0,0,0.92)", justifyContent: "center" },
  close: { position: "absolute", top: 56, right: 20, zIndex: 1 },
  image: { width: "100%", height: "80%" },
});
```

In `[id].tsx`: delete the inline Modal markup and its styles, import `ImageViewer`, and render
`<ImageViewer image={viewerImage} onClose={() => setViewerImage(null)} />`. Keep the existing
`viewerImage` state.

In the same file, add `allowsEditing: true` to the picker call in `handleAddMedia`:

```tsx
      const result = await ImagePicker.launchImageLibraryAsync({
        mediaTypes: ["images"],
        quality: 0.8,
        // The system cropper, shown after the photo is chosen and before upload.
        // Orthogonal to permission — see the comment above; do NOT add a
        // permission request alongside it.
        allowsEditing: true,
      });
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/mobile-admin && npx jest __tests__/add-product-media.test.tsx --forceExit 2>&1 | tail -8`
Expected: PASS.

Run: `cd apps/mobile-admin && npx jest 2>&1 | tail -6`
Expected: all suites pass.

Run both tsc gates. Expected: `0` and `0`.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/components/products/ImageViewer.tsx "apps/mobile-admin/app/(tabs)/products/[id].tsx" apps/mobile-admin/__tests__/add-product-media.test.tsx
git commit -m "feat(mobile-admin): crop photos at pick time and extract the image viewer"
```

---

### Task 11: Wire it together in the product detail screen

Composes Tasks 4-9 into `[id].tsx`. The screen becomes composition only.

**Routing rule (from `products.go:172`) — send only what changed:**
- `options` changed → `updateProduct(id, { options })` (aggregate path)
- `category_ids` changed → `updateProduct(id, { category_ids })` (aggregate path, needs Task 1)
- title/description/status changed → `updateProduct(id, { ...scalars })` (basics path)
- any variant field changed → `updateVariant(productId, variantId, body)` (quick-PATCH, no aggregate)
- media alt/position changed → `updateMedia(productId, mediaId, body)`

**Files:**
- Modify: `apps/mobile-admin/app/(tabs)/products/[id].tsx`
- Test: `apps/mobile-admin/__tests__/product-detail-sections.test.tsx`

**Interfaces:**
- Consumes: `useCategories` (Task 4), `useUpdateMedia` (Task 9), `VariantEditor` (Task 6),
  `CategoryPicker` (Task 7), `OptionsEditor` (Task 8), `MediaGrid` (Task 9), `ImageViewer` (Task 10),
  and the existing `useUpdateProduct` / `useUpdateVariant`.
- Produces: no new exports.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/product-detail-sections.test.tsx`:

```tsx
// The detail screen pulls the real api client (and Firebase auth) through its
// hook imports. Mock it the same way use-products.test.tsx does.
jest.mock("@/lib/api-client", () => ({
  useApiClient: () => ({}),
}));

const source = require("fs").readFileSync(
  require("path").join(__dirname, "../app/(tabs)/products/[id].tsx"),
  "utf8",
);

describe("product detail screen composition", () => {
  it("renders every new section", () => {
    expect(source).toContain("<OptionsEditor");
    expect(source).toContain("<CategoryPicker");
    expect(source).toContain("<VariantEditor");
    expect(source).toContain("<MediaGrid");
    expect(source).toContain("<ImageViewer");
  });

  it("never sends `variants` on a product PATCH — that matrix soft-deletes", () => {
    // UpdateAggregateRequest.Variants is a FULL DESIRED MATRIX; applyVariantsDiff
    // soft-deletes anything missing from it. Variant edits go through the
    // variant quick-PATCH instead.
    expect(source).not.toMatch(/updateProduct[\s\S]{0,200}variants:/);
  });

  it("stayed small after composing — it was 571 lines before extraction", () => {
    expect(source.split("\n").length).toBeLessThan(500);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/product-detail-sections.test.tsx --forceExit 2>&1 | tail -10`
Expected: FAIL — `expect(source).toContain("<OptionsEditor")` — the sections aren't wired yet.

- [ ] **Step 3: Write minimal implementation**

In `apps/mobile-admin/app/(tabs)/products/[id].tsx`:

Add imports:

```tsx
import { OptionsEditor } from "@/components/products/OptionsEditor";
import { CategoryPicker } from "@/components/products/CategoryPicker";
import { MediaGrid } from "@/components/products/MediaGrid";
import { useCategories, useUpdateMedia } from "@/lib/admin-api/product-crud";
import type { UpdateProductOptionBody } from "@repo/mobile-shared/api/products";
```

Inside the component, add:

```tsx
  const { data: categories } = useCategories();
  const updateMediaMutation = useUpdateMedia();

  // Options and categories both route through UpdateAggregate (products.go:172).
  // Send ONLY the changed section — never bundle `variants` in, because
  // UpdateAggregateRequest.Variants is a full desired matrix that soft-deletes
  // anything it omits.
  const handleOptionsChange = useCallback(
    (options: UpdateProductOptionBody[]) => {
      updateMutation.mutate({ id, body: { options } });
    },
    [id, updateMutation],
  );

  const handleCategoriesChange = useCallback(
    (category_ids: string[]) => {
      updateMutation.mutate({ id, body: { category_ids } });
    },
    [id, updateMutation],
  );

  const handleReorderMedia = useCallback(
    (mediaId: string, position: number) => {
      updateMediaMutation.mutate({ productId: id, mediaId, body: { position } });
    },
    [id, updateMediaMutation],
  );

  const handleAltChange = useCallback(
    (mediaId: string, alt: string) => {
      updateMediaMutation.mutate({ productId: id, mediaId, body: { alt } });
    },
    [id, updateMediaMutation],
  );
```

`useUpdateProduct`'s `mutationFn` takes `{id, body}` (`product-crud.ts:36`), so the calls above are
correct as written. Name the hook result to match whatever `[id].tsx` already calls it.

Replace the Photos section body with:

```tsx
        <MediaGrid
          media={product.media}
          onReorder={handleReorderMedia}
          onAltChange={handleAltChange}
          onPress={(m) => setViewerImage({ uri: m.url, alt: m.alt })}
          onLongPress={handleDeleteMedia}
        />
```

Add these sections after Details, following the existing `<Eyebrow label="…" />` pattern:

```tsx
        <Eyebrow label="Options" />
        <OptionsEditor options={product.options} onChange={handleOptionsChange} />

        <Eyebrow label="Categories" />
        <CategoryPicker
          categories={categories ?? []}
          selected={product.categories}
          onChange={handleCategoriesChange}
        />
```

Ensure the Variants section maps `product.variants` **sorted by position** into `<VariantEditor>` —
use the existing sort helper in `lib/product-display.ts` rather than writing a new one.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/mobile-admin && npx jest __tests__/product-detail-sections.test.tsx --forceExit 2>&1 | tail -8`
Expected: PASS.

Run the full suite and both tsc gates:
```bash
cd apps/mobile-admin && npx jest 2>&1 | tail -6
cd apps/mobile-admin && npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"
cd ../../packages/mobile-shared && npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"
```
Expected: all suites pass; `0`; `0`.

- [ ] **Step 5: Verify on the simulator**

Metro hot-reloads. Deep-link to the **list** (never `://products/<id>` — that pushes detail with no
list beneath it, so Back exits to the dashboard and looks like a navigation bug):

```bash
xcrun simctl openurl AD109A46-2F99-43C3-8AAA-FEE68DC8499E "mark8ly-admin://products"
xcrun simctl io AD109A46-2F99-43C3-8AAA-FEE68DC8499E screenshot /tmp/detail.png
```

You cannot tap (`idb` absent, AppleScript `-1719`) — **ask the human** to open a multi-variant
product and confirm: options render with their values; categories show the tree with the product's
own ticked; each variant shows SKU/weight/dimensions; photos reorder and the "Main" badge follows
position 0; Add photo opens the cropper.

Pick a genuinely multi-variant product — 8 of the 12 active ones are, and single-variant products
would hide every option bug.

- [ ] **Step 6: Commit**

```bash
git add "apps/mobile-admin/app/(tabs)/products/[id].tsx" apps/mobile-admin/__tests__/product-detail-sections.test.tsx
git commit -m "feat(mobile-admin): compose options, categories and media sections into product detail"
```

---

## Deploying Task 1

Task 1 touches `marketplace-api`, which **does** deploy (unlike mobile-admin, which ships via
`eas build` → TestFlight and has no prod deploy at all).

- Images promote via **Kargo Freight (uat→smoke→prod)**, NOT by editing ArgoCD `image.tag`.
  `kubectl` Promotions are RBAC-blocked — use the kargo CLI/UI.
- If CI fails with the GitHub Actions **billing** error, temporarily flip the repo public, run CI,
  then flip it back to private.
- `gh` is authed as the WORK account and **cannot see** `tesserix/mark8ly` (404). Run
  `gh auth switch --user mahesh-sangawar` first, and switch back after. Git push over SSH works
  regardless.

**The mobile client work (Tasks 2-11) does not depend on Task 1 being deployed** — only the
categories *section* does. Tasks 2-11 can land and be verified while Task 1 promotes.
