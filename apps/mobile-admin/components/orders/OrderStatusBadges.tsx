import { View, StyleSheet } from "react-native";
import { StatusBadge, type StatusTone } from "@/components/ui";
import { theme } from "@/lib/theme";

/**
 * The three orthogonal order axes (order.status.go): order status,
 * payment status, fulfillment status. Tones are functional — moss-tint
 * `success` for the "done/paid" states, amber-tint `warning` for pending,
 * `danger` for failed/cancelled, `muted`/`info` for the neutral states — so
 * no single view spends more than the restrained set of status colours.
 *
 * `fulfilled → success` is deliberate and correct. The spec's Guardrails
 * permit moss-TINT success badges generally ("never a solid moss fill"); the
 * "one accent per view" constraint is scoped to the Dashboard, where the moss
 * goes to the revenue chart and the Approve swipe. On Orders' Completed tab
 * there is no competing moss action at all — terminal orders get no swipe
 * (see `TERMINAL_STATUSES` in app/(tabs)/orders/index.tsx) — so the tint is
 * the only moss in the view and reads as the status it is.
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
