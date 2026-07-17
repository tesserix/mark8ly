import { View, TouchableOpacity, StyleSheet } from "react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { OrderStatusBadges } from "@/components/orders/OrderStatusBadges";
import type { Order } from "@repo/mobile-shared/api/types";

interface OrderRowProps {
  order: Order;
  onPress: (order: Order) => void;
  /** The active store's currency code (e.g. "AUD"). */
  currencyCode?: string;
}

function formatRelativeTime(dateString: string): string {
  const now = Date.now();
  const date = new Date(dateString).getTime();
  const diffMin = Math.floor((now - date) / 60_000);
  const diffHr = Math.floor((now - date) / 3_600_000);
  const diffDay = Math.floor((now - date) / 86_400_000);
  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHr < 24) return `${diffHr}h ago`;
  if (diffDay < 30) return `${diffDay}d ago`;
  return new Date(dateString).toLocaleDateString("en-AU");
}

export function OrderRow({ order, onPress, currencyCode }: OrderRowProps) {
  const currency = order.currency_code || currencyCode;
  const displayName = order.customer_name || order.customer_email;
  const total = formatMoney(order.grand_total, currency);

  return (
    <TouchableOpacity
      style={styles.container}
      onPress={() => onPress(order)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`Order ${order.order_number}, ${displayName}, ${total}, ${order.status}`}
    >
      <View style={styles.topRow}>
        <Text preset="bodyEmphasis" color="text">
          #{order.order_number}
        </Text>
        <OrderStatusBadges status={order.status} paymentStatus={order.payment_status} />
      </View>
      <Text preset="caption" color="textSecondary" numberOfLines={1} style={styles.customer}>
        {displayName}
      </Text>
      <View style={styles.bottomRow}>
        <Text preset="bodyEmphasis" color="text">
          {total}
        </Text>
        <Text preset="caption" color="textTertiary">
          {formatRelativeTime(order.placed_at)}
        </Text>
      </View>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  container: {
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
    gap: theme.spacing.xs,
  },
  topRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    gap: theme.spacing.sm,
  },
  customer: { marginTop: 2 },
  bottomRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginTop: 2,
  },
});
