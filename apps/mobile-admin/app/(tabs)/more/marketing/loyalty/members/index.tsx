import { useCallback } from "react";
import {
  View,
  FlatList,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { ChevronRight } from "lucide-react-native";
import { useLoyaltyMembers } from "@/lib/hooks/use-loyalty";
import { BackHeader, EmptyState, PressableRow, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { LoyaltyMember } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

function MemberRow({ member, onPress }: { member: LoyaltyMember; onPress: (m: LoyaltyMember) => void }) {
  return (
    <PressableRow
      lines={2}
      style={styles.row}
      onPress={() => onPress(member)}
      accessibilityLabel={`${member.customer_name || member.customer_email}, ${member.points_balance} points`}
    >
      <View style={styles.info}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
          {member.customer_name || member.customer_email}
        </Text>
        <Text preset="caption" color="textSecondary" numberOfLines={1}>
          {member.points_balance} pts · {member.tier}
        </Text>
      </View>
      <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
    </PressableRow>
  );
}

export default function LoyaltyMembersScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const { data, isLoading, isRefetching, isError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useLoyaltyMembers();

  const members = data?.pages.flatMap((page) => page.data) ?? [];

  const handlePress = useCallback(
    (member: LoyaltyMember) => router.push(`/(tabs)/more/marketing/loyalty/members/${member.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const renderItem = useCallback(
    ({ item }: { item: LoyaltyMember }) => <MemberRow member={item} onPress={handlePress} />,
    [handlePress],
  );

  return (
    <Screen>
      <BackHeader eyebrow="LOYALTY" title="Members" />
      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && members.length === 0 ? (
        <View style={styles.centered}>
          <EmptyState
            title="Couldn't load members"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <FlatList
          data={members}
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
            <EmptyState title="No members yet" message="Customers appear here once they earn points." />
          }
        />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  row: {
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
  },
  info: { flex: 1, gap: 4 },
  list: { flexGrow: 1, paddingBottom: theme.spacing.huge },
  footer: { paddingVertical: theme.spacing.lg, alignItems: "center" },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
});
