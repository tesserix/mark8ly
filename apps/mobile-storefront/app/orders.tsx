import {
  ActivityIndicator,
  FlatList,
  RefreshControl,
  StyleSheet,
  TouchableOpacity,
  View,
} from "react-native";
import { Stack, useRouter } from "expo-router";
import { ChevronLeft, ChevronRight, Package } from "lucide-react-native";
import { useOrders } from "@/lib/hooks/use-orders";
import { Card, EmptyState, PageHeader, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/format";
import type { StorefrontOrderSummary } from "@repo/mobile-shared/api/storefront-types";

export default function OrdersScreen() {
  const router = useRouter();
  const { data, isLoading, isRefetching, refetch } = useOrders();
  const orders = data?.items ?? [];

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: false }} />
      <View style={styles.headerBar}>
        <TouchableOpacity
          onPress={() => router.back()}
          hitSlop={12}
          accessibilityRole="button"
          accessibilityLabel="Back"
          style={styles.backBtn}
        >
          <ChevronLeft size={22} color={theme.colors.text} strokeWidth={1.75} />
        </TouchableOpacity>
      </View>
      <PageHeader eyebrow="ACCOUNT" title="Orders" />

      <FlatList
        data={orders}
        keyExtractor={(o) => o.id}
        renderItem={({ item }) => (
          <OrderRow order={item} onPress={() => router.push(`/order/${item.id}`)} />
        )}
        ItemSeparatorComponent={() => <View style={{ height: theme.spacing.sm }} />}
        contentContainerStyle={styles.list}
        refreshControl={
          <RefreshControl refreshing={isRefetching} onRefresh={refetch} tintColor={theme.colors.text} />
        }
        ListEmptyComponent={
          isLoading ? (
            <View style={styles.center}>
              <ActivityIndicator size="small" color={theme.colors.text} />
            </View>
          ) : (
            <View style={styles.center}>
              <EmptyState
                icon={<Package size={28} color={theme.colors.textTertiary} strokeWidth={1.5} />}
                title="No orders yet"
                message="When you place an order, it'll show up here."
              />
            </View>
          )
        }
      />
    </Screen>
  );
}

function OrderRow({
  order,
  onPress,
}: {
  order: StorefrontOrderSummary;
  onPress: () => void;
}) {
  return (
    <TouchableOpacity
      onPress={onPress}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`Order ${order.order_number}, ${order.status}, ${formatMoney(order.total_amount, order.currency_code)}`}
    >
      <Card padding="md">
        <View style={styles.row}>
          <View style={{ flex: 1, gap: 2 }}>
            <Text preset="bodyEmphasis" color="text">
              #{order.order_number}
            </Text>
            <Text preset="caption" color="textTertiary">
              {new Date(order.created_at).toLocaleDateString(undefined, {
                month: "short",
                day: "numeric",
                year: "numeric",
              })}{" "}
              · {order.item_count} item{order.item_count === 1 ? "" : "s"}
            </Text>
          </View>
          <View style={{ alignItems: "flex-end", gap: 2 }}>
            <Text preset="price" color="text">
              {formatMoney(order.total_amount, order.currency_code)}
            </Text>
            <Text preset="caption" color="accent">
              {order.status.toUpperCase()}
            </Text>
          </View>
          <ChevronRight
            size={16}
            color={theme.colors.textTertiary}
            strokeWidth={1.75}
            style={{ marginLeft: theme.spacing.sm }}
          />
        </View>
      </Card>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  headerBar: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.sm },
  backBtn: {
    width: 36,
    height: 36,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.radii.pill,
  },
  list: { paddingHorizontal: theme.spacing.lg, paddingBottom: theme.spacing.huge },
  row: { flexDirection: "row", alignItems: "center" },
  center: { flex: 1, paddingTop: theme.spacing.huge },
});
