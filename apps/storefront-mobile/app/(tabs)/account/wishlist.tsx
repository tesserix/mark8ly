import { useMemo, useCallback } from "react";
import { View, Text, StyleSheet, FlatList, Pressable, ActivityIndicator, useWindowDimensions } from "react-native";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { Heart, ShoppingCart } from "lucide-react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useWishlist, useRemoveFromWishlist } from "@/lib/hooks/use-wishlist";
import { useCartStore } from "@/stores/cart-store";
import { haptics } from "@repo/mobile-shared/haptics/feedback";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";

const COLUMN_GAP = 12;
const PADDING = 16;

export default function WishlistScreen() {
  const router = useRouter();
  const theme = useTheme();
  const { width: screenWidth } = useWindowDimensions();
  const cardWidth = (screenWidth - PADDING * 2 - COLUMN_GAP) / 2;
  const { data: items, isLoading } = useWishlist();
  const removeMutation = useRemoveFromWishlist();
  const addItem = useCartStore((s) => s.addItem);

  const themed = useMemo(() => ({
    container: { backgroundColor: theme.background },
    centered: { backgroundColor: theme.background },
    emptyTitle: { color: theme.text },
    emptySubtitle: { color: theme.textSecondary },
    browseButton: { backgroundColor: theme.primary },
    browseButtonText: { color: theme.elevated },
    card: { backgroundColor: theme.elevated, borderColor: theme.border },
    imageContainer: { backgroundColor: theme.background },
    imagePlaceholder: { backgroundColor: theme.border },
    removeButton: { backgroundColor: theme.elevated },
    title: { color: theme.text },
    price: { color: theme.text },
    cartButton: { backgroundColor: theme.primary },
    cartButtonText: { color: theme.elevated },
  }), [theme]);

  const handleRemove = useCallback(async (productId: string) => {
    await haptics.wishlistToggle();
    removeMutation.mutate(productId);
  }, [removeMutation]);

  const handleAddToCart = useCallback(async (product: StorefrontProduct) => {
    const defaultVariant = product.variants?.[0];
    addItem({ productId: product.id, variantId: defaultVariant?.id ?? product.id, handle: product.handle, title: product.title, priceAmount: parseFloat(product.price_amount), currencyCode: product.currency_code, imageUrl: product.images[0]?.url });
    await haptics.addToCart();
  }, [addItem]);

  const renderItem = useCallback(({ item }: { item: StorefrontProduct }) => {
    const thumbnail = item.images[0]?.url;
    return (
      <Pressable style={[styles.card, themed.card, { width: cardWidth }]} onPress={() => router.push(`/(tabs)/browse/product/${item.handle}`)} accessibilityRole="button" accessibilityLabel={item.title}>
        <View style={[styles.imageContainer, themed.imageContainer]}>
          {thumbnail ? (<Image source={{ uri: thumbnail }} style={styles.image} contentFit="cover" transition={200} accessibilityLabel={item.title} />) : (<View style={[styles.imagePlaceholder, themed.imagePlaceholder]} />)}
          <Pressable style={[styles.removeButton, themed.removeButton]} onPress={() => handleRemove(item.id)} hitSlop={8} accessibilityRole="button" accessibilityLabel="Remove from wishlist">
            <Heart size={18} color={theme.text} fill={theme.text} />
          </Pressable>
        </View>
        <View style={styles.info}>
          <Text style={[styles.title, themed.title]} numberOfLines={2}>{item.title}</Text>
          <Text style={[styles.price, themed.price]}>{item.currency_code} {item.price_amount}</Text>
          <Pressable style={[styles.cartButton, themed.cartButton]} onPress={() => handleAddToCart(item)} accessibilityRole="button" accessibilityLabel="Add to cart">
            <ShoppingCart size={14} color={theme.elevated} />
            <Text style={[styles.cartButtonText, themed.cartButtonText]}>Add to cart</Text>
          </Pressable>
        </View>
      </Pressable>
    );
  }, [themed, cardWidth, theme, router, handleRemove, handleAddToCart]);

  if (isLoading) { return (<View style={[styles.centered, themed.centered]}><ActivityIndicator size="large" color={theme.primary} /></View>); }

  if (!items || items.length === 0) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <Heart size={48} color="#CCCCCC" />
        <Text style={[styles.emptyTitle, themed.emptyTitle]}>No saved items</Text>
        <Text style={[styles.emptySubtitle, themed.emptySubtitle]}>Tap the heart on products you love to save them here.</Text>
        <Pressable style={[styles.browseButton, themed.browseButton]} onPress={() => router.push("/(tabs)/browse")} accessibilityRole="button" accessibilityLabel="Browse products">
          <Text style={[styles.browseButtonText, themed.browseButtonText]}>Browse products</Text>
        </Pressable>
      </View>
    );
  }

  return (<FlatList style={[styles.container, themed.container]} contentContainerStyle={styles.listContent} data={items} keyExtractor={(item) => item.id} renderItem={renderItem} numColumns={2} columnWrapperStyle={styles.row} />);
}

const styles = StyleSheet.create({
  container: { flex: 1 }, listContent: { padding: PADDING, gap: COLUMN_GAP }, row: { gap: COLUMN_GAP },
  centered: { flex: 1, alignItems: "center", justifyContent: "center", paddingHorizontal: 32, gap: 10 },
  emptyTitle: { fontSize: 18, fontWeight: "700", marginTop: 12 }, emptySubtitle: { fontSize: 14, textAlign: "center", lineHeight: 20 },
  browseButton: { height: 44, borderRadius: 6, alignItems: "center", justifyContent: "center", paddingHorizontal: 32, marginTop: 8 },
  browseButtonText: { fontSize: 15, fontWeight: "600" },
  card: { borderRadius: 6, overflow: "hidden", borderWidth: StyleSheet.hairlineWidth },
  imageContainer: { aspectRatio: 1 }, image: { width: "100%", height: "100%" }, imagePlaceholder: { width: "100%", height: "100%" },
  removeButton: { position: "absolute", top: 8, right: 8, width: 44, height: 44, borderRadius: 22, alignItems: "center", justifyContent: "center", shadowColor: "#000", shadowOffset: { width: 0, height: 1 }, shadowOpacity: 0.08, shadowRadius: 3, elevation: 2 },
  info: { padding: 10, gap: 4 }, title: { fontSize: 13, fontWeight: "500", lineHeight: 17 }, price: { fontSize: 14, fontWeight: "600" },
  cartButton: { flexDirection: "row", height: 44, borderRadius: 6, alignItems: "center", justifyContent: "center", gap: 6, marginTop: 6 },
  cartButtonText: { fontSize: 12, fontWeight: "600" },
});
