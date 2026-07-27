import { buildQueue, type QueueItem, type QueueSources } from "@/lib/queue";
import { formatMoney } from "@/lib/money";
import type { RecentOrder, LowStockItem, Review, Ticket } from "@repo/mobile-shared/api/types";

/**
 * Narrows a `QueueItem` to the `"item"` variant for tests that assert on
 * fields (`amount`, `imageUrl`) that only exist on that branch of the
 * discriminated union — see lib/queue.ts's `QueueItem` doc.
 */
function asItem(item: QueueItem): Extract<QueueItem, { kind: "item" }> {
  if (item.kind !== "item") {
    throw new Error(`expected an "item" row, got kind "${item.kind}"`);
  }
  return item;
}

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
  // NOTE: this test is named for what it actually proves — that hitting
  // TYPE_CAP exactly does NOT overflow. It is NOT a cap-value test: it
  // feeds exactly 3 orders with `orders_pending: 3`, which passes unchanged
  // under any TYPE_CAP >= 3 (verified by mutating TYPE_CAP to 2 and to 4 —
  // both left this test green; see inc2-task-7-report.md "Fix round 1").
  // The cap value itself is exercised by the overflow tests below, which
  // DO fail under those same mutations.
  it("shows no 'See all' row when a type's count does not exceed the cap", () => {
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
    // affordance, not a queue entry. `kind` (not `badgeTone`'s presence) is
    // the discriminator now — see queue-row.test.tsx and the QueueItem doc.
    expect(seeAll.kind).toBe("seeAll");
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

describe("buildQueue — overflow is driven by the authoritative count, not the local array length", () => {
  // Reproduces the reviewer's finding: `recent_orders` is the API's last-5-
  // orders-of-ANY-status feed (services/marketplace-api dashboard handler),
  // not a pending-orders feed. `buildQueue` filters it to pending, so
  // `pendingOrders.length` can be far smaller than `stats.orders_pending` —
  // a busy store can have 20 pending orders and only 2 of them land in the
  // last-5 feed. Before this fix, overflow (and therefore the "See all"
  // row) was decided by `pendingOrders.length`, so those 2 rows rendered
  // with NO way to reach the other 18.
  it("appends a 'See all' row driven by stats.orders_pending even when only 2 of 20 pending orders are in the local slice", () => {
    const items = buildQueue({
      ...EMPTY,
      // Only 2 of the "last 5 orders of any status" happen to be pending —
      // the other 3 are fulfilled/cancelled and never reach buildQueue
      // (the Dashboard screen only passes recentOrders through; buildQueue
      // itself re-filters to status === "pending").
      recentOrders: [order({ id: "p1" }), order({ id: "p2" })],
      stats: { ...EMPTY.stats, orders_pending: 20 },
    });

    expect(items.map((i) => i.id)).toEqual(["p1", "p2", "see-all-order"]);
    const seeAll = items[2];
    expect(seeAll.kind).toBe("seeAll");
    expect(seeAll.primary).toContain("20");
  });

  // The degenerate case: ALL 5 most-recent orders are already
  // fulfilled/cancelled, so the local pending-orders slice is empty even
  // though 20 orders are genuinely pending. Before this fix, an empty
  // local array meant an empty "money waiting" section — no rows, no "See
  // all" row, nothing telling the merchant 20 orders need them.
  it("appends a 'See all' row even when the local pending-orders slice is empty", () => {
    const items = buildQueue({
      ...EMPTY,
      recentOrders: [
        order({ id: "f1", status: "fulfilled" }),
        order({ id: "f2", status: "fulfilled" }),
        order({ id: "c1", status: "cancelled" }),
      ],
      stats: { ...EMPTY.stats, orders_pending: 20 },
    });

    expect(items).toHaveLength(1);
    expect(items[0].kind).toBe("seeAll");
    expect(items[0].type).toBe("order");
    expect(items[0].primary).toContain("20");
  });

  // Same defect, same fix, on the other type with an authority
  // (`stats.pending_reviews`) — guards against a fix that only special-cased
  // orders.
  it("appends a 'See all' row driven by stats.pending_reviews even when the local reviews slice is empty", () => {
    const items = buildQueue({
      ...EMPTY,
      reviews: [],
      stats: { ...EMPTY.stats, pending_reviews: 15 },
    });

    expect(items).toHaveLength(1);
    expect(items[0].kind).toBe("seeAll");
    expect(items[0].type).toBe("review");
    expect(items[0].primary).toContain("15");
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

  // Reproduces the reviewer's finding: a plain `.slice(0, TOTAL_CAP)` cuts
  // whichever row happens to land on the boundary — including a group's own
  // "See all" row, which is exactly the affordance the merchant needs when
  // that group is truncated. orders(4 rows: 3 + See all) + stock(4 rows: 3 +
  // See all) + tickets(2 rows, no overflow) = 10, leaving a budget of 2 for
  // the reviews group ([r1, r2, r3, "See all 40 pending reviews"], 4 rows).
  // A plain slice would keep [r1, r2] and drop the "See all" row entirely —
  // the merchant would see 2 of 40 pending reviews with no way to reach the
  // other 38.
  it("keeps a group's own 'See all' row (in place of the last surviving item) instead of dropping it when the total cap cuts that group mid-way", () => {
    const items = buildQueue({
      stats: { ...EMPTY.stats, orders_pending: 4, pending_reviews: 40 },
      recentOrders: [
        order({ id: "o1" }),
        order({ id: "o2" }),
        order({ id: "o3" }),
        order({ id: "o4" }),
      ],
      lowStock: [
        stockItem({ id: "v1" }),
        stockItem({ id: "v2" }),
        stockItem({ id: "v3" }),
        stockItem({ id: "v4" }),
      ],
      tickets: [ticket({ id: "t1" }), ticket({ id: "t2" })],
      reviews: [review({ id: "r1" }), review({ id: "r2" }), review({ id: "r3" })],
    });

    expect(items).toHaveLength(12);
    expect(items.map((i) => i.id).slice(-2)).toEqual(["r1", "see-all-review"]);
    const last = items[items.length - 1];
    expect(last.kind).toBe("seeAll");
    expect(last.primary).toContain("40");
  });

  // A group with no overflow of its own has no "See all" row to preserve —
  // a mid-way cut on that group is a plain, harmless truncation (nothing
  // is silently destroyed; there was never an affordance to reach "the
  // rest", because there is no "rest" beyond what's already shown for a
  // non-overflowing group).
  it("plainly truncates a group with no 'See all' row of its own when the total cap cuts it mid-way", () => {
    const items = buildQueue({
      stats: { ...EMPTY.stats, orders_pending: 4, pending_reviews: 0 },
      recentOrders: [
        order({ id: "o1" }),
        order({ id: "o2" }),
        order({ id: "o3" }),
        order({ id: "o4" }),
      ],
      lowStock: [
        stockItem({ id: "v1" }),
        stockItem({ id: "v2" }),
        stockItem({ id: "v3" }),
        stockItem({ id: "v4" }),
      ],
      tickets: [ticket({ id: "t1" }), ticket({ id: "t2" }), ticket({ id: "t3" })],
      // No authoritative count (pending_reviews: 0) and only 2 items, so
      // this group never overflows and never grows a "See all" row of its
      // own to preserve.
      reviews: [review({ id: "r1" }), review({ id: "r2" })],
    });

    // orders(4 rows) + stock(4 rows) + tickets(3 rows) = 11, leaving a
    // budget of 1 for the 2-item reviews group — a plain truncation to the
    // first item, since there is no "See all" row to keep instead.
    expect(items).toHaveLength(12);
    expect(items[items.length - 1]).toMatchObject({ id: "r1", kind: "item" });
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

    expect(asItem(items[0]).imageUrl).toBeUndefined();
  });

  it("carries image_url through when present", () => {
    const items = buildQueue({
      ...EMPTY,
      recentOrders: [order({ image_url: "https://cdn.example/p.jpg" })],
    });

    expect(asItem(items[0]).imageUrl).toBe("https://cdn.example/p.jpg");
  });

  it("formats the amount in the store's currency", () => {
    const items = buildQueue({
      ...EMPTY,
      recentOrders: [order({ grand_total: 99.5 })],
      currencyCode: "AUD",
    });

    expect(asItem(items[0]).amount).toBe(formatMoney(99.5, "AUD"));
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

    const badgeTones = items.filter((i) => i.kind === "item").map((i) => i.badgeTone);
    expect(badgeTones).not.toContain("success");
  });
});

describe("QueueItem — 'seeAll' can't accidentally masquerade as a real row (compile-time guard)", () => {
  // Prior to this fix, `QueueItem` was one flat interface with `badgeTone`
  // optional even on real rows, and `QueueRow` used `badgeTone === undefined`
  // as the row-kind discriminator. That let a fully-populated real order —
  // amount, imageUrl, everything — type-check with `badgeTone` simply
  // forgotten, and it would silently render as a single-line "See all" link,
  // dropping the amount/photo/badge with no error anywhere.
  //
  // `QueueItem` is now a discriminated union on `kind`, with `badgeTone`
  // REQUIRED on the `"item"` variant, so that state is no longer
  // constructible — this is a type error, not a runtime check. `tsc --noEmit`
  // (Gate 2) is what actually enforces it; this test only pins the
  // expectation so a regression is visible in this file too.
  //
  // Mutation-verified: temporarily reverting `QueueItem` to the pre-fix flat
  // shape (`badgeTone?: StatusTone` on one non-discriminated interface) made
  // the object below type-check with no error, which turns the
  // `@ts-expect-error` below into an "Unused '@ts-expect-error' directive"
  // error under `tsc --noEmit` — RED. Restoring the discriminated union made
  // `tsc --noEmit` clean again — GREEN.
  it("does not allow a 'kind: item' row to omit badgeTone", () => {
    // @ts-expect-error — `badgeTone` is required whenever `kind: "item"`;
    // only the `"seeAll"` variant may omit it (and cannot carry `amount`/
    // `imageUrl` at all). If this stops erroring, the illegal state this
    // finding was about is representable again.
    const illegal: QueueItem = {
      kind: "item",
      id: "x",
      type: "order",
      primary: "Test Customer",
      secondary: "Order #1",
      amount: "$1.00",
      imageUrl: "https://example.com/x.jpg",
      onPressRoute: "/x",
    };

    expect(illegal).toBeDefined();
  });
});
