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
import { useGiftCards } from "@/lib/hooks/use-gift-cards";
import { GiftCardRow } from "@/components/marketing/GiftCardRow";
import { BackHeader, EmptyState, FilterChips, IconButton, Screen } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { GiftCard } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "active" | "redeemed" | "expired" | "disabled";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "active", label: "Active" },
  { key: "redeemed", label: "Redeemed" },
  { key: "expired", label: "Expired" },
  { key: "disabled", label: "Off" },
];

export default function GiftCardsScreen() {
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
  } = useGiftCards(filter !== "all" ? { status: filter } : undefined);

  const cards = data?.pages.flatMap((page) => page.data) ?? [];

  const handlePress = useCallback(
    (card: GiftCard) => router.push(`/(tabs)/more/marketing/gift-cards/${card.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const renderItem = useCallback(
    ({ item }: { item: GiftCard }) => <GiftCardRow card={item} onPress={handlePress} />,
    [handlePress],
  );

  return (
    <Screen>
      <BackHeader
        eyebrow="MARKETING"
        title="Gift cards"
        rightSlot={
          <IconButton
            onPress={() => router.push("/(tabs)/more/marketing/gift-cards/new")}
            accessibilityLabel="Issue gift card"
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
      ) : isError && cards.length === 0 ? (
        <View style={styles.centered}>
          <EmptyState
            title="Couldn't load gift cards"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <FlatList
          data={cards}
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
              title="No gift cards yet"
              message={
                filter !== "all"
                  ? "No gift cards with this status."
                  : "Issue a gift card for a customer."
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
});
