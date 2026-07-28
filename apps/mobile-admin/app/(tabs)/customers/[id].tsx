import { useCallback, useRef, useState } from "react";
import {
  Platform,
  View,
  ScrollView,
  Pressable,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import type { CustomerAddress } from "@repo/mobile-shared/api/schemas/customers";
import { useCustomer } from "../../../lib/hooks/use-customers";
import { useBlockCustomer, useUnblockCustomer } from "../../../lib/admin-api/customer-actions";
import {
  BackHeader,
  Eyebrow,
  Hairline,
  Monogram,
  Screen,
  StatusBadge,
  Text,
  titleMinimumFontScale,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { addressLines } from "@/lib/customer-address";
import { customerIdentity } from "@/lib/customer-identity";
import { BlockReasonSheet, type BlockReasonSheetHandle } from "@/components/customers/BlockReasonSheet";
import { useDockClearance } from "@/components/navigation/dock-metrics";

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

/** Monogram tile for the detail head — bigger than the list row's, same shape. */
const AVATAR = 72;

/**
 * Floor for the identity title's shrink-to-fit, as a fraction of h2 — the same
 * `theme.text.caption.fontSize` (13pt) floor `CollapsingHeader` uses, via the
 * same helper rather than a second hand-written ratio.
 *
 * Reused because the reasoning transfers exactly: a bare `0.5` would authorise
 * a 12pt identity line at the default text size, below iOS's 11pt legibility
 * guidance, whereas pinning the floor to a real design-system size means the
 * customer's own name can never render smaller than the caption beside it, at
 * any text size (`minimumFontScale` is a fraction of the ALREADY-scaled size,
 * so `13 / 24` holds at `13 x fontScale` for every scale).
 *
 * WHY it is needed here at all, and why the LINE ALLOWANCE cannot substitute:
 * `CollapsingHeader` chose "wrap first, shrink second" because a shop name is
 * prose with spaces to break at. An email has none. Left to wrap,
 * `mahesh.sangawar@gmail.com` broke as `mahesh.sangawar@gmai` / `l.com` — an
 * address split mid-token reads as if it ended at the line break, which is the
 * "money has to read as a single token" case `CollapsingHeader`'s own comment
 * carves out, not the shop-name case. So an email title gets ONE line and
 * shrinks; a real name still gets two and wraps. That single difference is the
 * whole rule.
 */
const IDENTITY_TITLE_MIN_SCALE = titleMinimumFontScale(theme.text.h2.fontSize);

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

  // NativeWind's JSX interop doesn't resolve a function `style` prop the way
  // it resolves a plain array — press state is tracked explicitly instead.
  const [blockBtnPressed, setBlockBtnPressed] = useState(false);

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

  // ONE source for "who is this", shared with the list row — see
  // lib/customer-identity.ts. `subtitle` is absent when the email IS the
  // title, which is what stops this screen printing the same address twice.
  const identity = customerIdentity(customer);

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
      {/* No `title`. The back bar is STATIC — it does not collapse — so a title
          here is drawn at the same time as the identity block 20pt below it,
          which is precisely the repetition being fixed: a no-name customer had
          their email in the bar, again as the h2, and again as the subtitle.
          The eyebrow is the label this bar should carry; it says WHAT kind of
          record you are on, which the body never repeats.

          The alternative — reveal the identity in the bar only once the body
          has scrolled past it — was considered and rejected here, not on
          merit but on consistency: it needs a scroll-linked header, none of
          this app's ten detail screens has one, and adding the behaviour to
          exactly one of them would create a fresh cross-screen inconsistency
          in the middle of a fix whose whole point is consistency. If it is
          ever wanted it belongs to all ten at once, as an additive
          `BackHeader` prop. Between the two lines, the h2 is the one that
          survives: the bar's 1-line centred `bodyEmphasis` truncated the email
          outright, so it was the worse rendering of the two. */}
      <BackHeader eyebrow="CUSTOMER" />
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
        <View style={styles.profile}>
          {/* Monogram ABOVE the identity, not beside it. Beside it, the title
              column was `402 - 32 gutters - 72 tile - 12 gap = 286pt` on an
              iPhone 17 Pro — under what a 25-character email needs at h2 even
              at the DEFAULT text size, which is why it wrapped mid-address
              there and nowhere else. Stacking returns the full 370pt column,
              so the email fits on one line unshrunk at 1x and still clears the
              13pt floor at the 2x cap without ellipsising. Applied to BOTH
              cases, named and not: one head layout per screen beats a head
              that rearranges itself depending on whether this particular
              customer filled in a name. */}
          <Monogram
            label={identity.title}
            size={AVATAR}
            textPreset="h2"
            testID="customer-detail-monogram"
          />
          <View style={styles.identity}>
            {/* One line + shrink for an email, two lines + wrap for a name —
                see IDENTITY_TITLE_MIN_SCALE for why the two differ. */}
            <Text
              preset="h2"
              color="text"
              numberOfLines={identity.titleIsEmail ? 1 : 2}
              adjustsFontSizeToFit
              minimumFontScale={IDENTITY_TITLE_MIN_SCALE}
              testID="customer-detail-title"
            >
              {identity.title}
            </Text>
            {/* Present only when it says something the title doesn't. For a
                customer with no name the email IS the title, and repeating it
                here was two of the three copies of the same address. */}
            {identity.subtitle ? (
              <Text preset="body" color="textSecondary" testID="customer-detail-subtitle">
                {identity.subtitle}
              </Text>
            ) : null}
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
          <Pressable
            onPress={handleBlockToggle}
            onPressIn={() => setBlockBtnPressed(true)}
            onPressOut={() => setBlockBtnPressed(false)}
            disabled={isMutating}
            accessibilityRole="button"
            accessibilityLabel={
              isBlocked
                ? unblockMutation.isPending ? "Unblocking customer" : "Unblock customer"
                : blockMutation.isPending ? "Blocking customer" : "Block customer"
            }
            android_ripple={isBlocked ? theme.press.rippleOnDark : theme.press.rippleDanger}
            style={[
              isBlocked ? styles.unblockBtn : styles.blockBtn,
              blockBtnPressed && Platform.OS === "ios"
                ? {
                    opacity: isBlocked
                      ? theme.press.opacitySolidFill
                      : theme.press.opacityStandard,
                  }
                : null,
            ]}
          >
            <Text
              preset="bodyEmphasis"
              color={isBlocked ? "inverse" : "danger"}
            >
              {isMutating
                ? isBlocked ? "Unblocking…" : "Blocking…"
                : isBlocked ? "Unblock Customer" : "Block Customer"}
            </Text>
          </Pressable>
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
  // No `flex: 1` and no `justifyContent` — the identity is now a block under
  // the monogram rather than a column beside it, so it simply takes the
  // profile's full width.
  identity: {
    marginTop: theme.spacing.md,
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
