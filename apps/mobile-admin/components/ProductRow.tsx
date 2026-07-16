import { View, Image, TouchableOpacity, StyleSheet } from "react-native";
import { Package } from "lucide-react-native";
import { StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Product } from "@repo/mobile-shared/api/types";
import {
  formatMoney,
  productCurrency,
  productPrice,
  productStock,
  productThumb,
} from "@/lib/product-display";

interface ProductRowProps {
  product: Product;
  onPress: (product: Product) => void;
}

export function ProductRow({ product, onPress }: ProductRowProps) {
  const isActive = product.status === "active";
  const price = productPrice(product);
  const stock = productStock(product);
  const thumb = productThumb(product);
  const priceLabel = price === undefined ? "—" : formatMoney(price, productCurrency(product));
  const lowStock = stock <= 5;

  return (
    <TouchableOpacity
      style={styles.container}
      onPress={() => onPress(product)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`${product.title}, ${priceLabel}, stock ${stock}, ${product.status}`}
    >
      {thumb ? (
        <Image
          source={{ uri: thumb }}
          style={styles.thumb}
          accessibilityLabel={`${product.title} thumbnail`}
        />
      ) : (
        <View style={[styles.thumb, styles.thumbPlaceholder]}>
          <Package size={20} color={theme.colors.textTertiary} strokeWidth={1.5} />
        </View>
      )}

      <View style={styles.info}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
          {product.title}
        </Text>
        <View style={styles.metaRow}>
          <Text preset="caption" color="text">
            {priceLabel}
          </Text>
          <Text preset="caption" color={lowStock ? "danger" : "textTertiary"}>
            {stock} in stock
          </Text>
        </View>
      </View>

      <StatusBadge
        label={isActive ? "Active" : product.status}
        tone={isActive ? "success" : "muted"}
      />
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  container: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
    gap: theme.spacing.md,
  },
  thumb: {
    width: 52,
    height: 52,
    borderRadius: theme.radii.sm,
    backgroundColor: theme.colors.surfaceAlt,
  },
  thumbPlaceholder: {
    alignItems: "center",
    justifyContent: "center",
  },
  info: { flex: 1, gap: 4 },
  metaRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.md,
  },
});
