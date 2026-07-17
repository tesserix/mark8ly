import { View, StyleSheet } from "react-native";
import { StatusBadge, type StatusTone } from "@/components/ui";
import { theme } from "@/lib/theme";

/**
 * The three orthogonal order axes (order.status.go): order status,
 * payment status, fulfillment status. Tones are functional — moss-tint
 * `success` for the "done/paid" states, amber-tint `warning` for pending,
 * `danger` for failed/cancelled, `muted`/`info` for the neutral states — so
 * no single view spends more than the restrained set of status colours.
 */
const ORDER_TONE: Record<string, StatusTone> = {
  pending: "warning",
  confirmed: "info",
  fulfilled: "success",
  cancelled: "danger",
};

const PAYMENT_TONE: Record<string, StatusTone> = {
  pending: "warning",
  authorized: "info",
  paid: "success",
  failed: "danger",
  refunded: "muted",
  partially_refunded: "muted",
};

const FULFILLMENT_TONE: Record<string, StatusTone> = {
  unfulfilled: "muted",
  partial: "info",
  fulfilled: "success",
};

function titleize(value: string): string {
  return value
    .split("_")
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

export function orderStatusTone(status: string): StatusTone {
  return ORDER_TONE[status] ?? "muted";
}

interface OrderStatusBadgesProps {
  status: string;
  paymentStatus?: string;
  fulfillmentStatus?: string;
}

/** All axes passed → renders each badge; the row wraps. */
export function OrderStatusBadges({
  status,
  paymentStatus,
  fulfillmentStatus,
}: OrderStatusBadgesProps) {
  return (
    <View style={styles.row}>
      <StatusBadge label={titleize(status)} tone={orderStatusTone(status)} />
      {paymentStatus ? (
        <StatusBadge label={titleize(paymentStatus)} tone={PAYMENT_TONE[paymentStatus] ?? "muted"} />
      ) : null}
      {fulfillmentStatus ? (
        <StatusBadge
          label={titleize(fulfillmentStatus)}
          tone={FULFILLMENT_TONE[fulfillmentStatus] ?? "muted"}
        />
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", flexWrap: "wrap", gap: theme.spacing.xs },
});
