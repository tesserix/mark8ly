import { View, Text, Image, TouchableOpacity, StyleSheet } from "react-native";
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
          { backgroundColor: isActive ? "#2D4A2B" : "#0E0E0C40" },
        ]}
      />
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  container: {
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    padding: 12,
    marginHorizontal: 16,
    marginBottom: 8,
    borderWidth: 0.5,
    borderColor: "#0E0E0C10",
    flexDirection: "row",
    alignItems: "center",
  },
  thumbnail: {
    width: 48,
    height: 48,
    borderRadius: 6,
  },
  thumbnailPlaceholder: {
    backgroundColor: "#F7F6F2",
  },
  info: {
    flex: 1,
    marginLeft: 12,
  },
  name: {
    fontSize: 15,
    fontWeight: "600",
    color: "#0E0E0C",
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
    color: "#0E0E0C",
  },
  stock: {
    fontSize: 12,
    color: "#0E0E0C",
    opacity: 0.5,
  },
  statusDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    marginLeft: 8,
  },
});
