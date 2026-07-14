import { useState, useCallback, useEffect } from "react";
import {
  View,
  FlatList,
  RefreshControl,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import { useOrders } from "../../../lib/hooks/use-orders";
import { OrderRow } from "../../../components/OrderRow";
import {
  EmptyState,
  PageHeader,
  Screen,
  SearchField,
  SegmentedControl,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Order } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "active" | "completed" | "cancelled";

const FILTERS: { key: FilterKey; label: string; status?: string }[] = [
  { key: "all", label: "All" },
  { key: "active", label: "Active", status: "pending,confirmed" },
  { key: "completed", label: "Completed", status: "fulfilled" },
  { key: "cancelled", label: "Cancelled", status: "cancelled" },
];

function useDebounce(value: string, delay: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

export default function OrdersScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const [activeFilter, setActiveFilter] = useState<FilterKey>("all");
  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);

  const selectedFilter = FILTERS.find((f) => f.key === activeFilter);
  const { data, isLoading, isRefetching, refetch } = useOrders(
    selectedFilter?.status,
    debouncedSearch || undefined,
  );

  const handleOrderPress = useCallback(
    (order: Order) => router.push(`/(tabs)/orders/${order.id}`),
    [router],
  );

  const renderItem = useCallback(
    ({ item }: { item: Order }) => <OrderRow order={item} onPress={handleOrderPress} />,
    [handleOrderPress],
  );

  return (
    <Screen>
      <PageHeader eyebrow="ORDERS" title="Inbox" />
      <View style={styles.search}>
        <SearchField
          value={searchText}
          onChangeText={setSearchText}
          placeholder="Search orders…"
          accessibilityLabel="Search orders"
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
      ) : (
        <FlatList
          data={data?.items ?? []}
          renderItem={renderItem}
          keyExtractor={(item) => item.id}
          contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
          refreshControl={
            <RefreshControl
              refreshing={isRefetching}
              onRefresh={refetch}
              tintColor={theme.colors.text}
            />
          }
          ListEmptyComponent={
            <EmptyState
              title="No orders found"
              message={
                debouncedSearch
                  ? "Try a different search term."
                  : "Orders will appear here once placed."
              }
            />
          }
        />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  search: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.xs,
  },
  list: {
    flexGrow: 1,
    paddingBottom: theme.spacing.huge,
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
});
