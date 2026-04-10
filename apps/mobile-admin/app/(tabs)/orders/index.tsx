import { useState, useCallback, useEffect } from "react";
import {
  View,
  Text,
  TextInput,
  FlatList,
  RefreshControl,
  TouchableOpacity,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import { useOrders } from "../../../lib/hooks/use-orders";
import { OrderRow } from "../../../components/OrderRow";
import { theme } from "@/lib/theme";
import type { Order } from "@repo/mobile-shared/api/types";

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
    (order: Order) => {
      router.push(`/(tabs)/orders/${order.id}`);
    },
    [router],
  );

  const renderItem = useCallback(
    ({ item }: { item: Order }) => (
      <OrderRow order={item} onPress={handleOrderPress} />
    ),
    [handleOrderPress],
  );

  const keyExtractor = useCallback((item: Order) => item.id, []);

  return (
    <View style={styles.screen}>
      <View style={styles.filtersRow}>
        {FILTERS.map((filter) => {
          const isActive = activeFilter === filter.key;
          return (
            <TouchableOpacity
              key={filter.key}
              style={[styles.filterBtn, isActive && styles.filterBtnActive]}
              onPress={() => setActiveFilter(filter.key)}
              accessibilityRole="button"
              accessibilityState={{ selected: isActive }}
              accessibilityLabel={`Filter by ${filter.label}`}
            >
              <Text
                style={[
                  styles.filterText,
                  isActive && styles.filterTextActive,
                ]}
              >
                {filter.label}
              </Text>
            </TouchableOpacity>
          );
        })}
      </View>

      <View style={styles.searchContainer}>
        <TextInput
          style={styles.searchInput}
          placeholder="Search orders..."
          placeholderTextColor={`${theme.colors.text}50`}
          value={searchText}
          onChangeText={setSearchText}
          autoCapitalize="none"
          autoCorrect={false}
          returnKeyType="search"
          accessibilityLabel="Search orders"
        />
      </View>

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="large" color={theme.colors.text} />
        </View>
      ) : (
        <FlatList
          data={data?.items ?? []}
          renderItem={renderItem}
          keyExtractor={keyExtractor}
          contentContainerStyle={styles.listContent}
          refreshControl={
            <RefreshControl
              refreshing={isRefetching}
              onRefresh={refetch}
              tintColor={theme.colors.text}
            />
          }
          ListEmptyComponent={
            <View style={styles.centered}>
              <Text style={styles.emptyTitle}>No orders found</Text>
              <Text style={styles.emptySubtitle}>
                {debouncedSearch
                  ? "Try a different search term"
                  : "Orders will appear here once placed"}
              </Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: theme.colors.background,
  },
  filtersRow: {
    flexDirection: "row",
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.md,
    paddingBottom: theme.spacing.sm,
    gap: theme.spacing.sm,
  },
  filterBtn: {
    paddingHorizontal: 14,
    paddingVertical: 7,
    borderRadius: theme.radius,
    backgroundColor: "transparent",
  },
  filterBtnActive: {
    backgroundColor: theme.colors.text,
  },
  filterText: {
    fontSize: 13,
    fontWeight: "600",
    color: theme.colors.text,
  },
  filterTextActive: {
    color: theme.colors.background,
  },
  searchContainer: {
    paddingHorizontal: theme.spacing.lg,
    paddingBottom: theme.spacing.md,
  },
  searchInput: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    paddingHorizontal: 14,
    paddingVertical: 10,
    fontSize: 14,
    color: theme.colors.text,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  listContent: {
    paddingTop: theme.spacing.xs,
    paddingBottom: theme.spacing.xxl,
    flexGrow: 1,
  },
  centered: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    paddingTop: 80,
  },
  emptyTitle: {
    fontSize: 16,
    fontWeight: "600",
    color: theme.colors.text,
    marginBottom: theme.spacing.xs,
  },
  emptySubtitle: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.5,
  },
});
