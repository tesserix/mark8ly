import { useState, useCallback, useMemo } from "react";
import {
  View,
  Text,
  FlatList,
  StyleSheet,
  Pressable,
  ActivityIndicator,
  useWindowDimensions,
  RefreshControl,
  Modal,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useTheme } from "@/lib/theme/theme-provider";
import { useCategoryProducts } from "@/lib/hooks/use-categories";
import { ProductCard } from "@/components/ProductCard";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";

const GRID_GAP = 12;
const GRID_PADDING = 16;

const SORT_OPTIONS = [
  { label: "Newest", value: "newest" },
  { label: "Price: Low to High", value: "price_asc" },
  { label: "Price: High to Low", value: "price_desc" },
] as const;

type SortValue = (typeof SORT_OPTIONS)[number]["value"];

export default function CategoryScreen() {
  const { slug } = useLocalSearchParams<{ slug: string }>();
  const theme = useTheme();
  const { width: screenWidth } = useWindowDimensions();
  const cardWidth = (screenWidth - GRID_PADDING * 2 - GRID_GAP) / 2;
  const [sort, setSort] = useState<SortValue>("newest");
  const [showSort, setShowSort] = useState(false);

  const {
    data,
    isLoading,
    refetch,
    isRefetching,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useCategoryProducts(slug ?? "", { sort });

  const products = data?.pages.flatMap((p) => p.products) ?? [];
  const total = data?.pages[0]?.total ?? 0;

  const themed = useMemo(
    () => ({
      container: { backgroundColor: theme.background },
      toolbar: { borderBottomColor: theme.border },
      resultCount: { color: theme.textSecondary },
      sortButton: { borderColor: theme.border, backgroundColor: theme.elevated },
      sortButtonText: { color: theme.text },
      emptyTitle: { color: theme.text },
      emptySubtitle: { color: theme.textSecondary },
      modalSheet: { backgroundColor: theme.elevated },
      modalHandle: { backgroundColor: theme.border },
      modalTitle: { color: theme.text },
      modalOption: { borderBottomColor: theme.border },
      modalOptionActive: { backgroundColor: theme.background },
      modalOptionText: { color: theme.text },
      modalOptionTextActive: { color: theme.accent },
    }),
    [theme],
  );

  const handleSortSelect = useCallback((value: SortValue) => {
    setSort(value);
    setShowSort(false);
  }, []);

  const currentSortLabel =
    SORT_OPTIONS.find((o) => o.value === sort)?.label ?? "Sort";

  const renderItem = useCallback(
    ({ item }: { item: StorefrontProduct }) => (
      <View style={[styles.gridItem, { width: cardWidth }]}>
        <ProductCard product={item} />
      </View>
    ),
    [cardWidth],
  );

  if (isLoading) {
    return (
      <View style={[styles.centered, themed.container]}>
        <ActivityIndicator size="large" color={theme.primary} />
      </View>
    );
  }

  return (
    <View style={[styles.container, themed.container]}>
      <View style={[styles.toolbar, themed.toolbar]}>
        <Text style={[styles.resultCount, themed.resultCount]}>
          {total} {total === 1 ? "product" : "products"}
        </Text>
        <Pressable
          style={[styles.sortButton, themed.sortButton]}
          onPress={() => setShowSort(true)}
          accessibilityRole="button"
          accessibilityLabel={`Sort by ${currentSortLabel}`}
        >
          <Text style={[styles.sortButtonText, themed.sortButtonText]}>
            {currentSortLabel}
          </Text>
        </Pressable>
      </View>

      {products.length === 0 ? (
        <View style={styles.centered}>
          <Text style={[styles.emptyTitle, themed.emptyTitle]}>No products</Text>
          <Text style={[styles.emptySubtitle, themed.emptySubtitle]}>
            This category doesn't have any products yet.
          </Text>
        </View>
      ) : (
        <FlatList
          data={products}
          keyExtractor={(item) => item.id}
          renderItem={renderItem}
          numColumns={2}
          columnWrapperStyle={styles.gridRow}
          contentContainerStyle={styles.gridContent}
          refreshControl={
            <RefreshControl
              refreshing={isRefetching}
              onRefresh={refetch}
              tintColor={theme.primary}
            />
          }
          onEndReached={() => {
            if (hasNextPage && !isFetchingNextPage) {
              fetchNextPage();
            }
          }}
          onEndReachedThreshold={0.5}
          ListFooterComponent={
            isFetchingNextPage ? (
              <ActivityIndicator
                size="small"
                color={theme.primary}
                style={styles.footerLoader}
              />
            ) : null
          }
        />
      )}

      <Modal
        visible={showSort}
        transparent
        animationType="slide"
        onRequestClose={() => setShowSort(false)}
      >
        <Pressable
          style={styles.modalOverlay}
          onPress={() => setShowSort(false)}
          accessibilityRole="button"
          accessibilityLabel="Close sort menu"
        >
          <View style={[styles.modalSheet, themed.modalSheet]}>
            <View style={[styles.modalHandle, themed.modalHandle]} />
            <Text style={[styles.modalTitle, themed.modalTitle]}>Sort by</Text>
            {SORT_OPTIONS.map((option) => (
              <Pressable
                key={option.value}
                style={[
                  styles.modalOption,
                  themed.modalOption,
                  sort === option.value && [styles.modalOptionActive, themed.modalOptionActive],
                ]}
                onPress={() => handleSortSelect(option.value)}
                accessibilityRole="radio"
                accessibilityState={{ selected: sort === option.value }}
                accessibilityLabel={option.label}
              >
                <Text
                  style={[
                    styles.modalOptionText,
                    themed.modalOptionText,
                    sort === option.value && [styles.modalOptionTextActive, themed.modalOptionTextActive],
                  ]}
                >
                  {option.label}
                </Text>
              </Pressable>
            ))}
          </View>
        </Pressable>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    padding: 32,
  },
  toolbar: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingVertical: 12,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  resultCount: {
    fontSize: 13,
  },
  sortButton: {
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 6,
    borderWidth: StyleSheet.hairlineWidth,
  },
  sortButtonText: {
    fontSize: 13,
    fontWeight: "500",
  },
  gridContent: {
    padding: GRID_PADDING,
  },
  gridRow: {
    gap: GRID_GAP,
    marginBottom: GRID_GAP,
  },
  gridItem: {
    flex: 1,
  },
  footerLoader: {
    paddingVertical: 20,
  },
  emptyTitle: {
    fontSize: 18,
    fontWeight: "600",
    marginBottom: 8,
  },
  emptySubtitle: {
    fontSize: 14,
    textAlign: "center",
  },
  modalOverlay: {
    flex: 1,
    backgroundColor: "rgba(0, 0, 0, 0.4)",
    justifyContent: "flex-end",
  },
  modalSheet: {
    borderTopLeftRadius: 16,
    borderTopRightRadius: 16,
    paddingBottom: 40,
    paddingHorizontal: 16,
    paddingTop: 12,
  },
  modalHandle: {
    width: 36,
    height: 4,
    borderRadius: 2,
    alignSelf: "center",
    marginBottom: 16,
  },
  modalTitle: {
    fontSize: 16,
    fontWeight: "600",
    marginBottom: 16,
  },
  modalOption: {
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  modalOptionActive: {
    marginHorizontal: -16,
    paddingHorizontal: 16,
  },
  modalOptionText: {
    fontSize: 15,
  },
  modalOptionTextActive: {
    fontWeight: "600",
  },
});
