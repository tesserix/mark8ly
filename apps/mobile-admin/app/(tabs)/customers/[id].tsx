import { useCallback } from "react";
import {
  View,
  ScrollView,
  TouchableOpacity,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useCustomer } from "../../../lib/hooks/use-customers";
import { useBlockCustomer, useUnblockCustomer } from "../../../lib/admin-api/customer-actions";
import { BackHeader, Hairline, Screen, StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";

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

function getInitial(firstName: string | undefined, lastName: string | undefined, email: string): string {
  const name = firstName || lastName || email;
  return name.charAt(0).toUpperCase();
}

function getDisplayName(firstName: string | undefined, lastName: string | undefined, email: string): string {
  if (firstName || lastName) return [firstName, lastName].filter(Boolean).join(" ");
  return email;
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.infoRow}>
      <Text preset="caption" color="textTertiary" style={styles.infoLabel}>
        {label}
      </Text>
      <Text preset="body" color="text" style={styles.infoValue}>
        {value}
      </Text>
    </View>
  );
}

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

export default function CustomerDetailScreen() {
  const dockPad = useDockClearance();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: customer, isLoading, error } = useCustomer(id);

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
          isBlocked
            ? unblockMutation.mutate(customer.id)
            : blockMutation.mutate({ id: customer.id, reason: "Blocked from mobile admin" }),
      },
    ]);
  }, [customer, isBlocked, blockMutation, unblockMutation]);

  if (error) {
    return (
      <Screen>
        <BackHeader eyebrow="CUSTOMER" />
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load customer
          </Text>
        </View>
      </Screen>
    );
  }

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

  // The backend has no average_order_value — deriving it here is exactly what
  // a server-side field would compute, without a deploy. Guard the divide:
  // the only real customer has order_count 0.
  const averageOrderValue =
    customer.order_count > 0 ? customer.total_spent / customer.order_count : 0;

  return (
    <Screen>
      <BackHeader eyebrow="CUSTOMER" title={displayName} />
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
        <View style={styles.profile}>
          <View style={styles.identityRow}>
            <View style={styles.avatar}>
              <Text preset="h2" color="text">
                {initial}
              </Text>
            </View>
            <View style={styles.identity}>
              <Text preset="h2" color="text">
                {displayName}
              </Text>
              <Text preset="body" color="textSecondary">
                {customer.email}
              </Text>
              {isBlocked ? (
                <View style={styles.blockedRow}>
                  <StatusBadge label="Blocked" tone="danger" />
                </View>
              ) : null}
            </View>
          </View>

          <View style={styles.infoList}>
            {customer.phone ? (
              <>
                <InfoRow label="Phone" value={customer.phone} />
                <Hairline />
              </>
            ) : null}
            <InfoRow label="Joined" value={formatDate(customer.created_at)} />
          </View>
        </View>

        <View style={styles.statsRow}>
          <StatTile label="Orders" value={String(customer.order_count)} />
          <StatTile label="Spent" value={formatCurrency(customer.total_spent)} />
          <StatTile label="Avg" value={formatCurrency(averageOrderValue)} />
        </View>

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
  },
  identityRow: {
    flexDirection: "row",
    gap: theme.spacing.md,
  },
  avatar: {
    width: 72,
    height: 72,
    borderRadius: 36,
    backgroundColor: theme.colors.surfaceAlt,
    alignItems: "center",
    justifyContent: "center",
  },
  identity: {
    flex: 1,
    justifyContent: "center",
    gap: 2,
  },
  blockedRow: { marginTop: theme.spacing.xs },
  infoList: { marginTop: theme.spacing.lg },
  infoRow: {
    flexDirection: "row",
    paddingVertical: theme.spacing.sm,
  },
  infoLabel: { flex: 1 },
  infoValue: { flex: 2 },
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
