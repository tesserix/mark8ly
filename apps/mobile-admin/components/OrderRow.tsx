import { View, TouchableOpacity, StyleSheet } from "react-native";
import { StatusBadge, Text, type StatusTone } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Order } from "@repo/mobile-shared/api/types";

interface OrderRowProps {
  order: Order;
  onPress: (order: Order) => void;
}

const STATUS_TONE: Record<string, StatusTone> = {
  pending: "warning",
  confirmed: "success",
  fulfilled: "neutral",
  cancelled: "danger",
};

function formatCurrency(amount: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
  }).format(amount);
}

function formatRelativeTime(dateString: string): string {
  const now = Date.now();
  const date = new Date(dateString).getTime();
  const diffMs = now - date;
  const diffMin = Math.floor(diffMs / 60_000);
  const diffHr = Math.floor(diffMs / 3_600_000);
  const diffDay = Math.floor(diffMs / 86_400_000);

  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHr < 24) return `${diffHr}h ago`;
  if (diffDay < 30) return `${diffDay}d ago`;
  return new Date(dateString).toLocaleDateString();
}

export function OrderRow({ order, onPress }: OrderRowProps) {
  const tone = STATUS_TONE[order.status] ?? "neutral";
  const displayName = order.customer_name || order.customer_email;

  return (
    <TouchableOpacity
      style={styles.container}
      onPress={() => onPress(order)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`Order ${order.order_number}, ${displayName}, ${formatCurrency(order.grand_total)}, ${order.status}`}
    >
      <View style={styles.topRow}>
        <Text preset="bodyEmphasis" color="text">
          #{order.order_number}
        </Text>
        <StatusBadge label={order.status} tone={tone} />
      </View>
      <Text preset="caption" color="textSecondary" numberOfLines={1} style={styles.customer}>
        {displayName}
      </Text>
      <View style={styles.bottomRow}>
        <Text preset="bodyEmphasis" color="text">
          {formatCurrency(order.grand_total)}
        </Text>
        <Text preset="caption" color="textTertiary">
          {formatRelativeTime(order.created_at)}
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
  },
  customer: { marginTop: 2 },
  bottomRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginTop: 2,
  },
});
