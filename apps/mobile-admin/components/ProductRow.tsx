import { View, Image, TouchableOpacity, StyleSheet } from "react-native";
import { Package } from "lucide-react-native";
import { StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Product } from "@repo/mobile-shared/api/types";

interface ProductRowProps {
  product: Product;
  onPress: (product: Product) => void;
}

function formatCurrency(amount: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
  }).format(amount);
}

export function ProductRow({ product, onPress }: ProductRowProps) {
  const isActive = product.status === "active";
  const lowStock = product.stock <= 5;

  return (
    <TouchableOpacity
      style={styles.container}
      onPress={() => onPress(product)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`${product.name}, ${formatCurrency(product.price)}, stock ${product.stock}, ${product.status}`}
    >
      {product.thumbnail_url ? (
        <Image
          source={{ uri: product.thumbnail_url }}
          style={styles.thumb}
          accessibilityLabel={`${product.name} thumbnail`}
        />
      ) : (
        <View style={[styles.thumb, styles.thumbPlaceholder]}>
          <Package size={20} color={theme.colors.textTertiary} strokeWidth={1.5} />
        </View>
      )}

      <View style={styles.info}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
          {product.name}
        </Text>
        <View style={styles.metaRow}>
          <Text preset="caption" color="text">
            {formatCurrency(product.price)}
          </Text>
          <Text preset="caption" color={lowStock ? "danger" : "textTertiary"}>
            {product.stock} in stock
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
