import { StyleSheet, TouchableOpacity, View } from "react-native";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/format";

interface ProductCardProps {
  product: StorefrontProduct;
  layout?: "grid" | "row";
}

export function ProductCard({ product, layout = "grid" }: ProductCardProps) {
  const router = useRouter();
  const onSale =
    product.compare_at_price &&
    Number(product.compare_at_price) > Number(product.price_amount);
  const cover = product.images?.[0]?.url;

  return (
    <TouchableOpacity
      onPress={() => router.push(`/products/${product.handle}`)}
      activeOpacity={0.7}
      style={layout === "grid" ? styles.gridCard : styles.rowCard}
      accessibilityRole="button"
      accessibilityLabel={`${product.title}, ${formatMoney(product.price_amount, product.currency_code)}`}
    >
      <View style={[styles.imageWrap, layout === "row" && styles.imageWrapRow]}>
        {cover ? (
          <Image source={{ uri: cover }} style={styles.image} contentFit="cover" />
        ) : (
          <View style={styles.imageFallback} />
        )}
      </View>
      <View style={styles.body}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={2}>
          {product.title}
        </Text>
        <View style={styles.priceRow}>
          <Text preset="price" color="text">
            {formatMoney(product.price_amount, product.currency_code)}
          </Text>
          {onSale ? (
            <Text preset="caption" color="textTertiary" style={styles.compare}>
              {formatMoney(product.compare_at_price, product.currency_code)}
            </Text>
          ) : null}
        </View>
      </View>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  gridCard: {
    flex: 1,
    gap: theme.spacing.sm,
  },
  rowCard: {
    flexDirection: "row",
    gap: theme.spacing.md,
    paddingVertical: theme.spacing.sm,
  },
  imageWrap: {
    aspectRatio: 1,
    width: "100%",
    backgroundColor: theme.colors.surfaceAlt,
    borderRadius: theme.radii.md,
    overflow: "hidden",
  },
  imageWrapRow: {
    width: 88,
    height: 88,
    aspectRatio: undefined,
  },
  image: { width: "100%", height: "100%" },
  imageFallback: { flex: 1, backgroundColor: theme.colors.surfaceAlt },
  body: { flex: 1, gap: 4 },
  priceRow: { flexDirection: "row", alignItems: "baseline", gap: theme.spacing.sm },
  compare: { textDecorationLine: "line-through" },
});
