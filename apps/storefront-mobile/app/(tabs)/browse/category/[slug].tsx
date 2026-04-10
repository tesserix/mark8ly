import { useState, useCallback } from "react";
import {
  View,
  Text,
  FlatList,
  StyleSheet,
  Pressable,
  ActivityIndicator,
  Dimensions,
  RefreshControl,
  Modal,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useCategoryProducts } from "@/lib/hooks/use-categories";
import { ProductCard } from "@/components/ProductCard";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";

const SCREEN_WIDTH = Dimensions.get("window").width;
const GRID_GAP = 12;
const GRID_PADDING = 16;
const CARD_WIDTH = (SCREEN_WIDTH - GRID_PADDING * 2 - GRID_GAP) / 2;

const SORT_OPTIONS = [
  { label: "Newest", value: "newest" },
  { label: "Price: Low to High", value: "price_asc" },
  { label: "Price: High to Low", value: "price_desc" },
] as const;

type SortValue = (typeof SORT_OPTIONS)[number]["value"];

export default function CategoryScreen() {
  const { slug } = useLocalSearchParams<{ slug: string }>();
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

  const handleSortSelect = useCallback((value: SortValue) => {
    setSort(value);
    setShowSort(false);
  }, []);

  const currentSortLabel =
    SORT_OPTIONS.find((o) => o.value === sort)?.label ?? "Sort";

  if (isLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#0E0E0C" />
      </View>
    );
  }

  const renderItem = ({ item }: { item: StorefrontProduct }) => (
    <View style={[styles.gridItem, { width: CARD_WIDTH }]}>
      <ProductCard product={item} />
    </View>
  );

  return (
    <View style={styles.container}>
      <View style={styles.toolbar}>
        <Text style={styles.resultCount}>
          {total} {total === 1 ? "product" : "products"}
        </Text>
        <Pressable style={styles.sortButton} onPress={() => setShowSort(true)}>
          <Text style={styles.sortButtonText}>{currentSortLabel}</Text>
        </Pressable>
      </View>

      {products.length === 0 ? (
        <View style={styles.centered}>
          <Text style={styles.emptyTitle}>No products</Text>
          <Text style={styles.emptySubtitle}>
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
              tintColor="#0E0E0C"
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
                color="#0E0E0C"
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
        <Pressable style={styles.modalOverlay} onPress={() => setShowSort(false)}>
          <View style={styles.modalSheet}>
            <View style={styles.modalHandle} />
            <Text style={styles.modalTitle}>Sort by</Text>
            {SORT_OPTIONS.map((option) => (
              <Pressable
                key={option.value}
                style={[
                  styles.modalOption,
                  sort === option.value && styles.modalOptionActive,
                ]}
                onPress={() => handleSortSelect(option.value)}
              >
                <Text
                  style={[
                    styles.modalOptionText,
                    sort === option.value && styles.modalOptionTextActive,
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
    backgroundColor: "#F7F6F2",
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
    borderBottomColor: "#E5E4DF",
  },
  resultCount: {
    fontSize: 13,
    color: "#666666",
  },
  sortButton: {
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 6,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
    backgroundColor: "#FFFFFF",
  },
  sortButtonText: {
    fontSize: 13,
    fontWeight: "500",
    color: "#0E0E0C",
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
    color: "#0E0E0C",
    marginBottom: 8,
  },
  emptySubtitle: {
    fontSize: 14,
    color: "#666666",
    textAlign: "center",
  },
  modalOverlay: {
    flex: 1,
    backgroundColor: "rgba(0, 0, 0, 0.4)",
    justifyContent: "flex-end",
  },
  modalSheet: {
    backgroundColor: "#FFFFFF",
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
    backgroundColor: "#E5E4DF",
    alignSelf: "center",
    marginBottom: 16,
  },
  modalTitle: {
    fontSize: 16,
    fontWeight: "600",
    color: "#0E0E0C",
    marginBottom: 16,
  },
  modalOption: {
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: "#E5E4DF",
  },
  modalOptionActive: {
    backgroundColor: "#F7F6F2",
    marginHorizontal: -16,
    paddingHorizontal: 16,
  },
  modalOptionText: {
    fontSize: 15,
    color: "#0E0E0C",
  },
  modalOptionTextActive: {
    fontWeight: "600",
    color: "#2D4A2B",
  },
});
