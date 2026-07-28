import { useCallback } from "react";
import { View, ScrollView, Switch, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { Users, Gift } from "lucide-react-native";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useLoyaltyProgram } from "@/lib/hooks/use-loyalty";
import { useUpdateLoyaltyProgram } from "@/lib/admin-api/loyalty-actions";
import { MarketingRow } from "@/components/marketing/MarketingRow";
import { BackHeader, Eyebrow, EmptyState, Screen, StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";
import type { LoyaltyProgram } from "@repo/mobile-shared/api/types";

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

/** Re-send the whole program with is_active flipped — PUT requires the full body. */
function toUpdateBody(p: LoyaltyProgram, isActive: boolean) {
  return {
    is_active: isActive,
    points_per_unit: p.points_per_unit,
    points_currency: p.points_currency,
    signup_bonus: p.signup_bonus,
    referral_bonus: p.referral_bonus,
    referee_bonus: p.referee_bonus,
    point_expiry_days: p.point_expiry_days,
    min_redeem_points: p.min_redeem_points,
    points_value: p.points_value,
    tiers: p.tiers.map((t) => ({ name: t.name, min_points: t.min_points, multiplier: t.multiplier })),
  };
}

export default function LoyaltyScreen() {
  const router = useRouter();
  const { data: program, isLoading, error } = useLoyaltyProgram();
  const update = useUpdateLoyaltyProgram();
  const dockPad = useDockClearance();

  const onToggle = useCallback(
    (next: boolean) => {
      if (!program) return;
      update.mutate(toUpdateBody(program, next), {
        onError: (err) =>
          Alert.alert(
            "Couldn't update program",
            err instanceof ApiError ? err.message : "Please try again.",
          ),
      });
    },
    [program, update],
  );

  const membersRow = (
    <MarketingRow
      icon={<Users size={20} color={theme.colors.text} strokeWidth={1.75} />}
      label="Members"
      description="Points balances and history"
      onPress={() => router.push("/(tabs)/more/marketing/loyalty/members")}
    />
  );
  const referralsRow = (
    <MarketingRow
      icon={<Gift size={20} color={theme.colors.text} strokeWidth={1.75} />}
      label="Referrals"
      description="Who referred whom"
      onPress={() => router.push("/(tabs)/more/marketing/loyalty/referrals")}
    />
  );

  if (isLoading) {
    return (
      <Screen>
        <BackHeader eyebrow="MARKETING" title="Loyalty" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  if (error) {
    return (
      <Screen>
        <BackHeader eyebrow="MARKETING" title="Loyalty" />
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load loyalty program
          </Text>
        </View>
      </Screen>
    );
  }

  return (
    <Screen>
      <BackHeader eyebrow="MARKETING" title="Loyalty" />
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
        {!program ? (
          <>
            <EmptyState
              align="left"
              title="Loyalty isn't set up"
              message="Configure your points program on the web dashboard, then manage members here."
            />
            {membersRow}
            {referralsRow}
          </>
        ) : (
          <>
            <View style={styles.heading}>
              <View style={styles.statusRow}>
                <StatusBadge
                  label={program.is_active ? "Active" : "Inactive"}
                  tone={program.is_active ? "success" : "muted"}
                />
                <Switch
                  value={program.is_active}
                  onValueChange={onToggle}
                  disabled={update.isPending}
                  trackColor={{ true: theme.colors.accent }}
                />
              </View>
            </View>

            <Eyebrow label="Earning" style={styles.section} />
            <View style={styles.card}>
              <Row label="Points per unit spent" value={String(program.points_per_unit)} />
              <Row label="Point value" value={`${program.points_value} ${program.points_currency}`} />
              <Row label="Minimum to redeem" value={`${program.min_redeem_points} pts`} />
              <Row
                label="Points expiry"
                value={program.point_expiry_days != null ? `${program.point_expiry_days} days` : "Never"}
              />
            </View>

            <Eyebrow label="Bonuses" style={styles.section} />
            <View style={styles.card}>
              <Row label="Signup" value={`${program.signup_bonus} pts`} />
              <Row label="Referral" value={`${program.referral_bonus} pts`} />
              <Row label="Referee" value={`${program.referee_bonus} pts`} />
              <Row label="Tiers" value={String(program.tiers.length)} />
            </View>

            <Eyebrow label="Manage" style={styles.section} />
            <View style={styles.card}>
              {membersRow}
              {referralsRow}
            </View>

            <Text preset="caption" color="textTertiary" style={styles.note}>
              Earning rules and tiers are configured on the web dashboard.
            </Text>
          </>
        )}
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingBottom: theme.spacing.huge },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  heading: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.lg },
  statusRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  section: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.xl },
  card: { paddingHorizontal: theme.spacing.lg, marginTop: theme.spacing.sm },
  row: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: theme.spacing.xs,
    gap: theme.spacing.md,
  },
  note: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.md },
});
