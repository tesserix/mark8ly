import { View, StyleSheet } from "react-native";
import { PressableRow, StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { orderStatusTone } from "@/components/orders/OrderStatusBadges";
import type { Order } from "@repo/mobile-shared/api/types";

interface OrderRowProps {
  order: Order;
  onPress: (order: Order) => void;
  /**
   * Opens the long-press action menu on the Orders screen. Optional so the
   * row stays usable on any list that has no per-row menu.
   */
  onLongPress?: (order: Order) => void;
  /** The active store's currency code (e.g. "AUD"). */
  currencyCode?: string;
}

function titleize(value: string): string {
  return value
    .split("_")
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
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

/**
 * A row in the Orders triage list — 88pt, two lines:
 *
 *  1. `#1042 · Ana Ruiz` at 17/600 (`bodyEmphasis`), status badge hard right.
 *  2. The total in serif tabular at `h3`, relative time hard right.
 *
 * Money is serif tabular so the eye tracks value down a column — the same
 * treatment the Dashboard's queue rows use for their amount, and mockup
 * point 04 ("push the serif harder"). Setting it at `h3` on the SECOND line
 * rather than in a right-hand column is what makes the money, not the badge,
 * the thing the eye lands on second.
 *
 * ONE badge, the order status. The row used to render the order status AND
 * the payment status side by side; on a row whose second line already
 * carries a figure and a timestamp, two chips made the right edge the
 * busiest thing on a calm screen. The detail screen still shows all three
 * axes, and payment state remains filterable via `useOrders`'s
 * `payment_status` param.
 *
 * Deliberately NOT wrapped in `SwipeRow` — the caller wraps it, the same
 * split `QueueRow` uses, because the legal swipe actions depend on the
 * order's status and this row is also rendered inside the search results
 * where no swipe applies.
 */
export function OrderRow({ order, onPress, onLongPress, currencyCode }: OrderRowProps) {
  const currency = order.currency_code || currencyCode;
  const displayName = order.customer_name || order.customer_email;
  const total = formatMoney(order.grand_total, currency);
  const status = titleize(order.status);
  const placed = formatRelativeTime(order.placed_at);

  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(order)}
      onLongPress={onLongPress ? () => onLongPress(order) : undefined}
      style={styles.row}
      testID={`order-row-${order.id}`}
      accessibilityLabel={`Order ${order.order_number}, ${displayName}, ${total}, ${status}`}
      accessibilityHint={onLongPress ? "Long press for more actions" : undefined}
    >
      <View style={styles.stack}>
        <View style={styles.line}>
          <Text preset="bodyEmphasis" color="text" numberOfLines={1} style={styles.grow}>
            #{order.order_number} · {displayName}
          </Text>
          <StatusBadge
            label={status}
            tone={orderStatusTone(order.status)}
            testID={`order-row-${order.id}-badge`}
          />
        </View>
        <View style={styles.line}>
          <Text
            preset="h3"
            color="text"
            numberOfLines={1}
            style={styles.total}
            testID={`order-row-${order.id}-total`}
          >
            {total}
          </Text>
          <Text preset="caption" color="textTertiary" numberOfLines={1}>
            {placed}
          </Text>
        </View>
      </View>
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  row: {
    // Overrides PressableRow's base flexDirection: "row" — `style` is applied
    // after base/lines and before the pressed-state background (which only
    // sets backgroundColor), so it wins. The two lines stack.
    flexDirection: "column",
    alignItems: "stretch",
    // Paper, not `elevated`: these rows sit directly on the screen ground and
    // are separated by hairlines, not by being cards. (The row used to paint
    // itself white with its own bottom border — a bordered card in a design
    // system whose first rule is hairline rules BETWEEN sections, not
    // bordered cards. The screen draws the hairlines now.)
    backgroundColor: theme.colors.background,
  },
  stack: { flex: 1, gap: theme.spacing.xs },
  line: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: theme.spacing.sm,
  },
  grow: { flexShrink: 1 },
  total: { fontVariant: ["tabular-nums"] },
});
