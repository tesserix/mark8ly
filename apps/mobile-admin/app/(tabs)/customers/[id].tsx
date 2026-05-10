import { useCallback } from "react";
import {
  View,
  ScrollView,
  TouchableOpacity,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useCustomer } from "../../../lib/hooks/use-customers";
import { useBlockCustomer, useUnblockCustomer } from "../../../lib/admin-api/customer-actions";
import {
  BackHeader,
  Card,
  Eyebrow,
  Hairline,
  Screen,
  StatusBadge,
  Text,
  type StatusTone,
} from "@/components/ui";
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
  if (firstName || lastName) return [firstName, lastName].filter(Boolean).join(" ");
  return email;
}

const ORDER_STATUS_TONE: Record<string, StatusTone> = {
  pending: "warning",
  confirmed: "success",
  fulfilled: "neutral",
  cancelled: "danger",
};

function StatTile({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.statTile}>
      <Text preset="h3" color="text">
        {value}
      </Text>
      <Text preset="caption" color="textTertiary">
        {label}
      </Text>
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
  const tone = ORDER_STATUS_TONE[order.status] ?? "neutral";
  return (
    <TouchableOpacity
      style={styles.orderRow}
      onPress={() => onPress(order.id)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`Order ${order.order_number}, ${formatCurrency(order.grand_total)}, ${order.status}`}
    >
      <View style={styles.orderInfo}>
        <Text preset="bodyEmphasis" color="text">
          #{order.order_number}
        </Text>
        <Text preset="caption" color="textTertiary">
          {formatDate(order.created_at)}
        </Text>
      </View>
      <View style={styles.orderRight}>
        <Text preset="bodyEmphasis" color="text">
          {formatCurrency(order.grand_total)}
        </Text>
        <StatusBadge label={order.status} tone={tone} />
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
        onPress: () =>
          isBlocked ? unblockMutation.mutate(customer.id) : blockMutation.mutate(customer.id),
      },
    ]);
  }, [customer, isBlocked, blockMutation, unblockMutation]);

  const handleOrderPress = useCallback(
    (orderId: string) => router.push(`/(tabs)/orders/${orderId}`),
    [router],
  );

  if (isLoading || !customer) {
    return (
      <Screen>
        <BackHeader eyebrow="CUSTOMER" title="Loading…" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  const displayName = getDisplayName(customer.first_name, customer.last_name, customer.email);
  const initial = getInitial(customer.first_name, customer.last_name, customer.email);

  return (
    <Screen>
      <BackHeader eyebrow="CUSTOMER" title={displayName} />
      <ScrollView contentContainerStyle={styles.scroll}>
        <View style={styles.profile}>
          <View style={styles.avatar}>
            <Text preset="h2" color="inverse">
              {initial}
            </Text>
          </View>
          <Text preset="h1" color="text" align="center" style={styles.profileName}>
            {displayName}
          </Text>
          <Text preset="body" color="textSecondary" align="center">
            {customer.email}
          </Text>
          {customer.phone ? (
            <Text preset="caption" color="textTertiary" align="center">
              {customer.phone}
            </Text>
          ) : null}
          <Text preset="caption" color="textTertiary" align="center" style={styles.joined}>
            Joined {formatDate(customer.created_at)}
          </Text>
          {isBlocked ? (
            <View style={styles.blockedRow}>
              <StatusBadge label="Blocked" tone="danger" />
            </View>
          ) : null}
        </View>

        <View style={styles.statsRow}>
          <StatTile label="Orders" value={String(customer.order_count)} />
          <StatTile label="Spent" value={formatCurrency(customer.total_spent)} />
          <StatTile label="Avg" value={formatCurrency(customer.average_order_value)} />
        </View>

        <Eyebrow label="Recent Orders" />
        <Card padding={0} style={styles.card}>
          {customer.recent_orders.length > 0 ? (
            customer.recent_orders.map((order, i) => (
              <View key={order.id}>
                {i > 0 ? <Hairline inset={theme.spacing.lg} /> : null}
                <RecentOrderRow order={order} onPress={handleOrderPress} />
              </View>
            ))
          ) : (
            <View style={styles.empty}>
              <Text preset="caption" color="textTertiary">
                No orders yet.
              </Text>
            </View>
          )}
        </Card>

        <View style={styles.actions}>
          <TouchableOpacity
            style={isBlocked ? styles.unblockBtn : styles.blockBtn}
            onPress={handleBlockToggle}
            disabled={isMutating}
            accessibilityRole="button"
            accessibilityLabel={
              isBlocked
                ? unblockMutation.isPending ? "Unblocking customer" : "Unblock customer"
                : blockMutation.isPending ? "Blocking customer" : "Block customer"
            }
          >
            <Text
              preset="bodyEmphasis"
              color={isBlocked ? "inverse" : "danger"}
            >
              {isMutating
                ? isBlocked ? "Unblocking…" : "Blocking…"
                : isBlocked ? "Unblock Customer" : "Block Customer"}
            </Text>
          </TouchableOpacity>
        </View>
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingBottom: theme.spacing.huge },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
  profile: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.xl,
    paddingBottom: theme.spacing.xl,
    alignItems: "center",
  },
  avatar: {
    width: 72,
    height: 72,
    borderRadius: 36,
    backgroundColor: theme.colors.accent,
    alignItems: "center",
    justifyContent: "center",
    marginBottom: theme.spacing.md,
  },
  profileName: { marginBottom: 2 },
  joined: { marginTop: theme.spacing.xs },
  blockedRow: { marginTop: theme.spacing.sm },
  statsRow: {
    flexDirection: "row",
    gap: theme.spacing.sm,
    marginHorizontal: theme.spacing.lg,
    marginBottom: theme.spacing.md,
  },
  statTile: {
    flex: 1,
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    paddingVertical: theme.spacing.md,
    alignItems: "center",
    gap: 2,
  },
  card: { marginHorizontal: theme.spacing.lg },
  orderRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    minHeight: 56,
    gap: theme.spacing.md,
  },
  orderInfo: { flex: 1, gap: 2 },
  orderRight: { alignItems: "flex-end", gap: 4 },
  empty: { padding: theme.spacing.lg },
  actions: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.xl,
  },
  blockBtn: {
    backgroundColor: "transparent",
    borderWidth: 1,
    borderColor: theme.colors.danger,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  unblockBtn: {
    backgroundColor: theme.colors.accent,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
});
