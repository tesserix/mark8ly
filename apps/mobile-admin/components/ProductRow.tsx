import { View, StyleSheet } from "react-native";
import { PressableRow, StatusBadge, Text, Thumb } from "@/components/ui";
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
  const priceLabel =
    price === undefined ? "—" : formatMoney(price, productCurrency(product));
  const lowStock = stock <= 5;

  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(product)}
      style={styles.row}
      testID={`product-row-${product.id}`}
      accessibilityLabel={`${product.title}, ${priceLabel}, stock ${stock}, ${product.status}`}
    >
      <Thumb
        uri={thumb}
        recyclingKey={product.id}
        accessibilityLabel={`${product.title} thumbnail`}
      />

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
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  row: {
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
  },
  info: { flex: 1, gap: 4, minWidth: 0 },
  metaRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.md,
  },
});
