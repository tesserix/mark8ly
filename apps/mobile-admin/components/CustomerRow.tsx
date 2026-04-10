import { View, Text, TouchableOpacity, StyleSheet } from "react-native";
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
      activeOpacity={0.7}
      accessibilityRole="button"
      accessibilityLabel={`${displayName}, ${customer.email}, ${customer.order_count} orders, ${formatCurrency(customer.total_spent)} spent`}
    >
      <View style={styles.avatar} accessibilityLabel={`Avatar for ${displayName}`}>
        <Text style={styles.avatarText}>{getInitial(customer)}</Text>
      </View>
      <View style={styles.info}>
        <Text style={styles.name} numberOfLines={1}>
          {displayName}
        </Text>
        <Text style={styles.email} numberOfLines={1}>
          {customer.email}
        </Text>
      </View>
      <View style={styles.stats}>
        <Text style={styles.spent}>{formatCurrency(customer.total_spent)}</Text>
        <Text style={styles.orders}>
          {customer.order_count} {customer.order_count === 1 ? "order" : "orders"}
        </Text>
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
    flexDirection: "row",
    alignItems: "center",
  },
  avatar: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: theme.colors.accent,
    justifyContent: "center",
    alignItems: "center",
    marginRight: theme.spacing.md,
  },
  avatarText: {
    fontSize: 16,
    fontWeight: "700",
    color: theme.colors.elevated,
  },
  info: {
    flex: 1,
    marginRight: theme.spacing.sm,
  },
  name: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.text,
    marginBottom: 2,
  },
  email: {
    fontSize: 12,
    color: theme.colors.text,
    opacity: 0.5,
  },
  stats: {
    alignItems: "flex-end",
  },
  spent: {
    fontSize: 14,
    fontWeight: "700",
    color: theme.colors.text,
    marginBottom: 2,
  },
  orders: {
    fontSize: 11,
    color: theme.colors.text,
    opacity: 0.4,
  },
});
