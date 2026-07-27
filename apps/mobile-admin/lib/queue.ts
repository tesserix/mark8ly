import type { StatusTone } from "@/components/ui";
import { formatMoney } from "@/lib/money";
import type {
  DashboardStats,
  RecentOrder,
  LowStockItem,
  Review,
  Ticket,
} from "@repo/mobile-shared/api/types";

export type QueueItemType = "order" | "review" | "stock" | "ticket";

/**
 * `QueueItem` per inc2-task-7-brief.md, with two deliberate deviations from
 * the brief's literal interface (recorded in inc2-task-7-report.md):
 *
 * 1. `badgeTone` is typed as `StatusTone` (components/ui/StatusBadge.tsx),
 *    not the brief's parallel `"amber" | "moss" | "mute" | "blood"` union —
 *    those four names are pure aliases of StatusTone's existing
 *    `warning`/`success`/`muted`/`danger` and inventing a second vocabulary
 *    plus a mapping layer would only let the two drift.
 * 2. `badgeTone` and `badgeLabel` are both OPTIONAL. The brief's own prose
 *    calls for "a typed badge" — StatusBadge is `{label, tone}` — but the
 *    brief's interface has no label field. A "See all N" row also has no
 *    per-item status to badge at all: it's a navigational affordance, not a
 *    queue entry, so both fields are simply absent for it. `badgeLabel` is
 *    additive (not in the brief's list) to make the badge renderable.
 */
export interface QueueItem {
  id: string;
  type: QueueItemType;
  primary: string;
  secondary: string;
  amount?: string;
  imageUrl?: string;
  badgeTone?: StatusTone;
  badgeLabel?: string;
  onPressRoute: string;
}

/**
 * Already-fetched payloads `buildQueue` composes from. `recentOrders` and
 * `lowStock` come straight off the dashboard payload (`recent_orders`,
 * `low_stock`); `reviews`/`tickets` are separate queries the Dashboard
 * screen (Task 8) fetches scoped to `status=pending`/`status=open` — this
 * module still defensively re-filters both (see the per-type filters below)
 * so a caller that forgets the query param can't leak a resolved ticket or
 * an approved review into a "needs you" queue.
 */
export interface QueueSources {
  stats: DashboardStats;
  recentOrders: RecentOrder[];
  lowStock: LowStockItem[];
  reviews: Review[];
  tickets: Ticket[];
  /** Store currency for formatting `amount`. Undefined falls back to a plain number (see formatMoney). */
  currencyCode?: string;
}

const TYPE_CAP = 3;
const TOTAL_CAP = 12;

const SEE_ALL_NOUN: Record<QueueItemType, string> = {
  order: "pending orders",
  stock: "low stock items",
  ticket: "open tickets",
  review: "pending reviews",
};

const SEE_ALL_ROUTE: Record<QueueItemType, string> = {
  order: "/(tabs)/orders",
  stock: "/(tabs)/products",
  ticket: "/(tabs)/more/settings/tickets",
  review: "/(tabs)/customers/reviews",
};

function sortByRecency<T extends { created_at: string }>(items: T[]): T[] {
  return [...items].sort(
    (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
  );
}

function seeAllRow(type: QueueItemType, count: number | undefined): QueueItem {
  const noun = SEE_ALL_NOUN[type];
  const primary = count !== undefined ? `See all ${count} ${noun}` : `See all ${noun}`;
  return {
    id: `see-all-${type}`,
    type,
    primary,
    secondary: "",
    onPressRoute: SEE_ALL_ROUTE[type],
  };
}

/** Caps `items` at TYPE_CAP and appends a "See all" row only on overflow. */
function buildTypeGroup<T>(
  items: T[],
  type: QueueItemType,
  toItem: (item: T) => QueueItem,
  authoritativeCount: number | undefined,
): QueueItem[] {
  const rows = items.slice(0, TYPE_CAP).map(toItem);
  if (items.length > TYPE_CAP) {
    rows.push(seeAllRow(type, authoritativeCount));
  }
  return rows;
}

function orderToQueueItem(order: RecentOrder, currencyCode: string | undefined): QueueItem {
  return {
    id: order.id,
    type: "order",
    // *string + omitempty -> absent, not null (see dashboard.ts schema
    // comment) — falls back to the email every order always has.
    primary: order.customer_name || order.customer_email,
    secondary: `Order #${order.order_number}`,
    amount: formatMoney(order.grand_total, currencyCode),
    imageUrl: order.image_url,
    badgeTone: "warning",
    badgeLabel: "Pending",
    onPressRoute: `/(tabs)/orders/${order.id}`,
  };
}

function stockToQueueItem(item: LowStockItem): QueueItem {
  const secondary = item.variant_title
    ? `${item.variant_title} · ${item.quantity} left`
    : `${item.quantity} left`;
  return {
    id: item.id,
    type: "stock",
    primary: item.title,
    secondary,
    imageUrl: item.image_url,
    badgeTone: "danger",
    badgeLabel: "Low stock",
    // `id` is the VARIANT id (see dashboard.ts) — navigate with product_id.
    // When absent (a client build shipped before the API deploy), route to
    // the products LIST rather than an interpolated "/products/undefined" —
    // the fix that already shipped in increment 1, preserved here.
    onPressRoute: item.product_id ? `/(tabs)/products/${item.product_id}` : "/(tabs)/products",
  };
}

function ticketToQueueItem(ticket: Ticket): QueueItem {
  return {
    id: ticket.id,
    type: "ticket",
    primary: ticket.submitted_by_name,
    secondary: ticket.subject,
    // No moss here — see the "badge tone" module doc note below.
    badgeTone: "muted",
    badgeLabel: "Open",
    onPressRoute: `/(tabs)/more/settings/tickets/${ticket.id}`,
  };
}

function reviewToQueueItem(review: Review): QueueItem {
  return {
    id: review.id,
    type: "review",
    primary: review.customer_name,
    secondary: review.title || review.content,
    badgeTone: "muted",
    badgeLabel: "New review",
    onPressRoute: `/(tabs)/customers/reviews/${review.id}`,
  };
}

/**
 * Pure composition: already-fetched payloads in, the sorted/capped/typed
 * "needs you" queue out. No fetching, no navigation, no rendering — see
 * inc2-task-7-brief.md for why this is kept separate from the screen.
 *
 * Ordering is urgency then recency, grouped by type in this fixed order:
 * pending orders (money waiting) -> low stock (sales at risk) -> unanswered
 * tickets -> pending reviews. Within a type, the most recently created item
 * comes first. Each type is capped at 3 (see TYPE_CAP) with an overflow
 * "See all" row; the whole list is then capped at 12 (TOTAL_CAP) by simply
 * slicing — because the groups are already ordered by priority, a total-cap
 * trim drops the LEAST urgent rows first (reviews before tickets before
 * stock before orders), never the reverse.
 */
export function buildQueue(sources: QueueSources): QueueItem[] {
  const pendingOrders = sortByRecency(
    sources.recentOrders.filter((o) => o.status === "pending"),
  );
  const openTickets = sortByRecency(sources.tickets.filter((t) => t.status === "open"));
  const pendingReviews = sortByRecency(sources.reviews.filter((r) => r.status === "pending"));

  const orderItems = buildTypeGroup(
    pendingOrders,
    "order",
    (o) => orderToQueueItem(o, sources.currencyCode),
    sources.stats.orders_pending,
  );
  const stockItems = buildTypeGroup(sources.lowStock, "stock", stockToQueueItem, undefined);
  const ticketItems = buildTypeGroup(openTickets, "ticket", ticketToQueueItem, undefined);
  const reviewItems = buildTypeGroup(
    pendingReviews,
    "review",
    reviewToQueueItem,
    sources.stats.pending_reviews,
  );

  return [...orderItems, ...stockItems, ...ticketItems, ...reviewItems].slice(0, TOTAL_CAP);
}
