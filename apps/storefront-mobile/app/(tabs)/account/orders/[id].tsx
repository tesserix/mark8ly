import { useMemo } from "react";
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  ActivityIndicator,
} from "react-native";
import { Image } from "expo-image";
import { useLocalSearchParams } from "expo-router";
import { useTheme } from "@/lib/theme/theme-provider";
import { useOrder } from "@/lib/hooks/use-orders";
import type { OrderLineItem, OrderEvent } from "@/lib/storefront-api/orders";

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
    hour: "numeric",
    minute: "2-digit",
  });
}

function formatShortDate(iso: string): string {
  const date = new Date(iso);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
  });
}

export default function OrderDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const { data: order, isLoading, error } = useOrder(id ?? "");

  const themed = useMemo(
    () => ({
      container: { backgroundColor: theme.background },
      centered: { backgroundColor: theme.background },
      orderNumber: { color: theme.text },
      orderDate: { color: theme.textSecondary },
      divider: { backgroundColor: theme.border },
      sectionTitle: { color: theme.text },
      lineItemTitle: { color: theme.text },
      lineItemVariant: { color: theme.textSecondary },
      lineItemQty: { color: theme.textSecondary },
      lineItemPrice: { color: theme.text },
      lineItemImage: { backgroundColor: theme.border },
      thumbnailPlaceholder: { backgroundColor: theme.border },
      detailLabel: { color: theme.text },
      detailValue: { color: theme.textSecondary },
      trackingValue: { color: theme.accent },
      totalLabel: { color: theme.textSecondary },
      totalValue: { color: theme.text },
      totalBold: { color: theme.text },
      totalDivider: { backgroundColor: theme.border },
      timelineDot: { backgroundColor: theme.primary },
      timelineLine: { backgroundColor: theme.border },
      timelineStatus: { color: theme.text },
      timelineDescription: { color: theme.textSecondary },
      timelineDate: { color: theme.textSecondary },
    }),
    [theme],
  );

  if (isLoading) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <ActivityIndicator size="large" color={theme.primary} />
      </View>
    );
  }

  if (error || !order) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <Text style={styles.errorText}>
          {error instanceof Error ? error.message : "Failed to load order"}
        </Text>
      </View>
    );
  }

  const statusColor = STATUS_COLORS[order.status] ?? "#666666";

  return (
    <ScrollView
      style={[styles.container, themed.container]}
      contentContainerStyle={styles.content}
      showsVerticalScrollIndicator={false}
    >
      <View style={styles.header}>
        <View>
          <Text style={[styles.orderNumber, themed.orderNumber]}>Order #{order.order_number}</Text>
          <Text style={[styles.orderDate, themed.orderDate]}>{formatDate(order.created_at)}</Text>
        </View>
        <View style={[styles.statusBadge, { backgroundColor: `${statusColor}15` }]}>
          <Text style={[styles.statusText, { color: statusColor }]}>
            {order.status.charAt(0).toUpperCase() + order.status.slice(1)}
          </Text>
        </View>
      </View>

      <View style={[styles.divider, themed.divider]} />

      <Text style={[styles.sectionTitle, themed.sectionTitle]}>Items</Text>
      {order.line_items.map((item) => (
        <LineItemRow key={item.id} item={item} currency={order.currency_code} themed={themed} />
      ))}

      <View style={[styles.divider, themed.divider]} />

      <Text style={[styles.sectionTitle, themed.sectionTitle]}>Shipping</Text>
      <View style={styles.detailCard}>
        <Text style={[styles.detailLabel, themed.detailLabel]}>Address</Text>
        <Text style={[styles.detailValue, themed.detailValue]}>
          {order.shipping_address.line1}
          {order.shipping_address.line2 ? `, ${order.shipping_address.line2}` : ""}
        </Text>
        <Text style={[styles.detailValue, themed.detailValue]}>
          {order.shipping_address.city}, {order.shipping_address.state}{" "}
          {order.shipping_address.postal_code}
        </Text>
        <Text style={[styles.detailValue, themed.detailValue]}>{order.shipping_address.country}</Text>

        <View style={styles.detailRow}>
          <Text style={[styles.detailLabel, themed.detailLabel]}>Method</Text>
          <Text style={[styles.detailValue, themed.detailValue]}>{order.shipping_method}</Text>
        </View>

        {order.tracking_number && (
          <View style={styles.detailRow}>
            <Text style={[styles.detailLabel, themed.detailLabel]}>Tracking</Text>
            <Text style={[styles.trackingValue, themed.trackingValue]}>{order.tracking_number}</Text>
          </View>
        )}
      </View>

      <View style={[styles.divider, themed.divider]} />

      <Text style={[styles.sectionTitle, themed.sectionTitle]}>Payment</Text>
      <View style={styles.detailCard}>
        <View style={styles.detailRow}>
          <Text style={[styles.detailLabel, themed.detailLabel]}>Method</Text>
          <Text style={[styles.detailValue, themed.detailValue]}>{order.payment_method}</Text>
        </View>
      </View>

      <View style={[styles.divider, themed.divider]} />

      <Text style={[styles.sectionTitle, themed.sectionTitle]}>Summary</Text>
      <View style={styles.detailCard}>
        <TotalRow label="Subtotal" value={`${order.currency_code} ${order.subtotal}`} themed={themed} />
        <TotalRow label="Shipping" value={`${order.currency_code} ${order.shipping_total}`} themed={themed} />
        {parseFloat(order.discount_total) > 0 && (
          <TotalRow
            label="Discounts"
            value={`-${order.currency_code} ${order.discount_total}`}
            valueColor={theme.accent}
            themed={themed}
          />
        )}
        <TotalRow label="Tax" value={`${order.currency_code} ${order.tax_total}`} themed={themed} />
        <View style={[styles.totalDivider, themed.totalDivider]} />
        <TotalRow
          label="Total"
          value={`${order.currency_code} ${order.total}`}
          bold
          themed={themed}
        />
      </View>

      {order.events.length > 0 && (
        <>
          <View style={[styles.divider, themed.divider]} />
          <Text style={[styles.sectionTitle, themed.sectionTitle]}>Timeline</Text>
          <View style={styles.timeline}>
            {order.events.map((event, index) => (
              <TimelineEvent
                key={event.id}
                event={event}
                isLast={index === order.events.length - 1}
                themed={themed}
              />
            ))}
          </View>
        </>
      )}
    </ScrollView>
  );
}

function LineItemRow({
  item,
  currency,
  themed,
}: {
  item: OrderLineItem;
  currency: string;
  themed: Record<string, { backgroundColor?: string; color?: string }>;
}) {
  return (
    <View style={styles.lineItem}>
      <View style={[styles.lineItemImage, themed.lineItemImage]}>
        {item.thumbnail_url ? (
          <Image
            source={{ uri: item.thumbnail_url }}
            style={styles.thumbnail}
            contentFit="cover"
            accessibilityLabel={item.product_title}
          />
        ) : (
          <View style={[styles.thumbnailPlaceholder, themed.thumbnailPlaceholder]} />
        )}
      </View>
      <View style={styles.lineItemInfo}>
        <Text style={[styles.lineItemTitle, themed.lineItemTitle]} numberOfLines={2}>
          {item.product_title}
        </Text>
        {item.variant_title && (
          <Text style={[styles.lineItemVariant, themed.lineItemVariant]}>{item.variant_title}</Text>
        )}
        <Text style={[styles.lineItemQty, themed.lineItemQty]}>Qty: {item.quantity}</Text>
      </View>
      <Text style={[styles.lineItemPrice, themed.lineItemPrice]}>
        {currency} {item.total_price}
      </Text>
    </View>
  );
}

function TotalRow({
  label,
  value,
  bold = false,
  valueColor,
  themed,
}: {
  label: string;
  value: string;
  bold?: boolean;
  valueColor?: string;
  themed: Record<string, { color?: string }>;
}) {
  return (
    <View style={styles.totalRow}>
      <Text style={[styles.totalLabel, themed.totalLabel, bold && [styles.totalBold, themed.totalBold]]}>{label}</Text>
      <Text
        style={[
          styles.totalValue,
          themed.totalValue,
          bold && [styles.totalBold, themed.totalBold],
          valueColor ? { color: valueColor } : undefined,
        ]}
      >
        {value}
      </Text>
    </View>
  );
}

function TimelineEvent({
  event,
  isLast,
  themed,
}: {
  event: OrderEvent;
  isLast: boolean;
  themed: Record<string, { backgroundColor?: string; color?: string }>;
}) {
  return (
    <View style={styles.timelineEvent}>
      <View style={styles.timelineDotColumn}>
        <View style={[styles.timelineDot, themed.timelineDot]} />
        {!isLast && <View style={[styles.timelineLine, themed.timelineLine]} />}
      </View>
      <View style={styles.timelineContent}>
        <Text style={[styles.timelineStatus, themed.timelineStatus]}>
          {event.status.charAt(0).toUpperCase() + event.status.slice(1)}
        </Text>
        <Text style={[styles.timelineDescription, themed.timelineDescription]}>{event.description}</Text>
        <Text style={[styles.timelineDate, themed.timelineDate]}>{formatShortDate(event.created_at)}</Text>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  content: { padding: 16, paddingBottom: 40 },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  errorText: { fontSize: 15, color: "#8B2020" },
  header: { flexDirection: "row", justifyContent: "space-between", alignItems: "flex-start" },
  orderNumber: { fontSize: 18, fontWeight: "700" },
  orderDate: { fontSize: 13, marginTop: 2 },
  statusBadge: { paddingHorizontal: 10, paddingVertical: 4, borderRadius: 4 },
  statusText: { fontSize: 12, fontWeight: "600" },
  divider: { height: StyleSheet.hairlineWidth, marginVertical: 16 },
  sectionTitle: { fontSize: 15, fontWeight: "700", marginBottom: 12 },
  lineItem: { flexDirection: "row", gap: 12, marginBottom: 12 },
  lineItemImage: { width: 56, height: 56, borderRadius: 6, overflow: "hidden" },
  thumbnail: { width: "100%", height: "100%" },
  thumbnailPlaceholder: { width: "100%", height: "100%" },
  lineItemInfo: { flex: 1, gap: 2 },
  lineItemTitle: { fontSize: 14, fontWeight: "500" },
  lineItemVariant: { fontSize: 12 },
  lineItemQty: { fontSize: 12 },
  lineItemPrice: { fontSize: 14, fontWeight: "600" },
  detailCard: { gap: 8 },
  detailRow: { flexDirection: "row", justifyContent: "space-between", marginTop: 8 },
  detailLabel: { fontSize: 13, fontWeight: "600" },
  detailValue: { fontSize: 13 },
  trackingValue: { fontSize: 13, fontWeight: "500" },
  totalRow: { flexDirection: "row", justifyContent: "space-between", paddingVertical: 2 },
  totalLabel: { fontSize: 14 },
  totalValue: { fontSize: 14 },
  totalBold: { fontWeight: "700" },
  totalDivider: { height: StyleSheet.hairlineWidth, marginVertical: 6 },
  timeline: { gap: 0 },
  timelineEvent: { flexDirection: "row", gap: 12 },
  timelineDotColumn: { alignItems: "center", width: 12 },
  timelineDot: { width: 10, height: 10, borderRadius: 5, marginTop: 2 },
  timelineLine: { width: 1, flex: 1, marginVertical: 4 },
  timelineContent: { flex: 1, paddingBottom: 16 },
  timelineStatus: { fontSize: 14, fontWeight: "600" },
  timelineDescription: { fontSize: 13, marginTop: 2 },
  timelineDate: { fontSize: 12, marginTop: 4 },
});
