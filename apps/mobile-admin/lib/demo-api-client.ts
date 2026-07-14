import type { createApiClient } from "@repo/mobile-shared/api/client";
import type {
  Customer,
  CustomerDetail,
  DashboardResponse,
  Order,
  OrderDetail,
  PaginatedResponse,
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
};

function iso(daysAgo: number): string {
  // Fixed base date (Math.random/Date.now avoided elsewhere in the repo for
  // determinism; here a stable string keeps demo snapshots reproducible).
  const base = new Date("2026-07-14T09:00:00Z").getTime();
  return new Date(base - daysAgo * 86_400_000).toISOString();
}

const DEMO_ORDERS: Order[] = [
  { id: "o-1001", order_number: "1001", status: "pending", customer_email: "maya@example.com", customer_name: "Maya Chen", grand_total: 8400, item_count: 2, created_at: iso(0), updated_at: iso(0) },
  { id: "o-1000", order_number: "1000", status: "fulfilled", customer_email: "leo@example.com", customer_name: "Leo Park", grand_total: 12900, item_count: 3, created_at: iso(1), updated_at: iso(1) },
  { id: "o-0999", order_number: "0999", status: "cancelled", customer_email: "ida@example.com", customer_name: "Ida Rossi", grand_total: 5200, item_count: 1, created_at: iso(3), updated_at: iso(2) },
];

const DEMO_PRODUCTS: Product[] = [
  { id: "p-1", name: "Linen Camp Shirt", status: "active", price: 8900, compare_at_price: 11900, sku: "LCS-001", stock: 42, thumbnail_url: null, created_at: iso(20) },
  { id: "p-2", name: "Merino Beanie", status: "active", price: 3900, compare_at_price: null, sku: "MB-014", stock: 7, thumbnail_url: null, created_at: iso(15) },
  { id: "p-3", name: "Canvas Weekender", status: "draft", price: 14900, compare_at_price: null, sku: "CW-220", stock: 0, thumbnail_url: null, created_at: iso(9) },
];

const DEMO_CUSTOMERS: Customer[] = [
  { id: "c-1", email: "maya@example.com", first_name: "Maya", last_name: "Chen", phone: "+61 400 111 222", order_count: 6, total_spent: 48200, status: "active", created_at: iso(120) },
  { id: "c-2", email: "leo@example.com", first_name: "Leo", last_name: "Park", phone: null, order_count: 2, total_spent: 15800, status: "active", created_at: iso(40) },
  { id: "c-3", email: "ida@example.com", first_name: "Ida", last_name: "Rossi", phone: "+61 400 333 444", order_count: 1, total_spent: 5200, status: "active", created_at: iso(12) },
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
    { id: "p-1", name: "Linen Camp Shirt", total_sold: 86, revenue: 765400 },
    { id: "p-2", name: "Merino Beanie", total_sold: 54, revenue: 210600 },
  ],
  low_stock: [{ id: "p-2", name: "Merino Beanie", stock: 7, thumbnail_url: null }],
  setup_checklist: {
    has_products: true,
    has_payment: true,
    has_shipping: true,
    has_domain: false,
    has_branding: true,
  },
};

function page<T>(items: T[]): PaginatedResponse<T> {
  return { items, total: items.length, next_cursor: null, has_more: false };
}

function orderDetail(id: string): OrderDetail {
  const base = DEMO_ORDERS.find((o) => o.id === id) ?? DEMO_ORDERS[0]!;
  return {
    ...base,
    line_items: [
      { id: "li-1", product_id: "p-1", product_name: "Linen Camp Shirt", variant_name: "M", quantity: 1, unit_price: 8900, thumbnail_url: null },
    ],
    shipping_address: { line1: "12 Campbell Pde", line2: null, city: "Bondi Beach", state: "NSW", postal_code: "2026", country: "AU" },
    shipping_method: "Standard",
    tracking_number: null,
    payment_method: "card",
    payment_transaction_id: "demo_txn_001",
    timeline: [{ type: "created", message: "Order placed", created_at: base.created_at }],
  };
}

function productDetail(id: string): ProductDetail {
  const base = DEMO_PRODUCTS.find((p) => p.id === id) ?? DEMO_PRODUCTS[0]!;
  return {
    ...base,
    description: "A demo product. Real data appears once real GIP auth is wired.",
    barcode: null,
    category_id: null,
    category_name: "Apparel",
    tags: ["demo"],
    media: [],
    variants: [],
  };
}

function customerDetail(id: string): CustomerDetail {
  const base = DEMO_CUSTOMERS.find((c) => c.id === id) ?? DEMO_CUSTOMERS[0]!;
  return {
    ...base,
    avatar_url: null,
    average_order_value: base.order_count ? Math.round(base.total_spent / base.order_count) : 0,
    recent_orders: DEMO_DASHBOARD.recent_orders.slice(0, 2),
    review_count: 0,
  };
}

// Match the raw path the hooks pass (e.g. "/dashboard", "/orders", "/orders/o-1001").
function resolve(path: string): unknown {
  const clean = path.split("?")[0]!.replace(/\/+$/, "");
  if (clean === "/stores") return [DEMO_STORE];
  if (clean === "/dashboard") return DEMO_DASHBOARD;

  const orderId = clean.match(/^\/orders\/(.+)$/);
  if (orderId) return orderDetail(orderId[1]!);
  if (clean === "/orders") return page(DEMO_ORDERS);

  const productId = clean.match(/^\/products\/(.+)$/);
  if (productId) return productDetail(productId[1]!);
  if (clean === "/products") return page(DEMO_PRODUCTS);

  const customerId = clean.match(/^\/customers\/(.+)$/);
  if (customerId) return customerDetail(customerId[1]!);
  if (clean === "/customers") return page(DEMO_CUSTOMERS);

  // Notifications, and any endpoint we haven't canned: safe empty page so
  // list screens render an empty state instead of crashing.
  return page([]);
}

export function createDemoApiClient(): ApiClient {
  return {
    get: (async (path: string) => resolve(path)) as ApiClient["get"],
    getTenant: (async (path: string) => resolve(path)) as ApiClient["getTenant"],
    // Mutations succeed no-op: echo the body so optimistic UI has something.
    post: (async (_path: string, body?: unknown) => body ?? { success: true }) as ApiClient["post"],
    patch: (async (_path: string, body?: unknown) => body ?? { success: true }) as ApiClient["patch"],
    delete: (async () => ({ success: true })) as ApiClient["delete"],
    uploadMedia: (async () => ({ id: "demo-media", url: "" })) as ApiClient["uploadMedia"],
  };
}
