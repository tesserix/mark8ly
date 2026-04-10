import {
  View,
  Text,
  FlatList,
  StyleSheet,
  ActivityIndicator,
  Pressable,
  Dimensions,
  RefreshControl,
} from "react-native";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { useCategories } from "@/lib/hooks/use-categories";
import type { StorefrontCategory } from "@repo/mobile-shared/api/storefront-types";

const SCREEN_WIDTH = Dimensions.get("window").width;
const GRID_GAP = 12;
const GRID_PADDING = 16;
const CARD_WIDTH = (SCREEN_WIDTH - GRID_PADDING * 2 - GRID_GAP) / 2;

export default function BrowseScreen() {
  const router = useRouter();
  const { data: categories, isLoading, refetch, isRefetching } = useCategories();

  if (isLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#0E0E0C" />
      </View>
    );
  }

  if (!categories || categories.length === 0) {
    return (
      <View style={styles.centered}>
        <Text style={styles.emptyTitle}>No categories yet</Text>
        <Text style={styles.emptySubtitle}>
          Categories will appear here once the store is configured.
        </Text>
      </View>
    );
  }

  const renderItem = ({ item }: { item: StorefrontCategory }) => (
    <Pressable
      style={[styles.card, { width: CARD_WIDTH }]}
      onPress={() => router.push(`/(tabs)/browse/category/${item.slug}`)}
    >
      <View style={styles.imageContainer}>
        {item.image_url ? (
          <Image
            source={{ uri: item.image_url }}
            style={styles.image}
            contentFit="cover"
            transition={200}
          />
        ) : (
          <View style={styles.imagePlaceholder}>
            <Text style={styles.placeholderText}>
              {item.name.charAt(0).toUpperCase()}
            </Text>
          </View>
        )}
      </View>
      <View style={styles.cardInfo}>
        <Text style={styles.cardName} numberOfLines={1}>
          {item.name}
        </Text>
        <Text style={styles.cardCount}>
          {item.product_count} {item.product_count === 1 ? "product" : "products"}
        </Text>
      </View>
    </Pressable>
  );

  return (
    <FlatList
      data={categories}
      keyExtractor={(item) => item.id}
      renderItem={renderItem}
      numColumns={2}
      columnWrapperStyle={styles.row}
      contentContainerStyle={styles.content}
      refreshControl={
        <RefreshControl
          refreshing={isRefetching}
          onRefresh={refetch}
          tintColor="#0E0E0C"
        />
      }
    />
  );
}

const styles = StyleSheet.create({
  centered: {
    flex: 1,
    backgroundColor: "#F7F6F2",
    alignItems: "center",
    justifyContent: "center",
    padding: 32,
  },
  content: {
    backgroundColor: "#F7F6F2",
    padding: GRID_PADDING,
  },
  row: {
    gap: GRID_GAP,
    marginBottom: GRID_GAP,
  },
  card: {
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    overflow: "hidden",
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
  },
  imageContainer: {
    aspectRatio: 1,
    backgroundColor: "#F7F6F2",
  },
  image: {
    width: "100%",
    height: "100%",
  },
  imagePlaceholder: {
    width: "100%",
    height: "100%",
    backgroundColor: "#E5E4DF",
    alignItems: "center",
    justifyContent: "center",
  },
  placeholderText: {
    fontSize: 32,
    fontWeight: "700",
    color: "#666666",
    fontFamily: "SourceSerif4",
  },
  cardInfo: {
    padding: 10,
  },
  cardName: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  cardCount: {
    fontSize: 12,
    color: "#666666",
    marginTop: 2,
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
    lineHeight: 20,
  },
});
