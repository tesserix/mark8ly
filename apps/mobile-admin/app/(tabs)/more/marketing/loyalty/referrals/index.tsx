import { useCallback } from "react";
import { View, FlatList, RefreshControl, ActivityIndicator, StyleSheet } from "react-native";
import { useReferrals } from "@/lib/hooks/use-loyalty";
import { BackHeader, EmptyState, Screen, StatusBadge, Text, type StatusTone } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Referral } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

const STATUS_TONE: Record<string, StatusTone> = {
  pending: "warning",
  completed: "success",
  expired: "muted",
};

function titleize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function formatDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("en-AU", { year: "numeric", month: "short", day: "numeric" });
}

function ReferralRow({ referral }: { referral: Referral }) {
  return (
    <View style={styles.row}>
      <View style={styles.info}>
        <Text preset="bodyEmphasis" color="text">
          +{referral.referrer_bonus} / +{referral.referee_bonus} pts
        </Text>
        <Text preset="caption" color="textTertiary">
          {formatDate(referral.created_at)}
        </Text>
      </View>
      <StatusBadge label={titleize(referral.status)} tone={STATUS_TONE[referral.status] ?? "muted"} />
    </View>
  );
}

export default function ReferralsScreen() {
  const dockPad = useDockClearance();
  const { data, isLoading, isRefetching, isError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useReferrals();

  const referrals = data?.pages.flatMap((page) => page.data) ?? [];

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const renderItem = useCallback(({ item }: { item: Referral }) => <ReferralRow referral={item} />, []);

  return (
    <Screen>
      <BackHeader eyebrow="LOYALTY" title="Referrals" />
      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && referrals.length === 0 ? (
        <View style={styles.centered}>
          <EmptyState
            title="Couldn't load referrals"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <FlatList
          data={referrals}
          renderItem={renderItem}
          keyExtractor={(item) => item.id}
          contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
          onEndReached={handleEndReached}
          onEndReachedThreshold={0.5}
          refreshControl={
            <RefreshControl refreshing={isRefetching} onRefresh={refetch} tintColor={theme.colors.text} />
          }
          ListFooterComponent={
            isFetchingNextPage ? (
              <View style={styles.footer}>
                <ActivityIndicator size="small" color={theme.colors.text} />
              </View>
            ) : null
          }
          ListEmptyComponent={
            <EmptyState title="No referrals yet" message="Referrals show up here as customers invite friends." />
          }
        />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
    gap: theme.spacing.md,
  },
  info: { flex: 1, gap: 2 },
  list: { flexGrow: 1, paddingBottom: theme.spacing.huge },
  footer: { paddingVertical: theme.spacing.lg, alignItems: "center" },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
});
