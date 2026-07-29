import { View, StyleSheet } from "react-native";
import { Monogram, PressableRow, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { customerIdentity } from "@/lib/customer-identity";
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
  // The email still belongs in the spoken label even when it isn't drawn
  // twice — but only once. VoiceOver reading it back-to-back was the same
  // defect in the audio channel.
  const a11yParts = [
    identity.title,
    ...(identity.subtitle ? [identity.subtitle] : []),
    `${customer.order_count} orders`,
    `${spent} spent`,
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
        <Text preset="caption" color="textTertiary">
          {customer.order_count} {customer.order_count === 1 ? "order" : "orders"}
        </Text>
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
});
