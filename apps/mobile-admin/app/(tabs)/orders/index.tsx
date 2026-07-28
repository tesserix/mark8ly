import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { View, RefreshControl, StyleSheet, ActivityIndicator } from "react-native";
import Animated, {
  useAnimatedScrollHandler,
  useSharedValue,
} from "react-native-reanimated";
import { useRouter } from "expo-router";
import { Check, X } from "lucide-react-native";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { useOrders } from "@/lib/hooks/use-orders";
import { useShipment } from "@/lib/hooks/use-shipment";
import {
  useConfirmOrder,
  useFulfillOrder,
  useCancelOrder,
  useRefundOrder,
} from "@/lib/admin-api/order-actions";
import { useEmailLabel } from "@/lib/admin-api/shipment-actions";
import { OrderRow } from "@/components/OrderRow";
import {
  ActionSheet,
  CollapsingHeader,
  EmptyState,
  FilterChips,
  Hairline,
  Screen,
  SearchField,
  SwipeRow,
  Text,
  type ActionSheetItem,
  type SwipeAction,
} from "@/components/ui";
import {
  CancelReasonSheet,
  type CancelReasonSheetHandle,
} from "@/components/orders/CancelReasonSheet";
import { RefundSheet, type RefundSheetHandle } from "@/components/orders/RefundSheet";
import {
  EmailLabelSheet,
  type EmailLabelSheetHandle,
} from "@/components/orders/EmailLabelSheet";
import { useDockClearance } from "@/components/navigation/dock-metrics";
import { theme } from "@/lib/theme";
import type { Order } from "@repo/mobile-shared/api/types";

type FilterKey = "all" | "pending" | "confirmed" | "completed" | "cancelled";

const FILTERS: { key: FilterKey; label: string; status?: string }[] = [
  { key: "all", label: "All" },
  // One real status per chip. The backend matches status exactly
  // (orders.go:170 `status = ?`), so a comma-joined "pending,confirmed"
  // silently matches nothing — which is what this tab used to do.
  { key: "pending", label: "Pending", status: "pending" },
  { key: "confirmed", label: "Confirmed", status: "confirmed" },
  { key: "completed", label: "Completed", status: "fulfilled" },
  { key: "cancelled", label: "Cancelled", status: "cancelled" },
];

const ICON_SIZE = 20;

const CANCEL_ERROR = "Couldn't cancel this order. Try again.";
const REFUND_ERROR = "Couldn't issue the refund. Try again.";

/** Order statuses past which nothing can be confirmed, cancelled or fulfilled. */
const TERMINAL_STATUSES = new Set(["fulfilled", "cancelled"]);

/** Payment states a refund can be issued against — mirrors orders/[id].tsx. */
const REFUNDABLE_PAYMENT_STATUSES = new Set(["paid", "partially_refunded"]);

function useDebounce(value: string, delay: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

/**
 * Orders — gesture triage.
 *
 * The list a merchant works THROUGH, not one they read. Three affordances,
 * in increasing order of commitment:
 *
 *  1. Swipe right → Approve (confirm). Swipe left → Cancel. Per the app-wide
 *     convention: constructive at the leading edge in moss, destructive at
 *     the trailing edge in danger. Neither fires on the swipe itself —
 *     `SwipeRow` settles open and the revealed button is tapped. This app
 *     has no undo, so nothing here opts into `autoFireOnFullSwipe`.
 *  2. Long press → the full four-action menu.
 *  3. Tap → the order detail, where everything is confirmable.
 *
 * Search is PINNED, visible at rest, between the header and the filter row —
 * the same place `products/index.tsx` and `customers/index.tsx` put theirs.
 * It briefly lived inside the scroll content with the list parked past it,
 * which bought back 64pt of 874 (7%) and cost the field every trace of an
 * affordance: no peek, no icon, no hint, rows butting straight against the
 * chips. On device it was undiscoverable to the person who had just written
 * it. Orders is also the screen a merchant searches most — an order-number
 * lookup while a customer is on the phone — the gesture is not the platform
 * pattern (iOS's own search is visible at rest and collapses on scroll, not
 * the reverse), and the empty state showed the field unconditionally anyway,
 * so the screen was inconsistent with itself. Pinned, permanently.
 *
 * NO OPTIMISTIC HIDE — deliberately, and unlike the Dashboard. `useOrders`
 * is keyed `["orders","list",…]` and every order mutation invalidates the
 * `["orders"]` prefix (see lib/admin-api/order-actions.ts), so the refetch
 * is authoritative about this list's own contents: under a status filter the
 * actioned order drops out by itself, and under "All" its badge simply
 * flips. The Dashboard needed a local overlay because its queue is built
 * from a DIFFERENT query (`["dashboard"]`) that its own row actions did not
 * invalidate; that asymmetry does not exist here, and re-creating the
 * dismissed/watermark machinery would import a four-round bug for no gain.
 *
 * What the absent hide does leave open is a second swipe on a row whose
 * mutation is still in flight. That is closed by disabling THAT row's
 * gesture while its request is open (`busyOrderIds`) — a guard on the
 * control, keyed purely on the mutation lifecycle. Note the distinction
 * that cost the Dashboard two shipped bugs: "the request stopped" is a fine
 * reason to re-enable a button, and NOT evidence that fresh data arrived.
 * Nothing here infers list contents from mutation state.
 */
export default function OrdersScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const currencyCode = useTenantStore((s) => s.activeStore?.currency_code);

  const [activeFilter, setActiveFilter] = useState<FilterKey>("all");
  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);

  // Scroll offset the CollapsingHeader reads. Straight through, no rebase:
  // the list starts at 0 now that nothing is parked above it, so the
  // header's "expanded" rest state and the list's resting position already
  // agree. (The rebase existed only to cancel out the `contentOffset` that
  // hid the search field — both went together.)
  const scrollY = useSharedValue(0);
  const scrollHandler = useAnimatedScrollHandler((event) => {
    "worklet";
    scrollY.value = Math.max(0, event.contentOffset.y);
  });

  const selectedFilter = FILTERS.find((f) => f.key === activeFilter);
  const listQuery = useOrders({
    ...(selectedFilter?.status ? { status: selectedFilter.status } : {}),
    ...(debouncedSearch ? { search: debouncedSearch } : {}),
  });

  // The header's pending count. Its own status-pinned query rather than a
  // count derived from the visible list, which is filtered and paginated and
  // would report "3 pending" on the Cancelled tab. When the Pending chip is
  // active with no search this resolves to the SAME react-query key as the
  // list above, so it costs no extra request in the case it matters most.
  const pendingQuery = useOrders({ status: "pending" });
  const pendingCount = pendingQuery.data?.pages[0]?.meta.total ?? 0;

  const confirmOrder = useConfirmOrder();
  const fulfillOrder = useFulfillOrder();
  const cancelOrder = useCancelOrder();
  const refundOrder = useRefundOrder();
  const emailLabel = useEmailLabel();

  // The order whose long-press menu is open. Also the only thing that keeps
  // the menu mounted — `ActionSheet` is a controlled component.
  const [menuOrder, setMenuOrder] = useState<Order | null>(null);
  // The rows whose gestures are suppressed because their own request is
  // open. A SET, not one slot: triage is a queue, and a merchant working
  // down it fires the next row long before the previous one's request comes
  // back. With a single slot, approving A then B overwrote the guard — and
  // A's `onSuccess` then cleared it outright, re-arming B while B's own
  // request was still open. A per-row guard has to be per row.
  //
  // Replaced immutably (never `.add`/`.delete` on the live value), so React
  // sees a new identity and `renderItem`'s memo actually re-runs.
  const [busyOrderIds, setBusyOrderIds] = useState<ReadonlySet<string>>(() => new Set());
  const [cancelTarget, setCancelTarget] = useState<Order | null>(null);
  const [refundTarget, setRefundTarget] = useState<Order | null>(null);
  const [emailTarget, setEmailTarget] = useState<
    { orderId: string; shipmentId: string } | null
  >(null);
  // Local, NOT `mutation.error`. react-query never resets a mutation error,
  // so binding a sheet straight to it means one failed cancel greets the
  // merchant on EVERY subsequent order's sheet before they type anything.
  // Same pattern as orders/[id].tsx and the Dashboard; cleared on present.
  const [cancelError, setCancelError] = useState<string | null>(null);
  const [refundError, setRefundError] = useState<string | null>(null);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const cancelSheetRef = useRef<CancelReasonSheetHandle>(null);
  const refundSheetRef = useRef<RefundSheetHandle>(null);
  const emailSheetRef = useRef<EmailLabelSheetHandle>(null);

  /**
   * The order we currently need shipment facts about. Every surface whose
   * copy depends on whether this order shipped has to appear here, or that
   * copy is silently suppressed:
   *
   *  - the long-press menu — "Email label" needs a shipment id, which the
   *    LIST payload does not carry;
   *  - the cancel sheet — its copy names the carrier whose shipment is
   *    cancelled alongside the order;
   *  - the refund sheet — its copy warns that a FULL refund also cancels or
   *    returns the shipment at the carrier. `refundTarget` was missing from
   *    this chain, and because tapping Refund dismisses the menu (clearing
   *    `menuOrder`), the probe went disabled at exactly the moment the sheet
   *    needed it: merchants issuing a full refund on a shipped order were
   *    never told, and there is no undo.
   *
   * One lazy query for one order at a time; `enabled` keeps it off entirely
   * until a merchant asks for it, so scrolling a 50-row list fires nothing.
   * The three targets are mutually exclusive in practice — each is cleared
   * when its own sheet/menu closes (see the `onDismiss` handlers below), so
   * the order of this chain is a tiebreak that should never be needed.
   */
  const probeOrderId = menuOrder?.id ?? cancelTarget?.id ?? refundTarget?.id ?? null;
  const { data: shipment } = useShipment(probeOrderId ?? "", Boolean(probeOrderId));

  const orders = useMemo(
    () => listQuery.data?.pages.flatMap((page) => page.data) ?? [],
    [listQuery.data],
  );

  const handleFilterChange = useCallback((key: FilterKey) => setActiveFilter(key), []);

  const handleOrderPress = useCallback(
    (order: Order) => router.push(`/(tabs)/orders/${order.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (listQuery.hasNextPage && !listQuery.isFetchingNextPage) listQuery.fetchNextPage();
  }, [listQuery]);

  const refresh = useCallback(async () => {
    setIsRefreshing(true);
    try {
      await Promise.resolve(listQuery.refetch()).catch(() => {});
    } finally {
      setIsRefreshing(false);
    }
  }, [listQuery]);

  const markBusy = useCallback((id: string) => {
    setBusyOrderIds((prev) => (prev.has(id) ? prev : new Set(prev).add(id)));
  }, []);

  const clearBusy = useCallback((id: string) => {
    setBusyOrderIds((prev) => {
      if (!prev.has(id)) return prev;
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  }, []);

  /**
   * Callbacks for the two direct mutations (confirm, fulfil). Releases THAT
   * order's gesture guard — by id, so settling one row leaves every other
   * in-flight row guarded — and reports the outcome in the hand. There is
   * nothing to roll back; no local state ever claimed the row had changed.
   */
  const settleCallbacks = useCallback(
    (id: string) => ({
      onSuccess: () => {
        clearBusy(id);
        void adminHaptics.actionSucceeded();
      },
      onError: () => {
        clearBusy(id);
        void adminHaptics.actionFailed();
      },
    }),
    [clearBusy],
  );

  const openCancelSheet = useCallback((order: Order) => {
    setCancelTarget(order);
    setCancelError(null);
    cancelSheetRef.current?.present();
  }, []);

  const openRefundSheet = useCallback((order: Order) => {
    setRefundTarget(order);
    setRefundError(null);
    refundSheetRef.current?.present();
  }, []);

  const submitCancel = useCallback(
    (reason: string) => {
      const id = cancelTarget?.id;
      if (!id) return;
      cancelOrder.mutate(
        { id, reason },
        {
          onSuccess: () => {
            cancelSheetRef.current?.dismiss();
            setCancelTarget(null);
            void adminHaptics.actionSucceeded();
          },
          onError: () => {
            setCancelError(CANCEL_ERROR);
            void adminHaptics.actionFailed();
          },
        },
      );
    },
    [cancelTarget, cancelOrder],
  );

  const submitRefund = useCallback(
    ({ amount, refundRequestId }: { amount?: number; refundRequestId: string }) => {
      const id = refundTarget?.id;
      if (!id) return;
      refundOrder.mutate(
        { id, body: { amount, refund_request_id: refundRequestId } },
        {
          onSuccess: () => {
            refundSheetRef.current?.dismiss();
            setRefundTarget(null);
            void adminHaptics.actionSucceeded();
          },
          onError: () => {
            setRefundError(REFUND_ERROR);
            void adminHaptics.actionFailed();
          },
        },
      );
    },
    [refundTarget, refundOrder],
  );

  const submitEmailLabel = useCallback(
    (recipient: string) => {
      if (!emailTarget) return;
      emailLabel.mutate(
        { ...emailTarget, recipient },
        {
          onSuccess: () => void adminHaptics.actionSucceeded(),
          onError: () => void adminHaptics.actionFailed(),
        },
      );
    },
    [emailTarget, emailLabel],
  );

  /**
   * Swipe actions for one order, gated on what is actually legal for it.
   *
   * Confirming an order that is already confirmed, or cancelling one that is
   * fulfilled, is a guaranteed 4xx — and in an app with no undo, an armed
   * gesture that can only fail is worse than no gesture. A terminal order
   * therefore gets no `SwipeRow` at all rather than an empty one (the same
   * shape the Dashboard uses for its low-stock rows).
   *
   * Cancel does NOT fire a mutation: `ordersApi.cancel(id, reason)` has a
   * REQUIRED reason, so it opens `CancelReasonSheet` exactly as the order
   * detail screen does. A revealed action opening a sheet is fully within
   * the `SwipeRow` contract — a revealed action is always tapped, never
   * auto-fired.
   */
  const actionsFor = useCallback(
    (order: Order): { leading?: SwipeAction[]; trailing?: SwipeAction[] } | null => {
      if (TERMINAL_STATUSES.has(order.status)) return null;

      const leading: SwipeAction[] =
        order.status === "pending"
          ? [
              {
                key: "approve",
                label: "Approve",
                tone: "accent",
                icon: <Check size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
                onPress: () => {
                  markBusy(order.id);
                  confirmOrder.mutate({ id: order.id }, settleCallbacks(order.id));
                },
              },
            ]
          : [];

      return {
        leading,
        trailing: [
          {
            key: "cancel",
            label: "Cancel",
            tone: "danger",
            icon: <X size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
            onPress: () => openCancelSheet(order),
          },
        ],
      };
    },
    [confirmOrder, markBusy, settleCallbacks, openCancelSheet],
  );

  /**
   * The long-press menu. ALWAYS these four items, in this order — illegal
   * ones are disabled rather than dropped, so the sheet never resizes under
   * the merchant's thumb and the menu reads the same on every order. See
   * `ActionSheetItem.disabled`.
   *
   * Fulfil is the only one that fires a mutation directly: it needs no extra
   * input. The other three open the sheets the order detail screen already
   * uses, because each has required input the API will otherwise 400 on.
   */
  const menuItems = useMemo((): ActionSheetItem[] => {
    const target = menuOrder;
    if (!target) return [];
    const isTerminal = TERMINAL_STATUSES.has(target.status);
    return [
      {
        key: "fulfil",
        label: "Fulfil",
        disabled: target.status !== "confirmed",
        onPress: () => {
          markBusy(target.id);
          fulfillOrder.mutate(target.id, settleCallbacks(target.id));
        },
      },
      {
        key: "email-label",
        label: "Email label",
        // The label belongs to a SHIPMENT, whose id the orders list payload
        // does not carry — it is fetched lazily for this order only (see
        // `probeOrderId`). Disabled until one exists, which also covers the
        // orders that simply have no shipment yet.
        disabled: !shipment,
        onPress: () => {
          if (!shipment) return;
          setEmailTarget({ orderId: target.id, shipmentId: shipment.id });
          emailSheetRef.current?.present();
        },
      },
      {
        key: "refund",
        label: "Refund",
        disabled: !REFUNDABLE_PAYMENT_STATUSES.has(target.payment_status),
        onPress: () => openRefundSheet(target),
      },
      {
        key: "cancel",
        label: "Cancel order",
        tone: "danger",
        disabled: isTerminal,
        onPress: () => openCancelSheet(target),
      },
    ];
  }, [
    menuOrder,
    shipment,
    fulfillOrder,
    markBusy,
    settleCallbacks,
    openRefundSheet,
    openCancelSheet,
  ]);

  const renderItem = useCallback(
    ({ item, index }: { item: Order; index: number }) => {
      const actions = actionsFor(item);
      const row = (
        <OrderRow
          order={item}
          onPress={handleOrderPress}
          onLongPress={setMenuOrder}
          currencyCode={currencyCode}
        />
      );
      return (
        <View>
          {index > 0 ? <Hairline inset={theme.spacing.xl} /> : null}
          {actions ? (
            <SwipeRow
              testID={`swipe-${item.id}`}
              leadingActions={actions.leading}
              trailingActions={actions.trailing}
              // Suppressed while THIS row's own request is open, so a
              // still-visible row can't be fired at twice. Not a claim about
              // the data — see the screen's doc comment.
              enabled={!busyOrderIds.has(item.id)}
            >
              {row}
            </SwipeRow>
          ) : (
            row
          )}
        </View>
      );
    },
    [actionsFor, handleOrderPress, currencyCode, busyOrderIds],
  );

  const showError = listQuery.isError && orders.length === 0;

  return (
    <Screen>
      <CollapsingHeader
        eyebrow="Orders"
        title="Inbox"
        rightSlot={
          pendingCount > 0 ? (
            <Text
              preset="caption"
              color="textTertiary"
              style={styles.pendingCount}
              numberOfLines={1}
              testID="orders-pending-count"
            >
              {pendingCount} pending
            </Text>
          ) : undefined
        }
        scrollY={scrollY}
      />

      {/* Pinned, above the filter row: search then scope, the order iOS's own
          search + scope bar uses and the order products/customers already use
          here. Sits outside the list, so it never scrolls away. */}
      <View style={styles.searchBlock} testID="orders-search-block">
        <SearchField
          value={searchText}
          onChangeText={setSearchText}
          placeholder="Search orders…"
          accessibilityLabel="Search orders"
        />
      </View>

      <FilterChips<FilterKey>
        chips={FILTERS}
        value={activeFilter}
        onChange={handleFilterChange}
        contentContainerStyle={styles.chips}
      />

      {listQuery.isLoading && orders.length === 0 ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : showError ? (
        <View style={styles.centered}>
          <EmptyState
            title="Couldn't load orders"
            message="Something went wrong. Check your connection and try again."
            action={{
              label: "Try again",
              onPress: () => {
                listQuery.refetch();
              },
            }}
          />
        </View>
      ) : (
        <Animated.FlatList
          testID="orders-list"
          data={orders}
          renderItem={renderItem}
          keyExtractor={(item) => (item as Order).id}
          onScroll={scrollHandler}
          scrollEventThrottle={16}
          // No `ListHeaderComponent` and no `contentOffset`: search is
          // pinned above the list now, so the list starts at the top and
          // shows rows from its first pixel.
          //
          // Explicit flex, not leftover space: the list is the only child of
          // `Screen` that should absorb the remaining height.
          style={styles.listFlex}
          contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
          onEndReached={handleEndReached}
          onEndReachedThreshold={0.5}
          keyboardShouldPersistTaps="handled"
          refreshControl={
            <RefreshControl
              refreshing={isRefreshing}
              onRefresh={refresh}
              tintColor={theme.colors.text}
            />
          }
          ListFooterComponent={
            listQuery.isFetchingNextPage ? (
              <View style={styles.footer}>
                <ActivityIndicator size="small" color={theme.colors.text} />
              </View>
            ) : null
          }
          ListEmptyComponent={
            <EmptyState
              title="No orders found"
              message={
                debouncedSearch
                  ? "Try a different search term."
                  : "Orders will appear here once placed."
              }
            />
          }
        />
      )}

      <ActionSheet
        title={menuOrder ? `Order #${menuOrder.order_number}` : undefined}
        items={menuItems}
        visible={menuOrder !== null}
        onDismiss={() => setMenuOrder(null)}
      />

      <CancelReasonSheet
        ref={cancelSheetRef}
        onSubmit={submitCancel}
        isSubmitting={cancelOrder.isPending}
        hasShipment={Boolean(shipment)}
        carrier={shipment?.provider}
        error={cancelError}
        // Released on EVERY close, not only on a successful cancel. Backing
        // out of the sheet used to leave this order pinned for the life of
        // the screen — and because `probeOrderId` reads it, the next order's
        // refund sheet then warned about THIS order's carrier shipment.
        onDismiss={() => setCancelTarget(null)}
      />
      <RefundSheet
        ref={refundSheetRef}
        onSubmit={submitRefund}
        isSubmitting={refundOrder.isPending}
        hasShipment={Boolean(shipment)}
        refundableAmount={Math.max(
          (refundTarget?.grand_total ?? 0) - (refundTarget?.refunded_amount ?? 0),
          0,
        )}
        currencyCode={refundTarget?.currency_code || currencyCode}
        error={refundError}
        // Same contract as the cancel sheet above.
        onDismiss={() => setRefundTarget(null)}
      />
      <EmailLabelSheet
        ref={emailSheetRef}
        onSubmit={submitEmailLabel}
        isSubmitting={emailLabel.isPending}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  // Padding, NOT a fixed height. The block used to be pinned to an exact
  // SEARCH_BLOCK_HEIGHT because the list's `contentOffset` had to match it
  // to the pixel; with the field pinned, nothing depends on its height, so
  // it hugs `SearchField` and grows freely if the field ever does. One less
  // fixed height is one less silent-clipping trap at raised text sizes.
  searchBlock: {
    // Screen gutter: theme.spacing.xl (20) — the SAME left edge the header's
    // eyebrow/title, the filter chips and every row's paddingH sit on.
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xs,
  },
  chips: { paddingVertical: theme.spacing.sm },
  listFlex: { flex: 1 },
  list: { flexGrow: 1 },
  footer: { paddingVertical: theme.spacing.lg, alignItems: "center" },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  pendingCount: { fontVariant: ["tabular-nums"] },
});
