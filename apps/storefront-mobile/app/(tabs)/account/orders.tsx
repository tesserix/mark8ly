import { useMemo, useCallback } from "react";
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
import { useTheme } from "@/lib/theme/theme-provider";
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
  const theme = useTheme();
  const {
    data: orders,
    isLoading,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    refetch,
    isRefetching,
  } = useOrders();

  const themed = useMemo(
    () => ({
      container: { backgroundColor: theme.background },
      centered: { backgroundColor: theme.background },
      emptyTitle: { color: theme.text },
      emptySubtitle: { color: theme.textSecondary },
      shopButton: { backgroundColor: theme.primary },
      shopButtonText: { color: theme.elevated },
      orderCard: { backgroundColor: theme.elevated, borderColor: theme.border },
      orderNumber: { color: theme.text },
      orderDate: { color: theme.textSecondary },
      orderMeta: { color: theme.textSecondary },
      orderTotal: { color: theme.text },
    }),
    [theme],
  );

  const handleLoadMore = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) {
      fetchNextPage();
    }
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const renderItem = useCallback(
    ({ item }: { item: OrderSummary }) => {
      const statusColor = STATUS_COLORS[item.status] ?? "#666666";

      return (
        <Pressable
          style={[styles.orderCard, themed.orderCard]}
          onPress={() => router.push(`/(tabs)/account/orders/${item.id}`)}
          accessibilityRole="button"
          accessibilityLabel={`Order ${item.order_number}, ${item.status}`}
        >
          <View style={styles.orderHeader}>
            <Text style={[styles.orderNumber, themed.orderNumber]}>#{item.order_number}</Text>
            <View style={[styles.statusBadge, { backgroundColor: `${statusColor}15` }]}>
              <Text style={[styles.statusText, { color: statusColor }]}>
                {item.status.charAt(0).toUpperCase() + item.status.slice(1)}
              </Text>
            </View>
          </View>
          <View style={styles.orderDetails}>
            <Text style={[styles.orderDate, themed.orderDate]}>{formatDate(item.created_at)}</Text>
            <Text style={[styles.orderMeta, themed.orderMeta]}>
              {item.item_count} {item.item_count === 1 ? "item" : "items"}
            </Text>
          </View>
          <Text style={[styles.orderTotal, themed.orderTotal]}>
            {item.currency_code} {item.total}
          </Text>
        </Pressable>
      );
    },
    [themed, router],
  );

  if (isLoading) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <ActivityIndicator size="large" color={theme.primary} />
      </View>
    );
  }

  if (!orders || orders.length === 0) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <ShoppingBag size={48} color="#CCCCCC" />
        <Text style={[styles.emptyTitle, themed.emptyTitle]}>No orders yet</Text>
        <Text style={[styles.emptySubtitle, themed.emptySubtitle]}>
          When you place an order, it will appear here.
        </Text>
        <Pressable
          style={[styles.shopButton, themed.shopButton]}
          onPress={() => router.push("/(tabs)/browse")}
          accessibilityRole="button"
          accessibilityLabel="Start shopping"
        >
          <Text style={[styles.shopButtonText, themed.shopButtonText]}>Start shopping</Text>
        </Pressable>
      </View>
    );
  }

  return (
    <FlatList
      style={[styles.container, themed.container]}
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
          tintColor={theme.primary}
        />
      }
      ListFooterComponent={
        isFetchingNextPage ? (
          <View style={styles.footer}>
            <ActivityIndicator size="small" color={theme.primary} />
          </View>
        ) : null
      }
    />
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
  },
  listContent: {
    padding: 16,
    gap: 12,
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    gap: 10,
  },
  emptyTitle: {
    fontSize: 18,
    fontWeight: "700",
    marginTop: 12,
  },
  emptySubtitle: {
    fontSize: 14,
    textAlign: "center",
    lineHeight: 20,
  },
  shopButton: {
    height: 44,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    marginTop: 8,
  },
  shopButtonText: {
    fontSize: 15,
    fontWeight: "600",
  },
  orderCard: {
    borderRadius: 6,
    padding: 16,
    gap: 8,
    borderWidth: StyleSheet.hairlineWidth,
  },
  orderHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  orderNumber: {
    fontSize: 15,
    fontWeight: "700",
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
  },
  orderMeta: {
    fontSize: 13,
  },
  orderTotal: {
    fontSize: 15,
    fontWeight: "600",
  },
  footer: {
    paddingVertical: 20,
    alignItems: "center",
  },
});
