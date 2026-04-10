import {
  RefreshControl,
  ScrollView,
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
} from "react-native";
import { useRouter } from "expo-router";
import { useDashboard } from "@/lib/hooks/use-dashboard";
import { DashboardStats } from "@/components/DashboardStats";
import type { RecentOrder, LowStockItem } from "@repo/mobile-shared/api/types";

function RecentOrderRow({
  order,
  onPress,
}: {
  order: RecentOrder;
  onPress: () => void;
}) {
  return (
    <TouchableOpacity
      style={styles.orderRow}
      onPress={onPress}
      activeOpacity={0.7}
    >
      <View style={{ flex: 1 }}>
        <Text style={styles.orderNumber}>#{order.order_number}</Text>
        <Text style={styles.orderEmail}>{order.customer_email}</Text>
      </View>
      <Text style={styles.orderTotal}>${order.grand_total.toFixed(2)}</Text>
    </TouchableOpacity>
  );
}

function LowStockRow({
  item,
  onPress,
}: {
  item: LowStockItem;
  onPress: () => void;
}) {
  return (
    <TouchableOpacity
      style={styles.orderRow}
      onPress={onPress}
      activeOpacity={0.7}
    >
      <View style={{ flex: 1 }}>
        <Text style={styles.orderNumber}>{item.name}</Text>
      </View>
      <Text style={[styles.orderTotal, { color: "#8B2020" }]}>
        {item.stock} left
      </Text>
    </TouchableOpacity>
  );
}

export default function DashboardScreen() {
  const { data, isLoading, refetch, isRefetching } = useDashboard();
  const router = useRouter();

  if (isLoading && !data) {
    return (
      <View style={styles.centered}>
        <Text style={styles.loadingText}>Loading dashboard...</Text>
      </View>
    );
  }

  if (!data) {
    return (
      <View style={styles.centered}>
        <Text style={styles.loadingText}>Failed to load dashboard</Text>
        <TouchableOpacity onPress={() => refetch()} style={styles.retryButton}>
          <Text style={styles.retryText}>Retry</Text>
        </TouchableOpacity>
      </View>
    );
  }

  return (
    <ScrollView
      style={styles.container}
      contentContainerStyle={styles.content}
      refreshControl={
        <RefreshControl
          refreshing={isRefetching}
          onRefresh={refetch}
          tintColor="#0E0E0C"
        />
      }
    >
      <DashboardStats stats={data.stats} />

      {data.recent_orders.length > 0 && (
        <View style={styles.section}>
          <View style={styles.sectionHeader}>
            <Text style={styles.sectionTitle}>Recent Orders</Text>
            <TouchableOpacity onPress={() => router.push("/(tabs)/orders")}>
              <Text style={styles.viewAll}>View all</Text>
            </TouchableOpacity>
          </View>
          {data.recent_orders.map((order) => (
            <RecentOrderRow
              key={order.id}
              order={order}
              onPress={() => router.push(`/(tabs)/orders/${order.id}`)}
            />
          ))}
        </View>
      )}

      {data.low_stock.length > 0 && (
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Low Stock</Text>
          {data.low_stock.map((item) => (
            <LowStockRow
              key={item.id}
              item={item}
              onPress={() => router.push(`/(tabs)/products/${item.id}`)}
            />
          ))}
        </View>
      )}

      {data.top_products.length > 0 && (
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Top Products</Text>
          {data.top_products.map((product) => (
            <TouchableOpacity
              key={product.id}
              style={styles.orderRow}
              onPress={() => router.push(`/(tabs)/products/${product.id}`)}
              activeOpacity={0.7}
            >
              <View style={{ flex: 1 }}>
                <Text style={styles.orderNumber}>{product.name}</Text>
                <Text style={styles.orderEmail}>
                  {product.total_sold} sold
                </Text>
              </View>
              <Text style={styles.orderTotal}>
                ${product.revenue.toFixed(0)}
              </Text>
            </TouchableOpacity>
          ))}
        </View>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#F7F6F2" },
  content: { padding: 16, gap: 24, paddingBottom: 32 },
  centered: {
    flex: 1,
    backgroundColor: "#F7F6F2",
    justifyContent: "center",
    alignItems: "center",
  },
  loadingText: { color: "#0E0E0C", opacity: 0.5, fontSize: 16 },
  retryButton: {
    marginTop: 12,
    paddingHorizontal: 20,
    paddingVertical: 10,
    backgroundColor: "#0E0E0C",
    borderRadius: 6,
  },
  retryText: { color: "#F7F6F2", fontSize: 14, fontWeight: "600" },
  section: { gap: 8 },
  sectionHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  sectionTitle: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
    opacity: 0.5,
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  viewAll: { fontSize: 14, color: "#2D4A2B", fontWeight: "500" },
  orderRow: {
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    padding: 12,
    borderWidth: 0.5,
    borderColor: "#0E0E0C10",
  },
  orderNumber: { fontSize: 14, fontWeight: "600", color: "#0E0E0C" },
  orderEmail: {
    fontSize: 12,
    color: "#0E0E0C",
    opacity: 0.5,
    marginTop: 2,
  },
  orderTotal: { fontSize: 16, fontWeight: "700", color: "#0E0E0C" },
});
