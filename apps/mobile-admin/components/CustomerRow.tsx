import { View, StyleSheet } from "react-native";
import { PressableRow, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import type { Customer } from "@repo/mobile-shared/api/types";

interface CustomerRowProps {
  customer: Customer;
  onPress: (customer: Customer) => void;
  /** The active store's currency code (e.g. "AUD"); undefined falls back to a plain amount. */
  currencyCode?: string;
}

function getInitial(customer: Customer): string {
  const name = customer.first_name || customer.last_name || customer.email;
  return name.charAt(0).toUpperCase();
}

function getDisplayName(customer: Customer): string {
  if (customer.first_name || customer.last_name) {
    return [customer.first_name, customer.last_name].filter(Boolean).join(" ");
  }
  return customer.email;
}

export function CustomerRow({ customer, onPress, currencyCode }: CustomerRowProps) {
  const displayName = getDisplayName(customer);
  const spent = formatMoney(customer.total_spent, currencyCode);
  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(customer)}
      style={styles.row}
      testID={`customer-row-${customer.id}`}
      accessibilityLabel={`${displayName}, ${customer.email}, ${customer.order_count} orders, ${spent} spent`}
    >
      <View style={styles.avatar}>
        <Text preset="bodyEmphasis" color="text">
          {getInitial(customer)}
        </Text>
      </View>
      <View style={styles.info}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
          {displayName}
        </Text>
        <Text preset="caption" color="textTertiary" numberOfLines={1}>
          {customer.email}
        </Text>
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
  // Avatar stays a neutral surface tint — never moss. Moss is reserved for
  // the app's single accent (links, focus, key CTAs), not decoration.
  avatar: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: theme.colors.surfaceAlt,
    alignItems: "center",
    justifyContent: "center",
  },
  info: { flex: 1, gap: 2 },
  stats: { alignItems: "flex-end", gap: 2 },
});
