import {
  ActivityIndicator,
  ScrollView,
  StyleSheet,
  TouchableOpacity,
  View,
} from "react-native";
import { Image } from "expo-image";
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { ChevronLeft } from "lucide-react-native";
import { useOrder } from "@/lib/hooks/use-orders";
import { Card, EmptyState, Hairline, PageHeader, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/format";
import type {
  StorefrontOrderEvent,
  StorefrontOrderLineItem,
} from "@repo/mobile-shared/api/storefront-types";

export default function OrderDetailScreen() {
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: order, isLoading } = useOrder(typeof id === "string" ? id : "");

  if (isLoading) {
    return (
      <Screen>
        <View style={styles.center}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  if (!order) {
    return (
      <Screen>
        <View style={styles.center}>
          <EmptyState title="Order not found" />
        </View>
      </Screen>
    );
  }

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

      <ScrollView contentContainerStyle={styles.scroll}>
        <PageHeader
          eyebrow={`ORDER · ${order.status.toUpperCase()}`}
          title={`#${order.order_number}`}
          subtitle={new Date(order.created_at).toLocaleString(undefined, {
            month: "short",
            day: "numeric",
            year: "numeric",
            hour: "numeric",
            minute: "2-digit",
          })}
        />

        <Card padding={0} style={styles.card}>
          {order.line_items.map((item, i) => (
            <View key={item.product_id + item.variant_id}>
              {i > 0 ? <Hairline inset={88 + theme.spacing.lg * 2} /> : null}
              <LineItemRow item={item} currency={order.currency_code} />
            </View>
          ))}
        </Card>

        <Card padding="md" style={styles.card}>
          <Text preset="eyebrow" color="textTertiary" style={{ marginBottom: theme.spacing.sm }}>
            SHIPPING
          </Text>
          {order.shipping_address ? (
            <View style={{ gap: 2 }}>
              <Text preset="body" color="text">
                {order.shipping_address.name}
              </Text>
              <Text preset="caption" color="textSecondary">
                {order.shipping_address.line1}
                {order.shipping_address.line2 ? `, ${order.shipping_address.line2}` : ""}
              </Text>
              <Text preset="caption" color="textSecondary">
                {order.shipping_address.city}, {order.shipping_address.region}{" "}
                {order.shipping_address.postal_code}
              </Text>
              <Text preset="caption" color="textSecondary">
                {order.shipping_address.country}
              </Text>
            </View>
          ) : (
            <Text preset="caption" color="textTertiary">
              —
            </Text>
          )}
          {order.tracking_number ? (
            <>
              <Hairline style={{ marginVertical: theme.spacing.sm }} />
              <Text preset="caption" color="textTertiary">
                TRACKING
              </Text>
              <Text preset="bodyEmphasis" color="text">
                {order.tracking_number}
              </Text>
            </>
          ) : null}
        </Card>

        <Card padding="md" style={styles.card}>
          <Text preset="eyebrow" color="textTertiary" style={{ marginBottom: theme.spacing.sm }}>
            TOTALS
          </Text>
          <TotalsRow label="Subtotal" value={formatMoney(order.subtotal, order.currency_code)} />
          {Number(order.discount_amount) > 0 ? (
            <TotalsRow
              label="Discount"
              value={`-${formatMoney(order.discount_amount, order.currency_code)}`}
            />
          ) : null}
          <TotalsRow label="Shipping" value={formatMoney(order.shipping_cost, order.currency_code)} />
          <TotalsRow label="Tax" value={formatMoney(order.tax_amount, order.currency_code)} />
          <Hairline style={{ marginVertical: theme.spacing.sm }} />
          <TotalsRow
            label="Total"
            value={formatMoney(order.total_amount, order.currency_code)}
            emphasis
          />
        </Card>

        {order.timeline?.length ? (
          <Card padding="md" style={styles.card}>
            <Text preset="eyebrow" color="textTertiary" style={{ marginBottom: theme.spacing.sm }}>
              TIMELINE
            </Text>
            <View style={{ gap: theme.spacing.md }}>
              {order.timeline.map((event, i) => (
                <TimelineRow key={`${event.type}-${i}`} event={event} />
              ))}
            </View>
          </Card>
        ) : null}
      </ScrollView>
    </Screen>
  );
}

function LineItemRow({
  item,
  currency,
}: {
  item: StorefrontOrderLineItem;
  currency: string;
}) {
  return (
    <View style={styles.lineItem}>
      <View style={styles.thumb}>
        {item.image_url ? (
          <Image source={{ uri: item.image_url }} style={{ width: "100%", height: "100%" }} contentFit="cover" />
        ) : null}
      </View>
      <View style={{ flex: 1, gap: 2 }}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={2}>
          {item.title}
        </Text>
        {item.variant_title ? (
          <Text preset="caption" color="textTertiary">
            {item.variant_title}
          </Text>
        ) : null}
        <Text preset="caption" color="textTertiary">
          Qty {item.quantity}
        </Text>
      </View>
      <Text preset="price" color="text">
        {formatMoney(item.line_total, currency)}
      </Text>
    </View>
  );
}

function TotalsRow({
  label,
  value,
  emphasis,
}: {
  label: string;
  value: string;
  emphasis?: boolean;
}) {
  return (
    <View style={styles.totalsRow}>
      <Text preset={emphasis ? "bodyEmphasis" : "body"} color={emphasis ? "text" : "textSecondary"}>
        {label}
      </Text>
      <Text preset={emphasis ? "price" : "body"} color="text">
        {value}
      </Text>
    </View>
  );
}

function TimelineRow({ event }: { event: StorefrontOrderEvent }) {
  return (
    <View>
      <Text preset="bodyEmphasis" color="text">
        {event.description}
      </Text>
      <Text preset="caption" color="textTertiary">
        {new Date(event.created_at).toLocaleString(undefined, {
          month: "short",
          day: "numeric",
          hour: "numeric",
          minute: "2-digit",
        })}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  center: { flex: 1, alignItems: "center", justifyContent: "center" },
  headerBar: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.sm },
  backBtn: {
    width: 36,
    height: 36,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.radii.pill,
  },
  scroll: { paddingBottom: theme.spacing.huge, gap: theme.spacing.md },
  card: { marginHorizontal: theme.spacing.lg },
  lineItem: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.md,
    padding: theme.spacing.md,
  },
  thumb: {
    width: 56,
    height: 56,
    borderRadius: theme.radii.md,
    overflow: "hidden",
    backgroundColor: theme.colors.surfaceAlt,
  },
  totalsRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: 4,
  },
});
