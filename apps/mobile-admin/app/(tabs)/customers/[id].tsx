import { useCallback, useRef } from "react";
import {
  View,
  ScrollView,
  TouchableOpacity,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import type { CustomerAddress } from "@repo/mobile-shared/api/schemas/customers";
import { useCustomer } from "../../../lib/hooks/use-customers";
import { useBlockCustomer, useUnblockCustomer } from "../../../lib/admin-api/customer-actions";
import { BackHeader, Eyebrow, Hairline, Screen, StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { addressLines } from "@/lib/customer-address";
import { BlockReasonSheet, type BlockReasonSheetHandle } from "@/components/customers/BlockReasonSheet";
import { useDockClearance } from "@/components/navigation/dock-metrics";

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

function StatTile({ label, value, divider }: { label: string; value: string; divider?: boolean }) {
  return (
    <View style={[styles.statTile, divider ? styles.statTileDivider : null]}>
      <Text preset="h3" color="text">
        {value}
      </Text>
      <Text preset="caption" color="textTertiary">
        {label}
      </Text>
    </View>
  );
}

function AddressItem({ address }: { address: CustomerAddress }) {
  return (
    <View style={styles.addressItem}>
      <View style={styles.addressHead}>
        <Text preset="bodyEmphasis" color="text">
          {address.name}
        </Text>
        {address.is_default ? (
          <Text preset="caption" color="textTertiary">
            Default
          </Text>
        ) : null}
      </View>
      {address.label ? (
        <Text preset="caption" color="textTertiary">
          {address.label}
        </Text>
      ) : null}
      {addressLines(address).map((line, i) => (
        <Text key={i} preset="body" color="textSecondary">
          {line}
        </Text>
      ))}
    </View>
  );
}

export default function CustomerDetailScreen() {
  const dockPad = useDockClearance();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: customer, isLoading, error } = useCustomer(id);
  const currencyCode = useTenantStore((s) => s.activeStore?.currency_code);

  const blockMutation = useBlockCustomer();
  const unblockMutation = useUnblockCustomer();
  const blockSheetRef = useRef<BlockReasonSheetHandle>(null);

  const isBlocked = customer?.status === "blocked";
  const isMutating = blockMutation.isPending || unblockMutation.isPending;

  const handleBlockToggle = useCallback(() => {
    if (!customer) return;
    if (isBlocked) {
      Alert.alert("Unblock Customer", "This customer will be able to place orders again.", [
        { text: "Cancel", style: "cancel" },
        { text: "Unblock", onPress: () => unblockMutation.mutate(customer.id) },
      ]);
      return;
    }
    // Blocking requires a typed reason (matches web) — collected in the sheet.
    blockSheetRef.current?.present();
  }, [customer, isBlocked, unblockMutation]);

  const handleBlockSubmit = useCallback(
    (reason: string) => {
      if (!customer) return;
      blockMutation.mutate({ id: customer.id, reason });
    },
    [customer, blockMutation],
  );

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

  const infoRows: { label: string; value: string }[] = [
    ...(customer.phone ? [{ label: "Phone", value: customer.phone }] : []),
    { label: "Joined", value: formatDate(customer.created_at) },
    ...(customer.last_order_at
      ? [{ label: "Last order", value: formatDate(customer.last_order_at) }]
      : []),
    { label: "Marketing", value: customer.marketing_opt_in ? "Subscribed" : "Not subscribed" },
  ];

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
                  {customer.block_reason ? (
                    <Text preset="caption" color="textTertiary" style={styles.blockReason}>
                      {customer.block_reason}
                    </Text>
                  ) : null}
                </View>
              ) : null}
            </View>
          </View>

          <View style={styles.infoList}>
            {infoRows.map((row, i) => (
              <View key={row.label}>
                {i > 0 ? <Hairline /> : null}
                <InfoRow label={row.label} value={row.value} />
              </View>
            ))}
          </View>
        </View>

        <Hairline style={styles.statsRule} />
        <View style={styles.statsRow}>
          <StatTile label="Orders" value={String(customer.order_count)} />
          <StatTile label="Spent" value={formatMoney(customer.total_spent, currencyCode)} divider />
          <StatTile label="Avg" value={formatMoney(averageOrderValue, currencyCode)} divider />
        </View>

        {customer.addresses.length > 0 ? (
          <View style={styles.section}>
            <Eyebrow label="Addresses" />
            <View style={styles.addressList}>
              {customer.addresses.map((address, i) => (
                <View key={address.id}>
                  {i > 0 ? <Hairline style={styles.addressDivider} /> : null}
                  <AddressItem address={address} />
                </View>
              ))}
            </View>
          </View>
        ) : null}

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

      <BlockReasonSheet
        ref={blockSheetRef}
        onSubmit={handleBlockSubmit}
        isSubmitting={blockMutation.isPending}
      />
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
  blockedRow: { marginTop: theme.spacing.xs, gap: 4 },
  blockReason: { marginTop: 2 },
  infoList: { marginTop: theme.spacing.lg },
  infoRow: {
    flexDirection: "row",
    paddingVertical: theme.spacing.sm,
  },
  infoLabel: { flex: 1 },
  infoValue: { flex: 2 },
  statsRow: {
    flexDirection: "row",
    marginHorizontal: theme.spacing.lg,
    marginBottom: theme.spacing.md,
  },
  statsRule: {
    marginHorizontal: theme.spacing.lg,
    marginTop: theme.spacing.md,
  },
  statTile: {
    flex: 1,
    paddingVertical: theme.spacing.md,
    alignItems: "flex-start",
    gap: 2,
  },
  statTileDivider: {
    borderLeftWidth: theme.hairline,
    borderLeftColor: theme.colors.hairline,
    paddingLeft: theme.spacing.md,
  },
  section: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.lg,
  },
  addressList: { marginTop: theme.spacing.sm },
  addressItem: {
    paddingVertical: theme.spacing.md,
    gap: 2,
  },
  addressHead: {
    flexDirection: "row",
    alignItems: "baseline",
    justifyContent: "space-between",
  },
  addressDivider: { marginVertical: 0 },
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
