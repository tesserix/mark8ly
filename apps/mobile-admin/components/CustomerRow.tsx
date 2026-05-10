import { View, TouchableOpacity, StyleSheet } from "react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Customer } from "@repo/mobile-shared/api/types";

interface CustomerRowProps {
  customer: Customer;
  onPress: (customer: Customer) => void;
}

function formatCurrency(amount: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
  }).format(amount);
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

export function CustomerRow({ customer, onPress }: CustomerRowProps) {
  const displayName = getDisplayName(customer);
  return (
    <TouchableOpacity
      style={styles.container}
      onPress={() => onPress(customer)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`${displayName}, ${customer.email}, ${customer.order_count} orders, ${formatCurrency(customer.total_spent)} spent`}
    >
      <View style={styles.avatar}>
        <Text preset="bodyEmphasis" color="inverse">
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
          {formatCurrency(customer.total_spent)}
        </Text>
        <Text preset="caption" color="textTertiary">
          {customer.order_count} {customer.order_count === 1 ? "order" : "orders"}
        </Text>
      </View>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  container: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
    gap: theme.spacing.md,
  },
  avatar: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: theme.colors.accent,
    alignItems: "center",
    justifyContent: "center",
  },
  info: { flex: 1, gap: 2 },
  stats: { alignItems: "flex-end", gap: 2 },
});
