import {
  View,
  Text,
  StyleSheet,
  FlatList,
  Pressable,
  ActivityIndicator,
  RefreshControl,
} from "react-native";
import { useRouter } from "expo-router";
import { useOrders } from "@/lib/hooks/use-orders";
import { ShoppingBag } from "lucide-react-native";
import type { OrderSummary } from "@/lib/storefront-api/orders";

const STATUS_COLORS: Record<string, string> = {
  pending: "#B8860B",
  processing: "#2D4A2B",
  shipped: "#2D4A2B",
  delivered: "#1A7A1A",
  cancelled: "#8B2020",
  refunded: "#666666",
};

function formatDate(iso: string): string {
  const date = new Date(iso);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export default function OrdersScreen() {
  const router = useRouter();
  const {
    data: orders,
    isLoading,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    refetch,
    isRefetching,
  } = useOrders();

  if (isLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#0E0E0C" />
      </View>
    );
  }

  if (!orders || orders.length === 0) {
    return (
      <View style={styles.centered}>
        <ShoppingBag size={48} color="#CCCCCC" />
        <Text style={styles.emptyTitle}>No orders yet</Text>
        <Text style={styles.emptySubtitle}>
          When you place an order, it will appear here.
        </Text>
        <Pressable
          style={styles.shopButton}
          onPress={() => router.push("/(tabs)/browse")}
          accessibilityRole="button"
        >
          <Text style={styles.shopButtonText}>Start shopping</Text>
        </Pressable>
      </View>
    );
  }

  const handleLoadMore = () => {
    if (hasNextPage && !isFetchingNextPage) {
      fetchNextPage();
    }
  };

  const renderItem = ({ item }: { item: OrderSummary }) => {
    const statusColor = STATUS_COLORS[item.status] ?? "#666666";

    return (
      <Pressable
        style={styles.orderCard}
        onPress={() => router.push(`/(tabs)/account/orders/${item.id}`)}
        accessibilityRole="button"
        accessibilityLabel={`Order ${item.order_number}`}
      >
        <View style={styles.orderHeader}>
          <Text style={styles.orderNumber}>#{item.order_number}</Text>
          <View style={[styles.statusBadge, { backgroundColor: `${statusColor}15` }]}>
            <Text style={[styles.statusText, { color: statusColor }]}>
              {item.status.charAt(0).toUpperCase() + item.status.slice(1)}
            </Text>
          </View>
        </View>
        <View style={styles.orderDetails}>
          <Text style={styles.orderDate}>{formatDate(item.created_at)}</Text>
          <Text style={styles.orderMeta}>
            {item.item_count} {item.item_count === 1 ? "item" : "items"}
          </Text>
        </View>
        <Text style={styles.orderTotal}>
          {item.currency_code} {item.total}
        </Text>
      </Pressable>
    );
  };

  return (
    <FlatList
      style={styles.container}
      contentContainerStyle={styles.listContent}
      data={orders}
      keyExtractor={(item) => item.id}
      renderItem={renderItem}
      onEndReached={handleLoadMore}
      onEndReachedThreshold={0.5}
      refreshControl={
        <RefreshControl
          refreshing={isRefetching}
          onRefresh={refetch}
          tintColor="#0E0E0C"
        />
      }
      ListFooterComponent={
        isFetchingNextPage ? (
          <View style={styles.footer}>
            <ActivityIndicator size="small" color="#0E0E0C" />
          </View>
        ) : null
      }
    />
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#F7F6F2",
  },
  listContent: {
    padding: 16,
    gap: 12,
  },
  centered: {
    flex: 1,
    backgroundColor: "#F7F6F2",
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    gap: 10,
  },
  emptyTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: "#0E0E0C",
    marginTop: 12,
  },
  emptySubtitle: {
    fontSize: 14,
    color: "#666666",
    textAlign: "center",
    lineHeight: 20,
  },
  shopButton: {
    backgroundColor: "#0E0E0C",
    height: 44,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    marginTop: 8,
  },
  shopButtonText: {
    color: "#FFFFFF",
    fontSize: 15,
    fontWeight: "600",
  },
  orderCard: {
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    padding: 16,
    gap: 8,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
  },
  orderHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  orderNumber: {
    fontSize: 15,
    fontWeight: "700",
    color: "#0E0E0C",
  },
  statusBadge: {
    paddingHorizontal: 10,
    paddingVertical: 4,
    borderRadius: 4,
  },
  statusText: {
    fontSize: 12,
    fontWeight: "600",
  },
  orderDetails: {
    flexDirection: "row",
    gap: 12,
  },
  orderDate: {
    fontSize: 13,
    color: "#666666",
  },
  orderMeta: {
    fontSize: 13,
    color: "#666666",
  },
  orderTotal: {
    fontSize: 15,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  footer: {
    paddingVertical: 20,
    alignItems: "center",
  },
});
