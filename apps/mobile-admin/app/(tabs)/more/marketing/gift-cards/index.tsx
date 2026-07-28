import { useCallback, useState } from "react";
import { View, RefreshControl, ActivityIndicator, StyleSheet } from "react-native";
import Animated from "react-native-reanimated";
import { useRouter } from "expo-router";
import { Plus } from "lucide-react-native";
import { useGiftCards } from "@/lib/hooks/use-gift-cards";
import { GiftCardRow } from "@/components/marketing/GiftCardRow";
import {
  CollapsingHeader,
  EmptyState,
  FilterChips,
  IconButton,
  Screen,
} from "@/components/ui";
import { useCollapsingScroll } from "@/lib/use-collapsing-scroll";
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

/**
 * Gift cards — a list the merchant READS, not one they work through.
 *
 * The screen increment 3's rollout kit is proved on, and it was chosen for
 * what it CANNOT do: the admin API has no enable, disable or delete for a
 * gift card, so there is nothing to swipe to and nothing to put in a
 * long-press menu. An armed gesture that can only 4xx is worse than no
 * gesture at all — the same rule that gives a terminal order no `SwipeRow` on
 * the Orders screen. Row press navigates to the detail; that is the whole
 * interaction surface, deliberately.
 *
 * That absence is the point rather than an oversight: it isolates the header
 * work (`CollapsingHeader`'s additive `onBack` and the shared
 * `useCollapsingScroll`) from any mutation risk, so a regression found on
 * this screen can only be a header regression.
 */
export default function GiftCardsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const { scrollY, onScroll } = useCollapsingScroll();
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
      <CollapsingHeader
        eyebrow="MARKETING"
        title="Gift cards"
        // This is a NESTED route — it used `BackHeader`, which had a chevron
        // this primitive did not. `onBack` is the additive prop that closed
        // that gap; without it the merchant would be stranded here.
        onBack={() => router.back()}
        rightSlot={
          <IconButton
            onPress={() => router.push("/(tabs)/more/marketing/gift-cards/new")}
            accessibilityLabel="Issue gift card"
          >
            <Plus size={22} color={theme.colors.text} strokeWidth={1.75} />
          </IconButton>
        }
        scrollY={scrollY}
      />
      {/* Pills, matching Orders. Pinned OUTSIDE the list and above it — not
          in `ListHeaderComponent`. `FilterChips` owns its own vertical rhythm
          and a hugging wrapper; inside the list that wrapper stretches and
          leaves ~110pt of dead paper between the header and the first row.
          Semantics untouched — `all` sends no `status`, every other key IS
          the `status` value. */}
      <FilterChips<FilterKey> chips={FILTERS} value={filter} onChange={setFilter} />

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && cards.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load gift cards"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.FlatList
          testID="gift-cards-list"
          data={cards}
          renderItem={renderItem}
          keyExtractor={(item) => (item as GiftCard).id}
          // The pair that makes the header collapse at all: a plain FlatList
          // has nothing to feed `scrollY` and would freeze it expanded.
          onScroll={onScroll}
          scrollEventThrottle={16}
          style={styles.listFlex}
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
  // Explicit flex, not leftover space: the list is the only child of `Screen`
  // that should absorb the remaining height. Same as Orders.
  listFlex: { flex: 1 },
  // `paddingBottom` comes from `useDockClearance` at the call site — the old
  // hard-coded `theme.spacing.huge` here was a second, smaller answer to the
  // same question and lost to the inline one anyway.
  list: { flexGrow: 1 },
  footer: { paddingVertical: theme.spacing.lg, alignItems: "center" },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  // NOT `styles.centered` for the error state: that wrapper's
  // `alignItems: "center"` shrink-wraps and re-centres its child, silently
  // undoing `EmptyState align="left"`. Claims the remaining height only.
  errorSlot: { flex: 1 },
});
