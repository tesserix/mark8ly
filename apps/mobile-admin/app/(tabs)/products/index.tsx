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
import { useProducts } from "../../../lib/hooks/use-products";
import { ProductRow } from "../../../components/ProductRow";
import { theme } from "@/lib/theme";
import type { Product } from "@repo/mobile-shared/api/types";

type FilterKey = "all" | "low_stock" | "inactive";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "low_stock", label: "Low Stock" },
  { key: "inactive", label: "Inactive" },
];

function useDebounce(value: string, delay: number): string {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);

  return debounced;
}

export default function ProductsScreen() {
  const router = useRouter();
  const [activeFilter, setActiveFilter] = useState<FilterKey>("all");
  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);

  const queryParams = {
    ...(activeFilter === "inactive" ? { status: "inactive" } : {}),
    ...(activeFilter === "low_stock" ? { low_stock: true } : {}),
    ...(debouncedSearch ? { search: debouncedSearch } : {}),
  };

  const { data, isLoading, isRefetching, refetch } = useProducts(
    Object.keys(queryParams).length > 0 ? queryParams : undefined,
  );

  const handleProductPress = useCallback(
    (product: Product) => {
      router.push(`/(tabs)/products/${product.id}`);
    },
    [router],
  );

  const renderItem = useCallback(
    ({ item }: { item: Product }) => (
      <ProductRow product={item} onPress={handleProductPress} />
    ),
    [handleProductPress],
  );

  const keyExtractor = useCallback((item: Product) => item.id, []);

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
          placeholder="Search products..."
          placeholderTextColor={`${theme.colors.text}50`}
          value={searchText}
          onChangeText={setSearchText}
          autoCapitalize="none"
          autoCorrect={false}
          returnKeyType="search"
          accessibilityLabel="Search products"
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
              <Text style={styles.emptyTitle}>No products found</Text>
              <Text style={styles.emptySubtitle}>
                {debouncedSearch
                  ? "Try a different search term"
                  : "Add your first product to get started"}
              </Text>
            </View>
          }
        />
      )}

      <TouchableOpacity
        style={styles.fab}
        onPress={() => router.push("/(tabs)/products/new")}
        activeOpacity={0.8}
        accessibilityRole="button"
        accessibilityLabel="Add new product"
      >
        <Text style={styles.fabText}>+</Text>
      </TouchableOpacity>
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
    paddingBottom: 80,
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
  fab: {
    position: "absolute",
    bottom: theme.spacing.xxl,
    right: theme.spacing.xl,
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.2,
    shadowRadius: 6,
    elevation: 6,
  },
  fabText: {
    color: theme.colors.background,
    fontSize: 28,
    fontWeight: "400",
    marginTop: -2,
  },
});
