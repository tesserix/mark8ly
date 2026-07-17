import { useCallback, useRef } from "react";
import {
  View,
  ScrollView,
  TouchableOpacity,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { useOrder } from "../../../lib/hooks/use-orders";
import {
  useConfirmOrder,
  useFulfillOrder,
  useCancelOrder,
  useRefundOrder,
} from "../../../lib/admin-api/order-actions";
import { BackHeader, Eyebrow, Hairline, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { OrderStatusBadges } from "@/components/orders/OrderStatusBadges";
import { CancelReasonSheet, type CancelReasonSheetHandle } from "@/components/orders/CancelReasonSheet";
import { RefundSheet, type RefundSheetHandle } from "@/components/orders/RefundSheet";
import { ApiError } from "@repo/mobile-shared/api/client";
import type { OrderItem, OrderAddress } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

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

export default function OrderDetailScreen() {
  const dockPad = useDockClearance();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: order, isLoading, error } = useOrder(id);
  const storeCurrency = useTenantStore((s) => s.activeStore?.currency_code);

  const confirmMutation = useConfirmOrder();
  const fulfillMutation = useFulfillOrder();
  const cancelMutation = useCancelOrder();
  const refundMutation = useRefundOrder();
  const cancelSheetRef = useRef<CancelReasonSheetHandle>(null);
  const refundSheetRef = useRef<RefundSheetHandle>(null);

  const isMutating =
    confirmMutation.isPending ||
    fulfillMutation.isPending ||
    cancelMutation.isPending ||
    refundMutation.isPending;

  const handleConfirm = useCallback(() => {
    Alert.alert("Confirm order", "Mark this order as confirmed?", [
      { text: "Cancel", style: "cancel" },
      { text: "Confirm", onPress: () => confirmMutation.mutate({ id }) },
      {
        text: "Confirm & mark paid",
        onPress: () => confirmMutation.mutate({ id, body: { payment_status: "paid" } }),
      },
    ]);
  }, [id, confirmMutation]);

  const handleFulfill = useCallback(() => {
    Alert.alert("Mark fulfilled", "Mark this order as fulfilled?", [
      { text: "Cancel", style: "cancel" },
      { text: "Fulfill", onPress: () => fulfillMutation.mutate(id) },
    ]);
  }, [id, fulfillMutation]);

  const handleCancelSubmit = useCallback(
    (reason: string) => cancelMutation.mutate({ id, reason }),
    [id, cancelMutation],
  );

  const handleRefundSubmit = useCallback(
    ({ amount, refundRequestId }: { amount?: number; refundRequestId: string }) => {
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
            Alert.alert("Refund failed", msg);
          },
        },
      );
    },
    [id, refundMutation],
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
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
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

        <View style={styles.actions}>
          {order.status === "pending" ? (
            <ActionButton
              variant="primary"
              label={confirmMutation.isPending ? "Confirming…" : "Confirm Order"}
              onPress={handleConfirm}
              disabled={isMutating}
            />
          ) : null}
          {order.status === "confirmed" ? (
            <ActionButton
              variant="primary"
              label={fulfillMutation.isPending ? "Fulfilling…" : "Mark Fulfilled"}
              onPress={handleFulfill}
              disabled={isMutating}
            />
          ) : null}
          {order.payment_status === "paid" || order.payment_status === "partially_refunded" ? (
            <ActionButton
              variant="secondary"
              label={refundMutation.isPending ? "Refunding…" : "Refund"}
              onPress={() => refundSheetRef.current?.present()}
              disabled={isMutating}
            />
          ) : null}
          {order.status !== "cancelled" && order.status !== "fulfilled" ? (
            <ActionButton
              variant="danger"
              label={cancelMutation.isPending ? "Cancelling…" : "Cancel Order"}
              onPress={() => cancelSheetRef.current?.present()}
              disabled={isMutating}
            />
          ) : null}
        </View>
      </ScrollView>

      <CancelReasonSheet
        ref={cancelSheetRef}
        onSubmit={handleCancelSubmit}
        isSubmitting={cancelMutation.isPending}
      />
      <RefundSheet
        ref={refundSheetRef}
        onSubmit={handleRefundSubmit}
        isSubmitting={refundMutation.isPending}
      />
    </Screen>
  );
}

function ActionButton({
  variant,
  label,
  onPress,
  disabled,
}: {
  variant: "primary" | "secondary" | "danger";
  label: string;
  onPress: () => void;
  disabled?: boolean;
}) {
  const btnStyle =
    variant === "primary"
      ? styles.btnPrimary
      : variant === "danger"
        ? styles.btnDanger
        : styles.btnSecondary;
  const color = variant === "primary" ? "inverse" : variant === "danger" ? "danger" : "text";
  return (
    <TouchableOpacity
      style={[btnStyle, disabled && styles.btnDisabled]}
      onPress={onPress}
      disabled={disabled}
      activeOpacity={0.85}
      accessibilityRole="button"
      accessibilityLabel={label}
    >
      <Text preset="bodyEmphasis" color={color}>
        {label}
      </Text>
    </TouchableOpacity>
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
  actions: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.xl,
    gap: theme.spacing.sm,
  },
  btnPrimary: {
    backgroundColor: theme.colors.accent,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnSecondary: {
    borderWidth: 1,
    borderColor: theme.colors.border,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnDanger: {
    borderWidth: 1,
    borderColor: theme.colors.danger,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnDisabled: { opacity: 0.5 },
});
