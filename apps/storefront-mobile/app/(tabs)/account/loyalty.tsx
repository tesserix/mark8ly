import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  Pressable,
  ActivityIndicator,
  Share,
} from "react-native";
import * as Clipboard from "expo-clipboard";
import { Star, Copy, Share2 } from "lucide-react-native";
import { useLoyaltyProgram, useLoyaltyMembership, useEnrollLoyalty } from "@/lib/hooks/use-loyalty";
import type { LoyaltyTransaction } from "@/lib/storefront-api/loyalty";

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export default function LoyaltyScreen() {
  const { data: program, isLoading: programLoading } = useLoyaltyProgram();
  const { data: membership, isLoading: membershipLoading } = useLoyaltyMembership();
  const enrollMutation = useEnrollLoyalty();

  if (programLoading || membershipLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#0E0E0C" />
      </View>
    );
  }

  if (!membership || !membership.enrolled) {
    return (
      <View style={styles.centered}>
        <Star size={48} color="#CCCCCC" />
        <Text style={styles.enrollTitle}>Join our loyalty program</Text>
        <Text style={styles.enrollSubtitle}>
          Earn points on every purchase and unlock exclusive rewards.
        </Text>
        <Pressable
          style={[styles.enrollButton, enrollMutation.isPending && styles.enrollButtonDisabled]}
          onPress={() => enrollMutation.mutate()}
          disabled={enrollMutation.isPending}
          accessibilityRole="button"
          accessibilityLabel="Enroll in loyalty program"
        >
          {enrollMutation.isPending ? (
            <ActivityIndicator size="small" color="#FFFFFF" />
          ) : (
            <Text style={styles.enrollButtonText}>Enroll now</Text>
          )}
        </Pressable>
        {enrollMutation.isError && (
          <Text style={styles.errorText}>
            {enrollMutation.error instanceof Error
              ? enrollMutation.error.message
              : "Enrollment failed"}
          </Text>
        )}
      </View>
    );
  }

  const progressPercent =
    membership.points_to_next_tier && membership.points_to_next_tier > 0
      ? Math.min(
          (membership.points_balance /
            (membership.points_balance + membership.points_to_next_tier)) *
            100,
          100,
        )
      : 100;

  const handleCopyReferral = async () => {
    await Clipboard.setStringAsync(membership.referral_code);
  };

  const handleShareReferral = async () => {
    await Share.share({
      message: `Join me on ${program?.name ?? "our store"} and get rewards! Use my referral code: ${membership.referral_code}`,
    });
  };

  return (
    <ScrollView
      style={styles.container}
      contentContainerStyle={styles.content}
      showsVerticalScrollIndicator={false}
    >
      {/* Points Balance */}
      <View style={styles.balanceCard}>
        <Text style={styles.balanceLabel}>Your points</Text>
        <Text style={styles.balanceValue}>
          {membership.points_balance.toLocaleString()}
        </Text>
        <Text style={styles.lifetimeLabel}>
          {membership.lifetime_points.toLocaleString()} lifetime points
        </Text>
      </View>

      {/* Tier Progress */}
      <View style={styles.section}>
        <Text style={styles.sectionTitle}>Current tier</Text>
        <View style={styles.tierCard}>
          <View style={styles.tierHeader}>
            <Text style={styles.tierName}>{membership.current_tier}</Text>
            {membership.next_tier && (
              <Text style={styles.nextTier}>Next: {membership.next_tier}</Text>
            )}
          </View>
          <View style={styles.progressTrack}>
            <View
              style={[styles.progressFill, { width: `${progressPercent}%` }]}
            />
          </View>
          {membership.points_to_next_tier != null &&
            membership.points_to_next_tier > 0 && (
              <Text style={styles.progressLabel}>
                {membership.points_to_next_tier.toLocaleString()} points to next
                tier
              </Text>
            )}
        </View>
      </View>

      {/* Referral Code */}
      <View style={styles.section}>
        <Text style={styles.sectionTitle}>Referral code</Text>
        <View style={styles.referralCard}>
          <Text style={styles.referralCode}>{membership.referral_code}</Text>
          <View style={styles.referralActions}>
            <Pressable
              style={styles.referralButton}
              onPress={handleCopyReferral}
              accessibilityRole="button"
              accessibilityLabel="Copy referral code"
            >
              <Copy size={16} color="#0E0E0C" />
              <Text style={styles.referralButtonText}>Copy</Text>
            </Pressable>
            <Pressable
              style={styles.referralButton}
              onPress={handleShareReferral}
              accessibilityRole="button"
              accessibilityLabel="Share referral code"
            >
              <Share2 size={16} color="#0E0E0C" />
              <Text style={styles.referralButtonText}>Share</Text>
            </Pressable>
          </View>
        </View>
      </View>

      {/* Points History */}
      {membership.history.length > 0 && (
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Points history</Text>
          {membership.history.map((tx) => (
            <TransactionRow key={tx.id} transaction={tx} />
          ))}
        </View>
      )}
    </ScrollView>
  );
}

function TransactionRow({ transaction }: { transaction: LoyaltyTransaction }) {
  const isPositive = transaction.type === "earned" || transaction.type === "adjusted";
  const sign = isPositive ? "+" : "-";
  const color = isPositive ? "#2D4A2B" : "#8B2020";

  return (
    <View style={styles.txRow}>
      <View style={styles.txInfo}>
        <Text style={styles.txDescription}>{transaction.description}</Text>
        <Text style={styles.txDate}>{formatDate(transaction.created_at)}</Text>
      </View>
      <Text style={[styles.txPoints, { color }]}>
        {sign}{Math.abs(transaction.points).toLocaleString()}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#F7F6F2",
  },
  content: {
    padding: 16,
    paddingBottom: 40,
  },
  centered: {
    flex: 1,
    backgroundColor: "#F7F6F2",
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    gap: 10,
  },
  enrollTitle: {
    fontSize: 20,
    fontWeight: "700",
    color: "#0E0E0C",
    fontFamily: "SourceSerif4",
    marginTop: 12,
  },
  enrollSubtitle: {
    fontSize: 14,
    color: "#666666",
    textAlign: "center",
    lineHeight: 20,
  },
  enrollButton: {
    backgroundColor: "#0E0E0C",
    height: 48,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 40,
    marginTop: 8,
  },
  enrollButtonDisabled: {
    opacity: 0.6,
  },
  enrollButtonText: {
    color: "#FFFFFF",
    fontSize: 16,
    fontWeight: "600",
  },
  errorText: {
    fontSize: 13,
    color: "#8B2020",
    marginTop: 4,
  },
  balanceCard: {
    backgroundColor: "#0E0E0C",
    borderRadius: 6,
    padding: 24,
    alignItems: "center",
    gap: 4,
  },
  balanceLabel: {
    fontSize: 14,
    color: "#F7F6F2",
    opacity: 0.7,
  },
  balanceValue: {
    fontSize: 40,
    fontWeight: "800",
    color: "#F7F6F2",
    fontFamily: "SourceSerif4",
  },
  lifetimeLabel: {
    fontSize: 12,
    color: "#F7F6F2",
    opacity: 0.5,
    marginTop: 4,
  },
  section: {
    marginTop: 24,
    gap: 10,
  },
  sectionTitle: {
    fontSize: 15,
    fontWeight: "700",
    color: "#0E0E0C",
  },
  tierCard: {
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    padding: 16,
    gap: 10,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
  },
  tierHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  tierName: {
    fontSize: 16,
    fontWeight: "700",
    color: "#0E0E0C",
  },
  nextTier: {
    fontSize: 13,
    color: "#666666",
  },
  progressTrack: {
    height: 6,
    backgroundColor: "#E5E4DF",
    borderRadius: 3,
    overflow: "hidden",
  },
  progressFill: {
    height: "100%",
    backgroundColor: "#2D4A2B",
    borderRadius: 3,
  },
  progressLabel: {
    fontSize: 12,
    color: "#666666",
  },
  referralCard: {
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    padding: 16,
    gap: 12,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
  },
  referralCode: {
    fontSize: 20,
    fontWeight: "700",
    color: "#0E0E0C",
    letterSpacing: 2,
    textAlign: "center",
  },
  referralActions: {
    flexDirection: "row",
    justifyContent: "center",
    gap: 16,
  },
  referralButton: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: "#E5E4DF",
  },
  referralButtonText: {
    fontSize: 13,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  txRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    padding: 14,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
  },
  txInfo: {
    flex: 1,
    gap: 2,
  },
  txDescription: {
    fontSize: 14,
    color: "#0E0E0C",
  },
  txDate: {
    fontSize: 12,
    color: "#999999",
  },
  txPoints: {
    fontSize: 15,
    fontWeight: "700",
  },
});
