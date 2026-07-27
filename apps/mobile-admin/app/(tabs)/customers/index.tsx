import { useState, useCallback, useEffect } from "react";
import {
  View,
  FlatList,
  Pressable,
  RefreshControl,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import { Star } from "lucide-react-native";
import Animated, { FadeIn, useReducedMotion } from "react-native-reanimated";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { useCustomers } from "../../../lib/hooks/use-customers";
import { CustomerRow } from "../../../components/CustomerRow";
import {
  EmptyState,
  PageHeader,
  Screen,
  SearchField,
  SegmentedControl,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import { DISCLOSURE_EASING } from "@/components/products/disclosure-motion";
import type { Customer } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "active" | "blocked";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "active", label: "Active" },
  { key: "blocked", label: "Blocked" },
];

function useDebounce(value: string, delay: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

export default function CustomersScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const reduceMotion = useReducedMotion();
  const [activeFilter, setActiveFilter] = useState<FilterKey>("all");
  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);
  const currencyCode = useTenantStore((s) => s.activeStore?.currency_code);

  const {
    data,
    isLoading,
    isRefetching,
    isError,
    refetch,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useCustomers({
    ...(activeFilter !== "all" ? { status: activeFilter } : {}),
    ...(debouncedSearch ? { search: debouncedSearch } : {}),
  });

  const customers = data?.pages.flatMap((page) => page.data) ?? [];

  const handlePress = useCallback(
    (customer: Customer) => router.push(`/(tabs)/customers/${customer.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const renderItem = useCallback(
    ({ item }: { item: Customer }) => (
      <CustomerRow customer={item} onPress={handlePress} currencyCode={currencyCode} />
    ),
    [handlePress, currencyCode],
  );

  return (
    <Screen>
      <PageHeader
        eyebrow="CUSTOMERS"
        title="People"
        rightSlot={
          <Pressable
            style={styles.reviewsLink}
            onPress={() => router.push("/(tabs)/customers/reviews")}
            accessibilityRole="button"
            accessibilityLabel="View customer reviews"
          >
            <Star size={16} color={theme.colors.text} strokeWidth={1.75} />
            <Text preset="caption" color="text">
              Reviews
            </Text>
          </Pressable>
        }
      />
      <View style={styles.search}>
        <SearchField
          value={searchText}
          onChangeText={setSearchText}
          placeholder="Search customers…"
          accessibilityLabel="Search customers"
        />
      </View>
      <SegmentedControl<FilterKey>
        segments={FILTERS}
        value={activeFilter}
        onChange={setActiveFilter}
      />

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && customers.length === 0 ? (
        <View style={styles.centered}>
          <EmptyState
            title="Couldn't load customers"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.View
          testID="customers-list-wrap"
          style={styles.listWrap}
          entering={reduceMotion ? undefined : FadeIn.duration(180).easing(DISCLOSURE_EASING)}
        >
          <FlatList
            data={customers}
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
                title="No customers yet"
                message={
                  debouncedSearch || activeFilter !== "all"
                    ? "Try a different search or filter."
                    : "Customers appear here once they sign up."
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
  search: {
    // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH so
    // the search field aligns with the rows below it. Not theme.spacing.lg.
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xs,
  },
  reviewsLink: {
    flexDirection: "row",
    alignItems: "center",
    gap: 4,
    minHeight: 44,
  },
  listWrap: {
    flex: 1,
  },
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
