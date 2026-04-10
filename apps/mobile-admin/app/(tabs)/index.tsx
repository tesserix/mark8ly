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
import { theme } from "@/lib/theme";
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
      accessibilityRole="button"
      accessibilityLabel={`Order ${order.order_number}, ${order.customer_email}, $${order.grand_total.toFixed(2)}`}
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
      accessibilityRole="button"
      accessibilityLabel={`${item.name}, ${item.stock} left in stock`}
    >
      <View style={{ flex: 1 }}>
        <Text style={styles.orderNumber}>{item.name}</Text>
      </View>
      <Text style={[styles.orderTotal, { color: theme.colors.danger }]}>
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
        <TouchableOpacity
          onPress={() => refetch()}
          style={styles.retryButton}
          accessibilityRole="button"
          accessibilityLabel="Retry loading dashboard"
        >
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
          tintColor={theme.colors.text}
        />
      }
    >
      <DashboardStats stats={data.stats} />

      {data.recent_orders.length > 0 && (
        <View style={styles.section}>
          <View style={styles.sectionHeader}>
            <Text style={styles.sectionTitle}>Recent Orders</Text>
            <TouchableOpacity
              onPress={() => router.push("/(tabs)/orders")}
              accessibilityRole="link"
              accessibilityLabel="View all orders"
            >
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
              accessibilityRole="button"
              accessibilityLabel={`${product.name}, ${product.total_sold} sold, $${product.revenue.toFixed(0)} revenue`}
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
  container: { flex: 1, backgroundColor: theme.colors.background },
  content: { padding: theme.spacing.lg, gap: theme.spacing.xxl, paddingBottom: theme.spacing.xxxl },
  centered: {
    flex: 1,
    backgroundColor: theme.colors.background,
    justifyContent: "center",
    alignItems: "center",
  },
  loadingText: { color: theme.colors.text, opacity: 0.5, fontSize: 16 },
  retryButton: {
    marginTop: theme.spacing.md,
    paddingHorizontal: theme.spacing.xl,
    paddingVertical: 10,
    backgroundColor: theme.colors.text,
    borderRadius: theme.radius,
  },
  retryText: { color: theme.colors.background, fontSize: 14, fontWeight: "600" },
  section: { gap: theme.spacing.sm },
  sectionHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  sectionTitle: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.colors.text,
    opacity: 0.5,
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  viewAll: { fontSize: 14, color: theme.colors.accent, fontWeight: "500" },
  orderRow: {
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    padding: theme.spacing.md,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  orderNumber: { fontSize: 14, fontWeight: "600", color: theme.colors.text },
  orderEmail: {
    fontSize: 12,
    color: theme.colors.text,
    opacity: 0.5,
    marginTop: 2,
  },
  orderTotal: { fontSize: 16, fontWeight: "700", color: theme.colors.text },
});
