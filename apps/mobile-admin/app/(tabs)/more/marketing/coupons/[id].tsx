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
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useCoupon } from "@/lib/hooks/use-coupons";
import { usePatchCoupon, useDeleteCoupon } from "@/lib/admin-api/coupon-actions";
import { BackHeader, Eyebrow, Hairline, Screen, StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { formatCouponValue, couponStatusTone } from "@/lib/coupon-display";

function titleize(value: string): string {
  return value
    .split("_")
    .map((w) => (w ? w.charAt(0).toUpperCase() + w.slice(1) : w))
    .join(" ");
}

function formatDate(iso?: string | null): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleDateString("en-AU", { year: "numeric", month: "short", day: "numeric" });
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.row}>
      <Text preset="body" color="textSecondary">
        {label}
      </Text>
      <Text preset="body" color="text">
        {value}
      </Text>
    </View>
  );
}

export default function CouponDetailScreen() {
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const currency = useTenantStore((s) => s.activeStore?.currency_code) || "AUD";
  const { data, isLoading, error } = useCoupon(id);
  const patch = usePatchCoupon();
  const del = useDeleteCoupon();

  const busy = patch.isPending || del.isPending;
  const coupon = data?.data;

  const onToggle = useCallback(() => {
    if (!coupon) return;
    const next = coupon.status === "disabled" ? "active" : "disabled";
    patch.mutate(
      { id, body: { status: next } },
      {
        onError: (err) =>
          Alert.alert(
            "Couldn't update coupon",
            err instanceof ApiError ? err.message : "Please try again.",
          ),
      },
    );
  }, [coupon, id, patch]);

  const onDelete = useCallback(() => {
    Alert.alert("Delete coupon", "This disables the code permanently.", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Delete",
        style: "destructive",
        onPress: () =>
          del.mutate(id, {
            onSuccess: () => router.back(),
            onError: (err) =>
              Alert.alert(
                "Couldn't delete coupon",
                err instanceof ApiError ? err.message : "Please try again.",
              ),
          }),
      },
    ]);
  }, [id, del, router]);

  if (error) {
    return (
      <Screen>
        <BackHeader eyebrow="COUPON" />
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load coupon
          </Text>
        </View>
      </Screen>
    );
  }

  if (isLoading || !coupon) {
    return (
      <Screen>
        <BackHeader eyebrow="COUPON" title="Loading…" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  const startsAt = formatDate(coupon.starts_at);
  const endsAt = formatDate(coupon.ends_at);
  const usageTotal = data?.usage_total;

  return (
    <Screen>
      <BackHeader eyebrow="COUPON" title={coupon.code} />
      <ScrollView contentContainerStyle={styles.scroll}>
        <View style={styles.heading}>
          <Text preset="h1" color="text">
            {coupon.code}
          </Text>
          <StatusBadge label={titleize(coupon.status)} tone={couponStatusTone(coupon.status)} />
          <Text preset="bodyEmphasis" color="text">
            {formatCouponValue(coupon, currency)}
          </Text>
          <Text preset="body" color="textSecondary">
            {coupon.title}
          </Text>
          {coupon.description ? (
            <Text preset="body" color="textTertiary">
              {coupon.description}
            </Text>
          ) : null}
        </View>

        <Eyebrow label="Rules" style={styles.section} />
        <View style={styles.card}>
          <Row label="Type" value={titleize(coupon.type)} />
          {coupon.min_purchase != null ? (
            <Row label="Minimum spend" value={formatMoney(coupon.min_purchase, currency)} />
          ) : null}
          {coupon.max_discount != null ? (
            <Row label="Max discount" value={formatMoney(coupon.max_discount, currency)} />
          ) : null}
          <Row
            label="Usage limit"
            value={coupon.usage_limit != null ? String(coupon.usage_limit) : "Unlimited"}
          />
          <Row
            label="Per customer"
            value={coupon.per_customer > 0 ? String(coupon.per_customer) : "Unlimited"}
          />
          <Row label="Stackable" value={coupon.stackable ? "Yes" : "No"} />
          {startsAt ? <Row label="Starts" value={startsAt} /> : null}
          <Row label="Ends" value={endsAt ?? "No end date"} />
        </View>

        <Eyebrow label="Usage" style={styles.section} />
        <View style={styles.card}>
          <Row label="Times used" value={String(coupon.usage_count)} />
          {usageTotal != null && usageTotal !== coupon.usage_count ? (
            <Row label="Redemptions on record" value={String(usageTotal)} />
          ) : null}
        </View>

        <View style={styles.actions}>
          <ActionButton
            variant="secondary"
            label={
              patch.isPending
                ? "Updating…"
                : coupon.status === "disabled"
                  ? "Enable coupon"
                  : "Disable coupon"
            }
            onPress={onToggle}
            disabled={busy}
          />
          <ActionButton
            variant="danger"
            label={del.isPending ? "Deleting…" : "Delete coupon"}
            onPress={onDelete}
            disabled={busy}
          />
        </View>
      </ScrollView>
    </Screen>
  );
}

function ActionButton({
  variant,
  label,
  onPress,
  disabled,
}: {
  variant: "secondary" | "danger";
  label: string;
  onPress: () => void;
  disabled?: boolean;
}) {
  const btnStyle = variant === "danger" ? styles.btnDanger : styles.btnSecondary;
  const color = variant === "danger" ? "danger" : "text";
  return (
    <TouchableOpacity
      style={[btnStyle, disabled && styles.btnDisabled]}
      onPress={onPress}
      disabled={disabled}
      activeOpacity={0.85}
      accessibilityRole="button"
      accessibilityLabel={label}
    >
      <Text preset="bodyEmphasis" color={color}>
        {label}
      </Text>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingBottom: theme.spacing.huge },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  heading: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.lg, gap: theme.spacing.sm },
  section: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.xl },
  card: { paddingHorizontal: theme.spacing.lg, marginTop: theme.spacing.sm },
  row: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: theme.spacing.xs,
    gap: theme.spacing.md,
  },
  actions: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.xl, gap: theme.spacing.sm },
  btnSecondary: {
    borderWidth: 1,
    borderColor: theme.colors.border,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnDanger: {
    borderWidth: 1,
    borderColor: theme.colors.danger,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnDisabled: { opacity: 0.5 },
});
