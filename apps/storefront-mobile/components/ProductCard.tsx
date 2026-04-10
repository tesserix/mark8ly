import { useMemo } from "react";
import { View, Text, StyleSheet, Pressable } from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { StarRating } from "./StarRating";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";

interface ProductCardProps {
  product: StorefrontProduct;
  compact?: boolean;
}

export function ProductCard({ product, compact = false }: ProductCardProps) {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
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

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  card: {
    flex: 1,
    backgroundColor: theme.elevated,
    borderRadius: 6,
    overflow: "hidden",
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: theme.border,
  },
  imageContainer: {
    aspectRatio: 1,
    backgroundColor: theme.background,
  },
  image: {
    width: "100%",
    height: "100%",
  },
  imagePlaceholder: {
    width: "100%",
    height: "100%",
    backgroundColor: theme.border,
  },
  info: {
    padding: 10,
    gap: 4,
  },
  title: {
    fontSize: 14,
    fontWeight: "500",
    color: theme.text,
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
    color: theme.text,
  },
  comparePrice: {
    fontSize: 12,
    color: theme.textSecondary,
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
    backgroundColor: theme.background,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: theme.border,
  },
  compactImage: {
    width: "100%",
    height: "100%",
  },
  compactImagePlaceholder: {
    width: "100%",
    height: "100%",
    backgroundColor: theme.border,
  },
  compactTitle: {
    fontSize: 12,
    fontWeight: "500",
    color: theme.text,
    marginTop: 6,
  },
  compactPrice: {
    fontSize: 12,
    fontWeight: "600",
    color: theme.text,
    marginTop: 2,
  },
});
}
