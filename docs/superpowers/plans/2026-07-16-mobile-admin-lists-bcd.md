# mobile-admin Lists (B/C/D) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the 161 real products and 1 real customer that exist in the Bondi store actually appear in the mobile-admin app, by migrating every list hook off a fictional `{items}` envelope onto the real wire shape — and make this class of bug fail loudly instead of silently.

**Architecture:** Wire-truthful zod schemas are the single source of truth; TypeScript types come from `z.infer`, so types can never drift from validation again. Screens adapt to the real wire shape (never the reverse). The API client already throws `ApiError(500, "contract_mismatch", "<field.path>: <msg>")` on a schema miss — that is the debugging tool for this entire plan. Task 1 makes the demo client apply schemas too, which converts a whole class of silent runtime breakage into loud dev-time parse errors.

**Tech Stack:** TypeScript 5, Expo 56 / React Native 0.85, React 19, zod 4, @tanstack/react-query v5, jest.

**Spec:** `docs/superpowers/specs/2026-07-16-mobile-admin-lists-bcd-design.md`

## Global Constraints

Every task's requirements implicitly include this section.

- **Work from `apps/mobile-admin/`** for all gate commands.
- **Gate 1 — tsc:** `npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` → **2** for Tasks 1–5, **0** after Task 6. **`--pretty false` is MANDATORY** — without it tsc emits ANSI colour codes, the literal `error TS` never appears, `grep -c` returns **0 while real errors exist**, and the gate passes vacuously. **Count the errors — never grep by filename** (a per-file grep passed vacuously and missed 6 real errors in the previous session).
- **Gate 2 — jest:** `npx jest` → **all green**, and the **test** count at or above the floor named in your task. Today's baseline is **132 tests across 17 suites** — do not confuse the two numbers. The floors are cumulative: T1 ≥136 · T2 ≥142 · T3 ≥148 · T4 ≥165 · T5 ≥171 · T6 ≥171. "All green" is the binding half; the floor only catches tests that silently vanished. jest's summary lines are ANSI-coloured, so `grep "^Tests:"` silently matches nothing — **read the tail of the output**.
- **A single-file `npx jest <file>` HANGS FOREVER** for suites that render a react-query `QueryClient` — use `--forceExit` for single files. The full suite is fine.
- **NEVER** run `npm ci` / `npm install` / `--package-lock-only` / `rm -rf node_modules`. Metro runs against this tree. **Never touch anything inside any `node_modules/`.** A previous implementer deleted a nested zod despite being told not to; it was unrecoverable.
- **Do not add dependencies.** `expo-crypto` is NOT installed and cannot be. There is no `crypto.randomUUID` (Expo 56's winter runtime does not polyfill it — verified).
- **Do not touch** `metro.config.js`, `tsconfig.json`, `jest.config.js`, `babel.config.js`, tailwind config, `app.config.js`, `eas.json`.
- **Tests live in `apps/mobile-admin/__tests__/` ONLY.** `jest.mock` factory functions must be defined INSIDE the factory.
- `packages/mobile-shared` resolves everything from the monorepo root (zod **4.4.3**). `apps/mobile-admin` resolves `expo@56.0.15` / `expo-notifications@56.0.20` from its OWN `node_modules` (root has expo 52 for a different app — do not be misled by the root copy).
- **tsc from `apps/mobile-admin` DOES type-check `packages/mobile-shared` files — but only those actually IMPORTED.** Verified during planning by breaking `schema-helpers.ts` and watching the gate catch it. Corollary, and it bit me while writing this plan: **a new file nobody imports yet is invisible to the gate.** If you write a schema module and the gate stays at 2, that is not proof it compiles — it may just be outside tsc's program. It enters the program the moment an api module imports it (Step "wire the schema into the api module" in each task). **Never take a green gate on an unimported file as evidence of anything.**
- A bare `export type {X} from "./y"` creates **NO local binding**. If a name is referenced locally in the same file, it needs `import type` + a separate `export type`.
- **Commit directly to `main`.** Single-line conventional commit messages. No signatures. No PRs.
- **Immutability:** never mutate inputs. `.sort()` mutates in place — always `[...arr].sort()`.

## File Structure

**Create:**
- `packages/mobile-shared/api/schemas/customers.ts` — customer + address schemas, inferred types
- `packages/mobile-shared/api/schemas/orders.ts` — order list schema, inferred types
- `packages/mobile-shared/api/schemas/products.ts` — variant-aware product schemas, inferred types
- `packages/mobile-shared/api/schemas/notifications.ts` — notification schema, inferred types
- `apps/mobile-admin/lib/product-display.ts` — pure variant/media-picking helpers
- `apps/mobile-admin/__tests__/demo-api-client-schema.test.tsx`
- `apps/mobile-admin/__tests__/schemas-customers.test.tsx`
- `apps/mobile-admin/__tests__/schemas-orders.test.tsx`
- `apps/mobile-admin/__tests__/schemas-products.test.tsx`
- `apps/mobile-admin/__tests__/product-display.test.tsx`
- `apps/mobile-admin/__tests__/schemas-notifications.test.tsx`

**Modify:**
- `packages/mobile-shared/api/schema-helpers.ts` — add `legacyPaged` (Task 5)
- `packages/mobile-shared/api/{customers,orders,products,notifications}.ts` — pass schemas
- `packages/mobile-shared/api/types.ts` — re-export inferred types; delete `PaginatedResponse` (Task 5)
- `apps/mobile-admin/lib/demo-api-client.ts` — Tasks 1–5
- `apps/mobile-admin/lib/hooks/use-{customers,orders,products,notifications}.ts`
- `apps/mobile-admin/app/(tabs)/customers/{index,[id]}.tsx`, `orders/index.tsx`, `products/{index,[id],new}.tsx`, `more/{index,notifications}.tsx`, `_layout.tsx`
- `apps/mobile-admin/components/{ProductRow,OrderRow}.tsx`
- `apps/mobile-admin/lib/admin-api/{customer-actions,product-crud}.ts`
- `.github/workflows/ci.yml` (Task 6)

---

### Task 1: Demo client applies the schema it is passed

**Why first:** `createDemoApiClient` casts six methods through `as ApiClient[...]`, which **silently drops the `schema` argument** — demo mode skips validation entirely. Its `page<T>()` helper fabricates `{items, total, next_cursor, has_more}`, a shape no endpoint returns. The moment any later task swaps a hook to `paginated()`, demo mode returns `{items}`, `res.data` is `undefined`, and react-query v5 throws *"Query data cannot be undefined"*. The previous session hit this bug class **twice**. This task disarms it for all four remaining domains at once, and forces every demo fixture to become wire-truthful as its domain migrates.

**Files:**
- Modify: `apps/mobile-admin/lib/demo-api-client.ts`
- Test: `apps/mobile-admin/__tests__/demo-api-client-schema.test.tsx` (create)

**Interfaces:**
- Consumes: `createApiClient` from `@repo/mobile-shared/api/client`; `ApiError` (a **value**, not just a type — import without `type`).
- Produces: `createDemoApiClient(): ApiClient` whose `get`/`getTenant`/`post`/`patch` accept and **apply** an optional `schema` third/third argument, matching the real client's signature exactly with no `as` casts.

The real client's signatures (`packages/mobile-shared/api/client.ts:186-196`) that must be matched structurally:
```ts
get: <T>(path: string, params?: Record<string, string>, schema?: z.ZodType<T>) => Promise<T>
getTenant: <T>(path: string, params?: Record<string, string>, schema?: z.ZodType<T>) => Promise<T>
post: <T>(path: string, body?: unknown, schema?: z.ZodType<T>) => Promise<T>
patch: <T>(path: string, body?: unknown, schema?: z.ZodType<T>) => Promise<T>
delete: <T>(path: string) => Promise<T>
uploadMedia: (path: string, formData: FormData) => Promise<any>
```

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/demo-api-client-schema.test.tsx`:

```tsx
import { z } from "zod";
import { createDemoApiClient } from "@/lib/demo-api-client";
import { ApiError } from "@repo/mobile-shared/api/client";

describe("createDemoApiClient — schema application", () => {
  it("applies a passed schema and returns the parsed value", async () => {
    const client = createDemoApiClient();
    const schema = z.object({ data: z.array(z.object({ id: z.string() })) });
    const res = await client.get("/stores", undefined, schema);
    expect(res.data[0]!.id).toBe("demo-store");
  });

  it("throws contract_mismatch naming the field path when the fixture does not match", async () => {
    const client = createDemoApiClient();
    // /stores fixture has no `nope` key — the schema must reject it.
    const schema = z.object({ nope: z.string() });
    await expect(client.get("/stores", undefined, schema)).rejects.toMatchObject({
      name: "ApiError",
      status: 500,
      code: "contract_mismatch",
    });
    await expect(client.get("/stores", undefined, schema)).rejects.toThrow(/nope/);
  });

  it("returns raw data unchanged when no schema is passed", async () => {
    const client = createDemoApiClient();
    const res = await client.get<{ data: unknown[] }>("/stores");
    expect(Array.isArray(res.data)).toBe(true);
  });

  it("exposes ApiError as a real error instance", async () => {
    const client = createDemoApiClient();
    const schema = z.object({ nope: z.string() });
    await expect(client.get("/stores", undefined, schema)).rejects.toBeInstanceOf(ApiError);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/demo-api-client-schema.test.tsx --forceExit`

Expected: FAIL. The first test fails because `get` ignores the third argument and returns the raw fixture untyped; the mismatch tests fail because nothing throws.

- [ ] **Step 3: Write the implementation**

In `apps/mobile-admin/lib/demo-api-client.ts`, change the imports at the top of the file from:

```ts
import type { createApiClient } from "@repo/mobile-shared/api/client";
```

to:

```ts
import type { z } from "zod";
import { ApiError, type createApiClient } from "@repo/mobile-shared/api/client";
```

Then replace the whole `createDemoApiClient` function (currently the last block of the file) with:

```ts
/**
 * Applies a schema exactly the way the real client does (client.ts:169-182),
 * including the console.error, so a demo-mode contract break is debugged the
 * same way as a real one.
 *
 * This exists because the previous version cast every method through
 * `as ApiClient[...]`, which silently DROPPED the schema argument — demo mode
 * skipped validation entirely while looking like it did not. That let the
 * fixtures drift into shapes no endpoint returns.
 */
function parseOrThrow<T>(path: string, data: unknown, schema?: z.ZodType<T>): T {
  if (!schema) return data as T;
  const parsed = schema.safeParse(data);
  if (!parsed.success) {
    const issue = parsed.error.issues[0]!;
    const fieldPath = issue.path.join(".") || "(root)";
    const detail = `${fieldPath}: ${issue.message}`;
    console.error(`[demo-api] contract mismatch on ${path}: ${detail}`);
    throw new ApiError(500, "contract_mismatch", detail);
  }
  return parsed.data;
}

export function createDemoApiClient(): ApiClient {
  return {
    get: <T>(path: string, _params?: Record<string, string>, schema?: z.ZodType<T>) =>
      Promise.resolve(parseOrThrow(path, resolve(path), schema)),
    getTenant: <T>(path: string, _params?: Record<string, string>, schema?: z.ZodType<T>) =>
      Promise.resolve(parseOrThrow(path, resolve(path), schema)),
    // Mutations succeed no-op: echo the body so optimistic UI has something.
    // A schema, if passed, is still applied — a demo mutation whose echo does
    // not match its response schema SHOULD fail loudly rather than lie.
    post: <T>(path: string, body?: unknown, schema?: z.ZodType<T>) =>
      Promise.resolve(parseOrThrow(path, body ?? { success: true }, schema)),
    patch: <T>(path: string, body?: unknown, schema?: z.ZodType<T>) =>
      Promise.resolve(parseOrThrow(path, body ?? { success: true }, schema)),
    delete: <T>() => Promise.resolve({ success: true } as T),
    uploadMedia: async () => ({ id: "demo-media", url: "" }),
  };
}
```

Note the `as ApiClient[...]` casts are all gone — the generics now match structurally. The `ApiClient` type alias at the top of the file (`type ApiClient = ReturnType<typeof createApiClient>;`) stays as-is.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/mobile-admin && npx jest __tests__/demo-api-client-schema.test.tsx --forceExit`
Expected: PASS, 4 tests.

- [ ] **Step 5: Run both gates**

Run: `cd apps/mobile-admin && npx jest 2>&1 | tail -8`
Expected: all green, **≥ 136 tests** (132 baseline + 4 new).

Run: `cd apps/mobile-admin && npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"`
Expected: **2** (the pre-existing `app/(tabs)/_layout.tsx` errors, fixed in Task 6). Any other number: read the actual errors with `npx tsc --noEmit --pretty false 2>&1 | grep "error TS"` and fix your regression.

- [ ] **Step 6: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add apps/mobile-admin/lib/demo-api-client.ts apps/mobile-admin/__tests__/demo-api-client-schema.test.tsx
git commit -m "fix(mobile-admin): demo client applies the schema instead of dropping it"
```

---

### Task 2: B — customers on the real envelope

**Files:**
- Create: `packages/mobile-shared/api/schemas/customers.ts`
- Create: `apps/mobile-admin/__tests__/schemas-customers.test.tsx`
- Modify: `packages/mobile-shared/api/customers.ts`
- Modify: `packages/mobile-shared/api/types.ts`
- Modify: `apps/mobile-admin/lib/hooks/use-customers.ts`
- Modify: `apps/mobile-admin/lib/admin-api/customer-actions.ts`
- Modify: `apps/mobile-admin/app/(tabs)/customers/index.tsx:69`
- Modify: `apps/mobile-admin/app/(tabs)/customers/[id].tsx`
- Modify: `apps/mobile-admin/lib/demo-api-client.ts` (DEMO_CUSTOMERS + customerDetail)

**Interfaces:**
- Consumes: `money`, `paginated` from `@repo/mobile-shared/api/schema-helpers` (both already exist).
- Produces:
  - `customerSchema`, `customerDetailSchema`, `customerAddressSchema`, `customerListSchema`
  - types `Customer`, `CustomerDetail`, `CustomerAddress`
  - `createCustomersApi(client).list()` → `Promise<{data: Customer[], meta: PageMeta}>`
  - `createCustomersApi(client).get(id)` → `Promise<CustomerDetail>`
  - `createCustomersApi(client).block(id, reason)` — **reason is now REQUIRED**

**Live wire truth** (`GET /customers`, verified 2026-07-16 — this is the ONLY real customer):
```json
{"data":[{"id":"8b52db9e-5dcf-40e0-b81d-ea5d7bcc3152","email":"mahesh.sangawar@gmail.com",
"tags":[],"status":"active","marketing_opt_in":false,"order_count":0,"total_spent":0,
"created_at":"2026-05-06T02:27:38Z","updated_at":"2026-05-18T00:24:50Z"}],
"meta":{"page":1,"page_size":50,"total":1,"total_pages":1}}
```
`GET /customers/{id}` returns that same object **flat** (no `{data}` wrapper) plus `"addresses": []`.

**Critical:** `first_name`/`last_name`/`phone`/`avatar_url`/`block_reason`/`notes`/`last_order_at` are Go `omitempty` strings (`customers_dto.go:24-36`) — they are **ABSENT, not null**. They must be `.optional()`, **never** `.nullable()`. The current `Customer` type requires `first_name: string`, which would fail validation on the only real customer that exists.

There is **no** `recent_orders`, `average_order_value` or `review_count` anywhere in the backend. `average_order_value` is derived client-side; the Recent Orders card is removed (spec decision).

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/schemas-customers.test.tsx`:

```tsx
import {
  customerSchema,
  customerListSchema,
  customerDetailSchema,
} from "@repo/mobile-shared/api/schemas/customers";

// The exact payload GET /customers returned from prod on 2026-07-16.
const REAL_LIST = {
  data: [
    {
      id: "8b52db9e-5dcf-40e0-b81d-ea5d7bcc3152",
      email: "mahesh.sangawar@gmail.com",
      tags: [],
      status: "active",
      marketing_opt_in: false,
      order_count: 0,
      total_spent: 0,
      created_at: "2026-05-06T02:27:38Z",
      updated_at: "2026-05-18T00:24:50Z",
    },
  ],
  meta: { page: 1, page_size: 50, total: 1, total_pages: 1 },
};

describe("customerSchema", () => {
  it("accepts the real customer, which has NO first_name/last_name/phone", () => {
    const parsed = customerListSchema.parse(REAL_LIST);
    expect(parsed.data[0]!.email).toBe("mahesh.sangawar@gmail.com");
    expect(parsed.data[0]!.first_name).toBeUndefined();
    expect(parsed.meta.total).toBe(1);
  });

  it("accepts names when present", () => {
    const parsed = customerSchema.parse({
      ...REAL_LIST.data[0],
      first_name: "Maya",
      last_name: "Chen",
      phone: "+61 400 111 222",
    });
    expect(parsed.first_name).toBe("Maya");
  });

  it("coerces total_spent from a quoted decimal string", () => {
    const parsed = customerSchema.parse({ ...REAL_LIST.data[0], total_spent: "48.20" });
    expect(parsed.total_spent).toBe(48.2);
  });

  it("rejects a payload missing a required field, naming the path", () => {
    const bad = { ...REAL_LIST.data[0] } as Record<string, unknown>;
    delete bad.email;
    const res = customerSchema.safeParse(bad);
    expect(res.success).toBe(false);
    if (!res.success) expect(res.error.issues[0]!.path).toContain("email");
  });

  it("detail is flat with addresses, NOT wrapped in {data}", () => {
    const parsed = customerDetailSchema.parse({ ...REAL_LIST.data[0], addresses: [] });
    expect(parsed.addresses).toEqual([]);
    expect(parsed.id).toBe("8b52db9e-5dcf-40e0-b81d-ea5d7bcc3152");
  });

  it("detail accepts a populated address", () => {
    const parsed = customerDetailSchema.parse({
      ...REAL_LIST.data[0],
      addresses: [
        {
          id: "a-1",
          is_default: true,
          name: "Maya Chen",
          line1: "12 Campbell Pde",
          city: "Bondi Beach",
          country_code: "AU",
        },
      ],
    });
    expect(parsed.addresses[0]!.line1).toBe("12 Campbell Pde");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-customers.test.tsx --forceExit`
Expected: FAIL — "Cannot find module '@repo/mobile-shared/api/schemas/customers'".

- [ ] **Step 3: Write the schema module**

Create `packages/mobile-shared/api/schemas/customers.ts`:

```ts
import { z } from "zod";
import { money, paginated } from "../schema-helpers";

/**
 * Wire truth for the admin customer endpoints, verified against prod
 * 2026-07-16 (`GET /api/v1/mobile/admin/stores/{id}/customers`).
 *
 * Every optional field below is a Go `omitempty` string
 * (customers_dto.go:24-36) — it is ABSENT from the JSON, not null. Using
 * .nullable() here would reject the only real customer that exists.
 *
 * There is deliberately no `recent_orders`, `average_order_value` or
 * `review_count`: the backend has never had them. The app derives the
 * average from order_count/total_spent and does not show recent orders.
 */
export const customerSchema = z.object({
  id: z.string(),
  email: z.string(),
  first_name: z.string().optional(),
  last_name: z.string().optional(),
  phone: z.string().optional(),
  avatar_url: z.string().optional(),
  tags: z.array(z.string()),
  status: z.string(),
  block_reason: z.string().optional(),
  notes: z.string().optional(),
  marketing_opt_in: z.boolean(),
  order_count: z.number(),
  // float64 on the wire today, but money tolerates the quoted-decimal form
  // the rest of the API uses, so this survives a backend switch to decimal.
  total_spent: money,
  last_order_at: z.string().optional(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Customer = z.infer<typeof customerSchema>;

/** customers_dto.go:47-59. Only line1/city/country_code/name are guaranteed. */
export const customerAddressSchema = z.object({
  id: z.string(),
  label: z.string().optional(),
  is_default: z.boolean(),
  name: z.string(),
  line1: z.string(),
  line2: z.string().optional(),
  city: z.string(),
  region: z.string().optional(),
  postal_code: z.string().optional(),
  country_code: z.string(),
  phone: z.string().optional(),
});
export type CustomerAddress = z.infer<typeof customerAddressSchema>;

/**
 * The detail endpoint returns the customer FLAT plus `addresses` — it does
 * NOT use the `{data}` envelope (customers.go:108 `c.JSON(200, resp)`).
 */
export const customerDetailSchema = customerSchema.extend({
  addresses: z.array(customerAddressSchema),
});
export type CustomerDetail = z.infer<typeof customerDetailSchema>;

export const customerListSchema = paginated(customerSchema);
export type CustomerListResponse = z.infer<typeof customerListSchema>;
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-customers.test.tsx --forceExit`
Expected: PASS, 6 tests.

- [ ] **Step 5: Wire the schema into the api module**

Replace the whole of `packages/mobile-shared/api/customers.ts` with:

```ts
import type { createApiClient } from "./client";
import {
  customerDetailSchema,
  customerListSchema,
  type Customer,
  type CustomerDetail,
  type CustomerListResponse,
} from "./schemas/customers";

export interface ListCustomersParams {
  search?: string;
  status?: string;
  page?: string;
  page_size?: string;
}

export function createCustomersApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListCustomersParams) =>
      client.get<CustomerListResponse>(
        "/customers",
        params as Record<string, string>,
        customerListSchema,
      ),
    get: (id: string) =>
      client.get<CustomerDetail>(`/customers/${id}`, undefined, customerDetailSchema),
    /**
     * `reason` is REQUIRED by the backend (BlockCustomerRequest,
     * customers_dto.go:72-74 — `binding:"required"`). Omitting it, as this
     * client used to, is an unconditional 400.
     */
    block: (id: string, reason: string) => client.post(`/customers/${id}/block`, { reason }),
    unblock: (id: string) => client.post(`/customers/${id}/unblock`),
  };
}

export type { Customer, CustomerDetail };
```

Note `cursor`/`limit` are gone from `ListCustomersParams` — the API is page-based (`page`/`page_size`), never cursor-based. That was part of the same fiction.

- [ ] **Step 6: Re-export the inferred types**

In `packages/mobile-shared/api/types.ts`, delete the hand-written `Customer` interface (lines ~114-124) and the `CustomerDetail` interface (lines ~126-131) entirely, and add near the other schema re-exports:

```ts
export type {
  Customer,
  CustomerDetail,
  CustomerAddress,
} from "./schemas/customers";
```

`RecentOrder` is still imported at the top of `types.ts` for the dashboard — leave that import alone. `CustomerDetail` no longer references it, but `DashboardResponse` does.

- [ ] **Step 7: Update the hook**

Replace the whole of `apps/mobile-admin/lib/hooks/use-customers.ts` with:

```ts
import { useQuery } from "@tanstack/react-query";
import { createCustomersApi } from "@repo/mobile-shared/api/customers";
import type { CustomerDetail } from "@repo/mobile-shared/api/types";
import type { CustomerListResponse } from "@repo/mobile-shared/api/schemas/customers";
import { useApiClient } from "@/lib/api-client";

export function useCustomers(search?: string) {
  const client = useApiClient();
  const customersApi = createCustomersApi(client);

  return useQuery<CustomerListResponse>({
    queryKey: ["customers", search],
    queryFn: () =>
      customersApi.list({
        ...(search ? { search } : {}),
      }),
    refetchOnWindowFocus: true,
  });
}

export function useCustomer(id: string) {
  const client = useApiClient();
  const customersApi = createCustomersApi(client);

  return useQuery<CustomerDetail>({
    queryKey: ["customer", id],
    queryFn: () => customersApi.get(id),
    enabled: !!id,
  });
}
```

- [ ] **Step 8: Update the block action to pass a reason**

In `apps/mobile-admin/lib/admin-api/customer-actions.ts`, change `useBlockCustomer`'s mutation to take a reason:

```ts
export function useBlockCustomer() {
  const client = useApiClient();
  const customersApi = createCustomersApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      customersApi.block(id, reason),
    onSuccess: (_data, { id }) => {
      queryClient.invalidateQueries({ queryKey: ["customers"] });
      queryClient.invalidateQueries({ queryKey: ["customer", id] });
    },
  });
}
```

Leave `useUnblockCustomer` exactly as it is.

- [ ] **Step 9: Fix the list screen**

In `apps/mobile-admin/app/(tabs)/customers/index.tsx`, change line 69 from `data={data?.items ?? []}` to:

```tsx
          data={data?.data ?? []}
```

Then check the rest of that file for `first_name`/`last_name` usage. Because those are now `string | undefined`, any expression that passed them into a function taking `string` will be a tsc error. Where the file builds a display name, use the same helpers the detail screen uses (Step 10) — if the list defines its own local `getDisplayName(first, last, email)`, widen its parameters to `first?: string, last?: string`. The existing bodies already handle falsy values correctly (`firstName || lastName || email`), so only the types change.

- [ ] **Step 10: Fix the detail screen**

In `apps/mobile-admin/app/(tabs)/customers/[id].tsx`:

1. Widen the two helpers to accept optional names (lines 43-51):

```tsx
function getInitial(firstName: string | undefined, lastName: string | undefined, email: string): string {
  const name = firstName || lastName || email;
  return name.charAt(0).toUpperCase();
}

function getDisplayName(firstName: string | undefined, lastName: string | undefined, email: string): string {
  if (firstName || lastName) return [firstName, lastName].filter(Boolean).join(" ");
  return email;
}
```

2. Remove the now-unused `RecentOrder` import (line 24), the `RecentOrderRow` component (lines 73-105), the `ORDER_STATUS_TONE` map (lines 53-58), the `handleOrderPress` callback (lines 136-139) and the `useRouter`/`router` usage **only if nothing else in the file uses `router`** — check before deleting; if `router` is otherwise unused, drop the `useRouter` import too. Also drop `StatusTone` from the `@/components/ui` import if `ORDER_STATUS_TONE` was its only consumer, and `Hairline`/`Card` if the Recent Orders card was their only consumer. **Let tsc/eslint tell you** — do not guess.

3. Replace the stats row (currently lines ~183-187) so the average is derived, and delete the entire Recent Orders block (the `<Eyebrow label="Recent Orders" />` and the `<Card>` that follows it):

```tsx
        <View style={styles.statsRow}>
          <StatTile label="Orders" value={String(customer.order_count)} />
          <StatTile label="Spent" value={formatCurrency(customer.total_spent)} />
          <StatTile label="Avg" value={formatCurrency(averageOrderValue)} />
        </View>
```

with, just above the `return` (after the `displayName`/`initial` consts):

```tsx
  // The backend has no average_order_value — deriving it here is exactly what
  // a server-side field would compute, without a deploy. Guard the divide:
  // the only real customer has order_count 0.
  const averageOrderValue =
    customer.order_count > 0 ? customer.total_spent / customer.order_count : 0;
```

4. `formatCurrency` in this file hardcodes `currency: "USD"` while the store is AUD. **Leave it as-is** and add no new USD usages. This is deliberate, not an oversight: unlike products (where every variant carries `currency_code`), the customer wire shape has **no currency field at all** — `AdminCustomerResponse` (`customers_dto.go:21-38`) has none. The only source is the active store's `currency_code`, and threading store context into this screen is real scope with no test to justify it here. Recorded as a follow-up.

5. Update the block call site (lines ~130-132) — `blockMutation.mutate` now needs an object. The simplest honest change that keeps the confirm dialog is to send a fixed reason, because the UI has no free-text input and adding one is out of scope:

```tsx
        onPress: () =>
          isBlocked
            ? unblockMutation.mutate(customer.id)
            : blockMutation.mutate({ id: customer.id, reason: "Blocked from mobile admin" }),
```

- [ ] **Step 11: Make the demo fixtures wire-truthful**

In `apps/mobile-admin/lib/demo-api-client.ts`:

1. Replace `DEMO_CUSTOMERS` (lines 52-56) with the real shape:

```ts
const DEMO_CUSTOMERS: Customer[] = [
  { id: "c-1", email: "maya@example.com", first_name: "Maya", last_name: "Chen", phone: "+61 400 111 222", tags: [], status: "active", marketing_opt_in: true, order_count: 6, total_spent: 48200, created_at: iso(120), updated_at: iso(2) },
  { id: "c-2", email: "leo@example.com", first_name: "Leo", last_name: "Park", tags: [], status: "active", marketing_opt_in: false, order_count: 2, total_spent: 15800, created_at: iso(40), updated_at: iso(5) },
  // No names at all — mirrors the ONLY real customer in prod, and is the case
  // that would have caught this whole class two months ago.
  { id: "c-3", email: "ida@example.com", tags: [], status: "active", marketing_opt_in: false, order_count: 0, total_spent: 0, created_at: iso(12), updated_at: iso(12) },
];
```

2. Replace `customerDetail` (lines 140-149) with:

```ts
function customerDetail(id: string): CustomerDetail {
  const base = DEMO_CUSTOMERS.find((c) => c.id === id) ?? DEMO_CUSTOMERS[0]!;
  return { ...base, addresses: [] };
}
```

3. Replace the `/customers` list branch (line 171) so it uses the real envelope. Add this helper next to `page()`:

```ts
/** The real `{data, meta}` list envelope — see schema-helpers.paginated. */
function paged<T>(items: T[]): { data: T[]; meta: { page: number; page_size: number; total: number; total_pages: number } } {
  return { data: items, meta: { page: 1, page_size: 50, total: items.length, total_pages: 1 } };
}
```

and change `if (clean === "/customers") return page(DEMO_CUSTOMERS);` to:

```ts
  if (clean === "/customers") return paged(DEMO_CUSTOMERS);
```

Leave `page()` in place for now — `/orders`, `/products` and `/notifications` still use it. Tasks 3–5 remove their callers; Task 5 deletes `page()` itself.

- [ ] **Step 12: Run both gates**

Run: `cd apps/mobile-admin && npx jest 2>&1 | tail -8`
Expected: all green, **≥ 142 tests** (136 + 6 new).

Run: `cd apps/mobile-admin && npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"`
Expected: **2**. If higher, run `npx tsc --noEmit --pretty false 2>&1 | grep "error TS"` — the errors name every site still reading the old shape. Fix them; that is the leverage working as designed.

- [ ] **Step 13: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add packages/mobile-shared/api/schemas/customers.ts packages/mobile-shared/api/customers.ts packages/mobile-shared/api/types.ts apps/mobile-admin/lib/hooks/use-customers.ts apps/mobile-admin/lib/admin-api/customer-actions.ts "apps/mobile-admin/app/(tabs)/customers" apps/mobile-admin/lib/demo-api-client.ts apps/mobile-admin/__tests__/schemas-customers.test.tsx
git commit -m "fix(mobile-admin): customers read the real {data,meta} envelope and wire fields"
```

---

### Task 3: C — orders LIST on the real envelope

**Scope note (read this first):** This task migrates the **list only**. `useOrder`, `OrderDetail` and `app/(tabs)/orders/[id].tsx` are **explicitly out of scope** and must be left alone — see the spec's "C: orders" section. 6 of the 12 fields that screen reads do not exist on the wire (`line_items`, `shipping_address`, `timeline`, `tracking_number`, `payment_method`, `payment_transaction_id`); rewriting it against a store with zero orders is its own sub-project. Do **not** attach a schema to `get(id)` — doing so would make the detail screen throw `contract_mismatch` instead of failing the way it already does.

**Files:**
- Create: `packages/mobile-shared/api/schemas/orders.ts`
- Create: `apps/mobile-admin/__tests__/schemas-orders.test.tsx`
- Modify: `packages/mobile-shared/api/orders.ts`
- Modify: `packages/mobile-shared/api/types.ts`
- Modify: `apps/mobile-admin/lib/hooks/use-orders.ts`
- Modify: `apps/mobile-admin/app/(tabs)/orders/index.tsx:87` and the FILTERS array (lines 25-30)
- Modify: `apps/mobile-admin/components/OrderRow.tsx`
- Modify: `apps/mobile-admin/lib/demo-api-client.ts` (DEMO_ORDERS + the `/orders` branch)

**Interfaces:**
- Consumes: `money`, `paginated` from `../schema-helpers`.
- Produces: `orderSchema`, `orderListSchema`; types `Order`, `OrderListResponse`. `createOrdersApi(client).list()` → `Promise<{data: Order[], meta: PageMeta}>`.

**Wire truth** (`AdminOrderResponse`, `orders_dto.go:152-188`; list verified live returning `{"data":[],"meta":{...}}`). `OrderRow.tsx` reads exactly six fields and **all six exist**:
- `id`, `order_number`, `status`, `customer_email` — plain strings
- `customer_name` — `*string` + `omitempty` → **`.optional()`**
- `grand_total` — `decimal.Decimal` → **a quoted string** → **`money`**
- `created_at` — `time.Time` → RFC3339 string

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/schemas-orders.test.tsx`:

```tsx
import { orderSchema, orderListSchema } from "@repo/mobile-shared/api/schemas/orders";

// Shaped from AdminOrderResponse (orders_dto.go:152-188). The live Bondi
// store has zero orders, so this is built from the Go DTO — the only truth
// available. Money fields are decimal.Decimal, which marshals QUOTED.
const REAL_ORDER = {
  id: "11111111-1111-1111-1111-111111111111",
  tenant_id: "8c302556-b647-4824-8ce4-73f547ca456e",
  store_id: "8b69eea9-2537-4d36-9d99-bafcbad02dbc",
  order_number: "1001",
  idempotency_key: "idem-1",
  customer_email: "maya@example.com",
  status: "pending",
  payment_status: "paid",
  fulfillment_status: "unfulfilled",
  subtotal: "84.00",
  shipping_total: "0",
  tax_total: "0",
  discount_total: "0",
  grand_total: "84.00",
  refunded_amount: "0",
  currency_code: "AUD",
  items: [],
  addresses: [],
  placed_at: "2026-07-14T09:00:00Z",
  created_at: "2026-07-14T09:00:00Z",
  updated_at: "2026-07-14T09:00:00Z",
};

describe("orderSchema", () => {
  it("parses grand_total from the quoted decimal string the wire actually sends", () => {
    const parsed = orderSchema.parse(REAL_ORDER);
    expect(parsed.grand_total).toBe(84);
    expect(typeof parsed.grand_total).toBe("number");
  });

  it("accepts an order with no customer_name (omitempty)", () => {
    const parsed = orderSchema.parse(REAL_ORDER);
    expect(parsed.customer_name).toBeUndefined();
  });

  it("accepts customer_name when present", () => {
    const parsed = orderSchema.parse({ ...REAL_ORDER, customer_name: "Maya Chen" });
    expect(parsed.customer_name).toBe("Maya Chen");
  });

  it("parses the real empty list envelope from prod", () => {
    const parsed = orderListSchema.parse({
      data: [],
      meta: { page: 1, page_size: 50, total: 0, total_pages: 0 },
    });
    expect(parsed.data).toEqual([]);
    expect(parsed.meta.total).toBe(0);
  });

  it("rejects an {items} envelope — the fiction this whole change removes", () => {
    const res = orderListSchema.safeParse({ items: [], total: 0, next_cursor: null, has_more: false });
    expect(res.success).toBe(false);
  });

  it("names the field path when money is unparseable", () => {
    const res = orderSchema.safeParse({ ...REAL_ORDER, grand_total: "" });
    expect(res.success).toBe(false);
    if (!res.success) expect(res.error.issues[0]!.path).toContain("grand_total");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-orders.test.tsx --forceExit`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the schema module**

Create `packages/mobile-shared/api/schemas/orders.ts`:

```ts
import { z } from "zod";
import { money, paginated } from "../schema-helpers";

/**
 * Wire truth for the admin order LIST endpoint — AdminOrderResponse
 * (orders_dto.go:152-188). The Bondi store has zero orders, so unlike
 * products/customers this could not be verified against a live payload;
 * it is derived from the Go DTO, which is the only truth available.
 *
 * Every money field is a shopspring/decimal.Decimal, which marshals QUOTED
 * ("84.00") unless MarshalJSONWithoutQuotes is set — a repo-wide grep
 * confirms it never is. Hence `money`, not z.number().
 *
 * Deliberately NOT modelled here: the order DETAIL shape. The detail screen
 * still uses the hand-written OrderDetail type and passes no schema — see
 * the spec (2026-07-16-mobile-admin-lists-bcd-design.md, "C: orders").
 * Six of the twelve fields it reads do not exist on the wire; fixing that is
 * its own sub-project against a seeded order.
 */
export const orderSchema = z.object({
  id: z.string(),
  tenant_id: z.string(),
  store_id: z.string(),
  order_number: z.string(),
  idempotency_key: z.string(),
  customer_email: z.string(),
  // *string + omitempty -> ABSENT, not null.
  customer_name: z.string().optional(),
  status: z.string(),
  payment_status: z.string(),
  fulfillment_status: z.string(),
  subtotal: money,
  shipping_total: money,
  tax_total: money,
  discount_total: money,
  grand_total: money,
  refunded_amount: money,
  currency_code: z.string(),
  shipping_service: z.string().optional(),
  shipping_carrier: z.string().optional(),
  // Present on the wire; the list UI does not read them yet. Kept loose on
  // purpose: modelling them properly belongs with the detail sub-project.
  items: z.array(z.unknown()),
  addresses: z.array(z.unknown()),
  placed_at: z.string(),
  cancelled_at: z.string().optional(),
  fulfilled_at: z.string().optional(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Order = z.infer<typeof orderSchema>;

export const orderListSchema = paginated(orderSchema);
export type OrderListResponse = z.infer<typeof orderListSchema>;
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-orders.test.tsx --forceExit`
Expected: PASS, 6 tests.

- [ ] **Step 5: Wire the schema into the api module**

In `packages/mobile-shared/api/orders.ts`, change the imports and the `list` method. The file becomes:

```ts
import type { createApiClient } from "./client";
import type { OrderDetail } from "./types";
import { orderListSchema, type Order, type OrderListResponse } from "./schemas/orders";

export interface ListOrdersParams {
  status?: string;
  payment_status?: string;
  search?: string;
  page?: string;
  page_size?: string;
}

export function createOrdersApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListOrdersParams) =>
      client.get<OrderListResponse>("/orders", params as Record<string, string>, orderListSchema),
    /**
     * NO schema, deliberately. OrderDetail is still the hand-written
     * (and largely fictional) type: `line_items`, `shipping_address`,
     * `timeline`, `tracking_number`, `payment_method` and
     * `payment_transaction_id` do not exist on the wire. Attaching a schema
     * here would turn a broken screen into a thrown contract_mismatch
     * without fixing anything. The detail rewrite is its own sub-project —
     * see docs/superpowers/specs/2026-07-16-mobile-admin-lists-bcd-design.md.
     */
    get: (id: string) => client.get<OrderDetail>(`/orders/${id}`),
    confirm: (id: string) => client.post(`/orders/${id}/confirm`),
    fulfill: (id: string, trackingNumber: string) =>
      client.post(`/orders/${id}/fulfill`, { tracking_number: trackingNumber }),
    cancel: (id: string, reason?: string) => client.post(`/orders/${id}/cancel`, { reason }),
    refund: (id: string, amount: number) => client.post(`/orders/${id}/refund`, { amount }),
  };
}

export type { Order };
```

Leave `confirm`/`fulfill`/`cancel`/`refund` exactly as they are. They are broken (`fulfill`'s body is discarded by the handler, `cancel` needs a required `reason`, `refund` needs a required `refund_request_id`), but they belong to the deferred detail sub-project and touching them here would be scope creep with no way to verify.

- [ ] **Step 6: Re-export the inferred type**

In `packages/mobile-shared/api/types.ts`, delete the hand-written `Order` interface (lines ~31-41) and add to the re-export block:

```ts
export type { Order } from "./schemas/orders";
```

**Leave `OrderDetail`, `LineItem`, `Address` and `TimelineEvent` in place** — the deferred detail screen still uses them. `OrderDetail extends Order`, so it now extends the inferred type; that is fine and intentional (it keeps compiling while staying honest that the extra fields are fiction). Add this comment above `OrderDetail`:

```ts
/**
 * ⚠️ LARGELY FICTIONAL — deliberately left un-migrated. Verified against
 * AdminOrderResponse (orders_dto.go:152-188) on 2026-07-16: `line_items`
 * (the wire sends `items`), `shipping_address` (the wire sends `addresses[]`),
 * `timeline`, `tracking_number`, `payment_method` and
 * `payment_transaction_id` DO NOT EXIST on the wire. The detail screen has
 * always been broken and is unreachable in practice (the store has 0 orders).
 * Rewriting it is its own sub-project — see
 * docs/superpowers/specs/2026-07-16-mobile-admin-lists-bcd-design.md.
 * Nothing passes a schema for this type, so it fails the same way it always has.
 */
```

- [ ] **Step 7: Update the hook**

In `apps/mobile-admin/lib/hooks/use-orders.ts`, change the import line and `useOrders`'s generic:

```ts
import { useQuery } from "@tanstack/react-query";
import { createOrdersApi } from "@repo/mobile-shared/api/orders";
import type { OrderDetail } from "@repo/mobile-shared/api/types";
import type { OrderListResponse } from "@repo/mobile-shared/api/schemas/orders";
import { useApiClient } from "@/lib/api-client";

export function useOrders(status?: string, search?: string) {
  const client = useApiClient();
  const ordersApi = createOrdersApi(client);

  return useQuery<OrderListResponse>({
    queryKey: ["orders", status, search],
    queryFn: () =>
      ordersApi.list({
        ...(status ? { status } : {}),
        ...(search ? { search } : {}),
      }),
    refetchOnWindowFocus: true,
  });
}
```

Leave `useOrder(id)` exactly as it is.

- [ ] **Step 8: Fix the list screen and the broken Active filter**

In `apps/mobile-admin/app/(tabs)/orders/index.tsx`:

1. Change line 87 from `data={data?.items ?? []}` to:

```tsx
          data={data?.data ?? []}
```

2. Replace the FILTERS array (lines 25-30). The old "Active" tab sent `status: "pending,confirmed"`, but the handler does `tx.Where("status = ?", q.Status)` (`orders.go:174`) — an exact match that can never hit a comma-joined string, so that tab silently showed nothing forever. One real status per tab:

```tsx
const FILTERS: { key: FilterKey; label: string; status?: string }[] = [
  { key: "all", label: "All" },
  // One real status per tab. The backend matches status exactly
  // (orders.go:174 `status = ?`), so a comma-joined "pending,confirmed"
  // silently matches nothing — which is what this tab used to do.
  { key: "pending", label: "Pending", status: "pending" },
  { key: "confirmed", label: "Confirmed", status: "confirmed" },
  { key: "completed", label: "Completed", status: "fulfilled" },
  { key: "cancelled", label: "Cancelled", status: "cancelled" },
];
```

3. Update the `FilterKey` type (line ~23) to match:

```tsx
type FilterKey = "all" | "pending" | "confirmed" | "completed" | "cancelled";
```

- [ ] **Step 9: Fix OrderRow**

`OrderRow.tsx` reads `customer_name`, which is now `string | undefined`, and `grand_total`, which is now a real `number` (it was typed `number` before but arrived as a string — the display was wrong at runtime, not just in types). Wherever the file uses `order.customer_name` as a `string`, fall back to the email:

```tsx
  const displayName = order.customer_name || order.customer_email;
```

and use `displayName` in its place. Let tsc name the exact sites — run the gate and fix what it reports.

- [ ] **Step 10: Make the demo fixtures wire-truthful**

In `apps/mobile-admin/lib/demo-api-client.ts`:

1. Replace `DEMO_ORDERS` (lines 40-44). `Order` is now the inferred type, so every required field must be present, and money must be a quoted string to mirror the wire:

```ts
const DEMO_ORDERS: Order[] = [
  { id: "o-1001", tenant_id: "demo-tenant", store_id: "demo-store", order_number: "1001", idempotency_key: "idem-1001", status: "pending", payment_status: "paid", fulfillment_status: "unfulfilled", customer_email: "maya@example.com", customer_name: "Maya Chen", subtotal: 8400, shipping_total: 0, tax_total: 0, discount_total: 0, grand_total: 8400, refunded_amount: 0, currency_code: "AUD", items: [], addresses: [], placed_at: iso(0), created_at: iso(0), updated_at: iso(0) },
  { id: "o-1000", tenant_id: "demo-tenant", store_id: "demo-store", order_number: "1000", idempotency_key: "idem-1000", status: "fulfilled", payment_status: "paid", fulfillment_status: "fulfilled", customer_email: "leo@example.com", customer_name: "Leo Park", subtotal: 12900, shipping_total: 0, tax_total: 0, discount_total: 0, grand_total: 12900, refunded_amount: 0, currency_code: "AUD", items: [], addresses: [], placed_at: iso(1), created_at: iso(1), updated_at: iso(1) },
  // No customer_name — mirrors the omitempty case.
  { id: "o-0999", tenant_id: "demo-tenant", store_id: "demo-store", order_number: "0999", idempotency_key: "idem-0999", status: "cancelled", payment_status: "refunded", fulfillment_status: "unfulfilled", customer_email: "ida@example.com", subtotal: 5200, shipping_total: 0, tax_total: 0, discount_total: 0, grand_total: 5200, refunded_amount: 5200, currency_code: "AUD", items: [], addresses: [], placed_at: iso(3), created_at: iso(3), updated_at: iso(2) },
];
```

Note the money values here are plain numbers: `Order` is the **parsed** (post-`money`) type, so its money fields are `number`. The demo client returns fixtures that are already parsed — `money` accepts a number too, so this round-trips cleanly through `parseOrThrow`.

2. Change the `/orders` list branch (line 163) from `page(DEMO_ORDERS)` to:

```ts
  if (clean === "/orders") return paged(DEMO_ORDERS);
```

3. `orderDetail()` (lines 110-124) spreads `...base` and adds the fictional fields. Because `OrderDetail extends Order`, and `Order` now has more required fields, this still compiles. Leave `orderDetail` alone — it serves the deferred detail screen.

4. `DEMO_DASHBOARD.recent_orders` (lines 73-80) maps over `DEMO_ORDERS` picking `id`/`order_number`/`customer_email`/`grand_total`/`status`/`created_at`. All still exist. Leave it.

- [ ] **Step 11: Run both gates**

Run: `cd apps/mobile-admin && npx jest 2>&1 | tail -8`
Expected: all green, **≥ 148 tests** (142 + 6 new).

Run: `cd apps/mobile-admin && npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"`
Expected: **2**.

- [ ] **Step 12: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add packages/mobile-shared/api/schemas/orders.ts packages/mobile-shared/api/orders.ts packages/mobile-shared/api/types.ts apps/mobile-admin/lib/hooks/use-orders.ts "apps/mobile-admin/app/(tabs)/orders/index.tsx" apps/mobile-admin/components/OrderRow.tsx apps/mobile-admin/lib/demo-api-client.ts apps/mobile-admin/__tests__/schemas-orders.test.tsx
git commit -m "fix(mobile-admin): orders list reads {data,meta}; one real status per filter tab"
```

---

### Task 4: D — products, variant-aware

**This is the task that makes the 161 products appear.** It is the largest.

**Files:**
- Create: `packages/mobile-shared/api/schemas/products.ts`
- Create: `apps/mobile-admin/lib/product-display.ts`
- Create: `apps/mobile-admin/__tests__/schemas-products.test.tsx`
- Create: `apps/mobile-admin/__tests__/product-display.test.tsx`
- Modify: `packages/mobile-shared/api/products.ts`
- Modify: `packages/mobile-shared/api/types.ts`
- Modify: `apps/mobile-admin/lib/hooks/use-products.ts`
- Modify: `apps/mobile-admin/lib/admin-api/product-crud.ts`
- Modify: `apps/mobile-admin/components/ProductRow.tsx`
- Modify: `apps/mobile-admin/app/(tabs)/products/{index,[id],new}.tsx`
- Modify: `apps/mobile-admin/lib/demo-api-client.ts` (DEMO_PRODUCTS + productDetail + `/products` branch)

**Interfaces:**
- Consumes: `money`, `paginated` from `../schema-helpers`.
- Produces:
  - `productSchema`, `productVariantSchema`, `productMediaSchema`, `productListSchema`; types `Product`, `ProductVariant`, `ProductMedia`, `ProductListResponse`.
  - `apps/mobile-admin/lib/product-display.ts` exporting exactly:
    - `primaryVariant(p: Product): ProductVariant | undefined`
    - `productPrice(p: Product): number | undefined`
    - `productSku(p: Product): string | undefined`
    - `productStock(p: Product): number`
    - `productThumb(p: Product): string | undefined`
    - `productCurrency(p: Product): string | undefined`
    - `formatMoney(amount: number, currencyCode?: string): string`

**Live wire truth** — verified across **all 161 products** on 2026-07-16, not a single sample:

```json
{"id":"a28defe3-9bf0-4273-9247-6f57a5e5a5ab","store_id":"8b69eea9-…","handle":"palm-beach-linen-robe",
 "title":"Palm Beach Linen Robe","description":"An open-front linen robe…","status":"active",
 "tags":["linen","robe"],"seo_title":"…","seo_description":"…",
 "primary_category_id":"bdd640fb-…","categories":[],"options":[],
 "variants":[{"id":"3eabedcb-…","sku":"TBS-PBLR-XS-S","price":"199","currency_code":"AUD",
              "inventory_quantity":0,"inventory_policy":"deny","option_values":[],"position":0}],
 "media":[{"id":"4870d33f-…","url":"https://cdn.mark8ly.com/…","storage_key":"tenants/…",
           "alt":"The Bondi Store — Palm Beach Linen Robe","position":0,"media_type":"image"}],
 "published_at":"2026-05-04T23:48:01.08461Z","created_at":"2026-05-04T23:48:01.08461Z",
 "updated_at":"2026-05-04T23:48:01.08461Z"}
```

Facts that drive the design — every one measured, none assumed:
- **`variants` come back UNSORTED.** "Bondi Linen Beach Shirt" returns positions `2,3,4,0,1`. **`variants[0]` is NOT the primary variant** — it would show the M variant's SKU/stock instead of XS. **Sort by `position`.** Same for `media`.
- **Multi-variant is the common case**: 8 products have 2–5 variants, and all 8 are `active` — out of only **12 active products total**.
- Every `price` is a **quoted string**, including `"19.99"`. Every `currency_code` is `AUD`.
- **149 `draft` / 12 `active`.** Zero products have zero variants (but the schema/helpers must still tolerate it — do not assume). One product has **no media**. Every variant has a `sku`.
- `created_at` **does** exist at top level (the handoff said it did not). `compare_at_price` genuinely does not.
- The app's `Product` type — `name`, `price:number`, `compare_at_price`, `sku`, `stock`, `thumbnail_url` — is **entirely fictional**. Not one of those keys is on the wire.

- [ ] **Step 1: Write the failing schema test**

Create `apps/mobile-admin/__tests__/schemas-products.test.tsx`:

```tsx
import { productSchema, productListSchema } from "@repo/mobile-shared/api/schemas/products";

// A verbatim product from prod (2026-07-16), trimmed to the fields that matter.
const REAL_PRODUCT = {
  id: "a28defe3-9bf0-4273-9247-6f57a5e5a5ab",
  store_id: "8b69eea9-2537-4d36-9d99-bafcbad02dbc",
  handle: "palm-beach-linen-robe",
  title: "Palm Beach Linen Robe",
  description: "An open-front linen robe to throw over swimwear.",
  status: "active",
  tags: ["linen", "robe"],
  seo_title: "Palm Beach Linen Robe — The Bondi Store",
  seo_description: "Open-front linen beach robe with tie waist.",
  primary_category_id: "bdd640fb-0667-4ad1-9c80-317fa3b1799d",
  categories: [],
  options: [],
  variants: [
    { id: "3eabedcb", sku: "TBS-PBLR-XS-S", price: "199", currency_code: "AUD", inventory_quantity: 0, inventory_policy: "deny", option_values: [], position: 0 },
    { id: "451b4cf3", sku: "TBS-PBLR-M-L", price: "199", currency_code: "AUD", inventory_quantity: 4, inventory_policy: "deny", option_values: [], position: 1 },
  ],
  media: [
    { id: "4870d33f", url: "https://cdn.mark8ly.com/a.png", storage_key: "tenants/x/a.png", alt: "front", position: 0, media_type: "image" },
  ],
  published_at: "2026-05-04T23:48:01.08461Z",
  created_at: "2026-05-04T23:48:01.08461Z",
  updated_at: "2026-05-04T23:48:01.08461Z",
};

describe("productSchema", () => {
  it("parses a real product and coerces the QUOTED price string to a number", () => {
    const parsed = productSchema.parse(REAL_PRODUCT);
    expect(parsed.title).toBe("Palm Beach Linen Robe");
    expect(parsed.variants[0]!.price).toBe(199);
    expect(typeof parsed.variants[0]!.price).toBe("number");
  });

  it("parses a decimal price string like \"19.99\"", () => {
    const p = { ...REAL_PRODUCT, variants: [{ ...REAL_PRODUCT.variants[0], price: "19.99" }] };
    expect(productSchema.parse(p).variants[0]!.price).toBe(19.99);
  });

  it("accepts a product with no media", () => {
    const parsed = productSchema.parse({ ...REAL_PRODUCT, media: [] });
    expect(parsed.media).toEqual([]);
  });

  it("accepts a product with no variants without throwing", () => {
    const parsed = productSchema.parse({ ...REAL_PRODUCT, variants: [] });
    expect(parsed.variants).toEqual([]);
  });

  it("parses the real list envelope, meta.total 161", () => {
    const parsed = productListSchema.parse({
      data: [REAL_PRODUCT],
      meta: { page: 1, page_size: 20, total: 161, total_pages: 9 },
    });
    expect(parsed.meta.total).toBe(161);
    expect(parsed.data[0]!.title).toBe("Palm Beach Linen Robe");
  });

  it("rejects the {items} fiction", () => {
    expect(productListSchema.safeParse({ items: [], total: 0 }).success).toBe(false);
  });

  it("names the field path on a bad price", () => {
    const bad = { ...REAL_PRODUCT, variants: [{ ...REAL_PRODUCT.variants[0], price: "abc" }] };
    const res = productSchema.safeParse(bad);
    expect(res.success).toBe(false);
    if (!res.success) expect(res.error.issues[0]!.path.join(".")).toContain("price");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-products.test.tsx --forceExit`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the schema module**

Create `packages/mobile-shared/api/schemas/products.ts`:

```ts
import { z } from "zod";
import { money, paginated } from "../schema-helpers";

/**
 * Wire truth for the admin product endpoints. Verified 2026-07-16 against
 * ALL 161 products in the Bondi store (not a single sample).
 *
 * The app's previous Product type was entirely fictional: `name`, `price`,
 * `compare_at_price`, `sku`, `stock`, `thumbnail_url` — not one of those keys
 * exists on the wire. The real shape is `title` plus `variants[]` and
 * `media[]`, exactly like the web admin.
 *
 * Variant `price` is a shopspring/decimal.Decimal and arrives QUOTED
 * ("199", "19.99") — live proof of why `money` is a number|string union.
 */
export const variantOptionValueSchema = z.object({
  option_name: z.string(),
  value: z.string(),
});

export const productVariantSchema = z.object({
  id: z.string(),
  sku: z.string(),
  price: money,
  compare_at_price: money.optional(),
  currency_code: z.string(),
  inventory_quantity: z.number(),
  inventory_policy: z.string(),
  option_values: z.array(variantOptionValueSchema),
  /**
   * The wire does NOT sort variants by position — a real product came back
   * as 2,3,4,0,1. Anything picking a "primary" variant must sort by this
   * field; variants[0] is not it. See lib/product-display.ts.
   */
  position: z.number(),
});
export type ProductVariant = z.infer<typeof productVariantSchema>;

export const productMediaSchema = z.object({
  id: z.string(),
  url: z.string(),
  storage_key: z.string(),
  alt: z.string().optional(),
  position: z.number(),
  media_type: z.string().optional(),
});
export type ProductMedia = z.infer<typeof productMediaSchema>;

export const productOptionSchema = z.object({
  name: z.string(),
  values: z.array(z.string()),
});

export const productSchema = z.object({
  id: z.string(),
  store_id: z.string(),
  handle: z.string(),
  title: z.string(),
  description: z.string().optional(),
  // Backend enum: draft | active | archived. NOT "inactive" — sending that
  // to ?status= is a 400 (verified live).
  status: z.string(),
  tags: z.array(z.string()),
  seo_title: z.string().optional(),
  seo_description: z.string().optional(),
  primary_category_id: z.string().optional(),
  categories: z.array(z.unknown()),
  options: z.array(productOptionSchema),
  variants: z.array(productVariantSchema),
  media: z.array(productMediaSchema),
  published_at: z.string().optional(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Product = z.infer<typeof productSchema>;

/** The detail endpoint returns the same product object, unwrapped. */
export const productDetailSchema = productSchema;
export type ProductDetail = z.infer<typeof productDetailSchema>;

export const productListSchema = paginated(productSchema);
export type ProductListResponse = z.infer<typeof productListSchema>;
```

- [ ] **Step 4: Run the schema test to verify it passes**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-products.test.tsx --forceExit`
Expected: PASS, 7 tests.

- [ ] **Step 5: Write the failing product-display test**

Create `apps/mobile-admin/__tests__/product-display.test.tsx`:

```tsx
import {
  primaryVariant,
  productPrice,
  productSku,
  productStock,
  productThumb,
  productCurrency,
  formatMoney,
} from "@/lib/product-display";
import type { Product } from "@repo/mobile-shared/api/types";

function makeProduct(over: Partial<Product> = {}): Product {
  return {
    id: "p-1",
    store_id: "s-1",
    handle: "h",
    title: "T",
    status: "active",
    tags: [],
    categories: [],
    options: [],
    variants: [],
    media: [],
    created_at: "2026-05-04T23:48:01Z",
    updated_at: "2026-05-04T23:48:01Z",
    ...over,
  } as Product;
}

const v = (id: string, position: number, price: number, sku: string, qty: number) => ({
  id, sku, price, currency_code: "AUD",
  inventory_quantity: qty, inventory_policy: "deny",
  option_values: [], position,
});

describe("primaryVariant", () => {
  it("picks by POSITION, not array order — the wire returns them unsorted", () => {
    // Exactly the real "Bondi Linen Beach Shirt" ordering: 2,3,4,0,1.
    const p = makeProduct({
      variants: [
        v("m", 2, 149, "TBS-BLBS-M", 5),
        v("l", 3, 149, "TBS-BLBS-L", 6),
        v("xl", 4, 149, "TBS-BLBS-XL", 7),
        v("xs", 0, 149, "TBS-BLBS-XS", 1),
        v("s", 1, 149, "TBS-BLBS-S", 2),
      ],
    });
    expect(primaryVariant(p)!.id).toBe("xs");
    expect(productSku(p)).toBe("TBS-BLBS-XS");
    expect(productStock(p)).toBe(1);
  });

  it("returns undefined when there are no variants", () => {
    expect(primaryVariant(makeProduct())).toBeUndefined();
    expect(productPrice(makeProduct())).toBeUndefined();
    expect(productSku(makeProduct())).toBeUndefined();
    expect(productCurrency(makeProduct())).toBeUndefined();
  });

  it("does not mutate the input array", () => {
    const variants = [v("b", 1, 1, "B", 0), v("a", 0, 1, "A", 0)];
    const p = makeProduct({ variants });
    primaryVariant(p);
    expect(variants[0]!.id).toBe("b");
  });

  it("handles the single-variant case", () => {
    const p = makeProduct({ variants: [v("only", 0, 21, "BND-49", 100)] });
    expect(productPrice(p)).toBe(21);
    expect(productStock(p)).toBe(100);
    expect(productCurrency(p)).toBe("AUD");
  });
});

describe("productStock", () => {
  it("sums inventory across ALL variants, not just the primary", () => {
    const p = makeProduct({
      variants: [v("a", 0, 1, "A", 3), v("b", 1, 1, "B", 4)],
    });
    expect(productStock(p)).toBe(7);
  });

  it("is 0 when there are no variants", () => {
    expect(productStock(makeProduct())).toBe(0);
  });
});

describe("productThumb", () => {
  it("picks the lowest-position media, not media[0]", () => {
    const p = makeProduct({
      media: [
        { id: "b", url: "b.png", storage_key: "b", position: 1 },
        { id: "a", url: "a.png", storage_key: "a", position: 0 },
      ],
    });
    expect(productThumb(p)).toBe("a.png");
  });

  it("returns undefined when a product has no media (1 real product does not)", () => {
    expect(productThumb(makeProduct())).toBeUndefined();
  });
});

describe("formatMoney", () => {
  it("uses the product's real currency, not a hardcoded USD", () => {
    expect(formatMoney(199, "AUD")).toContain("199");
    expect(formatMoney(199, "AUD")).not.toBe(formatMoney(199, "USD"));
  });

  it("falls back to a bare number when no currency is known", () => {
    expect(formatMoney(199)).toContain("199");
  });
});
```

- [ ] **Step 6: Run the test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/product-display.test.tsx --forceExit`
Expected: FAIL — cannot find module `@/lib/product-display`.

- [ ] **Step 7: Write product-display.ts**

Create `apps/mobile-admin/lib/product-display.ts`:

```ts
import type { Product, ProductVariant } from "@repo/mobile-shared/api/types";

/**
 * Variant/media picking, in one place so `variants[0]` never scatters across
 * screens — and because `variants[0]` is WRONG.
 *
 * The API does not sort variants by position: a real product ("Bondi Linen
 * Beach Shirt", verified 2026-07-16) comes back as positions 2,3,4,0,1, so
 * variants[0] is the M variant, not XS. Every one of these helpers sorts.
 *
 * Multi-variant is not an edge case here: 8 of the store's 12 ACTIVE products
 * have 2-5 variants.
 */

/** Lowest `position` wins. Never mutates the input (`.sort` is in-place). */
export function primaryVariant(product: Product): ProductVariant | undefined {
  if (product.variants.length === 0) return undefined;
  return [...product.variants].sort((a, b) => a.position - b.position)[0];
}

export function productPrice(product: Product): number | undefined {
  return primaryVariant(product)?.price;
}

export function productSku(product: Product): string | undefined {
  return primaryVariant(product)?.sku;
}

export function productCurrency(product: Product): string | undefined {
  return primaryVariant(product)?.currency_code;
}

/**
 * Total stock across every variant — what a merchant means by "how many do I
 * have". The primary variant's count alone would understate a 5-variant
 * product by 80%.
 */
export function productStock(product: Product): number {
  return product.variants.reduce((sum, v) => sum + v.inventory_quantity, 0);
}

/** Lowest-position media URL. One real product has no media at all. */
export function productThumb(product: Product): string | undefined {
  if (product.media.length === 0) return undefined;
  return [...product.media].sort((a, b) => a.position - b.position)[0]!.url;
}

/**
 * Formats money in the currency the wire actually reported. Every price in
 * this app used to render as USD via a hardcoded Intl option, while the store
 * is AUD and every variant carries currency_code: "AUD".
 */
export function formatMoney(amount: number, currencyCode?: string): string {
  if (!currencyCode) {
    return new Intl.NumberFormat("en-AU", { minimumFractionDigits: 2 }).format(amount);
  }
  return new Intl.NumberFormat("en-AU", {
    style: "currency",
    currency: currencyCode,
    minimumFractionDigits: 2,
  }).format(amount);
}
```

- [ ] **Step 8: Run the test to verify it passes**

Run: `cd apps/mobile-admin && npx jest __tests__/product-display.test.tsx --forceExit`
Expected: PASS, 10 tests.

- [ ] **Step 9: Wire the schema into the api module**

Replace the whole of `packages/mobile-shared/api/products.ts` with:

```ts
import type { createApiClient } from "./client";
import {
  productDetailSchema,
  productListSchema,
  type Product,
  type ProductDetail,
  type ProductListResponse,
} from "./schemas/products";

export interface ListProductsParams {
  /** draft | active | archived. "inactive" is a 400 — it is not a real status. */
  status?: string;
  search?: string;
  page?: string;
  page_size?: string;
}

/**
 * CreateProductRequest (validation.go:231-249) requires `title` and at least
 * one variant, each with a required `sku` and `price`. The old body — name /
 * price / stock — was an unconditional 400.
 */
export interface CreateProductVariantBody {
  sku: string;
  price: number;
  currency_code?: string;
  inventory_quantity?: number;
  position?: number;
}

export interface CreateProductBody {
  title: string;
  description?: string;
  status?: string;
  tags?: string[];
  variants: CreateProductVariantBody[];
}

export interface UpdateProductBody {
  title?: string;
  description?: string;
  status?: string;
  tags?: string[];
}

/** UpdateVariantRequest (validation.go:43-55). There is no `stock` field. */
export interface UpdateVariantBody {
  sku?: string;
  price?: number;
  inventory_quantity?: number;
}

export function createProductsApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListProductsParams) =>
      client.get<ProductListResponse>(
        "/products",
        params as Record<string, string>,
        productListSchema,
      ),
    get: (id: string) =>
      client.get<ProductDetail>(`/products/${id}`, undefined, productDetailSchema),
    create: (body: CreateProductBody) =>
      client.post<ProductDetail>("/products", body, productDetailSchema),
    update: (id: string, body: UpdateProductBody) =>
      client.patch<ProductDetail>(`/products/${id}`, body, productDetailSchema),
    /**
     * `inventory_quantity`, NOT `stock`. UpdateVariantRequest has no `stock`
     * field, so the old body's stock edits were silently discarded with a 200.
     */
    updateVariant: (productId: string, variantId: string, body: UpdateVariantBody) =>
      client.patch(`/products/${productId}/variants/${variantId}`, body),
    deleteMedia: (productId: string, mediaId: string) =>
      client.delete(`/products/${productId}/media/${mediaId}`),
  };
}

export type { Product, ProductDetail };
```

Removed on purpose, because these routes do not exist and leaving them is a trap:
- `createVariant` / `listVariants` — there is no `POST /products/:id/variants` and no `GET .../variants`. Only `PATCH /products/:id/variants/:variantId` is mounted (`mobile_routes.go:86`).
- `uploadMedia` / `reorderMedia` — media needs a 3-step signed-URL flow (`POST /media/upload-url` → PUT → `POST /media`), not a multipart POST. Out of scope per the spec; removing the method means tsc names every caller instead of it failing at runtime.
- `low_stock` from `ListProductsParams` — the backend ignores it entirely (verified: `?low_stock=true` returns all 161).

- [ ] **Step 10: Re-export the inferred types**

In `packages/mobile-shared/api/types.ts`, delete the hand-written `Product`, `ProductDetail`, `MediaItem` and `Variant` interfaces (lines ~78-112) and add:

```ts
export type {
  Product,
  ProductDetail,
  ProductVariant,
  ProductMedia,
} from "./schemas/products";
```

If `MediaItem`/`Variant` are referenced anywhere else, tsc will name those sites — update them to `ProductMedia`/`ProductVariant`.

- [ ] **Step 11: Update the hook**

Replace `apps/mobile-admin/lib/hooks/use-products.ts` with:

```ts
import { useQuery } from "@tanstack/react-query";
import { createProductsApi } from "@repo/mobile-shared/api/products";
import type { ProductDetail } from "@repo/mobile-shared/api/types";
import type { ProductListResponse } from "@repo/mobile-shared/api/schemas/products";
import { useApiClient } from "@/lib/api-client";

interface ProductListParams {
  status?: string;
  search?: string;
}

export function useProducts(params?: ProductListParams) {
  const client = useApiClient();
  const productsApi = createProductsApi(client);

  return useQuery<ProductListResponse>({
    queryKey: ["products", params?.status, params?.search],
    queryFn: () =>
      productsApi.list({
        ...(params?.status ? { status: params.status } : {}),
        ...(params?.search ? { search: params.search } : {}),
      }),
    refetchOnWindowFocus: true,
  });
}

export function useProduct(id: string) {
  const client = useApiClient();
  const productsApi = createProductsApi(client);

  return useQuery<ProductDetail>({
    queryKey: ["product", id],
    queryFn: () => productsApi.get(id),
    enabled: !!id,
  });
}
```

`low_stock` is gone from the params — the filter does not exist server-side.

- [ ] **Step 12: Update product-crud.ts**

In `apps/mobile-admin/lib/admin-api/product-crud.ts`:
- Delete `useUploadMedia` and `useCreateVariant` entirely (their API methods no longer exist).
- Change `useUpdateProduct`'s body type from `Record<string, unknown>` to `UpdateProductBody`.
- Change `useUpdateVariant`'s body type to `UpdateVariantBody`.
- Keep `useCreateProduct` and `useDeleteMedia`, updating their imported types.

```ts
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createProductsApi,
  type CreateProductBody,
  type UpdateProductBody,
  type UpdateVariantBody,
} from "@repo/mobile-shared/api/products";
import { useApiClient } from "@/lib/api-client";
```

and for the two update hooks:

```ts
export function useUpdateProduct() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: UpdateProductBody }) =>
      productsApi.update(id, body),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ["products"] });
      queryClient.invalidateQueries({ queryKey: ["product", variables.id] });
    },
  });
}

export function useUpdateVariant() {
  const client = useApiClient();
  const productsApi = createProductsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      productId,
      variantId,
      body,
    }: {
      productId: string;
      variantId: string;
      body: UpdateVariantBody;
    }) => productsApi.updateVariant(productId, variantId, body),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ["product", variables.productId] });
    },
  });
}
```

- [ ] **Step 13: Fix ProductRow**

Replace the body of `apps/mobile-admin/components/ProductRow.tsx` so it reads the real shape through the helpers. Delete its local `formatCurrency` (which hardcoded USD) and use `formatMoney`:

```tsx
import { View, Image, TouchableOpacity, StyleSheet } from "react-native";
import { Package } from "lucide-react-native";
import { StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Product } from "@repo/mobile-shared/api/types";
import {
  formatMoney,
  productCurrency,
  productPrice,
  productStock,
  productThumb,
} from "@/lib/product-display";

interface ProductRowProps {
  product: Product;
  onPress: (product: Product) => void;
}

export function ProductRow({ product, onPress }: ProductRowProps) {
  const price = productPrice(product);
  const stock = productStock(product);
  const thumb = productThumb(product);
  const priceLabel = price === undefined ? "—" : formatMoney(price, productCurrency(product));
  const lowStock = stock <= 5;

  return (
    <TouchableOpacity
      style={styles.container}
      onPress={() => onPress(product)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`${product.title}, ${priceLabel}, stock ${stock}, ${product.status}`}
    >
      {thumb ? (
        <Image
          source={{ uri: thumb }}
          style={styles.thumb}
          accessibilityLabel={`${product.title} thumbnail`}
        />
      ) : (
        <View style={[styles.thumb, styles.thumbPlaceholder]}>
          <Package size={20} color={theme.colors.textTertiary} strokeWidth={1.5} />
        </View>
      )}

      <View style={styles.info}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
          {product.title}
        </Text>
        <View style={styles.metaRow}>
          <Text preset="caption" color="text">
            {priceLabel}
          </Text>
          {/* ...keep the rest of the existing meta/status JSX, replacing
              product.name -> product.title, product.price -> priceLabel,
              product.stock -> stock, product.thumbnail_url -> thumb. */}
        </View>
      </View>
    </TouchableOpacity>
  );
}
```

Keep the existing `styles` block and the remaining JSX below `metaRow` as-is, applying the same field substitutions. `isActive` (`product.status === "active"`) still works unchanged.

- [ ] **Step 14: Fix the products list screen**

In `apps/mobile-admin/app/(tabs)/products/index.tsx`:

1. Line 92: `data={data?.items ?? []}` → `data={data?.data ?? []}`.
2. Replace the FILTERS array and `FilterKey` (lines 25-31). "Inactive" sends `status=inactive`, which is a **verified 400** (`draft|active|archived` only). "Low Stock" sends a param the backend **silently ignores**, returning all 161 — a tab that claims to filter and does not:

```tsx
type FilterKey = "all" | "active" | "draft";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "active", label: "Active" },
  // Was "Inactive" -> status=inactive, a hard 400: the backend enum is
  // draft|active|archived. 149 of the store's 161 products are drafts.
  { key: "draft", label: "Draft" },
];
```

(There was no "Low Stock" replacement: the backend has no such filter. Removing the tab is the honest option — see the spec.)

3. Replace the `queryParams` construction (lines ~48-52):

```tsx
  const queryParams = {
    ...(activeFilter !== "all" ? { status: activeFilter } : {}),
    ...(debouncedSearch ? { search: debouncedSearch } : {}),
  };
```

- [ ] **Step 15: Fix the product detail and new-product screens**

`app/(tabs)/products/[id].tsx` (628 lines) and `app/(tabs)/products/new.tsx` both read the fictional shape. **Run tsc and let it name every site** — that is the entire point of the inferred types:

```bash
cd apps/mobile-admin && npx tsc --noEmit --pretty false 2>&1 | grep "error TS"
```

Apply these substitutions wherever tsc points:
- `product.name` → `product.title`
- `product.price` → `productPrice(product)` (may be `undefined` — render `—`)
- `product.sku` → `productSku(product)`
- `product.stock` → `productStock(product)`
- `product.thumbnail_url` → `productThumb(product)`
- `product.compare_at_price` → **delete the UI** — the field does not exist on the wire
- `product.category_name` / `product.barcode` / `product.category_id` → **delete** — none exist
- `variant.name` → the wire has no variant name. Use `variant.sku`, or `variant.option_values.map(o => o.value).join(" / ")` when option_values is non-empty (it is always `[]` today).
- `variant.stock` → `variant.inventory_quantity`
- `variant.price` is now a `number` after parsing — `String(variant.price)` still works for the TextInput.
- Any `formatCurrency(x)` → `formatMoney(x, productCurrency(product))`.
- Iterate variants in display order: `[...product.variants].sort((a, b) => a.position - b.position)`.

In `VariantRow`'s `handleBlurStock` (line ~58), the PATCH body must become `{ inventory_quantity: parsed }` — `stock` is silently discarded today:

```tsx
  const handleBlurStock = () => {
    const parsed = parseInt(stock, 10);
    if (!isNaN(parsed) && parsed !== variant.inventory_quantity) {
      onUpdate(variant.id, { inventory_quantity: parsed });
    }
  };
```

In `new.tsx`, the create body must satisfy `CreateProductBody` — `title` plus at least one variant with `sku` and `price`. If the screen has no SKU input, derive one from the title (the backend requires it, `max=100`):

```tsx
    createMutation.mutate({
      title,
      description: description || undefined,
      status: "draft",
      variants: [
        {
          sku: sku || `${title.trim().toUpperCase().replace(/[^A-Z0-9]+/g, "-").slice(0, 40)}-1`,
          price: parseFloat(price),
          currency_code: "AUD",
          inventory_quantity: parseInt(stock, 10) || 0,
          position: 0,
        },
      ],
    });
```

Also remove the `useUploadMedia`/`useCreateVariant` imports and their call sites in both screens (those API methods are gone). If `ProductMediaPicker` is only used for upload, remove its usage and leave the component file in place.

**Media upload is out of scope** — if `[id].tsx` has an upload button, remove the button rather than leaving a handler that calls a deleted method.

- [ ] **Step 16: Make the demo fixtures wire-truthful**

In `apps/mobile-admin/lib/demo-api-client.ts`:

1. Replace `DEMO_PRODUCTS` (lines 46-50). Include a multi-variant product with **out-of-order positions**, mirroring real prod data — this is the fixture that proves `primaryVariant` sorts:

```ts
const DEMO_PRODUCTS: Product[] = [
  {
    id: "p-1", store_id: "demo-store", handle: "linen-camp-shirt", title: "Linen Camp Shirt",
    description: "A demo product.", status: "active", tags: ["demo"], categories: [], options: [],
    // Deliberately out of position order — the real API returns them like this.
    variants: [
      { id: "v-m", sku: "LCS-001-M", price: 8900, currency_code: "AUD", inventory_quantity: 20, inventory_policy: "deny", option_values: [], position: 1 },
      { id: "v-s", sku: "LCS-001-S", price: 8900, currency_code: "AUD", inventory_quantity: 22, inventory_policy: "deny", option_values: [], position: 0 },
    ],
    media: [{ id: "m-1", url: "https://cdn.mark8ly.com/demo/shirt.png", storage_key: "demo/shirt.png", position: 0, media_type: "image" }],
    created_at: iso(20), updated_at: iso(20),
  },
  {
    id: "p-2", store_id: "demo-store", handle: "merino-beanie", title: "Merino Beanie",
    description: "A demo product.", status: "active", tags: [], categories: [], options: [],
    variants: [{ id: "v-b", sku: "MB-014", price: 3900, currency_code: "AUD", inventory_quantity: 7, inventory_policy: "deny", option_values: [], position: 0 }],
    media: [],
    created_at: iso(15), updated_at: iso(15),
  },
  {
    id: "p-3", store_id: "demo-store", handle: "canvas-weekender", title: "Canvas Weekender",
    description: "A demo product.", status: "draft", tags: [], categories: [], options: [],
    variants: [{ id: "v-w", sku: "CW-220", price: 14900, currency_code: "AUD", inventory_quantity: 0, inventory_policy: "deny", option_values: [], position: 0 }],
    media: [],
    created_at: iso(9), updated_at: iso(9),
  },
];
```

2. Replace `productDetail` (lines 126-138) — detail is the same shape as list:

```ts
function productDetail(id: string): ProductDetail {
  return DEMO_PRODUCTS.find((p) => p.id === id) ?? DEMO_PRODUCTS[0]!;
}
```

3. Change the `/products` branch (line 167) from `page(DEMO_PRODUCTS)` to:

```ts
  if (clean === "/products") return paged(DEMO_PRODUCTS);
```

4. Update the import block at the top of the file: `Product`/`ProductDetail` still come from `@repo/mobile-shared/api/types`, but `MediaItem`/`Variant` no longer exist — remove them if imported.

- [ ] **Step 17: Run both gates**

Run: `cd apps/mobile-admin && npx jest 2>&1 | tail -8`
Expected: all green, **≥ 165 tests** (148 + 7 schema + 10 display).

Run: `cd apps/mobile-admin && npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"`
Expected: **2**.

- [ ] **Step 18: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add packages/mobile-shared/api/schemas/products.ts packages/mobile-shared/api/products.ts packages/mobile-shared/api/types.ts apps/mobile-admin/lib/product-display.ts apps/mobile-admin/lib/hooks/use-products.ts apps/mobile-admin/lib/admin-api/product-crud.ts apps/mobile-admin/components/ProductRow.tsx "apps/mobile-admin/app/(tabs)/products" apps/mobile-admin/lib/demo-api-client.ts apps/mobile-admin/__tests__/schemas-products.test.tsx apps/mobile-admin/__tests__/product-display.test.tsx
git commit -m "fix(mobile-admin): products go variant-aware on the real wire shape"
```

---

### Task 5: Notifications — the third envelope, and `PaginatedResponse` dies

**Files:**
- Modify: `packages/mobile-shared/api/schema-helpers.ts` (add `legacyPaged`)
- Create: `packages/mobile-shared/api/schemas/notifications.ts`
- Create: `apps/mobile-admin/__tests__/schemas-notifications.test.tsx`
- Modify: `packages/mobile-shared/api/notifications.ts`
- Modify: `packages/mobile-shared/api/types.ts` (**delete `PaginatedResponse`**)
- Modify: `apps/mobile-admin/lib/hooks/use-notifications.ts`
- Modify: `apps/mobile-admin/app/(tabs)/more/notifications.tsx` and `app/(tabs)/more/index.tsx:56`
- Modify: `apps/mobile-admin/lib/demo-api-client.ts` (**delete `page()`**)

**Interfaces:**
- Consumes: nothing from Tasks 2–4.
- Produces: `legacyPaged(key, item)` in `schema-helpers.ts`; `notificationSchema`, `notificationListSchema`; type `Notification`.

**Wire truth** — verified live 2026-07-16: `GET /notifications` → `{"notifications":[],"page":1,"per_page":20,"total":0}`. This is **not** `{data, meta}`; `paginated()` would throw on it. Handler: `notifications.go:85`.

The item shape comes from `NotificationResponse` (`notifications.go:31-40`) — the endpoint is empty in prod so the item could not be verified live:
```go
ID string `json:"id"` · Type string `json:"type"` · Title string `json:"title"`
Message *string `json:"message,omitempty"` · ResourceType *string `json:"resource_type,omitempty"`
ResourceID *string `json:"resource_id,omitempty"` · IsRead bool `json:"is_read"`
CreatedAt string `json:"created_at"`
```

The app's `Notification` type invents `body`, `read` and `deep_link`. Real notification `type` constants (`notification/models.go:16-30`): `new_order`, `low_stock`, `return_requested`, `payment_received`, `review_submitted`, `order_cancelled`, `order_fulfilled`, `system_alert`. The screen's `TYPE_DOT` map keys (`order`/`payment`/`alert`/`system`) match **none** of them.

**`markAllRead` is a guaranteed 404**: the client calls `POST /notifications/mark-all-read`; the route is `PATCH /notifications/read-all` (`mobile_routes.go:159`). Wrong method AND wrong path.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/schemas-notifications.test.tsx`:

```tsx
import {
  notificationSchema,
  notificationListSchema,
} from "@repo/mobile-shared/api/schemas/notifications";

// The EXACT payload GET /notifications returned from prod on 2026-07-16.
const REAL_EMPTY = { notifications: [], page: 1, per_page: 20, total: 0 };

const ITEM = {
  id: "n-1",
  type: "new_order",
  title: "New order #1001",
  message: "Maya Chen placed an order",
  resource_type: "order",
  resource_id: "o-1001",
  is_read: false,
  created_at: "2026-07-14T09:00:00Z",
};

describe("notificationListSchema", () => {
  it("parses the real {notifications, page, per_page, total} envelope", () => {
    const parsed = notificationListSchema.parse(REAL_EMPTY);
    expect(parsed.notifications).toEqual([]);
    expect(parsed.total).toBe(0);
    expect(parsed.per_page).toBe(20);
  });

  it("rejects the {data, meta} envelope — notifications is NOT paginated()", () => {
    expect(
      notificationListSchema.safeParse({
        data: [],
        meta: { page: 1, page_size: 20, total: 0, total_pages: 0 },
      }).success,
    ).toBe(false);
  });

  it("rejects the {items} fiction", () => {
    expect(notificationListSchema.safeParse({ items: [], total: 0 }).success).toBe(false);
  });

  it("parses a populated notification", () => {
    const parsed = notificationListSchema.parse({ ...REAL_EMPTY, notifications: [ITEM], total: 1 });
    expect(parsed.notifications[0]!.is_read).toBe(false);
    expect(parsed.notifications[0]!.title).toBe("New order #1001");
  });

  it("accepts a notification with no message (omitempty)", () => {
    const bare = { ...ITEM } as Record<string, unknown>;
    delete bare.message;
    delete bare.resource_type;
    delete bare.resource_id;
    const parsed = notificationSchema.parse(bare);
    expect(parsed.message).toBeUndefined();
  });

  it("names the field path when is_read is the wrong type", () => {
    const res = notificationSchema.safeParse({ ...ITEM, is_read: "false" });
    expect(res.success).toBe(false);
    if (!res.success) expect(res.error.issues[0]!.path).toContain("is_read");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-notifications.test.tsx --forceExit`
Expected: FAIL — module not found.

- [ ] **Step 3: Add `legacyPaged` to schema-helpers**

Append to `packages/mobile-shared/api/schema-helpers.ts`:

```ts
/**
 * The odd envelope out: `{<key>: [...], page, per_page, total}`.
 *
 * Only /notifications uses it (notifications.go:85) — every other list
 * endpoint returns `{data, meta}`. Verified live 2026-07-16:
 * `{"notifications":[],"page":1,"per_page":20,"total":0}`.
 *
 * Named `legacy` because this SHOULD be normalised to `paginated` server-side
 * one day; until then the app must not pretend the shape is something it is
 * not. Note `per_page` here vs `page_size` in pageMeta — also inconsistent,
 * also real.
 */
export const legacyPaged = <K extends string, T extends z.ZodTypeAny>(key: K, item: T) =>
  z.object({
    [key]: z.array(item),
    page: z.number(),
    per_page: z.number(),
    total: z.number(),
  } as { [P in K]: z.ZodArray<T> } & {
    page: z.ZodNumber;
    per_page: z.ZodNumber;
    total: z.ZodNumber;
  });
```

- [ ] **Step 4: Write the schema module**

Create `packages/mobile-shared/api/schemas/notifications.ts`:

```ts
import { z } from "zod";
import { legacyPaged } from "../schema-helpers";

/**
 * Wire truth for the admin notifications endpoint —
 * NotificationResponse (notifications.go:31-40).
 *
 * The list envelope is NOT {data, meta}: it is
 * {notifications, page, per_page, total} (verified live 2026-07-16).
 *
 * The app's old type invented `body`, `read` and `deep_link`. The wire sends
 * `message`, `is_read`, and resource_type/resource_id. There is no deep_link
 * anywhere in the backend; tapping a notification is a no-op until a
 * resource_type -> route map can be built against real data (the endpoint is
 * empty in prod, so any mapping today would be a guess).
 */
export const notificationSchema = z.object({
  id: z.string(),
  /**
   * Real values (notification/models.go:16-30): new_order, low_stock,
   * return_requested, payment_received, review_submitted, order_cancelled,
   * order_fulfilled, system_alert. Kept as a plain string so a new backend
   * type never hard-fails a merchant's inbox.
   */
  type: z.string(),
  title: z.string(),
  message: z.string().optional(),
  resource_type: z.string().optional(),
  resource_id: z.string().optional(),
  is_read: z.boolean(),
  created_at: z.string(),
});
export type Notification = z.infer<typeof notificationSchema>;

export const notificationListSchema = legacyPaged("notifications", notificationSchema);
export type NotificationListResponse = z.infer<typeof notificationListSchema>;
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd apps/mobile-admin && npx jest __tests__/schemas-notifications.test.tsx --forceExit`
Expected: PASS, 6 tests.

**The `legacyPaged` generic above is verified, not guessed.** It was compiled against this exact tree (zod 4.4.3) during planning: `legacyPaged("notifications", z.object({id: z.string()}))` infers `parsed.notifications[0].id` as `string`, and assigning it to a `number` correctly errors. You should not need the fallback. If you somehow hit an inference problem, do not fight it — drop the generic and inline the object in `schemas/notifications.ts`, then delete `legacyPaged`, rather than ship an awkward abstraction for one caller:

```ts
export const notificationListSchema = z.object({
  notifications: z.array(notificationSchema),
  page: z.number(),
  per_page: z.number(),
  total: z.number(),
});
```

- [ ] **Step 6: Wire the schema into the api module**

Replace `packages/mobile-shared/api/notifications.ts` with:

```ts
import type { createApiClient } from "./client";
import {
  notificationListSchema,
  type Notification,
  type NotificationListResponse,
} from "./schemas/notifications";

export function createNotificationsApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: { page?: string; per_page?: string }) =>
      client.get<NotificationListResponse>(
        "/notifications",
        params as Record<string, string>,
        notificationListSchema,
      ),
    /**
     * PATCH /notifications/read-all (mobile_routes.go:159).
     * This used to be POST /notifications/mark-all-read — wrong method AND
     * wrong path, so it was an unconditional 404. It went unnoticed because
     * the list is always empty in prod, so the "Mark all" button never renders.
     */
    markAllRead: () => client.patch("/notifications/read-all"),
    registerPushToken: (token: string, platform: string, deviceId: string) =>
      client.post("/push-tokens", { token, platform, device_id: deviceId }),
    deletePushToken: (tokenId: string) => client.delete(`/push-tokens/${tokenId}`),
  };
}

export type { Notification };
```

- [ ] **Step 7: Delete `PaginatedResponse` and re-export the inferred type**

In `packages/mobile-shared/api/types.ts`:
1. **Delete the `PaginatedResponse<T>` interface (lines 1-6) entirely.** This was the last consumer. It is the fiction that hid 161 products for two months.
2. Delete the hand-written `Notification` interface (lines ~133-141).
3. Add: `export type { Notification } from "./schemas/notifications";`

- [ ] **Step 8: Update the hook**

Replace `apps/mobile-admin/lib/hooks/use-notifications.ts` with:

```ts
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { createNotificationsApi } from "@repo/mobile-shared/api/notifications";
import type { NotificationListResponse } from "@repo/mobile-shared/api/schemas/notifications";
import { useApiClient } from "@/lib/api-client";

export function useNotifications() {
  const client = useApiClient();
  const notificationsApi = createNotificationsApi(client);

  return useQuery<NotificationListResponse>({
    queryKey: ["notifications"],
    queryFn: () => notificationsApi.list(),
    refetchOnWindowFocus: true,
  });
}

export function useMarkAllRead() {
  const client = useApiClient();
  const notificationsApi = createNotificationsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => notificationsApi.markAllRead(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["notifications"] });
    },
  });
}
```

- [ ] **Step 9: Fix the notifications screen**

In `apps/mobile-admin/app/(tabs)/more/notifications.tsx`:

1. Correct `TYPE_DOT` (lines 22-27) to the real type constants:

```tsx
// Real values from notification/models.go:16-30. The previous map used
// order/payment/alert/system, which match NOTHING the backend emits, so every
// notification silently fell through to the default colour.
const TYPE_DOT: Record<string, string> = {
  new_order: theme.colors.accent,
  order_fulfilled: theme.colors.accent,
  order_cancelled: theme.colors.danger,
  payment_received: theme.colors.warning,
  low_stock: theme.colors.warning,
  return_requested: theme.colors.warning,
  review_submitted: theme.colors.text,
  system_alert: theme.colors.danger,
};
```

2. `isUnread`: `!notification.read` → `!notification.is_read`.
3. `notification.body` → `notification.message ?? ""` (it is optional).
4. Line 84: `data?.items.some((n) => !n.read)` → `data?.notifications.some((n) => !n.is_read)`.
5. Line 127: `data={data?.items ?? []}` → `data={data?.notifications ?? []}`.
6. `handlePress` (lines 86-91) reads `notification.deep_link`, which does not exist. Replace the callback with a no-op and make the row non-interactive rather than leaving a dead touch target:

```tsx
  // The wire has no deep_link — it sends resource_type/resource_id instead.
  // Mapping those to routes would be pure guesswork: the endpoint is empty in
  // prod, so no real notification has ever been observed. Deferred until there
  // is data to verify against.
  const handlePress = useCallback((_notification: Notification) => {}, []);
```

Leave the `TouchableOpacity` in place (removing it would churn the layout); the no-op press is honest and inert.

- [ ] **Step 10: Fix the unread badge**

In `apps/mobile-admin/app/(tabs)/more/index.tsx`, line 56:

```tsx
  const unreadCount = notifications?.notifications.filter((n) => !n.is_read).length ?? 0;
```

- [ ] **Step 11: Delete `page()` from the demo client**

In `apps/mobile-admin/lib/demo-api-client.ts`:
1. **Delete the `page<T>()` function (lines 106-108) entirely** — Tasks 2–4 removed its other callers; this removes the last.
2. Add a `DEMO_NOTIFICATIONS` fixture and serve the real envelope. Replace the fallback branch (lines 173-175):

```ts
const DEMO_NOTIFICATIONS: Notification[] = [
  { id: "n-1", type: "new_order", title: "New order #1001", message: "Maya Chen placed an order", resource_type: "order", resource_id: "o-1001", is_read: false, created_at: iso(0) },
  { id: "n-2", type: "low_stock", title: "Low stock: Merino Beanie", message: "7 remaining", is_read: true, created_at: iso(2) },
];
```

and in `resolve()`:

```ts
  if (clean === "/notifications") {
    return { notifications: DEMO_NOTIFICATIONS, page: 1, per_page: 20, total: DEMO_NOTIFICATIONS.length };
  }

  // Any endpoint we have not canned. `paged` mirrors the real {data, meta}
  // envelope that every list endpoint except /notifications uses, so an
  // un-canned list renders an empty state instead of failing validation.
  return paged([]);
```

3. Remove `PaginatedResponse` from the import block at the top of the file — it no longer exists.

- [ ] **Step 12: Run both gates**

Run: `cd apps/mobile-admin && npx jest 2>&1 | tail -8`
Expected: all green, **≥ 171 tests** (165 + 6 new).

Run: `cd apps/mobile-admin && npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"`
Expected: **2**.

Also confirm the fiction is gone:
```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
grep -rn "PaginatedResponse\|\.items" apps/mobile-admin/app apps/mobile-admin/lib apps/mobile-admin/components packages/mobile-shared/api | grep -v node_modules
```
Expected: **no matches** (a `.items` hit inside an unrelated word is fine; a `data?.items` hit is not).

- [ ] **Step 13: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add packages/mobile-shared/api/schema-helpers.ts packages/mobile-shared/api/schemas/notifications.ts packages/mobile-shared/api/notifications.ts packages/mobile-shared/api/types.ts apps/mobile-admin/lib/hooks/use-notifications.ts "apps/mobile-admin/app/(tabs)/more" apps/mobile-admin/lib/demo-api-client.ts apps/mobile-admin/__tests__/schemas-notifications.test.tsx
git commit -m "fix(mobile-admin): notifications use their real envelope; PaginatedResponse deleted"
```

---

### Task 6: CI gates + the two real `_layout.tsx` bugs

**Why this matters:** `ci.yml:108` excludes `@repo/mobile-admin` and `@repo/mobile-shared` from turbo `lint check-types build`, and there is no test job at all. Every gate in this plan is enforced only on one laptop. The compile-error leverage that makes Tasks 2–5 cheap is worth nothing to the next contributor without this.

**Files:**
- Modify: `apps/mobile-admin/app/(tabs)/_layout.tsx`
- Modify: `.github/workflows/ci.yml:108`

**Interfaces:** none — this task produces no API surface.

**The two errors are real runtime bugs, not baseline noise.** Verified against the *installed* `expo-notifications@56.0.20` (mobile-admin resolves its own copy; the root `node_modules` has expo 52 for a different app):
- `NotificationBehavior` (`Notifications.types.d.ts:611-623`) **requires** `shouldShowBanner` and `shouldShowList`. `shouldShowAlert` is `@deprecated` and optional.
- `removeNotificationSubscription` is **not exported at all** — that unmount cleanup **throws today**. The replacement is `subscription.remove()`. `Subscription` is a deprecated alias for `EventSubscription`.

- [ ] **Step 1: Confirm the baseline is exactly the two known errors**

Run: `cd apps/mobile-admin && npx tsc --noEmit --pretty false 2>&1 | grep "error TS"`

Expected — exactly these two, and nothing else:
```
app/(tabs)/_layout.tsx(11,35): error TS2322: Type 'Promise<{ shouldShowAlert: true; ... }>' is not assignable to type 'Promise<NotificationBehavior>'.
app/(tabs)/_layout.tsx(39,23): error TS2339: Property 'removeNotificationSubscription' does not exist on type 'typeof import(".../expo-notifications/build/index")'.
```

If anything else appears, an earlier task regressed — fix that first.

- [ ] **Step 2: Fix the notification handler**

In `apps/mobile-admin/app/(tabs)/_layout.tsx`, replace the `setNotificationHandler` block (lines 10-16):

```tsx
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    // shouldShowAlert is deprecated in expo-notifications 56; the banner/list
    // pair replaces it and both are required by NotificationBehavior.
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: true,
  }),
});
```

- [ ] **Step 3: Fix the subscription cleanup**

In the same file, change the ref type (line 20) and the cleanup (lines 36-40).

```tsx
  const responseListener = useRef<Notifications.EventSubscription | undefined>(undefined);
```

```tsx
    return () => {
      // expo-notifications 56 removed Notifications.removeNotificationSubscription;
      // calling it threw. Subscriptions remove themselves.
      responseListener.current?.remove();
    };
```

- [ ] **Step 4: Verify tsc is now clean**

Run: `cd apps/mobile-admin && npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"`
Expected: **0**.

If `Notifications.EventSubscription` is not exported under that name, use the `EventSubscription` type re-exported from `expo-notifications` (`Notifications.types.d.ts:742` re-exports it from `expo-modules-core`). Check with:
`grep -rn "EventSubscription" node_modules/expo-notifications/build/index.d.ts`

- [ ] **Step 5: Verify the app still boots**

Run: `cd apps/mobile-admin && npx jest 2>&1 | tail -8`
Expected: all green, **≥ 171 tests** (Task 6 adds none).

- [ ] **Step 6: Wire mobile into CI**

In `.github/workflows/ci.yml`, line 108, remove the two mobile filters. The line is currently:

```yaml
      - run: npx turbo run lint check-types build --filter='!@mark8ly/platform-api' --filter='!@mark8ly/auth-bff' --filter='!@mark8ly/otto' --filter='!@repo/storefront-mobile' --filter='!@repo/mobile-admin' --filter='!@repo/mobile-storefront' --filter='!@repo/mobile-shared'
```

Change it to:

```yaml
      - run: npx turbo run lint check-types build --filter='!@mark8ly/platform-api' --filter='!@mark8ly/auth-bff' --filter='!@mark8ly/otto' --filter='!@repo/storefront-mobile' --filter='!@repo/mobile-storefront'
```

`@repo/mobile-admin` declares `check-types: tsc --noEmit`, `lint: eslint .` and `build: echo 'use eas build'` — all three are safe. `@repo/mobile-shared` declares `check-types` and has no `build`/`lint`, which turbo skips silently.

- [ ] **Step 7: Add the test job**

`turbo.json` already defines a `test` task. Add a step immediately after the turbo line from Step 6, in the same job:

```yaml
      - run: npx turbo run test --filter='@repo/mobile-admin'
```

Scoped to `@repo/mobile-admin` deliberately: `@repo/mobile-shared` declares `test: vitest run`, but vitest is not installed in this tree — running it would fail CI on a dependency that does not exist. Widening this is a follow-up.

- [ ] **Step 8: Verify the CI commands locally**

Run, from the repo root:
```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
npx turbo run check-types --filter='@repo/mobile-admin' --filter='@repo/mobile-shared'
```
Expected: both succeed.

**Do NOT run `npx turbo run build`** — mobile-admin's build is a harmless echo, but turbo may try to build dependencies. If `check-types` passes for both packages, that is sufficient local evidence.

Run: `npx turbo run test --filter='@repo/mobile-admin'`
Expected: jest runs and passes.

- [ ] **Step 9: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add "apps/mobile-admin/app/(tabs)/_layout.tsx" .github/workflows/ci.yml
git commit -m "fix(mobile-admin): repair expo-notifications 56 API misuse and gate mobile in CI"
```

---

## Final verification (not a task — do this before calling the work done)

Gates are necessary and **not sufficient**. The entire point of this plan is that green gates hid 31 mismatches for two months.

- [ ] **The 161 products must be SEEN rendering on the simulator**, with real metro and a real build. Metro must already be running in real mode on :8081 (`npx expo start --dev-client --port 8081`, **no demo flag** — a stale demo metro silently serves the demo bundle).
- [ ] `xcrun simctl io AD109A46-2F99-43C3-8AAA-FEE68DC8499E screenshot out.png` to observe. **You CANNOT tap programmatically** — `idb` is not installed and AppleScript is blocked (`osascript is not allowed assistive access -1719`). **The human does the taps; screenshot around them.**
- [ ] Watch the metro log for `[api] contract mismatch on <path>: <field>` — that message is the debugger. A clean log plus rendered products is the pass condition.
- [ ] Expected on screen: **Products tab shows a populated catalog** (161 total, 20 per page), Customers shows **1**, **Orders is empty — that is correct, not a failure** (the store genuinely has zero orders).
- [ ] Then run the **opus whole-branch review**. Do not skip it: it found the Important that all 6 per-task reviews missed on the previous sub-project, and the one before that. **Verify its claims yourself** — one reviewer produced a false positive and another mis-cited line numbers.
- [ ] **Only after simulator verification**, do the deferred lockfile fix: stop metro, run a plain `npm install`, verify the diff touches only the zod entries, commit. It is deferred to last precisely because metro must stay up until the products are seen.

## Follow-ups this plan deliberately does not do

Recorded so they are not lost:
- **Order detail sub-project** — 6 of 12 fields are fiction; needs a seeded order to verify.
- **Backend (E):** customer `recent_orders` + `average_order_value`; real `low_stock` filtering; orders multi-status filter.
- **Product media** — the 3-step signed-URL upload flow (`upload-url` → PUT → `POST media`).
- Customer `addresses` rendering (the data is already on the wire and free).
- **Customer money still renders as USD.** `customers/[id].tsx` hardcodes `currency: "USD"` and the customer wire shape carries no currency field (`customers_dto.go:21-38`) — the only source is the active store's `currency_code`, which would need threading into the screen. Task 4 fixes this for products (where each variant carries `currency_code`); customers need the store-context wiring first.
- Notification deep-linking (needs real notifications to map `resource_type` against).
- `@repo/mobile-shared`'s `test: vitest run` — vitest is not installed.
- `extra.eas.projectId` is still `'your-eas-project-id'`, blocking `eas build`.
