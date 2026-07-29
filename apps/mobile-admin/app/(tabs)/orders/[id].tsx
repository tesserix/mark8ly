import { useCallback, useMemo, useRef, useState } from "react";
import {
  Platform,
  View,
  ScrollView,
  useWindowDimensions,
  Pressable,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { MoreHorizontal } from "lucide-react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { useOrder } from "../../../lib/hooks/use-orders";
import {
  useConfirmOrder,
  useFulfillOrder,
  useCancelOrder,
  useRefundOrder,
} from "../../../lib/admin-api/order-actions";
import {
  ActionFailureNotice,
  ActionSheet,
  BackHeader,
  Eyebrow,
  Hairline,
  IconButton,
  Screen,
  StickyActionBar,
  STICKY_BAR_CONTENT_HEIGHT,
  MAX_FONT_SCALE,
  Text,
  useStickyBarHeight,
} from "@/components/ui";
import type { ActionSheetItem } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { useActionFailure } from "@/lib/use-action-failure";
import { OrderStatusBadges } from "@/components/orders/OrderStatusBadges";
import { CancelReasonSheet, type CancelReasonSheetHandle } from "@/components/orders/CancelReasonSheet";
import { RefundSheet, type RefundSheetHandle } from "@/components/orders/RefundSheet";
import { ShippingPanel } from "@/components/orders/ShippingPanel";
import { useEmailInvoice, useEmailReceipt } from "@/lib/admin-api/shipment-actions";
import { useShipment } from "@/lib/hooks/use-shipment";
import { useApiClient } from "@/lib/api-client";
import { ApiError } from "@repo/mobile-shared/api/client";
import { createShipmentsApi } from "@repo/mobile-shared/api/shipments";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import type { OrderItem, OrderAddress } from "@repo/mobile-shared/api/types";
// NOT `useDockClearance()` for scroll padding: that helper sizes content
// clearance for the dock alone, and this screen's content now has to clear
// the sticky action bar the dock sits under as well. The two terms are
// composed explicitly below. `useDockClearance()` IS used further down, but
// only to work out how far `ActionFailureNotice`'s own anchoring differs from
// this screen's — see `noticeOffset`.
import {
  DOCK_BOTTOM_GAP,
  DOCK_HEIGHT,
  useDockClearance,
} from "@/components/navigation/dock-metrics";

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-AU", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

const LIFECYCLE = ["pending", "confirmed", "fulfilled"] as const;
const LIFECYCLE_LABEL: Record<string, string> = {
  pending: "Placed",
  confirmed: "Confirmed",
  fulfilled: "Fulfilled",
};

function Lifecycle({ status }: { status: string }) {
  if (status === "cancelled") {
    return (
      <View style={styles.lifecycleRow}>
        <Text preset="caption" color="danger">
          Cancelled
        </Text>
      </View>
    );
  }
  const activeIndex = LIFECYCLE.indexOf(status as (typeof LIFECYCLE)[number]);
  return (
    <View style={styles.lifecycleRow}>
      {LIFECYCLE.map((step, i) => {
        const done = i <= activeIndex;
        return (
          <View key={step} style={styles.lifecycleStep}>
            <View style={[styles.lifecycleDot, done ? styles.lifecycleDotDone : null]} />
            <Text preset="caption" color={done ? "text" : "textTertiary"}>
              {LIFECYCLE_LABEL[step]}
            </Text>
          </View>
        );
      })}
    </View>
  );
}

function ItemRow({ item, currency }: { item: OrderItem; currency: string }) {
  return (
    <View style={styles.item}>
      <View style={styles.itemInfo}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={2}>
          {item.title_snapshot}
        </Text>
        {item.option_summary ? (
          <Text preset="caption" color="textTertiary">
            {item.option_summary}
          </Text>
        ) : null}
        <Text preset="caption" color="textSecondary">
          {item.quantity} × {formatMoney(item.unit_price, currency)} · {item.sku_snapshot}
        </Text>
      </View>
      <Text preset="bodyEmphasis" color="text">
        {formatMoney(item.line_total, currency)}
      </Text>
    </View>
  );
}

function TotalRow({ label, value, emphasis }: { label: string; value: string; emphasis?: boolean }) {
  return (
    <View style={styles.totalRow}>
      <Text preset={emphasis ? "bodyEmphasis" : "body"} color={emphasis ? "text" : "textSecondary"}>
        {label}
      </Text>
      <Text preset={emphasis ? "h3" : "body"} color="text">
        {value}
      </Text>
    </View>
  );
}

function AddressBlock({ address }: { address: OrderAddress }) {
  const cityLine = [address.city, [address.region, address.postal_code].filter(Boolean).join(" ")]
    .filter(Boolean)
    .join(", ");
  const lines = [address.line1, address.line2, cityLine, address.country_code, address.phone].filter(
    (l): l is string => Boolean(l),
  );
  return (
    <View style={styles.addressBlock}>
      <Text preset="caption" color="textTertiary">
        {address.kind === "billing" ? "Billing" : "Shipping"}
      </Text>
      <Text preset="bodyEmphasis" color="text">
        {address.name}
      </Text>
      {lines.map((line, i) => (
        <Text key={i} preset="body" color="textSecondary">
          {line}
        </Text>
      ))}
    </View>
  );
}

/** The terminal-state caption that stands in for an action when none is legal. */
const TERMINAL_CAPTION: Record<string, string> = {
  fulfilled: "Order fulfilled",
  cancelled: "Order cancelled",
};

export default function OrderDetailScreen() {
  const insets = useSafeAreaInsets();
  const { fontScale } = useWindowDimensions();
  // The bar sits ABOVE the floating dock, so its own offset is the dock's
  // full footprint plus a hairline gap.
  const barBottom = insets.bottom + DOCK_BOTTOM_GAP + DOCK_HEIGHT + theme.spacing.xs;
  const estimatedBarHeight = useStickyBarHeight();
  // `stickyBarHeightFor` assumes a single scaled line box. A primary label
  // that wraps at accessibility sizes makes the real bar taller, and an
  // under-estimate hides the last row of content behind it — silently. Take
  // the larger of the estimate and what the bar actually measured, so the
  // padding can only ever grow, never shrink under the merchant's thumb.
  const [measuredBarHeight, setMeasuredBarHeight] = useState(0);
  const barHeight = Math.max(estimatedBarHeight, measuredBarHeight);
  const scrollPad = barBottom + barHeight + theme.spacing.md;
  // `ActionFailureNotice` anchors itself at `useDockClearance() + bottomOffset`
  // — a DIFFERENT base than `barBottom` above, built from the same dock
  // numbers plus a different trailing gap (12 vs `theme.spacing.xs`). Passing
  // the bar height straight through as `bottomOffset` lands the strip on top
  // of the bar rather than above it. Measuring the gap between the two live
  // values, rather than hand-deriving the constant the two bases differ by,
  // keeps this correct if either formula ever changes.
  const dockClearance = useDockClearance();
  const noticeOffset = Math.max(0, barBottom + barHeight - dockClearance) + theme.spacing.sm;
  // The primary slot's floor. Scaled, because `bodyEmphasis` is 32pt at the
  // app's 2× cap and a 48pt box cannot hold it — the eighth instance of that
  // exact defect is the one this task also had to fix in the sheets. Shared
  // by the button AND the caption so the bar cannot change height with the
  // order's state.
  const primaryMinHeight =
    STICKY_BAR_CONTENT_HEIGHT * Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
  const [menuOpen, setMenuOpen] = useState(false);
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: order, isLoading, error } = useOrder(id);
  const storeCurrency = useTenantStore((s) => s.activeStore?.currency_code);
  const { data: shipment } = useShipment(id);
  const apiClient = useApiClient();

  const confirmMutation = useConfirmOrder();
  const fulfillMutation = useFulfillOrder();
  const cancelMutation = useCancelOrder();
  const refundMutation = useRefundOrder();
  const emailInvoiceMutation = useEmailInvoice();
  const emailReceiptMutation = useEmailReceipt();
  const cancelSheetRef = useRef<CancelReasonSheetHandle>(null);
  const refundSheetRef = useRef<RefundSheetHandle>(null);
  const [cancelError, setCancelError] = useState<string | null>(null);
  const [refundError, setRefundError] = useState<string | null>(null);
  // Confirm and Fulfil were the two mutations on this screen with no failure
  // surface at all — see i3-task-18-brief.md. Cancel and Refund keep their
  // own inline sheet errors (the typed reason has to stay on screen beside
  // the message); this is for the two that fire directly off the bar.
  const { failure, reportFailure, clearFailure } = useActionFailure();

  const isMutating =
    confirmMutation.isPending ||
    fulfillMutation.isPending ||
    cancelMutation.isPending ||
    refundMutation.isPending;

  // Shared by both Confirm buttons AND Fulfil: same action class, same
  // outcome, only the phrase and mutation differ.
  const confirmCallbacks = useCallback(
    () => ({
      onSuccess: () => {
        clearFailure();
        void adminHaptics.actionSucceeded();
      },
      onError: (error?: unknown) => {
        reportFailure(error, "confirm this order");
        void adminHaptics.actionFailed();
      },
    }),
    [reportFailure, clearFailure],
  );

  const handleConfirm = useCallback(() => {
    Alert.alert("Confirm order", "Mark this order as confirmed?", [
      { text: "Cancel", style: "cancel" },
      { text: "Confirm", onPress: () => confirmMutation.mutate({ id }, confirmCallbacks()) },
      {
        text: "Confirm & mark paid",
        onPress: () =>
          confirmMutation.mutate({ id, body: { payment_status: "paid" } }, confirmCallbacks()),
      },
    ]);
    // `confirmMutation` is a new object every render; `.mutate` is stable.
    // `confirmCallbacks` is itself a stable `useCallback`.
  }, [id, confirmMutation.mutate, confirmCallbacks]);

  const handleFulfill = useCallback(() => {
    Alert.alert("Mark fulfilled", "Mark this order as fulfilled?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Fulfill",
        onPress: () =>
          fulfillMutation.mutate(id, {
            onSuccess: () => {
              clearFailure();
              void adminHaptics.actionSucceeded();
            },
            onError: (error?: unknown) => {
              reportFailure(error, "fulfil this order");
              void adminHaptics.actionFailed();
            },
          }),
      },
    ]);
    // `fulfillMutation` is a new object every render; `.mutate` is stable.
  }, [id, fulfillMutation.mutate, reportFailure, clearFailure]);

  const handleCancelSubmit = useCallback(
    (reason: string) => {
      setCancelError(null);
      cancelMutation.mutate(
        { id, reason },
        {
          onError: (err) => {
            const msg =
              err instanceof ApiError && err.status === 409
                ? "This order can no longer be cancelled — it may already be fulfilled or cancelled."
                : err instanceof ApiError
                  ? err.message
                  : "Couldn't cancel the order. Please try again.";
            setCancelError(msg);
            void adminHaptics.actionFailed();
          },
          onSuccess: async () => {
            void adminHaptics.actionSucceeded();
            cancelSheetRef.current?.dismiss();
            // The order cancel itself always succeeds independently of the
            // shipment — the server cancels/returns the shipment with the
            // carrier best-effort and records the outcome on the shipment
            // row (never on this response). Follow up with one read so a
            // carrier-side failure isn't silent. Best-effort: swallow — the
            // order cancel already went through, this is an informational
            // extra, and the shipment panel still shows the true state.
            try {
              const updated = await createShipmentsApi(apiClient).get(id);
              if (updated?.cancel_status === "failed" || updated?.cancel_status === "unsupported") {
                Alert.alert(
                  "Shipment needs attention",
                  updated.cancel_reason
                    ? `The order was cancelled, but the shipment couldn't be cancelled with the carrier: ${updated.cancel_reason}`
                    : "The order was cancelled, but the shipment couldn't be cancelled with the carrier. You may need to contact them directly.",
                );
              }
            } catch {
              // See comment above — this check is a bonus, not load-bearing.
            }
          },
        },
      );
    },
    // `cancelMutation` is a new object every render; `.mutate` is stable.
    [id, cancelMutation.mutate, apiClient],
  );

  const handleRefundSubmit = useCallback(
    ({ amount, refundRequestId }: { amount?: number; refundRequestId: string }) => {
      setRefundError(null);
      refundMutation.mutate(
        { id, body: { amount, refund_request_id: refundRequestId } },
        {
          onError: (err) => {
            const msg =
              err instanceof ApiError && err.status === 503
                ? "Refunds are not configured for this store yet."
                : err instanceof ApiError
                  ? err.message
                  : "Couldn't issue the refund. Please try again.";
            setRefundError(msg);
            void adminHaptics.actionFailed();
          },
          onSuccess: () => {
            void adminHaptics.actionSucceeded();
            refundSheetRef.current?.dismiss();
          },
        },
      );
    },
    // `refundMutation` is a new object every render; `.mutate` is stable.
    [id, refundMutation.mutate],
  );

  const handleEmailInvoice = useCallback(() => {
    emailInvoiceMutation.mutate(
      { orderId: id },
      {
        onSuccess: (res) =>
          Alert.alert("Invoice sent", `Emailed to ${res.recipient}.`),
        onError: (err) => {
          const msg =
            err instanceof ApiError && err.status === 422
              ? "This order has no customer email on file."
              : err instanceof ApiError
                ? err.message
                : "Couldn't send the invoice. Please try again.";
          Alert.alert("Couldn't send invoice", msg);
        },
      },
    );
    // `emailInvoiceMutation` is a new object every render; `.mutate` is
    // stable.
  }, [id, emailInvoiceMutation.mutate]);

  const handleEmailReceipt = useCallback(() => {
    emailReceiptMutation.mutate(
      { orderId: id },
      {
        onSuccess: (res) =>
          Alert.alert("Receipt sent", `Emailed to ${res.recipient}.`),
        onError: (err) => {
          const msg =
            err instanceof ApiError && err.status === 409
              ? "The receipt is available only after the shipment is delivered."
              : err instanceof ApiError && err.status === 422
                ? "This order has no customer email on file."
                : err instanceof ApiError
                  ? err.message
                  : "Couldn't send the receipt. Please try again.";
          Alert.alert("Couldn't send receipt", msg);
        },
      },
    );
    // `emailReceiptMutation` is a new object every render; `.mutate` is
    // stable.
  }, [id, emailReceiptMutation.mutate]);

  const openCancelSheet = useCallback(() => {
    setCancelError(null);
    cancelSheetRef.current?.present();
  }, []);

  const openRefundSheet = useCallback(() => {
    setRefundError(null);
    refundSheetRef.current?.present();
  }, []);

  /**
   * The overflow menu — ALWAYS these four items, in this order, with
   * `disabled` carrying legality.
   *
   * Identical construction to the Orders list menu and for the same reason:
   * `ActionSheet` derives its snap point from `items.length`, so dropping an
   * illegal action would resize the sheet between orders. A greyed row also
   * tells the merchant the action exists and why it isn't available; a
   * missing one tells them nothing.
   *
   * Neither Cancel nor Refund FIRES here — both open their sheet. Cancel
   * because `CancelOrderRequest.Reason` is `binding:"required"` server-side,
   * Refund because `refund_request_id` is the manual idempotency key and a
   * fresh one means a second real gateway refund.
   */
  const menuItems = useMemo<ActionSheetItem[]>(
    () => [
      {
        key: "refund",
        label: "Refund",
        disabled: !(
          order?.payment_status === "paid" || order?.payment_status === "partially_refunded"
        ),
        onPress: openRefundSheet,
      },
      // Never disabled: the server 422s when the order has no customer email
      // and `handleEmailInvoice` already Alerts that. Guessing at it here
      // would grey the row out on orders where it would have worked.
      { key: "invoice", label: "Email invoice", onPress: handleEmailInvoice },
      // Never disabled: the server 409s before delivery and
      // `handleEmailReceipt` already Alerts that.
      { key: "receipt", label: "Email receipt", onPress: handleEmailReceipt },
      {
        key: "cancel",
        label: "Cancel order",
        tone: "danger",
        disabled: order?.status === "cancelled" || order?.status === "fulfilled",
        onPress: openCancelSheet,
      },
    ],
    [
      order?.payment_status,
      order?.status,
      openRefundSheet,
      openCancelSheet,
      handleEmailInvoice,
      handleEmailReceipt,
    ],
  );

  if (error) {
    return (
      <Screen>
        <BackHeader eyebrow="ORDER" />
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load order
          </Text>
        </View>
      </Screen>
    );
  }

  if (isLoading || !order) {
    return (
      <Screen>
        <BackHeader eyebrow="ORDER" title="Loading…" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  const currency = order.currency_code || storeCurrency || "AUD";
  const refunded = order.refunded_amount > 0;

  return (
    <Screen>
      <BackHeader eyebrow="ORDER" title={`#${order.order_number}`} />
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: scrollPad }]}>
        <View style={styles.heading}>
          <Text preset="h1" color="text">
            #{order.order_number}
          </Text>
          <OrderStatusBadges
            status={order.status}
            paymentStatus={order.payment_status}
            fulfillmentStatus={order.fulfillment_status}
          />
          <Text preset="caption" color="textTertiary">
            {formatDate(order.placed_at)}
          </Text>
          <Lifecycle status={order.status} />
        </View>

        <Eyebrow label="Items" style={styles.section} />
        <View style={styles.card}>
          {order.items.map((item, i) => (
            <View key={item.id}>
              {i > 0 ? <Hairline /> : null}
              <ItemRow item={item} currency={currency} />
            </View>
          ))}
        </View>

        <Eyebrow label="Summary" style={styles.section} />
        <View style={styles.card}>
          <TotalRow label="Subtotal" value={formatMoney(order.subtotal, currency)} />
          <TotalRow label="Shipping" value={formatMoney(order.shipping_total, currency)} />
          <TotalRow label="Tax" value={formatMoney(order.tax_total, currency)} />
          {order.tax_lines?.map((t, i) => (
            <TotalRow key={i} label={`  ${t.description}`} value={formatMoney(t.amount, currency)} />
          ))}
          {order.discount_total > 0 ? (
            <TotalRow label="Discount" value={`−${formatMoney(order.discount_total, currency)}`} />
          ) : null}
          <Hairline />
          <TotalRow label="Total" value={formatMoney(order.grand_total, currency)} emphasis />
          {refunded ? (
            <TotalRow label="Refunded" value={`−${formatMoney(order.refunded_amount, currency)}`} />
          ) : null}
        </View>

        <Eyebrow label="Customer" style={styles.section} />
        <View style={styles.card}>
          {order.customer_name ? (
            <Text preset="bodyEmphasis" color="text">
              {order.customer_name}
            </Text>
          ) : null}
          <Text preset="body" color="textSecondary">
            {order.customer_email}
          </Text>
        </View>

        {order.addresses.length > 0 ? (
          <>
            <Eyebrow label="Addresses" style={styles.section} />
            <View style={styles.card}>
              {order.addresses.map((address, i) => (
                <View key={`${address.kind}-${i}`}>
                  {i > 0 ? <Hairline /> : null}
                  <AddressBlock address={address} />
                </View>
              ))}
            </View>
          </>
        ) : null}

        <ShippingPanel
          orderId={id}
          orderStatus={order.status}
          defaultCarrier={order.shipping_carrier}
          defaultService={order.shipping_service}
        />

        <Eyebrow label="Documents" style={styles.section} />
        {/* The two send buttons moved into the bar's overflow menu — their
            behaviour is unchanged, only their placement. The disclaimer stays
            here because it explains the document, not the control. */}
        <View style={styles.card}>
          <Text preset="caption" color="textTertiary">
            {emailInvoiceMutation.isPending || emailReceiptMutation.isPending
              ? "Sending…"
              : "Invoice and receipt are sent from the actions menu. The receipt sends only after the shipment is delivered."}
          </Text>
        </View>

      </ScrollView>

      {/* The bar's height is the SAME in every order state — when no action is
          legal the primary slot holds a caption in the same box rather than
          collapsing. A bar that appears and disappears reflows the screen
          under the merchant's thumb, and a `paddingBottom` that varies with
          state is the same class of defect as the silent clips this app keeps
          shipping. */}
      <StickyActionBar
        bottom={barBottom}
        onHeightChange={setMeasuredBarHeight}
        testID="order-action-bar"
      >
        {order.status === "pending" ? (
          <PrimaryAction
            label={confirmMutation.isPending ? "Confirming…" : "Confirm order"}
            onPress={handleConfirm}
            disabled={isMutating}
            minHeight={primaryMinHeight}
          />
        ) : order.status === "confirmed" ? (
          <PrimaryAction
            label={fulfillMutation.isPending ? "Fulfilling…" : "Mark fulfilled"}
            onPress={handleFulfill}
            disabled={isMutating}
            minHeight={primaryMinHeight}
          />
        ) : (
          /* A caption, NOT a disabled button: a disabled button still
             announces as "button, dimmed" and invites a tap that can never
             do anything. This is a statement of fact about the order. */
          <View
            testID="order-terminal-caption"
            style={[styles.primarySlot, { minHeight: primaryMinHeight }]}
          >
            <Text preset="bodyEmphasis" color="textSecondary" align="center">
              {TERMINAL_CAPTION[order.status] ?? `Order ${order.status}`}
            </Text>
          </View>
        )}
        <IconButton
          accessibilityLabel="More order actions"
          onPress={() => setMenuOpen(true)}
          testID="order-actions-overflow"
        >
          <MoreHorizontal size={20} color={theme.colors.text} />
        </IconButton>
      </StickyActionBar>

      {/* Confirm/Fulfil's only failure surface. `noticeOffset` lifts it clear
          of the sticky bar above — the strip's own default anchoring
          (`useDockClearance()`) knows nothing about that bar. */}
      <ActionFailureNotice failure={failure} onDismiss={clearFailure} bottomOffset={noticeOffset} />

      <ActionSheet
        title={`Order #${order.order_number}`}
        items={menuItems}
        visible={menuOpen}
        onDismiss={() => setMenuOpen(false)}
      />

      <CancelReasonSheet
        ref={cancelSheetRef}
        onSubmit={handleCancelSubmit}
        isSubmitting={cancelMutation.isPending}
        hasShipment={!!shipment}
        carrier={shipment?.provider}
        error={cancelError}
        onDismiss={() => setCancelError(null)}
      />
      <RefundSheet
        ref={refundSheetRef}
        onSubmit={handleRefundSubmit}
        isSubmitting={refundMutation.isPending}
        hasShipment={!!shipment}
        refundableAmount={Math.max(order.grand_total - order.refunded_amount, 0)}
        currencyCode={currency}
        error={refundError}
        onDismiss={() => setRefundError(null)}
      />
    </Screen>
  );
}

/**
 * The bar's primary action.
 *
 * `minHeight`, never `height`: at accessibility text sizes "Mark fulfilled"
 * is a ~45pt line box (and wraps to ~90pt on a narrow device), and a 48pt
 * fixed box clips it. The caller passes the scaled floor so this box and the
 * terminal caption that replaces it are always the same height.
 */
function PrimaryAction({
  label,
  onPress,
  disabled,
  minHeight,
}: {
  label: string;
  onPress: () => void;
  disabled?: boolean;
  minHeight: number;
}) {
  // NativeWind's JSX interop doesn't resolve a function `style` prop the way
  // it resolves a plain array — press state is tracked explicitly instead.
  const [pressed, setPressed] = useState(false);
  return (
    <Pressable
      onPress={onPress}
      onPressIn={() => setPressed(true)}
      onPressOut={() => setPressed(false)}
      disabled={disabled}
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ disabled: !!disabled }}
      android_ripple={theme.press.rippleOnDark}
      testID="order-primary-action"
      style={[
        styles.primarySlot,
        styles.btnPrimary,
        { minHeight },
        disabled ? styles.btnDisabled : null,
        pressed && !disabled && Platform.OS === "ios"
          ? { opacity: theme.press.opacitySolidFill }
          : null,
      ]}
    >
      <Text preset="bodyEmphasis" color="inverse" align="center">
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingBottom: theme.spacing.huge },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  heading: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.lg,
    gap: theme.spacing.sm,
  },
  lifecycleRow: { flexDirection: "row", gap: theme.spacing.lg, marginTop: theme.spacing.xs },
  lifecycleStep: { flexDirection: "row", alignItems: "center", gap: 6 },
  lifecycleDot: { width: 8, height: 8, borderRadius: 4, backgroundColor: theme.colors.border },
  lifecycleDotDone: { backgroundColor: theme.colors.text },
  section: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.xl },
  card: { paddingHorizontal: theme.spacing.lg, marginTop: theme.spacing.sm },
  item: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "flex-start",
    paddingVertical: theme.spacing.md,
    gap: theme.spacing.md,
  },
  itemInfo: { flex: 1, gap: 2 },
  totalRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: theme.spacing.xs,
  },
  addressBlock: { paddingVertical: theme.spacing.md, gap: 2 },
  // The one box the bar's primary slot uses, whichever of the two things is
  // in it. `minHeight` arrives from the caller (font-scaled) — there is
  // deliberately no `height` here.
  primarySlot: {
    flex: 1,
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.md,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnPrimary: { backgroundColor: theme.colors.accent },
  btnDisabled: { opacity: 0.5 },
});
