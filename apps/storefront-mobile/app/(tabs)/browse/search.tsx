import { useState, useCallback, useEffect, useRef, useMemo } from "react";
import {
  View,
  Text,
  TextInput,
  FlatList,
  StyleSheet,
  Pressable,
  ActivityIndicator,
  useWindowDimensions,
} from "react-native";
import { X } from "lucide-react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useProducts } from "@/lib/hooks/use-products";
import { useSearchHistoryStore } from "@/stores/search-history-store";
import { ProductCard } from "@/components/ProductCard";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";

const GRID_GAP = 12;
const GRID_PADDING = 16;
const DEBOUNCE_MS = 400;

export default function SearchScreen() {
  const theme = useTheme();
  const { width: screenWidth } = useWindowDimensions();
  const cardWidth = (screenWidth - GRID_PADDING * 2 - GRID_GAP) / 2;

  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const inputRef = useRef<TextInput>(null);
  const searches = useSearchHistoryStore((s) => s.searches);
  const addSearch = useSearchHistoryStore((s) => s.addSearch);
  const removeSearch = useSearchHistoryStore((s) => s.removeSearch);
  const clearHistory = useSearchHistoryStore((s) => s.clear);

  const themed = useMemo(
    () => ({
      container: { backgroundColor: theme.background },
      inputContainer: { backgroundColor: theme.elevated, borderColor: theme.border },
      input: { color: theme.text },
      recentTitle: { color: theme.text },
      clearText: { color: theme.accent },
      recentRow: { borderBottomColor: theme.border },
      recentTermText: { color: theme.text },
      emptyTitle: { color: theme.text },
      emptySubtitle: { color: theme.textSecondary },
    }),
    [theme],
  );

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

  const renderProduct = useCallback(
    ({ item }: { item: StorefrontProduct }) => (
      <View style={[styles.gridItem, { width: cardWidth }]}>
        <ProductCard product={item} />
      </View>
    ),
    [cardWidth],
  );

  if (!isSearching) {
    return (
      <View style={[styles.container, themed.container]}>
        <View style={[styles.inputContainer, themed.inputContainer]}>
          <TextInput
            ref={inputRef}
            style={[styles.input, themed.input]}
            placeholder="Search products..."
            placeholderTextColor={theme.textSecondary}
            value={query}
            onChangeText={setQuery}
            onSubmitEditing={handleSubmit}
            returnKeyType="search"
            autoCapitalize="none"
            autoCorrect={false}
            accessibilityLabel="Search products"
          />
          {query.length > 0 && (
            <Pressable
              onPress={() => setQuery("")}
              style={styles.clearButton}
              accessibilityRole="button"
              accessibilityLabel="Clear search"
            >
              <X size={18} color={theme.textSecondary} />
            </Pressable>
          )}
        </View>

        {searches.length > 0 && (
          <View style={styles.recentSection}>
            <View style={styles.recentHeader}>
              <Text style={[styles.recentTitle, themed.recentTitle]}>
                Recent searches
              </Text>
              <Pressable
                onPress={clearHistory}
                accessibilityRole="button"
                accessibilityLabel="Clear search history"
              >
                <Text style={[styles.clearText, themed.clearText]}>Clear</Text>
              </Pressable>
            </View>
            {searches.map((term) => (
              <View key={term} style={[styles.recentRow, themed.recentRow]}>
                <Pressable
                  style={styles.recentTerm}
                  onPress={() => handleRecentPress(term)}
                  accessibilityRole="button"
                  accessibilityLabel={`Search for ${term}`}
                >
                  <Text style={[styles.recentTermText, themed.recentTermText]}>
                    {term}
                  </Text>
                </Pressable>
                <Pressable
                  onPress={() => removeSearch(term)}
                  accessibilityRole="button"
                  accessibilityLabel={`Remove ${term} from history`}
                >
                  <X size={16} color={theme.textSecondary} />
                </Pressable>
              </View>
            ))}
          </View>
        )}
      </View>
    );
  }

  return (
    <View style={[styles.container, themed.container]}>
      <View style={[styles.inputContainer, themed.inputContainer]}>
        <TextInput
          ref={inputRef}
          style={[styles.input, themed.input]}
          placeholder="Search products..."
          placeholderTextColor={theme.textSecondary}
          value={query}
          onChangeText={setQuery}
          onSubmitEditing={handleSubmit}
          returnKeyType="search"
          autoCapitalize="none"
          autoCorrect={false}
          accessibilityLabel="Search products"
        />
        {query.length > 0 && (
          <Pressable
            onPress={() => setQuery("")}
            style={styles.clearButton}
            accessibilityRole="button"
            accessibilityLabel="Clear search"
          >
            <X size={18} color={theme.textSecondary} />
          </Pressable>
        )}
      </View>

      {isLoading ? (
        <View style={styles.centered}>
          <ActivityIndicator size="large" color={theme.primary} />
        </View>
      ) : products.length === 0 ? (
        <View style={styles.centered}>
          <Text style={[styles.emptyTitle, themed.emptyTitle]}>No results</Text>
          <Text style={[styles.emptySubtitle, themed.emptySubtitle]}>
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
                color={theme.primary}
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
    borderRadius: 6,
    borderWidth: StyleSheet.hairlineWidth,
    paddingHorizontal: 12,
  },
  input: {
    flex: 1,
    height: 44,
    fontSize: 15,
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
  },
  clearText: {
    fontSize: 13,
    fontWeight: "500",
  },
  recentRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: 10,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  recentTerm: {
    flex: 1,
  },
  recentTermText: {
    fontSize: 14,
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
    marginBottom: 8,
  },
  emptySubtitle: {
    fontSize: 14,
    textAlign: "center",
  },
});
