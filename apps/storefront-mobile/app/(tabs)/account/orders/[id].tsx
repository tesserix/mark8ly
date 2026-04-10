import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  ActivityIndicator,
} from "react-native";
import { Image } from "expo-image";
import { useLocalSearchParams } from "expo-router";
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
  const { data: order, isLoading, error } = useOrder(id ?? "");

  if (isLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#0E0E0C" />
      </View>
    );
  }

  if (error || !order) {
    return (
      <View style={styles.centered}>
        <Text style={styles.errorText}>
          {error instanceof Error ? error.message : "Failed to load order"}
        </Text>
      </View>
    );
  }

  const statusColor = STATUS_COLORS[order.status] ?? "#666666";

  return (
    <ScrollView
      style={styles.container}
      contentContainerStyle={styles.content}
      showsVerticalScrollIndicator={false}
    >
      {/* Header */}
      <View style={styles.header}>
        <View>
          <Text style={styles.orderNumber}>Order #{order.order_number}</Text>
          <Text style={styles.orderDate}>{formatDate(order.created_at)}</Text>
        </View>
        <View style={[styles.statusBadge, { backgroundColor: `${statusColor}15` }]}>
          <Text style={[styles.statusText, { color: statusColor }]}>
            {order.status.charAt(0).toUpperCase() + order.status.slice(1)}
          </Text>
        </View>
      </View>

      <View style={styles.divider} />

      {/* Line Items */}
      <Text style={styles.sectionTitle}>Items</Text>
      {order.line_items.map((item) => (
        <LineItemRow key={item.id} item={item} currency={order.currency_code} />
      ))}

      <View style={styles.divider} />

      {/* Shipping */}
      <Text style={styles.sectionTitle}>Shipping</Text>
      <View style={styles.detailCard}>
        <Text style={styles.detailLabel}>Address</Text>
        <Text style={styles.detailValue}>
          {order.shipping_address.line1}
          {order.shipping_address.line2 ? `, ${order.shipping_address.line2}` : ""}
        </Text>
        <Text style={styles.detailValue}>
          {order.shipping_address.city}, {order.shipping_address.state}{" "}
          {order.shipping_address.postal_code}
        </Text>
        <Text style={styles.detailValue}>{order.shipping_address.country}</Text>

        <View style={styles.detailRow}>
          <Text style={styles.detailLabel}>Method</Text>
          <Text style={styles.detailValue}>{order.shipping_method}</Text>
        </View>

        {order.tracking_number && (
          <View style={styles.detailRow}>
            <Text style={styles.detailLabel}>Tracking</Text>
            <Text style={styles.trackingValue}>{order.tracking_number}</Text>
          </View>
        )}
      </View>

      <View style={styles.divider} />

      {/* Payment */}
      <Text style={styles.sectionTitle}>Payment</Text>
      <View style={styles.detailCard}>
        <View style={styles.detailRow}>
          <Text style={styles.detailLabel}>Method</Text>
          <Text style={styles.detailValue}>{order.payment_method}</Text>
        </View>
      </View>

      <View style={styles.divider} />

      {/* Totals */}
      <Text style={styles.sectionTitle}>Summary</Text>
      <View style={styles.detailCard}>
        <TotalRow label="Subtotal" value={`${order.currency_code} ${order.subtotal}`} />
        <TotalRow label="Shipping" value={`${order.currency_code} ${order.shipping_total}`} />
        {parseFloat(order.discount_total) > 0 && (
          <TotalRow
            label="Discounts"
            value={`-${order.currency_code} ${order.discount_total}`}
            valueColor="#2D4A2B"
          />
        )}
        <TotalRow label="Tax" value={`${order.currency_code} ${order.tax_total}`} />
        <View style={styles.totalDivider} />
        <TotalRow
          label="Total"
          value={`${order.currency_code} ${order.total}`}
          bold
        />
      </View>

      {/* Timeline */}
      {order.events.length > 0 && (
        <>
          <View style={styles.divider} />
          <Text style={styles.sectionTitle}>Timeline</Text>
          <View style={styles.timeline}>
            {order.events.map((event, index) => (
              <TimelineEvent
                key={event.id}
                event={event}
                isLast={index === order.events.length - 1}
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
}: {
  item: OrderLineItem;
  currency: string;
}) {
  return (
    <View style={styles.lineItem}>
      <View style={styles.lineItemImage}>
        {item.thumbnail_url ? (
          <Image
            source={{ uri: item.thumbnail_url }}
            style={styles.thumbnail}
            contentFit="cover"
          />
        ) : (
          <View style={styles.thumbnailPlaceholder} />
        )}
      </View>
      <View style={styles.lineItemInfo}>
        <Text style={styles.lineItemTitle} numberOfLines={2}>
          {item.product_title}
        </Text>
        {item.variant_title && (
          <Text style={styles.lineItemVariant}>{item.variant_title}</Text>
        )}
        <Text style={styles.lineItemQty}>Qty: {item.quantity}</Text>
      </View>
      <Text style={styles.lineItemPrice}>
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
}: {
  label: string;
  value: string;
  bold?: boolean;
  valueColor?: string;
}) {
  return (
    <View style={styles.totalRow}>
      <Text style={[styles.totalLabel, bold && styles.totalBold]}>{label}</Text>
      <Text
        style={[
          styles.totalValue,
          bold && styles.totalBold,
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
}: {
  event: OrderEvent;
  isLast: boolean;
}) {
  return (
    <View style={styles.timelineEvent}>
      <View style={styles.timelineDotColumn}>
        <View style={styles.timelineDot} />
        {!isLast && <View style={styles.timelineLine} />}
      </View>
      <View style={styles.timelineContent}>
        <Text style={styles.timelineStatus}>
          {event.status.charAt(0).toUpperCase() + event.status.slice(1)}
        </Text>
        <Text style={styles.timelineDescription}>{event.description}</Text>
        <Text style={styles.timelineDate}>{formatShortDate(event.created_at)}</Text>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#F7F6F2",
  },
  content: {
    padding: 16,
    paddingBottom: 40,
  },
  centered: {
    flex: 1,
    backgroundColor: "#F7F6F2",
    alignItems: "center",
    justifyContent: "center",
  },
  errorText: {
    fontSize: 15,
    color: "#8B2020",
  },
  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "flex-start",
  },
  orderNumber: {
    fontSize: 18,
    fontWeight: "700",
    color: "#0E0E0C",
  },
  orderDate: {
    fontSize: 13,
    color: "#666666",
    marginTop: 2,
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
  divider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: "#E5E4DF",
    marginVertical: 16,
  },
  sectionTitle: {
    fontSize: 15,
    fontWeight: "700",
    color: "#0E0E0C",
    marginBottom: 12,
  },
  lineItem: {
    flexDirection: "row",
    gap: 12,
    marginBottom: 12,
  },
  lineItemImage: {
    width: 56,
    height: 56,
    borderRadius: 6,
    overflow: "hidden",
    backgroundColor: "#E5E4DF",
  },
  thumbnail: {
    width: "100%",
    height: "100%",
  },
  thumbnailPlaceholder: {
    width: "100%",
    height: "100%",
    backgroundColor: "#E5E4DF",
  },
  lineItemInfo: {
    flex: 1,
    gap: 2,
  },
  lineItemTitle: {
    fontSize: 14,
    fontWeight: "500",
    color: "#0E0E0C",
  },
  lineItemVariant: {
    fontSize: 12,
    color: "#666666",
  },
  lineItemQty: {
    fontSize: 12,
    color: "#666666",
  },
  lineItemPrice: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  detailCard: {
    gap: 8,
  },
  detailRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    marginTop: 8,
  },
  detailLabel: {
    fontSize: 13,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  detailValue: {
    fontSize: 13,
    color: "#666666",
  },
  trackingValue: {
    fontSize: 13,
    color: "#2D4A2B",
    fontWeight: "500",
  },
  totalRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    paddingVertical: 2,
  },
  totalLabel: {
    fontSize: 14,
    color: "#666666",
  },
  totalValue: {
    fontSize: 14,
    color: "#0E0E0C",
  },
  totalBold: {
    fontWeight: "700",
    color: "#0E0E0C",
  },
  totalDivider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: "#E5E4DF",
    marginVertical: 6,
  },
  timeline: {
    gap: 0,
  },
  timelineEvent: {
    flexDirection: "row",
    gap: 12,
  },
  timelineDotColumn: {
    alignItems: "center",
    width: 12,
  },
  timelineDot: {
    width: 10,
    height: 10,
    borderRadius: 5,
    backgroundColor: "#0E0E0C",
    marginTop: 2,
  },
  timelineLine: {
    width: 1,
    flex: 1,
    backgroundColor: "#E5E4DF",
    marginVertical: 4,
  },
  timelineContent: {
    flex: 1,
    paddingBottom: 16,
  },
  timelineStatus: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  timelineDescription: {
    fontSize: 13,
    color: "#666666",
    marginTop: 2,
  },
  timelineDate: {
    fontSize: 12,
    color: "#999999",
    marginTop: 4,
  },
});
