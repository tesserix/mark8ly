import { View, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { PressableRow, StatusBadge, Text } from "@/components/ui";
import { orderStatusTone } from "@/components/orders/OrderStatusBadges";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { formatRelativeTime } from "@/lib/relative-time";
import type { RecentOrder } from "@repo/mobile-shared/api/types";

interface DashboardOrderRowProps {
  order: RecentOrder;
  onPress: () => void;
  currencyCode?: string;
}

function titleize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

/**
 * A recent/awaiting-approval order row for the dashboard. Unlike the plain
 * dashboard list rows it carries the two things the old dashboard threw away:
 * the order status (as a badge) and how long ago it was placed — so a merchant
 * can tell at a glance which orders are new and waiting on them.
 */
export function DashboardOrderRow({
  order,
  onPress,
  currencyCode,
}: DashboardOrderRowProps) {
  const total = formatMoney(order.grand_total, currencyCode);
  const placed = formatRelativeTime(order.created_at);

  return (
    <PressableRow
      lines={1}
      onPress={onPress}
      testID={`dashboard-order-row-${order.id}`}
      accessibilityLabel={`Order ${order.order_number}, ${order.customer_email}, ${total}, ${order.status}, placed ${placed}`}
    >
      <View style={styles.main}>
        <View style={styles.topRow}>
          <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
            #{order.order_number}
          </Text>
          <StatusBadge label={titleize(order.status)} tone={orderStatusTone(order.status)} />
        </View>
        <Text preset="caption" color="textTertiary" numberOfLines={1}>
          {order.customer_email}
        </Text>
        <View style={styles.bottomRow}>
          <Text preset="bodyEmphasis" color="text">
            {total}
          </Text>
          {placed ? (
            <Text preset="caption" color="textTertiary">
              {placed}
            </Text>
          ) : null}
        </View>
      </View>
      <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  main: { flex: 1, gap: 3 },
  topRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: theme.spacing.sm,
  },
  bottomRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    marginTop: 1,
  },
});
