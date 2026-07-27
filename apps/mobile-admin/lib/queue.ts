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
 * `QueueItem` per inc2-task-7-brief.md, with deliberate deviations from the
 * brief's literal interface (recorded in inc2-task-7-report.md):
 *
 * 1. `badgeTone` is typed as `StatusTone` (components/ui/StatusBadge.tsx),
 *    not the brief's parallel `"amber" | "moss" | "mute" | "blood"` union —
 *    those four names are pure aliases of StatusTone's existing
 *    `warning`/`success`/`muted`/`danger` and inventing a second vocabulary
 *    plus a mapping layer would only let the two drift.
 * 2. `QueueItem` is a discriminated union on `kind: "item" | "seeAll"`
 *    (added in review round 1 — see inc2-task-7-report.md "Fix round 1").
 *    A "See all N" row has no per-item status to badge — it's a
 *    navigational affordance, not a queue entry — so the `"seeAll"` variant
 *    simply has no `badgeTone`/`badgeLabel`/`amount`/`imageUrl` fields at
 *    all, rather than an all-optional flat shape where `badgeTone`'s
 *    absence was overloaded as the row-kind discriminator. That overload
 *    made an illegal state representable: a real order item that forgot to
 *    set `badgeTone` type-checked and silently rendered as a "See all"
 *    link, dropping its amount/photo/badge with no error. `badgeLabel`
 *    stays optional on the `"item"` variant (additive vs. the brief, which
 *    has no label field at all) — every current producer sets it, but nothing
 *    depends on that being permanent.
 */
interface QueueItemFields {
  id: string;
  type: QueueItemType;
  primary: string;
  secondary: string;
  onPressRoute: string;
}

export type QueueItem =
  | (QueueItemFields & {
      kind: "item";
      amount?: string;
      imageUrl?: string;
      badgeTone: StatusTone;
      badgeLabel?: string;
    })
  | (QueueItemFields & {
      kind: "seeAll";
    });

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
    kind: "seeAll",
    id: `see-all-${type}`,
    type,
    primary,
    secondary: "",
    onPressRoute: SEE_ALL_ROUTE[type],
  };
}

/**
 * Caps `items` at TYPE_CAP and appends a "See all" row only on overflow.
 *
 * Overflow is decided by `authoritativeCount` when one exists (orders,
 * reviews), NOT by `items.length`. `items` here is only ever the LOCAL,
 * already-capped slice of a source payload — for orders specifically,
 * `recentOrders` is the API's last-5-orders-of-any-status feed (see
 * dashboard.ts handler), which `buildQueue` then filters to pending, so
 * `items.length` can be as low as 0 even when hundreds of orders are
 * pending. Falling back to `items.length` only when there's no authority
 * (stock, tickets — `DashboardStats` has no count for either) preserves the
 * old behaviour for those two types while fixing the mismatch for the two
 * that DO have an authoritative count. This can surface a "See all" row
 * even when the local slice is short or empty — that's the fix, not a bug:
 * it's the only way a merchant with 20 pending orders and 2 of them in the
 * "last 5 orders" feed ever sees a way to reach the other 18.
 */
function buildTypeGroup<T>(
  items: T[],
  type: QueueItemType,
  toItem: (item: T) => QueueItem,
  authoritativeCount: number | undefined,
): QueueItem[] {
  const rows = items.slice(0, TYPE_CAP).map(toItem);
  const total = authoritativeCount ?? items.length;
  if (total > TYPE_CAP) {
    rows.push(seeAllRow(type, authoritativeCount));
  }
  return rows;
}

function orderToQueueItem(order: RecentOrder, currencyCode: string | undefined): QueueItem {
  return {
    kind: "item",
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
    kind: "item",
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
    kind: "item",
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
    kind: "item",
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
 * A type-group's own trailing "See all" row, if it has one — the row an
 * overflowing group appended in `buildTypeGroup`. Detected by id prefix
 * rather than `kind` because a group with no overflow can validly end on a
 * plain `"item"` row too; the prefix is what's actually unique to the row
 * `seeAllRow` produces.
 */
function trailingSeeAllRow(group: QueueItem[]): QueueItem | undefined {
  const last = group[group.length - 1];
  return last?.id.startsWith("see-all-") ? last : undefined;
}

/**
 * Applies TOTAL_CAP across the priority-ordered type groups. Groups that
 * fit whole are kept whole; once a group would be cut mid-way, if that
 * group carries its own "See all" row (i.e. it already overflowed
 * TYPE_CAP), that row is kept as the LAST row taken from the group —
 * replacing one item, not appended past the cap — instead of a plain
 * `.slice()` silently discarding it. A plain `.slice()` at TOTAL_CAP would
 * otherwise cut a 4-row group (3 items + "See all") down to fewer rows and,
 * exactly when the cut lands mid-group, delete the "See all" row instead of
 * one of the 3 items — destroying the one affordance that would let the
 * merchant reach the rest of that overflowing group. A group with no
 * overflow (no "See all" row) has nothing to preserve, so it's still
 * plainly sliced. Once the budget hits 0, later (lower-priority) groups are
 * dropped whole, same as before.
 */
function applyTotalCap(groups: QueueItem[][]): QueueItem[] {
  const result: QueueItem[] = [];
  let remaining = TOTAL_CAP;

  for (const group of groups) {
    if (remaining <= 0) break;

    if (group.length <= remaining) {
      result.push(...group);
      remaining -= group.length;
      continue;
    }

    const seeAll = trailingSeeAllRow(group);
    if (seeAll) {
      const leadingCount = Math.max(remaining - 1, 0);
      result.push(...group.slice(0, leadingCount), seeAll);
    } else {
      result.push(...group.slice(0, remaining));
    }
    remaining = 0;
  }

  return result;
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
 * "See all" row; the whole list is then capped at 12 (TOTAL_CAP) via
 * `applyTotalCap` — because the groups are already ordered by priority, a
 * total-cap trim drops the LEAST urgent rows first (reviews before tickets
 * before stock before orders), never the reverse, and never silently
 * deletes a group's own "See all" row when the cut lands mid-group.
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

  return applyTotalCap([orderItems, stockItems, ticketItems, reviewItems]);
}
