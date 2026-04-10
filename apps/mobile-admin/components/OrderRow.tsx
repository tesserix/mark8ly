import { View, Text, TouchableOpacity, StyleSheet } from "react-native";
import { theme } from "@/lib/theme";
import type { Order } from "@repo/mobile-shared/api/types";

interface OrderRowProps {
  order: Order;
  onPress: (order: Order) => void;
}

const STATUS_COLORS: Record<string, string> = {
  pending: "#F59E0B",
  confirmed: theme.colors.accent,
  fulfilled: theme.colors.text,
  cancelled: theme.colors.danger,
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
  const badgeBg = STATUS_COLORS[order.status] ?? theme.colors.text;
  const displayName = order.customer_name || order.customer_email;

  return (
    <TouchableOpacity
      style={styles.container}
      onPress={() => onPress(order)}
      activeOpacity={0.7}
      accessibilityRole="button"
      accessibilityLabel={`Order ${order.order_number}, ${displayName}, ${formatCurrency(order.grand_total)}, ${order.status}`}
    >
      <View style={styles.topRow}>
        <Text style={styles.orderNumber}>{order.order_number}</Text>
        <View style={[styles.badge, { backgroundColor: badgeBg }]}>
          <Text style={styles.badgeText}>{order.status}</Text>
        </View>
      </View>
      <Text style={styles.customer} numberOfLines={1}>
        {displayName}
      </Text>
      <View style={styles.bottomRow}>
        <Text style={styles.total}>{formatCurrency(order.grand_total)}</Text>
        <Text style={styles.time}>{formatRelativeTime(order.created_at)}</Text>
      </View>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  container: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    padding: 14,
    marginHorizontal: theme.spacing.lg,
    marginBottom: theme.spacing.sm,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  topRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: theme.spacing.xs,
  },
  orderNumber: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.text,
  },
  badge: {
    paddingHorizontal: theme.spacing.sm,
    paddingVertical: 3,
    borderRadius: 4,
  },
  badgeText: {
    fontSize: 11,
    fontWeight: "600",
    color: theme.colors.elevated,
    textTransform: "capitalize",
  },
  customer: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.6,
    marginBottom: theme.spacing.sm,
  },
  bottomRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  total: {
    fontSize: 15,
    fontWeight: "700",
    color: theme.colors.text,
  },
  time: {
    fontSize: 12,
    color: theme.colors.text,
    opacity: 0.4,
  },
});
