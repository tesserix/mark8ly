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
  /**
   * Opens the long-press action menu on the Products screen. Optional so the
   * row stays usable on any list that has no per-row menu — same contract as
   * `OrderRow`'s.
   */
  onLongPress?: (product: Product) => void;
}

/** "archived" → "Archived". The badge sits beside "Active" in one column. */
function titleize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

/**
 * Deliberately NOT wrapped in `SwipeRow` — the caller wraps it, the same
 * split `OrderRow`/`QueueRow` use, because the legal swipe actions depend on
 * the product's status.
 */
export function ProductRow({ product, onPress, onLongPress }: ProductRowProps) {
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
      onLongPress={onLongPress ? () => onLongPress(product) : undefined}
      style={styles.row}
      testID={`product-row-${product.id}`}
      accessibilityLabel={`${product.title}, ${priceLabel}, stock ${stock}, ${product.status}`}
      accessibilityHint={onLongPress ? "Long press for more actions" : undefined}
    >
      <Thumb uri={thumb} recyclingKey={product.id} />

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
        label={titleize(product.status)}
        tone={isActive ? "success" : "muted"}
        testID={`product-row-${product.id}-badge`}
      />
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  // Paper, and no border of its own: these rows sit directly on the screen
  // ground and are separated by hairlines the SCREEN draws between them (see
  // products/index.tsx `renderItem`), not by being cards. Same move `OrderRow`
  // made — a self-painted white row with its own bottom rule is a bordered
  // card in a design system whose first rule is hairlines between sections.
  // It also double-ruled once the screen started drawing its own.
  row: {
    backgroundColor: theme.colors.background,
  },
  info: { flex: 1, gap: 4, minWidth: 0 },
  metaRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.md,
  },
});
