import { useCallback, useState } from "react";
import {
  View,
  FlatList,
  RefreshControl,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import Animated, { FadeIn, useReducedMotion } from "react-native-reanimated";
import { useReviews } from "../../../../lib/hooks/use-reviews";
import { ReviewRow } from "../../../../components/reviews/ReviewRow";
import { BackHeader, EmptyState, FilterChips, Screen } from "@/components/ui";
import { theme } from "@/lib/theme";
import { DISCLOSURE_EASING } from "@/components/products/disclosure-motion";
import type { Review } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "pending" | "approved" | "rejected";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "pending", label: "Pending" },
  { key: "approved", label: "Approved" },
  { key: "rejected", label: "Rejected" },
];

export default function ReviewsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const reduceMotion = useReducedMotion();
  const [activeFilter, setActiveFilter] = useState<FilterKey>("all");

  const {
    data,
    isLoading,
    isRefetching,
    isError,
    refetch,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useReviews(activeFilter !== "all" ? { status: activeFilter } : undefined);

  const reviews = data?.pages.flatMap((page) => page.data) ?? [];

  const handlePress = useCallback(
    (review: Review) => router.push(`/(tabs)/customers/reviews/${review.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const renderItem = useCallback(
    ({ item }: { item: Review }) => <ReviewRow review={item} onPress={handlePress} />,
    [handlePress],
  );

  return (
    <Screen>
      <BackHeader eyebrow="CUSTOMERS" title="Reviews" />
      {/* Pills, matching Orders. No wrapper: `FilterChips` owns the padding
          above and below the strip. Semantics untouched — `all` sends no
          `status`, every other key IS the `status` value. */}
      <FilterChips<FilterKey>
        chips={FILTERS}
        value={activeFilter}
        onChange={setActiveFilter}
      />

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && reviews.length === 0 ? (
        <View style={styles.centered}>
          <EmptyState
            title="Couldn't load reviews"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.View
          testID="reviews-list-wrap"
          style={styles.listWrap}
          entering={reduceMotion ? undefined : FadeIn.duration(180).easing(DISCLOSURE_EASING)}
        >
          <FlatList
            data={reviews}
            renderItem={renderItem}
            keyExtractor={(item) => item.id}
            contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
            onEndReached={handleEndReached}
            onEndReachedThreshold={0.5}
            refreshControl={
              <RefreshControl
                refreshing={isRefetching}
                onRefresh={refetch}
                tintColor={theme.colors.text}
              />
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
                title="No reviews yet"
                message={
                  activeFilter !== "all"
                    ? "No reviews with this status."
                    : "Customer reviews appear here for moderation."
                }
              />
            }
          />
        </Animated.View>
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  listWrap: { flex: 1 },
  list: {
    flexGrow: 1,
    paddingBottom: theme.spacing.huge,
  },
  footer: {
    paddingVertical: theme.spacing.lg,
    alignItems: "center",
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
});
