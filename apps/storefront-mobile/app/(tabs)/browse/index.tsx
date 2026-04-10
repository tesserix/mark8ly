import { useMemo, useCallback } from "react";
import {
  View,
  Text,
  FlatList,
  StyleSheet,
  ActivityIndicator,
  Pressable,
  useWindowDimensions,
  RefreshControl,
} from "react-native";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { useTheme } from "@/lib/theme/theme-provider";
import { useCategories } from "@/lib/hooks/use-categories";
import type { StorefrontCategory } from "@repo/mobile-shared/api/storefront-types";

const GRID_GAP = 12;
const GRID_PADDING = 16;

export default function BrowseScreen() {
  const router = useRouter();
  const theme = useTheme();
  const { width: screenWidth } = useWindowDimensions();
  const { data: categories, isLoading, refetch, isRefetching } = useCategories();

  const cardWidth = (screenWidth - GRID_PADDING * 2 - GRID_GAP) / 2;

  const themed = useMemo(
    () => ({
      centered: { backgroundColor: theme.background },
      content: { backgroundColor: theme.background },
      card: { backgroundColor: theme.elevated, borderColor: theme.border },
      imageContainer: { backgroundColor: theme.background },
      imagePlaceholder: { backgroundColor: theme.border },
      placeholderText: { color: theme.textSecondary },
      cardName: { color: theme.text },
      cardCount: { color: theme.textSecondary },
      emptyTitle: { color: theme.text },
      emptySubtitle: { color: theme.textSecondary },
    }),
    [theme],
  );

  const renderItem = useCallback(
    ({ item }: { item: StorefrontCategory }) => (
      <Pressable
        style={[styles.card, themed.card, { width: cardWidth }]}
        onPress={() => router.push(`/(tabs)/browse/category/${item.slug}`)}
        accessibilityRole="button"
        accessibilityLabel={`${item.name}, ${item.product_count} products`}
      >
        <View style={[styles.imageContainer, themed.imageContainer]}>
          {item.image_url ? (
            <Image
              source={{ uri: item.image_url }}
              style={styles.image}
              contentFit="cover"
              transition={200}
              accessibilityLabel={item.name}
            />
          ) : (
            <View style={[styles.imagePlaceholder, themed.imagePlaceholder]}>
              <Text style={[styles.placeholderText, themed.placeholderText]}>
                {item.name.charAt(0).toUpperCase()}
              </Text>
            </View>
          )}
        </View>
        <View style={styles.cardInfo}>
          <Text style={[styles.cardName, themed.cardName]} numberOfLines={1}>
            {item.name}
          </Text>
          <Text style={[styles.cardCount, themed.cardCount]}>
            {item.product_count} {item.product_count === 1 ? "product" : "products"}
          </Text>
        </View>
      </Pressable>
    ),
    [cardWidth, themed, router],
  );

  if (isLoading) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <ActivityIndicator size="large" color={theme.primary} />
      </View>
    );
  }

  if (!categories || categories.length === 0) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <Text style={[styles.emptyTitle, themed.emptyTitle]}>No categories yet</Text>
        <Text style={[styles.emptySubtitle, themed.emptySubtitle]}>
          Categories will appear here once the store is configured.
        </Text>
      </View>
    );
  }

  return (
    <FlatList
      data={categories}
      keyExtractor={(item) => item.id}
      renderItem={renderItem}
      numColumns={2}
      columnWrapperStyle={styles.row}
      contentContainerStyle={[styles.content, themed.content]}
      refreshControl={
        <RefreshControl
          refreshing={isRefetching}
          onRefresh={refetch}
          tintColor={theme.primary}
        />
      }
    />
  );
}

const styles = StyleSheet.create({
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    padding: 32,
  },
  content: {
    padding: GRID_PADDING,
  },
  row: {
    gap: GRID_GAP,
    marginBottom: GRID_GAP,
  },
  card: {
    borderRadius: 6,
    overflow: "hidden",
    borderWidth: StyleSheet.hairlineWidth,
  },
  imageContainer: {
    aspectRatio: 1,
  },
  image: {
    width: "100%",
    height: "100%",
  },
  imagePlaceholder: {
    width: "100%",
    height: "100%",
    alignItems: "center",
    justifyContent: "center",
  },
  placeholderText: {
    fontSize: 32,
    fontWeight: "700",
    fontFamily: "SourceSerif4",
  },
  cardInfo: {
    padding: 10,
  },
  cardName: {
    fontSize: 14,
    fontWeight: "600",
  },
  cardCount: {
    fontSize: 12,
    marginTop: 2,
  },
  emptyTitle: {
    fontSize: 18,
    fontWeight: "600",
    marginBottom: 8,
  },
  emptySubtitle: {
    fontSize: 14,
    textAlign: "center",
    lineHeight: 20,
  },
});
