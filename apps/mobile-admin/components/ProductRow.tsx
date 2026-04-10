import { View, Text, Image, TouchableOpacity, StyleSheet } from "react-native";
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

  return (
    <TouchableOpacity
      style={styles.container}
      onPress={() => onPress(product)}
      activeOpacity={0.7}
      accessibilityRole="button"
      accessibilityLabel={`${product.name}, ${formatCurrency(product.price)}, stock ${product.stock}, ${product.status}`}
    >
      {product.thumbnail_url ? (
        <Image
          source={{ uri: product.thumbnail_url }}
          style={styles.thumbnail}
          accessibilityLabel={`${product.name} thumbnail`}
        />
      ) : (
        <View style={[styles.thumbnail, styles.thumbnailPlaceholder]} />
      )}
      <View style={styles.info}>
        <Text style={styles.name} numberOfLines={1}>
          {product.name}
        </Text>
        <View style={styles.metaRow}>
          <Text style={styles.price}>{formatCurrency(product.price)}</Text>
          <Text style={styles.stock}>{product.stock} in stock</Text>
        </View>
      </View>
      <View
        style={[
          styles.statusDot,
          { backgroundColor: isActive ? theme.colors.accent : `${theme.colors.text}40` },
        ]}
        accessibilityLabel={isActive ? "Active" : "Inactive"}
      />
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  container: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    padding: theme.spacing.md,
    marginHorizontal: theme.spacing.lg,
    marginBottom: theme.spacing.sm,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
    flexDirection: "row",
    alignItems: "center",
  },
  thumbnail: {
    width: 48,
    height: 48,
    borderRadius: theme.radius,
  },
  thumbnailPlaceholder: {
    backgroundColor: theme.colors.background,
  },
  info: {
    flex: 1,
    marginLeft: theme.spacing.md,
  },
  name: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.text,
    marginBottom: 3,
  },
  metaRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
  },
  price: {
    fontSize: 14,
    fontWeight: "700",
    color: theme.colors.text,
  },
  stock: {
    fontSize: 12,
    color: theme.colors.text,
    opacity: 0.5,
  },
  statusDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    marginLeft: theme.spacing.sm,
  },
});
