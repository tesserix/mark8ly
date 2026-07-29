import { View, StyleSheet } from "react-native";
import { Monogram, PressableRow, StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { customerIdentity } from "@/lib/customer-identity";
import { customerStatusLabel, customerStatusTone } from "@/lib/customer-display";
import type { Customer } from "@repo/mobile-shared/api/types";

interface CustomerRowProps {
  customer: Customer;
  onPress: (customer: Customer) => void;
  /**
   * Opens the row's action menu. OPTIONAL and undefined-able on purpose: the
   * list screen passes `undefined` while this row's own request is in flight,
   * which is what actually disarms the gesture — a handler that returns early
   * would still let the row engage its long-press feedback.
   */
  onLongPress?: (customer: Customer) => void;
  /** The active store's currency code (e.g. "AUD"); undefined falls back to a plain amount. */
  currencyCode?: string;
}

/** 40pt, not `Thumb`'s 60 — this row's density predates the shared tile and isn't what's being fixed here. */
const AVATAR = 40;

export function CustomerRow({
  customer,
  onPress,
  onLongPress,
  currencyCode,
}: CustomerRowProps) {
  // ONE source for "who is this" — see lib/customer-identity.ts. `subtitle` is
  // absent for a customer with no name, because their email is already the
  // title; this row used to render the email unconditionally underneath and
  // printed it twice, stacked, for exactly that (normal) customer.
  const identity = customerIdentity(customer);
  const spent = formatMoney(customer.total_spent, currencyCode);
  // `active` is the expected, silent default — a badge on every row (the
  // way ProductRow does it) would make "Active" noise rather than
  // information on a list where almost every customer IS active. Everything
  // else — chiefly `blocked`, but also whatever the backend adds later,
  // since `status` is a bare `z.string()` on the wire — gets a badge,
  // because before this row had NO way to show it: under the default "All"
  // filter a blocked customer and an active one were byte-identical, which
  // is the actual defect this task exists to close.
  const showBadge = customer.status !== "active";
  // ONE string for the badge and the announcement — see
  // lib/customer-display.ts. This is the exact split Task 2 had to close on
  // ProductRow: the badge and the accessibilityLabel were computed
  // separately and disagreed.
  const statusLabel = customerStatusLabel(customer.status);
  // The email still belongs in the spoken label even when it isn't drawn
  // twice — but only once. VoiceOver reading it back-to-back was the same
  // defect in the audio channel.
  const a11yParts = [
    identity.title,
    ...(identity.subtitle ? [identity.subtitle] : []),
    `${customer.order_count} orders`,
    `${spent} spent`,
    ...(showBadge ? [statusLabel] : []),
  ];
  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(customer)}
      onLongPress={onLongPress ? () => onLongPress(customer) : undefined}
      style={styles.row}
      testID={`customer-row-${customer.id}`}
      accessibilityLabel={a11yParts.join(", ")}
      accessibilityHint={onLongPress ? "Long press for more actions" : undefined}
    >
      <Monogram
        label={identity.title}
        size={AVATAR}
        testID={`customer-row-${customer.id}-monogram`}
      />
      <View style={styles.info}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
          {identity.title}
        </Text>
        {identity.subtitle ? (
          <Text preset="caption" color="textTertiary" numberOfLines={1}>
            {identity.subtitle}
          </Text>
        ) : null}
      </View>
      <View style={styles.stats}>
        <Text preset="bodyEmphasis" color="text">
          {spent}
        </Text>
        {showBadge ? (
          // Swaps IN for the order-count line rather than sitting beside
          // it. The row is a fixed 88pt/2-line stack
          // (theme.row.minHeightDouble) and `info` (flex: 1) is what pays
          // for any width this column gains — stacking a THIRD line here to
          // keep both would also risk exactly the height overflow this
          // programme has hit eight times with fixed boxes around scalable
          // text. `StatusBadge` has no fixed width or height of its own (it
          // sizes to its label), so swapping costs no extra height at any
          // Dynamic Type size, and it only ever costs `info` about as much
          // width as the order-count text it replaces — order count itself
          // stays visible on the customer detail screen.
          <StatusBadge
            label={statusLabel}
            tone={customerStatusTone(customer.status)}
            style={styles.statsBadge}
            testID={`customer-row-${customer.id}-badge`}
          />
        ) : (
          <Text preset="caption" color="textTertiary">
            {customer.order_count} {customer.order_count === 1 ? "order" : "orders"}
          </Text>
        )}
      </View>
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  row: {
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
  },
  info: { flex: 1, gap: 2, minWidth: 0 },
  stats: { alignItems: "flex-end", gap: 2 },
  // `StatusBadge` defaults `alignSelf: "flex-start"` (StatusBadge.tsx) so it
  // reads naturally inside a left-to-right row; `stats` is right-aligned
  // (`alignItems: "flex-end"`), so without this override the badge would
  // sit flush against the column's left edge instead of under the amount
  // above it.
  statsBadge: { alignSelf: "flex-end" },
});
