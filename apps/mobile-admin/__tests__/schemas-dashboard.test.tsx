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
