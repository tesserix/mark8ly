import type { z } from "zod";
import { ApiError, type createApiClient } from "@repo/mobile-shared/api/client";
import type {
  Customer,
  CustomerDetail,
  DashboardResponse,
  Notification,
  Order,
  OrderDetail,
  Product,
  ProductDetail,
  Store,
} from "@repo/mobile-shared/api/types";

type ApiClient = ReturnType<typeof createApiClient>;

/**
 * In-memory API used only when EXPO_PUBLIC_AUTH_BACKEND=demo. It mirrors the
 * real api-client surface but serves canned data and never performs network
 * I/O — so a demo/simulator build (which has no GoogleService-Info.plist and
 * thus no real GIP token) can navigate the app without the real API 401-ing
 * and bouncing the user back to /login. Swapped in by useApiClient().
 */

const DEMO_STORE: Store = {
  id: "demo-store",
  name: "Bondi Beach Co.",
  slug: "bondi",
  country_code: "AU",
  currency_code: "AUD",
  status: "active",
};

function iso(daysAgo: number): string {
  // Fixed base date (Math.random/Date.now avoided elsewhere in the repo for
  // determinism; here a stable string keeps demo snapshots reproducible).
  const base = new Date("2026-07-14T09:00:00Z").getTime();
  return new Date(base - daysAgo * 86_400_000).toISOString();
}

const DEMO_ORDERS: Order[] = [
  { id: "o-1001", tenant_id: "demo-tenant", store_id: "demo-store", order_number: "1001", idempotency_key: "idem-1001", status: "pending", payment_status: "paid", fulfillment_status: "unfulfilled", customer_email: "maya@example.com", customer_name: "Maya Chen", subtotal: 8400, shipping_total: 0, tax_total: 0, discount_total: 0, grand_total: 8400, refunded_amount: 0, currency_code: "AUD", items: [], addresses: [], placed_at: iso(0), created_at: iso(0), updated_at: iso(0) },
  { id: "o-1000", tenant_id: "demo-tenant", store_id: "demo-store", order_number: "1000", idempotency_key: "idem-1000", status: "fulfilled", payment_status: "paid", fulfillment_status: "fulfilled", customer_email: "leo@example.com", customer_name: "Leo Park", subtotal: 12900, shipping_total: 0, tax_total: 0, discount_total: 0, grand_total: 12900, refunded_amount: 0, currency_code: "AUD", items: [], addresses: [], placed_at: iso(1), created_at: iso(1), updated_at: iso(1) },
  // No customer_name — mirrors the omitempty case.
  { id: "o-0999", tenant_id: "demo-tenant", store_id: "demo-store", order_number: "0999", idempotency_key: "idem-0999", status: "cancelled", payment_status: "refunded", fulfillment_status: "unfulfilled", customer_email: "ida@example.com", subtotal: 5200, shipping_total: 0, tax_total: 0, discount_total: 0, grand_total: 5200, refunded_amount: 5200, currency_code: "AUD", items: [], addresses: [], placed_at: iso(3), created_at: iso(3), updated_at: iso(2) },
];

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

const DEMO_CUSTOMERS: Customer[] = [
  { id: "c-1", email: "maya@example.com", first_name: "Maya", last_name: "Chen", phone: "+61 400 111 222", tags: [], status: "active", marketing_opt_in: true, order_count: 6, total_spent: 48200, created_at: iso(120), updated_at: iso(2) },
  { id: "c-2", email: "leo@example.com", first_name: "Leo", last_name: "Park", tags: [], status: "active", marketing_opt_in: false, order_count: 2, total_spent: 15800, created_at: iso(40), updated_at: iso(5) },
  // No names at all — mirrors the ONLY real customer in prod, and is the case
  // that would have caught this whole class two months ago.
  { id: "c-3", email: "ida@example.com", tags: [], status: "active", marketing_opt_in: false, order_count: 0, total_spent: 0, created_at: iso(12), updated_at: iso(12) },
];

const DEMO_DASHBOARD: DashboardResponse = {
  stats: {
    revenue_today: 21300,
    revenue_week: 148900,
    revenue_month: 612400,
    revenue_change_pct: 12.4,
    revenue_trend: [42, 55, 48, 61, 58, 73, 69],
    orders_today: 4,
    orders_pending: 3,
    orders_fulfilled: 128,
    orders_cancelled: 6,
    customers_total: 214,
    customers_new_this_week: 11,
    pending_reviews: 2,
  },
  recent_orders: DEMO_ORDERS.map((o) => ({
    id: o.id,
    order_number: o.order_number,
    customer_email: o.customer_email,
    grand_total: o.grand_total,
    status: o.status,
    created_at: o.created_at,
  })),
  top_products: [
    { id: "p-1", title: "Linen Camp Shirt", units_sold: 86, revenue: 765400, image_url: null },
    { id: "p-2", title: "Merino Beanie", units_sold: 54, revenue: 210600, image_url: null },
  ],
  low_stock: [
    {
      id: "p-2",
      title: "Merino Beanie",
      variant_title: "One Size",
      quantity: 7,
      low_stock_threshold: 10,
    },
  ],
  setup_checklist: {
    has_store: true,
    has_brand_assets: true,
    has_product: true,
    has_storefront_theme: true,
    has_payment_provider: true,
    has_shipping_carrier: true,
    has_return_policy: false,
    has_custom_domain: false,
  },
};

/** The real `{data, meta}` list envelope — see schema-helpers.paginated. */
function paged<T>(items: T[]): { data: T[]; meta: { page: number; page_size: number; total: number; total_pages: number } } {
  return { data: items, meta: { page: 1, page_size: 50, total: items.length, total_pages: 1 } };
}

const DEMO_NOTIFICATIONS: Notification[] = [
  { id: "n-1", type: "new_order", title: "New order #1001", message: "Maya Chen placed an order", resource_type: "order", resource_id: "o-1001", is_read: false, created_at: iso(0) },
  { id: "n-2", type: "low_stock", title: "Low stock: Merino Beanie", message: "7 remaining", is_read: true, created_at: iso(2) },
];

function orderDetail(id: string): OrderDetail {
  const base = DEMO_ORDERS.find((o) => o.id === id) ?? DEMO_ORDERS[0]!;
  return {
    ...base,
    items: [
      { id: "li-1", product_id: "p-1", variant_id: "v-1", title_snapshot: "Linen Camp Shirt", sku_snapshot: "LCS-M", option_summary: "Size: M", unit_price: 89, quantity: 1, line_total: 89, currency_code: "AUD" },
    ],
    addresses: [
      { kind: "shipping", name: "Maya Chen", line1: "12 Campbell Pde", city: "Bondi Beach", region: "NSW", postal_code: "2026", country_code: "AU" },
    ],
  };
}

/** Detail is the same shape as list — the endpoint returns the product object. */
function productDetail(id: string): ProductDetail {
  return DEMO_PRODUCTS.find((p) => p.id === id) ?? DEMO_PRODUCTS[0]!;
}

function customerDetail(id: string): CustomerDetail {
  const base = DEMO_CUSTOMERS.find((c) => c.id === id) ?? DEMO_CUSTOMERS[0]!;
  return { ...base, addresses: [] };
}

// Match the raw path the hooks pass (e.g. "/dashboard", "/orders", "/orders/o-1001").
function resolve(path: string): unknown {
  const clean = path.split("?")[0]!.replace(/\/+$/, "");
  // Must mirror the real API's wire shape: /stores returns { data } with no
  // meta (unlike the paginated list endpoints below, which use paged()).
  // Do not "simplify" this back to a bare array — see storesResponseSchema
  // in use-store.ts, which reads res.data and would break on undefined.
  if (clean === "/stores") return { data: [DEMO_STORE] };
  if (clean === "/dashboard") return DEMO_DASHBOARD;

  const orderId = clean.match(/^\/orders\/(.+)$/);
  if (orderId) return orderDetail(orderId[1]!);
  if (clean === "/orders") return paged(DEMO_ORDERS);

  const productId = clean.match(/^\/products\/(.+)$/);
  if (productId) return productDetail(productId[1]!);
  if (clean === "/products") return paged(DEMO_PRODUCTS);

  const customerId = clean.match(/^\/customers\/(.+)$/);
  if (customerId) return customerDetail(customerId[1]!);
  if (clean === "/customers") return paged(DEMO_CUSTOMERS);

  if (clean === "/notifications") {
    return { notifications: DEMO_NOTIFICATIONS, page: 1, per_page: 20, total: DEMO_NOTIFICATIONS.length };
  }

  if (clean === "/notification-preferences") {
    return {
      store_id: DEMO_STORE.id,
      preferences: {
        new_order: true,
        low_stock: true,
        return_requested: true,
        payment_received: true,
        review_submitted: false,
      },
    };
  }

  // Any endpoint we have not canned. `paged` mirrors the real {data, meta}
  // envelope that every list endpoint except /notifications uses, so an
  // un-canned list renders an empty state instead of failing validation.
  return paged([]);
}

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

/**
 * Product create/update pass `productDetailSchema` to validate the response
 * (see products.ts `create`/`update`). A `CreateProductBody`/`UpdateProductBody`
 * request — no `id`, `store_id`, `handle`, `media`, `created_at`, etc. — can
 * never satisfy that schema, so echoing the request body back (as the
 * generic mutation path below does) would make every demo product Save/Create
 * throw `contract_mismatch` on a well-formed request. Serve a real product
 * fixture instead: PATCH resolves the product being edited (falling back to
 * the first demo product), POST always resolves the first one. Other mutation
 * paths have no response schema today, so the generic echo is still correct
 * for them.
 */
function productMutationResult(path: string): Product {
  const productId = path.match(/^\/products\/([^/]+)$/);
  if (productId) return productDetail(productId[1]!);
  return DEMO_PRODUCTS[0]!;
}

export function createDemoApiClient(): ApiClient {
  return {
    get: async <T>(path: string, _params?: Record<string, string>, schema?: z.ZodType<T>) =>
      parseOrThrow(path, resolve(path), schema),
    getTenant: async <T>(path: string, _params?: Record<string, string>, schema?: z.ZodType<T>) =>
      parseOrThrow(path, resolve(path), schema),
    // Mutations succeed no-op: echo the body so optimistic UI has something.
    // A schema, if passed, is still applied — a demo mutation whose echo does
    // not match its response schema SHOULD fail loudly rather than lie.
    post: async <T>(path: string, body?: unknown, schema?: z.ZodType<T>) => {
      const clean = path.split("?")[0]!.replace(/\/+$/, "");
      if (clean === "/products") return parseOrThrow(path, productMutationResult(clean), schema);
      return parseOrThrow(path, body ?? { success: true }, schema);
    },
    patch: async <T>(path: string, body?: unknown, schema?: z.ZodType<T>) => {
      const clean = path.split("?")[0]!.replace(/\/+$/, "");
      if (/^\/products\/[^/]+$/.test(clean)) {
        return parseOrThrow(path, productMutationResult(clean), schema);
      }
      return parseOrThrow(path, body ?? { success: true }, schema);
    },
    put: async <T>(path: string, body?: unknown, schema?: z.ZodType<T>) =>
      parseOrThrow(path, body ?? { success: true }, schema),
    delete: async <T>() => ({ success: true } as T),
    uploadMedia: async () => ({ id: "demo-media", url: "" }),
  };
}
