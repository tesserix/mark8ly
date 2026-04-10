import { useState, useCallback, useEffect, useRef } from "react";
import {
  View,
  Text,
  TextInput,
  FlatList,
  StyleSheet,
  Pressable,
  ActivityIndicator,
  Dimensions,
} from "react-native";
import { X } from "lucide-react-native";
import { useProducts } from "@/lib/hooks/use-products";
import { useSearchHistoryStore } from "@/stores/search-history-store";
import { ProductCard } from "@/components/ProductCard";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";

const SCREEN_WIDTH = Dimensions.get("window").width;
const GRID_GAP = 12;
const GRID_PADDING = 16;
const CARD_WIDTH = (SCREEN_WIDTH - GRID_PADDING * 2 - GRID_GAP) / 2;
const DEBOUNCE_MS = 400;

export default function SearchScreen() {
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const inputRef = useRef<TextInput>(null);
  const searches = useSearchHistoryStore((s) => s.searches);
  const addSearch = useSearchHistoryStore((s) => s.addSearch);
  const removeSearch = useSearchHistoryStore((s) => s.removeSearch);
  const clearHistory = useSearchHistoryStore((s) => s.clear);

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQuery(query.trim());
    }, DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [query]);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const {
    data,
    isLoading,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useProducts({
    search: debouncedQuery || undefined,
  });

  const products = data?.pages.flatMap((p) => p.products) ?? [];
  const isSearching = debouncedQuery.length > 0;

  const handleSubmit = useCallback(() => {
    const trimmed = query.trim();
    if (trimmed) {
      addSearch(trimmed);
    }
  }, [query, addSearch]);

  const handleRecentPress = useCallback(
    (term: string) => {
      setQuery(term);
      setDebouncedQuery(term);
      addSearch(term);
    },
    [addSearch],
  );

  const renderProduct = ({ item }: { item: StorefrontProduct }) => (
    <View style={[styles.gridItem, { width: CARD_WIDTH }]}>
      <ProductCard product={item} />
    </View>
  );

  if (!isSearching) {
    return (
      <View style={styles.container}>
        <View style={styles.inputContainer}>
          <TextInput
            ref={inputRef}
            style={styles.input}
            placeholder="Search products..."
            placeholderTextColor="#999999"
            value={query}
            onChangeText={setQuery}
            onSubmitEditing={handleSubmit}
            returnKeyType="search"
            autoCapitalize="none"
            autoCorrect={false}
          />
          {query.length > 0 && (
            <Pressable onPress={() => setQuery("")} style={styles.clearButton}>
              <X size={18} color="#666666" />
            </Pressable>
          )}
        </View>

        {searches.length > 0 && (
          <View style={styles.recentSection}>
            <View style={styles.recentHeader}>
              <Text style={styles.recentTitle}>Recent searches</Text>
              <Pressable onPress={clearHistory}>
                <Text style={styles.clearText}>Clear</Text>
              </Pressable>
            </View>
            {searches.map((term) => (
              <View key={term} style={styles.recentRow}>
                <Pressable
                  style={styles.recentTerm}
                  onPress={() => handleRecentPress(term)}
                >
                  <Text style={styles.recentTermText}>{term}</Text>
                </Pressable>
                <Pressable onPress={() => removeSearch(term)}>
                  <X size={16} color="#999999" />
                </Pressable>
              </View>
            ))}
          </View>
        )}
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <View style={styles.inputContainer}>
        <TextInput
          ref={inputRef}
          style={styles.input}
          placeholder="Search products..."
          placeholderTextColor="#999999"
          value={query}
          onChangeText={setQuery}
          onSubmitEditing={handleSubmit}
          returnKeyType="search"
          autoCapitalize="none"
          autoCorrect={false}
        />
        {query.length > 0 && (
          <Pressable onPress={() => setQuery("")} style={styles.clearButton}>
            <X size={18} color="#666666" />
          </Pressable>
        )}
      </View>

      {isLoading ? (
        <View style={styles.centered}>
          <ActivityIndicator size="large" color="#0E0E0C" />
        </View>
      ) : products.length === 0 ? (
        <View style={styles.centered}>
          <Text style={styles.emptyTitle}>No results</Text>
          <Text style={styles.emptySubtitle}>
            No products match "{debouncedQuery}"
          </Text>
        </View>
      ) : (
        <FlatList
          data={products}
          keyExtractor={(item) => item.id}
          renderItem={renderProduct}
          numColumns={2}
          columnWrapperStyle={styles.gridRow}
          contentContainerStyle={styles.gridContent}
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
  inputContainer: {
    flexDirection: "row",
    alignItems: "center",
    margin: 16,
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
    paddingHorizontal: 12,
  },
  input: {
    flex: 1,
    height: 44,
    fontSize: 15,
    color: "#0E0E0C",
  },
  clearButton: {
    padding: 4,
  },
  recentSection: {
    paddingHorizontal: 16,
  },
  recentHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 12,
  },
  recentTitle: {
    fontSize: 15,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  clearText: {
    fontSize: 13,
    color: "#2D4A2B",
    fontWeight: "500",
  },
  recentRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: 10,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: "#E5E4DF",
  },
  recentTerm: {
    flex: 1,
  },
  recentTermText: {
    fontSize: 14,
    color: "#0E0E0C",
  },
  gridContent: {
    paddingHorizontal: GRID_PADDING,
    paddingBottom: 32,
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
});
