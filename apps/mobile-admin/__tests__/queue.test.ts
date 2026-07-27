import { buildQueue, type QueueSources } from "@/lib/queue";
import { formatMoney } from "@/lib/money";
import type { RecentOrder, LowStockItem, Review, Ticket } from "@repo/mobile-shared/api/types";

function order(over: Partial<RecentOrder> = {}): RecentOrder {
  return {
    id: "o-1",
    order_number: "1001",
    customer_email: "buyer@example.com",
    grand_total: 42,
    status: "pending",
    created_at: "2026-07-20T00:00:00Z",
    ...over,
  };
}

function stockItem(over: Partial<LowStockItem> = {}): LowStockItem {
  return {
    id: "variant-1",
    product_id: "product-1",
    title: "Linen Shirt",
    variant_title: "Small",
    quantity: 2,
    low_stock_threshold: 5,
    ...over,
  };
}

function ticket(over: Partial<Ticket> = {}): Ticket {
  return {
    id: "t-1",
    ticket_number: "T-100",
    subject: "Where is my order",
    description: "…",
    status: "open",
    priority: "normal",
    submitted_by_name: "Jamie Lee",
    submitted_by_email: "jamie@example.com",
    created_at: "2026-07-20T00:00:00Z",
    updated_at: "2026-07-20T00:00:00Z",
    replies: [],
    ...over,
  };
}

function review(over: Partial<Review> = {}): Review {
  return {
    id: "r-1",
    product_id: "product-1",
    customer_name: "Sam Rivera",
    customer_email: "sam@example.com",
    rating: 4,
    content: "Great fit, ran a little small.",
    status: "pending",
    verified_purchase: true,
    featured: false,
    helpful_count: 0,
    not_helpful_count: 0,
    created_at: "2026-07-20T00:00:00Z",
    updated_at: "2026-07-20T00:00:00Z",
    media: [],
    replies: [],
    ...over,
  };
}

const EMPTY: QueueSources = {
  stats: {
    revenue_today: 0,
    revenue_week: 0,
    revenue_month: 0,
    revenue_change_pct: 0,
    revenue_trend: [],
    orders_today: 0,
    orders_pending: 0,
    orders_fulfilled: 0,
    orders_cancelled: 0,
    customers_total: 0,
    customers_new_this_week: 0,
    pending_reviews: 0,
  },
  recentOrders: [],
  lowStock: [],
  tickets: [],
  reviews: [],
};

describe("buildQueue — empty input", () => {
  it("returns an empty array when every source is empty", () => {
    expect(buildQueue(EMPTY)).toEqual([]);
  });
});

describe("buildQueue — ordering", () => {
  it("groups by urgency then recency: orders, then stock, then tickets, then reviews", () => {
    const items = buildQueue({
      ...EMPTY,
      recentOrders: [order()],
      lowStock: [stockItem()],
      tickets: [ticket()],
      reviews: [review()],
    });

    expect(items.map((i) => i.type)).toEqual(["order", "stock", "ticket", "review"]);
  });

  it("sorts orders within the group newest-first", () => {
    const older = order({ id: "o-old", created_at: "2026-07-01T00:00:00Z" });
    const newer = order({ id: "o-new", created_at: "2026-07-25T00:00:00Z" });

    const items = buildQueue({ ...EMPTY, recentOrders: [older, newer] });

    expect(items.map((i) => i.id)).toEqual(["o-new", "o-old"]);
  });
});

describe("buildQueue — filters to the urgent slice of each source", () => {
  it("excludes non-pending orders — this is a 'needs you' queue, not a full recent-orders feed", () => {
    const items = buildQueue({
      ...EMPTY,
      recentOrders: [order({ id: "confirmed", status: "confirmed" }), order({ id: "pending" })],
    });

    expect(items.map((i) => i.id)).toEqual(["pending"]);
  });

  it("excludes resolved/closed tickets", () => {
    const items = buildQueue({
      ...EMPTY,
      tickets: [
        ticket({ id: "closed", status: "closed" }),
        ticket({ id: "open" }),
      ],
    });

    expect(items.map((i) => i.id)).toEqual(["open"]);
  });

  it("excludes non-pending reviews", () => {
    const items = buildQueue({
      ...EMPTY,
      reviews: [
        review({ id: "approved", status: "approved" }),
        review({ id: "pending" }),
      ],
    });

    expect(items.map((i) => i.id)).toEqual(["pending"]);
  });
});

describe("buildQueue — per-type cap and 'See all'", () => {
  it("caps each type at 3 and appends a 'See all' row only when that type overflows", () => {
    const orders = [order({ id: "o1" }), order({ id: "o2" }), order({ id: "o3" })];
    const items = buildQueue({
      ...EMPTY,
      recentOrders: orders,
      stats: { ...EMPTY.stats, orders_pending: 3 },
    });

    // Exactly 3 orders, no overflow, so no "See all" row.
    expect(items).toHaveLength(3);
    expect(items.every((i) => i.type === "order")).toBe(true);
  });

  it("appends one 'See all N' row per overflowing type, using the authoritative stats count", () => {
    const orders = [
      order({ id: "o1" }),
      order({ id: "o2" }),
      order({ id: "o3" }),
      order({ id: "o4" }),
    ];
    const items = buildQueue({
      ...EMPTY,
      recentOrders: orders,
      stats: { ...EMPTY.stats, orders_pending: 9 },
    });

    expect(items).toHaveLength(4);
    const seeAll = items[3];
    expect(seeAll.type).toBe("order");
    expect(seeAll.primary).toContain("9");
    // The overflow row carries no per-item status — it's a navigational
    // affordance, not a queue entry.
    expect(seeAll.badgeTone).toBeUndefined();
  });

  it("renders 'See all' with no number for a type with no authoritative stats count (stock)", () => {
    const items = buildQueue({
      ...EMPTY,
      lowStock: [
        stockItem({ id: "v1" }),
        stockItem({ id: "v2" }),
        stockItem({ id: "v3" }),
        stockItem({ id: "v4" }),
      ],
    });

    const seeAll = items[3];
    expect(seeAll.type).toBe("stock");
    expect(seeAll.primary.toLowerCase()).toContain("see all");
    expect(/\d/.test(seeAll.primary)).toBe(false);
  });

  it("renders 'See all' with no number for a type with no authoritative stats count (tickets)", () => {
    const items = buildQueue({
      ...EMPTY,
      tickets: [
        ticket({ id: "t1" }),
        ticket({ id: "t2" }),
        ticket({ id: "t3" }),
        ticket({ id: "t4" }),
      ],
    });

    const seeAll = items[3];
    expect(seeAll.type).toBe("ticket");
    expect(/\d/.test(seeAll.primary)).toBe(false);
  });
});

describe("buildQueue — total cap", () => {
  it("caps the total list at 12, dropping the lowest-priority rows first", () => {
    const items = buildQueue({
      stats: { ...EMPTY.stats, orders_pending: 4, pending_reviews: 4 },
      recentOrders: [order({ id: "o1" }), order({ id: "o2" }), order({ id: "o3" }), order({ id: "o4" })],
      lowStock: [stockItem({ id: "v1" }), stockItem({ id: "v2" }), stockItem({ id: "v3" }), stockItem({ id: "v4" })],
      tickets: [ticket({ id: "t1" }), ticket({ id: "t2" }), ticket({ id: "t3" }), ticket({ id: "t4" })],
      reviews: [review({ id: "r1" }), review({ id: "r2" }), review({ id: "r3" }), review({ id: "r4" })],
    });

    // 4 rows per overflowing type (3 items + "See all") x 3 types = 12,
    // already at the cap before reviews are even considered.
    expect(items).toHaveLength(12);
    expect(items.some((i) => i.type === "review")).toBe(false);
  });
});

describe("buildQueue — order field mapping", () => {
  it("falls back to customer_email when customer_name is absent", () => {
    const items = buildQueue({
      ...EMPTY,
      recentOrders: [order({ customer_name: undefined, customer_email: "no-name@example.com" })],
    });

    expect(items[0].primary).toBe("no-name@example.com");
  });

  it("prefers customer_name when present", () => {
    const items = buildQueue({
      ...EMPTY,
      recentOrders: [order({ customer_name: "Priya Shah", customer_email: "priya@example.com" })],
    });

    expect(items[0].primary).toBe("Priya Shah");
  });

  it("leaves imageUrl undefined (monogram fallback) when the order has no image_url", () => {
    const items = buildQueue({
      ...EMPTY,
      recentOrders: [order({ image_url: undefined })],
    });

    expect(items[0].imageUrl).toBeUndefined();
  });

  it("carries image_url through when present", () => {
    const items = buildQueue({
      ...EMPTY,
      recentOrders: [order({ image_url: "https://cdn.example/p.jpg" })],
    });

    expect(items[0].imageUrl).toBe("https://cdn.example/p.jpg");
  });

  it("formats the amount in the store's currency", () => {
    const items = buildQueue({
      ...EMPTY,
      recentOrders: [order({ grand_total: 99.5 })],
      currencyCode: "AUD",
    });

    expect(items[0].amount).toBe(formatMoney(99.5, "AUD"));
  });

  it("routes to the order detail screen", () => {
    const items = buildQueue({ ...EMPTY, recentOrders: [order({ id: "o-42" })] });
    expect(items[0].onPressRoute).toBe("/(tabs)/orders/o-42");
  });
});

describe("buildQueue — low stock field mapping", () => {
  it("routes to the product detail screen when product_id is present", () => {
    const items = buildQueue({
      ...EMPTY,
      lowStock: [stockItem({ product_id: "prod-9" })],
    });
    expect(items[0].onPressRoute).toBe("/(tabs)/products/prod-9");
  });

  it("routes to the products LIST (never an interpolated undefined) when product_id is absent", () => {
    const items = buildQueue({
      ...EMPTY,
      lowStock: [stockItem({ product_id: undefined })],
    });
    expect(items[0].onPressRoute).toBe("/(tabs)/products");
    expect(items[0].onPressRoute).not.toContain("undefined");
  });
});

describe("buildQueue — badge tone never spends the app's moss accent", () => {
  it("never emits a success tone — moss is already spent on the chart and the Approve swipe", () => {
    const items = buildQueue({
      ...EMPTY,
      recentOrders: [order()],
      lowStock: [stockItem()],
      tickets: [ticket()],
      reviews: [review()],
    });

    expect(items.map((i) => i.badgeTone)).not.toContain("success");
  });
});
