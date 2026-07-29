import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { RefreshControl, View, StyleSheet } from "react-native";
import Animated from "react-native-reanimated";
import { useRouter } from "expo-router";
import { Archive, Check, X } from "lucide-react-native";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { useDashboard } from "@/lib/hooks/use-dashboard";
import { useReviews } from "@/lib/hooks/use-reviews";
import { useTickets } from "@/lib/hooks/use-tickets";
import { useConfirmOrder, useCancelOrder } from "@/lib/admin-api/order-actions";
import { useApproveReview, useRejectReview } from "@/lib/admin-api/review-actions";
import { useUpdateTicketStatus } from "@/lib/admin-api/ticket-actions";
import { buildQueue, type QueueItem, type QueueItemType } from "@/lib/queue";
import { useCollapsingScroll } from "@/lib/use-collapsing-scroll";
import { useActionFailure } from "@/lib/use-action-failure";
import {
  ActionFailureNotice,
  CollapsingHeader,
  Hairline,
  Screen,
  SwipeRow,
  Text,
  type SwipeAction,
} from "@/components/ui";
import { QueueRow } from "@/components/dashboard/QueueRow";
import { MetricsCard } from "@/components/dashboard/MetricsCard";
import { QueueEmptyState } from "@/components/dashboard/QueueEmptyState";
import { SourceErrorRow } from "@/components/dashboard/SourceErrorRow";
import { TenantMonogram } from "@/components/dashboard/TenantMonogram";
import {
  CancelReasonSheet,
  type CancelReasonSheetHandle,
} from "@/components/orders/CancelReasonSheet";
import { useDockClearance } from "@/components/navigation/dock-metrics";
import { theme } from "@/lib/theme";
import type { DashboardStats } from "@repo/mobile-shared/api/types";

const LOCALE = "en-AU";

/**
 * "Monday, 27 July" — composed from two locale calls rather than one
 * options bag so the day-then-month order is guaranteed regardless of what
 * ordering the device locale would otherwise impose.
 */
function todayLabel(now: Date = new Date()): string {
  const weekday = now.toLocaleDateString(LOCALE, { weekday: "long" });
  const month = now.toLocaleDateString(LOCALE, { month: "long" });
  return `${weekday}, ${now.getDate()} ${month}`;
}

/** The brief's `limit=4` for the two side queries. */
const SIDE_QUERY_LIMIT = 4;

/**
 * Stand-in stats used ONLY to keep `buildQueue` callable when the dashboard
 * query itself is the source that failed. Every authoritative count is 0, so
 * no "See all N" overflow row is invented for data we couldn't read — the
 * reviews and tickets that DID load still render.
 */
const NO_STATS: DashboardStats = {
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
};

/** Per-call react-query mutation callbacks. */
interface MutationCallbacks {
  onSuccess: () => void;
  /**
   * The parameter is the mutation's error — react-query already passes it
   * (`onError(error, variables, context)`). It is what lets the failure
   * notice tell "your device couldn't reach the server" apart from "the
   * server refused this", which are different problems for the merchant.
   */
  onError: (error?: unknown) => void;
}

const ICON_SIZE = 20;

/** Shown inline in `CancelReasonSheet` when a cancel round-trip fails. */
const CANCEL_ERROR = "Couldn't cancel this order. Try again.";

/**
 * The three independent queries the queue is composed from. Four queue TYPES
 * map onto three sources — the dashboard payload carries both orders and low
 * stock.
 */
type QueueSource = "dashboard" | "reviews" | "tickets";

const SOURCE_FOR_TYPE: Record<QueueItemType, QueueSource> = {
  order: "dashboard",
  stock: "dashboard",
  review: "reviews",
  ticket: "tickets",
};

const NO_IDS: ReadonlySet<string> = new Set();

/** Optimistic hides, kept PER SOURCE — see `useClearWhenFresh`. */
type Dismissed = Readonly<Record<QueueSource, ReadonlySet<string>>>;

const NO_DISMISSALS: Dismissed = {
  dashboard: NO_IDS,
  reviews: NO_IDS,
  tickets: NO_IDS,
};

/**
 * Per source, the `dataUpdatedAt` of the newest answer known NOT to be
 * authoritative about the hides currently in place. Lives in a ref: it gates
 * a clear, it is never rendered.
 */
type Watermark = Record<QueueSource, number>;

const NO_WATERMARK: Watermark = { dashboard: 0, reviews: 0, tickets: 0 };

/**
 * Drops a source's optimistic hides once that source has produced an answer
 * that POSTDATES the action those hides represent.
 *
 * ONE rule, applied per source: clear when `dataUpdatedAt` advances past the
 * watermark; every answer that is not newer than the watermark advances the
 * watermark instead.
 *
 * Why clear at all: `dismissed` is a purely local overlay on the server's
 * answer, valid only until a fresh answer arrives. Without this, an id that
 * legitimately RETURNS to the queue (an approval the server rejected out of
 * band, a ticket reopened on another device) would stay invisible for the
 * life of the screen.
 *
 * Three failures this shape closes, each of which shipped once:
 *
 * 1. **Not the MAX of all three sources.** Keying the clear on
 *    `Math.max(dashboard, reviews, tickets .dataUpdatedAt)` meant a refetch of
 *    ONE query un-hid rows belonging to the OTHER two. Reachable on any
 *    staggered initial load (dashboard resolves first, tickets last) and on
 *    every `refetchOnWindowFocus`.
 * 2. **Settling is not an answer.** The clear was `if (isMutating) return;
 *    clear(source)` with `isMutating` in the dependency array, so the
 *    true→false transition was itself a clear trigger — nothing checked that
 *    data had arrived, only that the request had stopped. For orders that was
 *    permanent: the order mutations invalidated `["orders"]`, this screen
 *    reads `["dashboard"]`, so the dashboard never refetched and the
 *    just-approved order came back still labelled Pending and swipeable
 *    again. (`lib/admin-api/order-actions.ts` now invalidates `["dashboard"]`
 *    too; this hook must be correct without depending on that.)
 * 3. **An answer that lands MID-mutation predates it.** A window-focus
 *    refetch that resolves while the confirm is still in flight was computed
 *    before the confirm committed, so it still lists the row as pending. It
 *    is not evidence of anything: it advances the watermark rather than
 *    clearing, and stays non-authoritative after the mutation ends too.
 *
 * Deliberately NOT solved by returning the invalidation promise from the
 * mutations' `onSuccess` so `isPending` spans the refetch: that would make
 * the post-mutation refetch land while `isMutating` is still true, rule 3
 * would swallow it as non-authoritative, and the hides would never expire at
 * all. The watermark makes the promise unnecessary — it reads WHEN the data
 * arrived, not whether a request happened to be open at the time.
 */
function useExpireHidesOnFreshAnswer(
  source: QueueSource,
  dataUpdatedAt: number,
  isMutating: boolean,
  watermarks: { current: Watermark },
  clear: (source: QueueSource) => void,
): void {
  useEffect(() => {
    const seen = watermarks.current[source];
    watermarks.current = { ...watermarks.current, [source]: dataUpdatedAt };
    if (isMutating || dataUpdatedAt <= seen) return;
    clear(source);
  }, [source, dataUpdatedAt, isMutating, watermarks, clear]);
}

/**
 * The Dashboard is an ACTION QUEUE, not a report.
 *
 * Three layers, top to bottom:
 *  1. `CollapsingHeader` — the store's name in serif (it's the merchant's
 *     shop; name it), today's date above it, the tenant monogram + unread
 *     badge on the right.
 *  2. `MetricsCard` — the one elevated card on the whole screen.
 *  3. The "needs you" queue — hairline-separated rows on the Paper ground,
 *     each swipeable to its own actions.
 *
 * Composed from THREE independent queries (dashboard, pending reviews, open
 * tickets). None of them can take the screen down: whichever ones resolve
 * render, and the failures collapse into a single `SourceErrorRow`. That is
 * why `buildQueue` is fed `NO_STATS` rather than being skipped when the
 * dashboard query is the one that failed.
 */
export default function DashboardScreen() {
  const router = useRouter();
  const dockPad = useDockClearance();
  const { scrollY, onScroll: scrollHandler } = useCollapsingScroll();

  const storeName = useTenantStore((s) => s.activeStore?.name);
  const currencyCode = useTenantStore((s) => s.activeStore?.currency_code);

  const dashboard = useDashboard();
  const reviews = useReviews({ status: "pending" });
  const tickets = useTickets({ status: "open" });

  const confirmOrder = useConfirmOrder();
  const cancelOrder = useCancelOrder();
  const approveReview = useApproveReview();
  const rejectReview = useRejectReview();
  const updateTicketStatus = useUpdateTicketStatus();

  // Rows hidden optimistically while their mutation is in flight, bucketed by
  // the query that owns them. Rolled back in `onError` — a new Set (and a new
  // record) every time, never mutated in place.
  const [dismissed, setDismissed] = useState<Dismissed>(NO_DISMISSALS);
  const [cancelTargetId, setCancelTargetId] = useState<string | null>(null);
  // Local, NOT `cancelOrder.error`. The mutation's error is never reset, so
  // binding the sheet straight to it meant one failed cancel poisoned the
  // sheet for EVERY subsequent order: the merchant opened Cancel on an
  // untouched order and was greeted by a failure they hadn't caused. Same
  // pattern as the order detail screen (app/(tabs)/orders/[id].tsx), cleared
  // on present.
  const [cancelError, setCancelError] = useState<string | null>(null);
  // True only for a user-initiated pull-to-refresh, and only until ALL THREE
  // queries settle — `dashboard.isRefetching` alone stopped the spinner while
  // reviews and tickets were still in flight.
  const [isRefreshing, setIsRefreshing] = useState(false);
  const cancelSheetRef = useRef<CancelReasonSheetHandle>(null);
  // The Dashboard builds its own optimistic-hide callbacks rather than going
  // through `useBusyIds`, so it reaches the shared failure surface directly.
  // Same state, same copy rules, same strip as the three list screens.
  const { failure, reportFailure, clearFailure } = useActionFailure();

  const reviewItems = useMemo(
    () => reviews.data?.pages[0]?.data.slice(0, SIDE_QUERY_LIMIT) ?? [],
    [reviews.data],
  );
  const ticketItems = useMemo(
    () => tickets.data?.pages[0]?.data.slice(0, SIDE_QUERY_LIMIT) ?? [],
    [tickets.data],
  );

  const queue = useMemo(
    () =>
      buildQueue({
        stats: dashboard.data?.stats ?? NO_STATS,
        recentOrders: dashboard.data?.recent_orders ?? [],
        lowStock: dashboard.data?.low_stock ?? [],
        reviews: reviewItems,
        tickets: ticketItems,
        currencyCode,
      }),
    [dashboard.data, reviewItems, ticketItems, currencyCode],
  );

  const visibleQueue = useMemo(
    () => queue.filter((item) => !dismissed[SOURCE_FOR_TYPE[item.type]].has(item.id)),
    [queue, dismissed],
  );

  /**
   * The number beside "Needs you" counts WORK, not rows. `buildQueue` also
   * emits `kind: "seeAll"` rows ("See all 5 pending orders") — navigational
   * affordances, not things that need the merchant — and counting them read
   * "7" over a list of 5 actionable items plus 2 links.
   */
  const needsYouCount = useMemo(
    () => visibleQueue.filter((item) => item.kind === "item").length,
    [visibleQueue],
  );

  const failedSources = useMemo(() => {
    const failed: { label: string; refetch: () => void }[] = [];
    if (dashboard.isError) failed.push({ label: "orders and stock", refetch: dashboard.refetch });
    if (reviews.isError) failed.push({ label: "reviews", refetch: reviews.refetch });
    if (tickets.isError) failed.push({ label: "tickets", refetch: tickets.refetch });
    return failed;
  }, [dashboard.isError, dashboard.refetch, reviews.isError, reviews.refetch, tickets.isError, tickets.refetch]);

  /**
   * Awaits every refetch rather than discarding the promises, so the caller
   * knows when the whole fan-out has settled.
   *
   * A rejection here is deliberately absorbed, not swallowed: react-query has
   * already recorded it on the query, `failedSources` recomputes from
   * `isError`, and the merchant sees it as the `SourceErrorRow` with its own
   * Retry. Re-throwing would only surface as an unhandled rejection out of
   * `RefreshControl.onRefresh`.
   */
  const refetchAll = useCallback(async (sources: { refetch: () => unknown }[]) => {
    await Promise.all(sources.map((source) => Promise.resolve(source.refetch()).catch(() => {})));
  }, []);

  const retryFailed = useCallback(async () => {
    await refetchAll(failedSources);
  }, [failedSources, refetchAll]);

  const refreshAll = useCallback(async () => {
    setIsRefreshing(true);
    try {
      await refetchAll([dashboard, reviews, tickets]);
    } finally {
      setIsRefreshing(false);
    }
  }, [dashboard, reviews, tickets, refetchAll]);

  const hide = useCallback((source: QueueSource, id: string) => {
    setDismissed((prev) => {
      const next = new Set(prev[source]);
      next.add(id);
      return { ...prev, [source]: next };
    });
  }, []);

  const restore = useCallback((source: QueueSource, id: string) => {
    setDismissed((prev) => {
      const next = new Set(prev[source]);
      next.delete(id);
      return { ...prev, [source]: next };
    });
  }, []);

  // Returns `prev` untouched when there is nothing to clear, so the effects
  // below don't schedule a render on every mount and every refetch.
  const clearSource = useCallback((source: QueueSource) => {
    setDismissed((prev) =>
      prev[source].size === 0 ? prev : { ...prev, [source]: NO_IDS },
    );
  }, []);

  // A mutation is "pending" for the source whose list it will change. Cancel
  // belongs to the dashboard (it changes an order), not to a fourth bucket.
  const dashboardMutating = confirmOrder.isPending || cancelOrder.isPending;
  const reviewsMutating = approveReview.isPending || rejectReview.isPending;
  const ticketsMutating = updateTicketStatus.isPending;

  const watermarks = useRef<Watermark>(NO_WATERMARK);

  useExpireHidesOnFreshAnswer(
    "dashboard",
    dashboard.dataUpdatedAt ?? 0,
    dashboardMutating,
    watermarks,
    clearSource,
  );
  useExpireHidesOnFreshAnswer(
    "reviews",
    reviews.dataUpdatedAt ?? 0,
    reviewsMutating,
    watermarks,
    clearSource,
  );
  useExpireHidesOnFreshAnswer(
    "tickets",
    tickets.dataUpdatedAt ?? 0,
    ticketsMutating,
    watermarks,
    clearSource,
  );

  /**
   * Optimistic + rollback, at the screen rather than in the mutation hooks:
   * the row comes back if the server says no. The hooks themselves already
   * invalidate their query keys on success, so the authoritative list
   * replaces this local guess on the next fetch.
   *
   * PURE — it builds callbacks and nothing else. Hiding the row is the
   * caller's own explicit `hide(id)` statement next to the `mutate` call.
   * This used to call `hide(id)` as a side effect of CONSTRUCTING the object,
   * inline inside `mutate(...)`'s arguments: it read as a factory, and any
   * future memoisation of it would have silently stopped hiding rows.
   *
   * Restoring the row is necessary but NOT sufficient: on its own it just
   * makes the queue flicker and settle back to where it started, which reads
   * as "nothing happened" rather than "that failed". `action` is what turns
   * the rollback into an explanation.
   *
   * @param action A lower-case verb phrase, read after "Couldn't " — e.g.
   *   `"close this ticket"`.
   */
  const callbacksFor = useCallback(
    (source: QueueSource, id: string, action: string): MutationCallbacks => ({
      onSuccess: () => {
        // A notice about the attempt before a successful retry is untrue, and
        // it would sit there contradicting the row that just disappeared.
        clearFailure();
        void adminHaptics.actionSucceeded();
      },
      onError: (error?: unknown) => {
        restore(source, id);
        reportFailure(error, action);
        void adminHaptics.actionFailed();
      },
    }),
    [restore, reportFailure, clearFailure],
  );

  const openCancelSheet = useCallback((id: string) => {
    setCancelTargetId(id);
    setCancelError(null);
    cancelSheetRef.current?.present();
  }, []);

  const submitCancel = useCallback(
    (reason: string) => {
      const id = cancelTargetId;
      if (!id) return;
      cancelOrder.mutate(
        { id, reason },
        {
          onSuccess: () => {
            cancelSheetRef.current?.dismiss();
            setCancelTargetId(null);
            hide("dashboard", id);
            void adminHaptics.actionSucceeded();
          },
          onError: () => {
            setCancelError(CANCEL_ERROR);
            void adminHaptics.actionFailed();
          },
        },
      );
    },
    [cancelTargetId, cancelOrder, hide],
  );

  /**
   * Swipe actions per queue item type. Convention, app-wide: dragging RIGHT
   * reveals the CONSTRUCTIVE action at the leading edge, dragging LEFT the
   * destructive one at the trailing edge. Nothing here opts into
   * `autoFireOnFullSwipe` — this app has no undo.
   *
   * Cancel is the one action that does NOT fire a mutation: the API requires
   * a reason (`CancelOrderRequest.Reason` is `binding:"required"`), so the
   * revealed Cancel opens `CancelReasonSheet` exactly as the order detail
   * screen does. A revealed action being tapped to open a sheet is fully
   * within the `SwipeRow` contract — a revealed action is always tapped,
   * never auto-fired.
   */
  const actionsFor = useCallback(
    (item: QueueItem): { leading?: SwipeAction[]; trailing?: SwipeAction[] } | null => {
      if (item.kind === "seeAll") return null;

      switch (item.type) {
        case "order":
          return {
            leading: [
              {
                key: "approve",
                label: "Approve",
                tone: "accent",
                icon: <Check size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
                onPress: () => {
                  // Hiding is an explicit statement, not a side effect of
                  // building the callbacks object — see `callbacksFor`.
                  hide("dashboard", item.id);
                  confirmOrder.mutate(
                    { id: item.id },
                    callbacksFor("dashboard", item.id, "approve this order"),
                  );
                },
              },
            ],
            trailing: [
              {
                key: "cancel",
                label: "Cancel",
                tone: "danger",
                icon: <X size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
                onPress: () => openCancelSheet(item.id),
              },
            ],
          };
        case "review":
          return {
            leading: [
              {
                key: "approve",
                label: "Approve",
                tone: "accent",
                icon: <Check size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
                onPress: () => {
                  hide("reviews", item.id);
                  approveReview.mutate(
                    item.id,
                    callbacksFor("reviews", item.id, "approve this review"),
                  );
                },
              },
            ],
            trailing: [
              {
                key: "reject",
                label: "Reject",
                tone: "danger",
                icon: <X size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
                onPress: () => {
                  hide("reviews", item.id);
                  rejectReview.mutate(
                    item.id,
                    callbacksFor("reviews", item.id, "reject this review"),
                  );
                },
              },
            ],
          };
        case "ticket":
          return {
            trailing: [
              {
                key: "close",
                label: "Close",
                // Neutral, not danger: closing a resolved ticket is a normal
                // outcome, and the trailing edge is a position, not a tone.
                tone: "neutral",
                icon: <Archive size={ICON_SIZE} color={theme.colors.text} strokeWidth={2} />,
                onPress: () => {
                  hide("tickets", item.id);
                  updateTicketStatus.mutate(
                    { id: item.id, status: "closed" },
                    callbacksFor("tickets", item.id, "close this ticket"),
                  );
                },
              },
            ],
          };
        // Low stock has no one-tap resolution — restocking happens on the
        // product. No swipe rather than a swipe that only navigates.
        default:
          return null;
      }
    },
    [
      confirmOrder,
      approveReview,
      rejectReview,
      updateTicketStatus,
      callbacksFor,
      hide,
      openCancelSheet,
    ],
  );

  const anyLoading = dashboard.isLoading || reviews.isLoading || tickets.isLoading;
  const showEmptyState =
    visibleQueue.length === 0 && failedSources.length === 0 && !anyLoading;

  return (
    <Screen>
      <CollapsingHeader
        eyebrow={todayLabel()}
        // Sentence case, not the uppercase small-caps default: the dateline
        // is running prose ("Monday, 27 July"), the first thing the eye lands
        // on, and the mockup does not shout it.
        eyebrowPreset="caption"
        title={storeName ?? "Your store"}
        rightSlot={<TenantMonogram storeName={storeName} />}
        scrollY={scrollY}
      />

      <Animated.ScrollView
        onScroll={scrollHandler}
        scrollEventThrottle={16}
        contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}
        refreshControl={
          <RefreshControl
            refreshing={isRefreshing}
            onRefresh={refreshAll}
            tintColor={theme.colors.text}
          />
        }
      >
        {dashboard.data ? (
          <View style={styles.gutter}>
            <MetricsCard stats={dashboard.data.stats} currencyCode={currencyCode} />
          </View>
        ) : null}

        <View style={styles.queueHeader}>
          <Text preset="eyebrow" color="textTertiary">
            Needs you
          </Text>
          {needsYouCount > 0 ? (
            <Text
              preset="caption"
              color="textTertiary"
              style={styles.count}
              testID="dashboard-needs-you-count"
            >
              {String(needsYouCount)}
            </Text>
          ) : null}
        </View>

        {failedSources.length > 0 ? (
          <SourceErrorRow
            sources={failedSources.map((s) => s.label)}
            onRetry={retryFailed}
          />
        ) : null}

        {anyLoading && visibleQueue.length === 0 && failedSources.length === 0 ? (
          <Text preset="caption" color="textTertiary" style={styles.loading}>
            Loading…
          </Text>
        ) : null}

        {showEmptyState ? <QueueEmptyState /> : null}

        {visibleQueue.map((item, index) => {
          const actions = actionsFor(item);
          const row = (
            <QueueRow item={item} onPress={() => router.push(item.onPressRoute)} />
          );
          return (
            <View key={item.id}>
              {index > 0 ? <Hairline inset={theme.spacing.xl} /> : null}
              {actions ? (
                <SwipeRow
                  testID={`swipe-${item.id}`}
                  leadingActions={actions.leading}
                  trailingActions={actions.trailing}
                >
                  {row}
                </SwipeRow>
              ) : (
                row
              )}
            </View>
          );
        })}
      </Animated.ScrollView>

      {/* Why a row the merchant just swiped came back. Only the DIRECT
          mutations report here — cancel owns its own inline sheet error,
          which keeps the typed reason on screen beside the message and must
          not be duplicated behind the sheet. */}
      <ActionFailureNotice failure={failure} onDismiss={clearFailure} />

      <CancelReasonSheet
        ref={cancelSheetRef}
        onSubmit={submitCancel}
        isSubmitting={cancelOrder.isPending}
        error={cancelError}
        // Released on EVERY close, not only on a successful cancel — the same
        // contract Orders' list screen already had. The sheet renders through
        // a portal and is ALWAYS mounted, so an order left pinned by a
        // backed-out sheet is a live, armed submit for an order the merchant
        // explicitly walked away from. Backing out is a one-tap accident now
        // that the sheet has a backdrop.
        onDismiss={() => setCancelTargetId(null)}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingTop: theme.spacing.lg },
  // Screen gutter: theme.spacing.xl (20) — the SAME left edge the header's
  // eyebrow/title and every PressableRow's paddingH sit on. Do not change
  // this to fix an alignment problem; fix the offending block instead.
  gutter: { paddingHorizontal: theme.spacing.xl },
  queueHeader: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xxl,
    paddingBottom: theme.spacing.sm,
  },
  count: { fontVariant: ["tabular-nums"] },
  loading: {
    paddingHorizontal: theme.spacing.xl,
    paddingVertical: theme.spacing.md,
  },
});
