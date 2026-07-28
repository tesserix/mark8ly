import { useCallback, useState } from "react";
import {
  View,
  FlatList,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { Plus } from "lucide-react-native";
import { useCampaigns } from "@/lib/hooks/use-campaigns";
import { CampaignRow } from "@/components/marketing/CampaignRow";
import { BackHeader, EmptyState, FilterChips, IconButton, Screen } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Campaign } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "draft" | "scheduled" | "sent" | "paused";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "draft", label: "Draft" },
  { key: "scheduled", label: "Scheduled" },
  { key: "sent", label: "Sent" },
  { key: "paused", label: "Paused" },
];

export default function CampaignsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const [filter, setFilter] = useState<FilterKey>("all");

  const {
    data,
    isLoading,
    isRefetching,
    isError,
    refetch,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useCampaigns(filter !== "all" ? { status: filter } : undefined);

  const campaigns = data?.pages.flatMap((page) => page.data) ?? [];

  const handlePress = useCallback(
    (campaign: Campaign) => router.push(`/(tabs)/more/marketing/campaigns/${campaign.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const renderItem = useCallback(
    ({ item }: { item: Campaign }) => <CampaignRow campaign={item} onPress={handlePress} />,
    [handlePress],
  );

  return (
    <Screen>
      <BackHeader
        eyebrow="MARKETING"
        title="Campaigns"
        rightSlot={
          <IconButton
            onPress={() => router.push("/(tabs)/more/marketing/campaigns/new")}
            accessibilityLabel="New campaign"
          >
            <Plus size={22} color={theme.colors.text} strokeWidth={1.75} />
          </IconButton>
        }
      />
      {/* Pills, matching Orders. No wrapper: `FilterChips` owns the padding
          above and below the strip. Semantics untouched — `all` sends no
          `status`, every other key IS the `status` value. */}
      <FilterChips<FilterKey> chips={FILTERS} value={filter} onChange={setFilter} />

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && campaigns.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load campaigns"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <FlatList
          data={campaigns}
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
            <EmptyState
              align="left"
              title="No campaigns yet"
              message={
                filter !== "all"
                  ? "No campaigns with this status."
                  : "Create an email campaign to reach your customers."
              }
            />
          }
        />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  list: { flexGrow: 1, paddingBottom: theme.spacing.huge },
  footer: { paddingVertical: theme.spacing.lg, alignItems: "center" },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  // NOT `styles.centered` for the error state: that wrapper's
  // `alignItems: "center"` shrink-wraps and re-centres its child, silently
  // undoing `EmptyState align="left"`. Claims the remaining height only.
  errorSlot: { flex: 1 },
});
