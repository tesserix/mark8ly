import { useCallback, useState } from "react";
import { View, ScrollView, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { useLocalSearchParams } from "expo-router";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useLoyaltyMember } from "@/lib/hooks/use-loyalty";
import { useAdjustPoints } from "@/lib/admin-api/loyalty-actions";
import { BackHeader, Eyebrow, FieldInput, Hairline, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { LoyaltyTxn } from "@repo/mobile-shared/api/types";

function formatDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("en-AU", { year: "numeric", month: "short", day: "numeric" });
}

function titleize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
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

function TxnRow({ txn }: { txn: LoyaltyTxn }) {
  const sign = txn.points > 0 ? "+" : "";
  return (
    <View style={styles.txn}>
      <View style={styles.txnInfo}>
        <Text preset="bodyEmphasis" color="text">
          {titleize(txn.type)}
        </Text>
        <Text preset="caption" color="textTertiary" numberOfLines={1}>
          {formatDate(txn.created_at)}
          {txn.description ? ` · ${txn.description}` : ""}
        </Text>
      </View>
      <View style={styles.txnAmounts}>
        <Text preset="bodyEmphasis" color={txn.points >= 0 ? "text" : "danger"}>
          {sign}
          {txn.points}
        </Text>
        <Text preset="caption" color="textTertiary">
          Bal {txn.balance_after}
        </Text>
      </View>
    </View>
  );
}

export default function LoyaltyMemberScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data, isLoading, error } = useLoyaltyMember(id);
  const adjust = useAdjustPoints();

  const [points, setPoints] = useState("");
  const [reason, setReason] = useState("");

  const pointsNum = Number(points.trim());
  const canAdjust =
    points.trim() !== "" &&
    Number.isFinite(pointsNum) &&
    pointsNum !== 0 &&
    reason.trim().length > 0 &&
    !adjust.isPending;

  const onAdjust = useCallback(() => {
    if (!canAdjust) return;
    adjust.mutate(
      { id, body: { points: pointsNum, description: reason.trim() } },
      {
        onSuccess: () => {
          setPoints("");
          setReason("");
        },
        onError: (err) =>
          Alert.alert(
            "Couldn't adjust points",
            err instanceof ApiError ? err.message : "Please try again.",
          ),
      },
    );
  }, [canAdjust, adjust, id, pointsNum, reason]);

  if (error) {
    return (
      <Screen>
        <BackHeader eyebrow="MEMBER" />
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load member
          </Text>
        </View>
      </Screen>
    );
  }

  if (isLoading || !data) {
    return (
      <Screen>
        <BackHeader eyebrow="MEMBER" title="Loading…" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  const member = data.data;
  const txns = data.transactions.data;

  return (
    <Screen>
      <BackHeader eyebrow="MEMBER" title={member.customer_name || member.customer_email} />
      <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
        <View style={styles.heading}>
          <Text preset="h1" color="text">
            {member.points_balance} pts
          </Text>
          <Text preset="body" color="textSecondary">
            {member.tier} · {member.lifetime_points} lifetime
          </Text>
        </View>

        <Eyebrow label="Member" style={styles.section} />
        <View style={styles.card}>
          <Row label="Email" value={member.customer_email} />
          <Row label="Referral code" value={member.referral_code} />
          <Row label="Enrolled" value={formatDate(member.enrolled_at)} />
        </View>

        <Eyebrow label="Adjust points" style={styles.section} />
        <View style={styles.cardForm}>
          <FieldInput
            label="Points (use a negative number to deduct)"
            value={points}
            onChangeText={setPoints}
            placeholder="100"
            keyboardType="numbers-and-punctuation"
          />
          <FieldInput
            label="Reason"
            value={reason}
            onChangeText={setReason}
            placeholder="Goodwill gesture"
          />
          <Pressable
            style={[styles.btn, !canAdjust && styles.disabled]}
            onPress={onAdjust}
            disabled={!canAdjust}
            accessibilityRole="button"
            accessibilityLabel="Apply adjustment"
          >
            {adjust.isPending ? (
              <ActivityIndicator size="small" color={theme.colors.inverse} />
            ) : (
              <Text preset="bodyEmphasis" color="inverse">
                Apply adjustment
              </Text>
            )}
          </Pressable>
        </View>

        <Eyebrow label="History" style={styles.section} />
        <View style={styles.card}>
          {txns.length === 0 ? (
            <Text preset="body" color="textTertiary">
              No points activity yet.
            </Text>
          ) : (
            txns.map((txn, i) => (
              <View key={txn.id}>
                {i > 0 ? <Hairline /> : null}
                <TxnRow txn={txn} />
              </View>
            ))
          )}
        </View>
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingBottom: theme.spacing.huge },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  heading: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.lg, gap: theme.spacing.xs },
  section: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.xl },
  card: { paddingHorizontal: theme.spacing.lg, marginTop: theme.spacing.sm },
  cardForm: { paddingHorizontal: theme.spacing.lg, marginTop: theme.spacing.sm, gap: theme.spacing.md },
  row: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: theme.spacing.xs,
    gap: theme.spacing.md,
  },
  txn: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: theme.spacing.md,
    gap: theme.spacing.md,
  },
  txnInfo: { flex: 1, gap: 2 },
  txnAmounts: { alignItems: "flex-end", gap: 2 },
  btn: {
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
  disabled: { opacity: 0.4 },
});
