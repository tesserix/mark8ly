import {
  View,
  Text,
  StyleSheet,
  FlatList,
  Pressable,
  ActivityIndicator,
  Dimensions,
} from "react-native";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { Heart, ShoppingCart } from "lucide-react-native";
import { useWishlist, useRemoveFromWishlist } from "@/lib/hooks/use-wishlist";
import { useCartStore } from "@/stores/cart-store";
import { haptics } from "@repo/mobile-shared/haptics/feedback";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";

const COLUMN_GAP = 12;
const PADDING = 16;
const screenWidth = Dimensions.get("window").width;
const cardWidth = (screenWidth - PADDING * 2 - COLUMN_GAP) / 2;

export default function WishlistScreen() {
  const router = useRouter();
  const { data: items, isLoading } = useWishlist();
  const removeMutation = useRemoveFromWishlist();
  const addItem = useCartStore((s) => s.addItem);

  if (isLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#0E0E0C" />
      </View>
    );
  }

  if (!items || items.length === 0) {
    return (
      <View style={styles.centered}>
        <Heart size={48} color="#CCCCCC" />
        <Text style={styles.emptyTitle}>No saved items</Text>
        <Text style={styles.emptySubtitle}>
          Tap the heart on products you love to save them here.
        </Text>
        <Pressable
          style={styles.browseButton}
          onPress={() => router.push("/(tabs)/browse")}
          accessibilityRole="button"
        >
          <Text style={styles.browseButtonText}>Browse products</Text>
        </Pressable>
      </View>
    );
  }

  const handleRemove = async (productId: string) => {
    await haptics.wishlistToggle();
    removeMutation.mutate(productId);
  };

  const handleAddToCart = async (product: StorefrontProduct) => {
    const defaultVariant = product.variants?.[0];
    addItem({
      productId: product.id,
      variantId: defaultVariant?.id ?? product.id,
      handle: product.handle,
      title: product.title,
      priceAmount: parseFloat(product.price_amount),
      currencyCode: product.currency_code,
      imageUrl: product.images[0]?.url,
    });
    await haptics.addToCart();
  };

  const renderItem = ({ item }: { item: StorefrontProduct }) => {
    const thumbnail = item.images[0]?.url;

    return (
      <Pressable
        style={styles.card}
        onPress={() => router.push(`/(tabs)/browse/product/${item.handle}`)}
        accessibilityRole="button"
        accessibilityLabel={item.title}
      >
        <View style={styles.imageContainer}>
          {thumbnail ? (
            <Image
              source={{ uri: thumbnail }}
              style={styles.image}
              contentFit="cover"
              transition={200}
            />
          ) : (
            <View style={styles.imagePlaceholder} />
          )}
          <Pressable
            style={styles.removeButton}
            onPress={() => handleRemove(item.id)}
            hitSlop={8}
            accessibilityRole="button"
            accessibilityLabel="Remove from wishlist"
          >
            <Heart size={18} color="#0E0E0C" fill="#0E0E0C" />
          </Pressable>
        </View>
        <View style={styles.info}>
          <Text style={styles.title} numberOfLines={2}>
            {item.title}
          </Text>
          <Text style={styles.price}>
            {item.currency_code} {item.price_amount}
          </Text>
          <Pressable
            style={styles.cartButton}
            onPress={() => handleAddToCart(item)}
            accessibilityRole="button"
            accessibilityLabel="Add to cart"
          >
            <ShoppingCart size={14} color="#FFFFFF" />
            <Text style={styles.cartButtonText}>Add to cart</Text>
          </Pressable>
        </View>
      </Pressable>
    );
  };

  return (
    <FlatList
      style={styles.container}
      contentContainerStyle={styles.listContent}
      data={items}
      keyExtractor={(item) => item.id}
      renderItem={renderItem}
      numColumns={2}
      columnWrapperStyle={styles.row}
    />
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#F7F6F2",
  },
  listContent: {
    padding: PADDING,
    gap: COLUMN_GAP,
  },
  row: {
    gap: COLUMN_GAP,
  },
  centered: {
    flex: 1,
    backgroundColor: "#F7F6F2",
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    gap: 10,
  },
  emptyTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: "#0E0E0C",
    marginTop: 12,
  },
  emptySubtitle: {
    fontSize: 14,
    color: "#666666",
    textAlign: "center",
    lineHeight: 20,
  },
  browseButton: {
    backgroundColor: "#0E0E0C",
    height: 44,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    marginTop: 8,
  },
  browseButtonText: {
    color: "#FFFFFF",
    fontSize: 15,
    fontWeight: "600",
  },
  card: {
    width: cardWidth,
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
  },
  removeButton: {
    position: "absolute",
    top: 8,
    right: 8,
    width: 32,
    height: 32,
    borderRadius: 16,
    backgroundColor: "#FFFFFF",
    alignItems: "center",
    justifyContent: "center",
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 1 },
    shadowOpacity: 0.08,
    shadowRadius: 3,
    elevation: 2,
  },
  info: {
    padding: 10,
    gap: 4,
  },
  title: {
    fontSize: 13,
    fontWeight: "500",
    color: "#0E0E0C",
    lineHeight: 17,
  },
  price: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  cartButton: {
    flexDirection: "row",
    backgroundColor: "#0E0E0C",
    height: 32,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    gap: 6,
    marginTop: 6,
  },
  cartButtonText: {
    color: "#FFFFFF",
    fontSize: 12,
    fontWeight: "600",
  },
});
