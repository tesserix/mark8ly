import { useState, useCallback, useEffect } from "react";
import {
  View,
  FlatList,
  RefreshControl,
  TouchableOpacity,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import { Plus } from "lucide-react-native";
import { useProducts } from "../../../lib/hooks/use-products";
import { ProductRow } from "../../../components/ProductRow";
import {
  EmptyState,
  PageHeader,
  Screen,
  SearchField,
  SegmentedControl,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Product } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

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
  const dockPad = useDockClearance();
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

  const handlePress = useCallback(
    (product: Product) => router.push(`/(tabs)/products/${product.id}`),
    [router],
  );

  const renderItem = useCallback(
    ({ item }: { item: Product }) => <ProductRow product={item} onPress={handlePress} />,
    [handlePress],
  );

  return (
    <Screen>
      <PageHeader eyebrow="PRODUCTS" title="Catalog" />
      <View style={styles.search}>
        <SearchField
          value={searchText}
          onChangeText={setSearchText}
          placeholder="Search products…"
          accessibilityLabel="Search products"
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
              title="No products yet"
              message={
                debouncedSearch
                  ? "Try a different search term."
                  : "Add your first product to get started."
              }
            />
          }
        />
      )}

      <TouchableOpacity
        style={styles.fab}
        onPress={() => router.push("/(tabs)/products/new")}
        activeOpacity={0.85}
        accessibilityRole="button"
        accessibilityLabel="Add new product"
      >
        <Plus size={22} color={theme.colors.inverse} strokeWidth={2} />
      </TouchableOpacity>
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
    paddingBottom: 96,
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
  fab: {
    position: "absolute",
    right: theme.spacing.lg,
    bottom: theme.spacing.lg,
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: theme.colors.accent,
    alignItems: "center",
    justifyContent: "center",
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.18,
    shadowRadius: 10,
    elevation: 6,
  },
});
