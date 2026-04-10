import { useCallback } from "react";
import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useCustomer } from "../../../lib/hooks/use-customers";
import { useBlockCustomer, useUnblockCustomer } from "../../../lib/admin-api/customer-actions";
import { theme } from "@/lib/theme";
import type { RecentOrder } from "@repo/mobile-shared/api/types";

function formatCurrency(amount: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
  }).format(amount);
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function getInitial(firstName: string, lastName: string, email: string): string {
  const name = firstName || lastName || email;
  return name.charAt(0).toUpperCase();
}

function getDisplayName(firstName: string, lastName: string, email: string): string {
  if (firstName || lastName) {
    return [firstName, lastName].filter(Boolean).join(" ");
  }
  return email;
}

function SectionHeader({ title }: { title: string }) {
  return <Text style={styles.sectionHeader}>{title}</Text>;
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.statCard} accessibilityLabel={`${label}: ${value}`}>
      <Text style={styles.statValue}>{value}</Text>
      <Text style={styles.statLabel}>{label}</Text>
    </View>
  );
}

function RecentOrderRow({
  order,
  onPress,
}: {
  order: RecentOrder;
  onPress: (id: string) => void;
}) {
  return (
    <TouchableOpacity
      style={styles.orderRow}
      onPress={() => onPress(order.id)}
      activeOpacity={0.7}
      accessibilityRole="button"
      accessibilityLabel={`Order ${order.order_number}, ${formatCurrency(order.grand_total)}, ${order.status}`}
    >
      <View style={styles.orderInfo}>
        <Text style={styles.orderNumber}>{order.order_number}</Text>
        <Text style={styles.orderDate}>{formatDate(order.created_at)}</Text>
      </View>
      <View style={styles.orderRight}>
        <Text style={styles.orderTotal}>{formatCurrency(order.grand_total)}</Text>
        <View
          style={[
            styles.statusBadge,
            { backgroundColor: order.status === "cancelled" ? theme.colors.danger : theme.colors.accent },
          ]}
        >
          <Text style={styles.statusText}>{order.status}</Text>
        </View>
      </View>
    </TouchableOpacity>
  );
}

export default function CustomerDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { data: customer, isLoading } = useCustomer(id);

  const blockMutation = useBlockCustomer();
  const unblockMutation = useUnblockCustomer();

  const isBlocked = customer?.status === "blocked";
  const isMutating = blockMutation.isPending || unblockMutation.isPending;

  const handleBlockToggle = useCallback(() => {
    if (!customer) return;

    const action = isBlocked ? "Unblock" : "Block";
    const message = isBlocked
      ? "This customer will be able to place orders again."
      : "This customer will not be able to place new orders.";

    Alert.alert(`${action} Customer`, message, [
      { text: "Cancel", style: "cancel" },
      {
        text: action,
        style: isBlocked ? "default" : "destructive",
        onPress: () => {
          if (isBlocked) {
            unblockMutation.mutate(customer.id);
          } else {
            blockMutation.mutate(customer.id);
          }
        },
      },
    ]);
  }, [customer, isBlocked, blockMutation, unblockMutation]);

  const handleOrderPress = useCallback(
    (orderId: string) => {
      router.push(`/(tabs)/orders/${orderId}`);
    },
    [router],
  );

  if (isLoading || !customer) {
    return (
      <View style={styles.loadingContainer}>
        <ActivityIndicator size="large" color={theme.colors.text} />
      </View>
    );
  }

  const displayName = getDisplayName(customer.first_name, customer.last_name, customer.email);
  const initial = getInitial(customer.first_name, customer.last_name, customer.email);

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      {/* Profile header */}
      <View style={styles.card}>
        <View style={styles.profileHeader}>
          <View style={styles.avatar} accessibilityLabel={`Avatar for ${displayName}`}>
            <Text style={styles.avatarText}>{initial}</Text>
          </View>
          <Text style={styles.profileName}>{displayName}</Text>
          <Text style={styles.profileEmail}>{customer.email}</Text>
          {customer.phone && (
            <Text style={styles.profilePhone}>{customer.phone}</Text>
          )}
          <Text style={styles.profileJoined}>
            Joined {formatDate(customer.created_at)}
          </Text>
          {isBlocked && (
            <View style={styles.blockedBadge}>
              <Text style={styles.blockedText}>Blocked</Text>
            </View>
          )}
        </View>
      </View>

      {/* Stats */}
      <View style={styles.statsRow}>
        <StatCard label="Total Orders" value={String(customer.order_count)} />
        <StatCard label="Total Spent" value={formatCurrency(customer.total_spent)} />
        <StatCard
          label="Avg Order"
          value={formatCurrency(customer.average_order_value)}
        />
      </View>

      {/* Recent orders */}
      <View style={styles.card}>
        <SectionHeader title="Recent Orders" />
        {customer.recent_orders.length > 0 ? (
          customer.recent_orders.map((order) => (
            <RecentOrderRow
              key={order.id}
              order={order}
              onPress={handleOrderPress}
            />
          ))
        ) : (
          <Text style={styles.emptyText}>No orders yet</Text>
        )}
      </View>

      {/* Actions */}
      <View style={styles.actionsSection}>
        <TouchableOpacity
          style={isBlocked ? styles.primaryBtn : styles.dangerBtn}
          onPress={handleBlockToggle}
          disabled={isMutating}
          accessibilityRole="button"
          accessibilityLabel={
            isMutating
              ? isBlocked ? "Unblocking customer" : "Blocking customer"
              : isBlocked ? "Unblock customer" : "Block customer"
          }
        >
          <Text style={isBlocked ? styles.primaryBtnText : styles.dangerBtnText}>
            {isMutating
              ? isBlocked
                ? "Unblocking..."
                : "Blocking..."
              : isBlocked
                ? "Unblock Customer"
                : "Block Customer"}
          </Text>
        </TouchableOpacity>
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: theme.colors.background,
  },
  content: {
    padding: theme.spacing.lg,
    paddingBottom: 40,
    gap: theme.spacing.md,
  },
  loadingContainer: {
    flex: 1,
    backgroundColor: theme.colors.background,
    justifyContent: "center",
    alignItems: "center",
  },
  card: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    padding: 14,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  profileHeader: {
    alignItems: "center",
    paddingVertical: theme.spacing.sm,
  },
  avatar: {
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: theme.colors.accent,
    justifyContent: "center",
    alignItems: "center",
    marginBottom: theme.spacing.md,
  },
  avatarText: {
    fontSize: 22,
    fontWeight: "700",
    color: theme.colors.elevated,
  },
  profileName: {
    fontSize: 18,
    fontWeight: "700",
    color: theme.colors.text,
    marginBottom: 2,
  },
  profileEmail: {
    fontSize: 14,
    color: theme.colors.text,
    opacity: 0.6,
    marginBottom: 2,
  },
  profilePhone: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.5,
    marginBottom: theme.spacing.xs,
  },
  profileJoined: {
    fontSize: 12,
    color: theme.colors.text,
    opacity: 0.4,
    marginTop: theme.spacing.xs,
  },
  blockedBadge: {
    marginTop: theme.spacing.sm,
    backgroundColor: theme.colors.danger,
    paddingHorizontal: 10,
    paddingVertical: theme.spacing.xs,
    borderRadius: 4,
  },
  blockedText: {
    fontSize: 11,
    fontWeight: "600",
    color: theme.colors.elevated,
  },
  statsRow: {
    flexDirection: "row",
    gap: theme.spacing.sm,
  },
  statCard: {
    flex: 1,
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    padding: theme.spacing.md,
    alignItems: "center",
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  statValue: {
    fontSize: 16,
    fontWeight: "700",
    color: theme.colors.text,
    marginBottom: 2,
  },
  statLabel: {
    fontSize: 11,
    color: theme.colors.text,
    opacity: 0.5,
  },
  sectionHeader: {
    fontSize: 12,
    fontWeight: "600",
    color: theme.colors.text,
    opacity: 0.5,
    textTransform: "uppercase",
    letterSpacing: 0.5,
    marginBottom: 10,
  },
  orderRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: 10,
    borderBottomWidth: 0.5,
    borderBottomColor: `${theme.colors.text}10`,
  },
  orderInfo: {
    flex: 1,
  },
  orderNumber: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.colors.text,
    marginBottom: 2,
  },
  orderDate: {
    fontSize: 12,
    color: theme.colors.text,
    opacity: 0.4,
  },
  orderRight: {
    alignItems: "flex-end",
  },
  orderTotal: {
    fontSize: 14,
    fontWeight: "700",
    color: theme.colors.text,
    marginBottom: theme.spacing.xs,
  },
  statusBadge: {
    paddingHorizontal: theme.spacing.sm,
    paddingVertical: 2,
    borderRadius: 4,
  },
  statusText: {
    fontSize: 10,
    fontWeight: "600",
    color: theme.colors.elevated,
    textTransform: "capitalize",
  },
  emptyText: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.4,
    fontStyle: "italic",
  },
  actionsSection: {
    gap: 10,
    marginTop: theme.spacing.xs,
  },
  primaryBtn: {
    backgroundColor: theme.colors.accent,
    borderRadius: theme.radius,
    paddingVertical: 14,
    alignItems: "center",
  },
  primaryBtnText: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.elevated,
  },
  dangerBtn: {
    backgroundColor: "transparent",
    borderRadius: theme.radius,
    paddingVertical: 14,
    alignItems: "center",
    borderWidth: 1,
    borderColor: theme.colors.danger,
  },
  dangerBtnText: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.danger,
  },
});
