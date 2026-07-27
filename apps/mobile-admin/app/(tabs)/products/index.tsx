import { useState, useCallback, useEffect } from "react";
import {
  View,
  FlatList,
  RefreshControl,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import { Plus } from "lucide-react-native";
import Animated, { FadeIn, useReducedMotion } from "react-native-reanimated";
import { useProducts } from "../../../lib/hooks/use-products";
import { ProductRow } from "../../../components/ProductRow";
import {
  EmptyState,
  IconButton,
  PageHeader,
  Screen,
  SearchField,
  SegmentedControl,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import { DISCLOSURE_EASING } from "@/components/products/disclosure-motion";
import type { Product } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "active" | "draft";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "active", label: "Active" },
  // Was "Inactive" -> status=inactive, a hard 400: the backend enum is
  // draft|active|archived. 149 of the store's 161 products are drafts.
  { key: "draft", label: "Draft" },
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
  const reduceMotion = useReducedMotion();
  const [activeFilter, setActiveFilter] = useState<FilterKey>("all");
  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);

  const queryParams = {
    ...(activeFilter !== "all" ? { status: activeFilter } : {}),
    ...(debouncedSearch ? { search: debouncedSearch } : {}),
  };

  const { data, isLoading, isRefetching, isError, refetch } = useProducts(
    Object.keys(queryParams).length > 0 ? queryParams : undefined,
  );

  const products = data?.data ?? [];

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
      ) : isError && products.length === 0 ? (
        <View style={styles.centered}>
          <EmptyState
            title="Couldn't load products"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.View
          testID="products-list-wrap"
          style={styles.listWrap}
          entering={reduceMotion ? undefined : FadeIn.duration(180).easing(DISCLOSURE_EASING)}
        >
          <FlatList
            data={products}
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
        </Animated.View>
      )}

      <IconButton
        onPress={() => router.push("/(tabs)/products/new")}
        accessibilityLabel="Add new product"
        tone="onDark"
        style={[styles.fab, { bottom: dockPad }]}
      >
        <Plus size={22} color={theme.colors.inverse} strokeWidth={2} />
      </IconButton>
    </Screen>
  );
}

const styles = StyleSheet.create({
  search: {
    // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH so
    // the search field aligns with ProductRow below it. Not theme.spacing.lg.
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xs,
  },
  listWrap: {
    flex: 1,
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
  // `bottom` is applied at the call site from useDockClearance() — the dock
  // is absolutely positioned and renders above this, so a static bottom
  // (it was theme.spacing.lg) parked the FAB underneath it and Add-product
  // was unreachable.
  fab: {
    position: "absolute",
    right: theme.spacing.lg,
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
