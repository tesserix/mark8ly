import { View, Text, StyleSheet, Pressable } from "react-native";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { StarRating } from "./StarRating";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";

interface ProductCardProps {
  product: StorefrontProduct;
  compact?: boolean;
}

export function ProductCard({ product, compact = false }: ProductCardProps) {
  const router = useRouter();

  const hasDiscount =
    product.compare_at_price &&
    parseFloat(product.compare_at_price) > parseFloat(product.price_amount);

  const thumbnail = product.images[0]?.url;

  const handlePress = () => {
    router.push(`/(tabs)/browse/product/${product.handle}`);
  };

  if (compact) {
    return (
      <Pressable style={styles.compactCard} onPress={handlePress}>
        <View style={styles.compactImageContainer}>
          {thumbnail ? (
            <Image
              source={{ uri: thumbnail }}
              style={styles.compactImage}
              contentFit="cover"
              transition={200}
            />
          ) : (
            <View style={styles.compactImagePlaceholder} />
          )}
        </View>
        <Text style={styles.compactTitle} numberOfLines={1}>
          {product.title}
        </Text>
        <Text style={styles.compactPrice}>
          {product.currency_code} {product.price_amount}
        </Text>
      </Pressable>
    );
  }

  return (
    <Pressable style={styles.card} onPress={handlePress}>
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
      </View>
      <View style={styles.info}>
        <Text style={styles.title} numberOfLines={2}>
          {product.title}
        </Text>
        <View style={styles.priceRow}>
          <Text style={styles.price}>
            {product.currency_code} {product.price_amount}
          </Text>
          {hasDiscount && (
            <Text style={styles.comparePrice}>
              {product.currency_code} {product.compare_at_price}
            </Text>
          )}
        </View>
        {product.average_rating > 0 && (
          <StarRating
            rating={product.average_rating}
            count={product.review_count}
            size={12}
          />
        )}
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  card: {
    flex: 1,
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
  info: {
    padding: 10,
    gap: 4,
  },
  title: {
    fontSize: 14,
    fontWeight: "500",
    color: "#0E0E0C",
    lineHeight: 18,
  },
  priceRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
  },
  price: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  comparePrice: {
    fontSize: 12,
    color: "#666666",
    textDecorationLine: "line-through",
  },
  compactCard: {
    width: 120,
    marginRight: 12,
  },
  compactImageContainer: {
    width: 120,
    height: 120,
    borderRadius: 6,
    overflow: "hidden",
    backgroundColor: "#F7F6F2",
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
  },
  compactImage: {
    width: "100%",
    height: "100%",
  },
  compactImagePlaceholder: {
    width: "100%",
    height: "100%",
    backgroundColor: "#E5E4DF",
  },
  compactTitle: {
    fontSize: 12,
    fontWeight: "500",
    color: "#0E0E0C",
    marginTop: 6,
  },
  compactPrice: {
    fontSize: 12,
    fontWeight: "600",
    color: "#0E0E0C",
    marginTop: 2,
  },
});
