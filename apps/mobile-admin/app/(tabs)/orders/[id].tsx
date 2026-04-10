import { useState, useCallback } from "react";
import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  TextInput,
  Modal,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useOrder } from "../../../lib/hooks/use-orders";
import {
  useConfirmOrder,
  useFulfillOrder,
  useCancelOrder,
  useRefundOrder,
} from "../../../lib/admin-api/order-actions";
import type { LineItem, Address } from "@repo/mobile-shared/api/types";

const STATUS_COLORS: Record<string, string> = {
  pending: "#F59E0B",
  confirmed: "#2D4A2B",
  fulfilled: "#0E0E0C",
  cancelled: "#8B2020",
};

function formatCurrency(amount: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
  }).format(amount);
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatAddress(address: Address): string {
  const parts = [address.line1];
  if (address.line2) parts.push(address.line2);
  parts.push(`${address.city}, ${address.state} ${address.postal_code}`);
  parts.push(address.country);
  return parts.join("\n");
}

function StatusBadge({ status }: { status: string }) {
  const bg = STATUS_COLORS[status] ?? "#0E0E0C";
  return (
    <View style={[styles.badge, { backgroundColor: bg }]}>
      <Text style={styles.badgeText}>{status}</Text>
    </View>
  );
}

function SectionHeader({ title }: { title: string }) {
  return <Text style={styles.sectionHeader}>{title}</Text>;
}

function InfoRow({ label, value }: { label: string; value: string | null | undefined }) {
  if (!value) return null;
  return (
    <View style={styles.infoRow}>
      <Text style={styles.infoLabel}>{label}</Text>
      <Text style={styles.infoValue}>{value}</Text>
    </View>
  );
}

function LineItemRow({ item }: { item: LineItem }) {
  return (
    <View style={styles.lineItem}>
      <View style={styles.thumbnail} />
      <View style={styles.lineItemInfo}>
        <Text style={styles.lineItemName} numberOfLines={1}>
          {item.product_name}
        </Text>
        {item.variant_name && (
          <Text style={styles.lineItemVariant}>{item.variant_name}</Text>
        )}
        <Text style={styles.lineItemQty}>
          {item.quantity} x {formatCurrency(item.unit_price)}
        </Text>
      </View>
      <Text style={styles.lineItemTotal}>
        {formatCurrency(item.quantity * item.unit_price)}
      </Text>
    </View>
  );
}

function InputModal({
  visible,
  title,
  placeholder,
  value,
  onChangeText,
  onSubmit,
  onCancel,
  submitLabel,
  keyboardType,
}: {
  visible: boolean;
  title: string;
  placeholder: string;
  value: string;
  onChangeText: (text: string) => void;
  onSubmit: () => void;
  onCancel: () => void;
  submitLabel: string;
  keyboardType?: "default" | "decimal-pad";
}) {
  return (
    <Modal visible={visible} transparent animationType="fade">
      <View style={styles.modalOverlay}>
        <View style={styles.modalContent}>
          <Text style={styles.modalTitle}>{title}</Text>
          <TextInput
            style={styles.modalInput}
            placeholder={placeholder}
            placeholderTextColor="#0E0E0C50"
            value={value}
            onChangeText={onChangeText}
            keyboardType={keyboardType ?? "default"}
            autoFocus
          />
          <View style={styles.modalActions}>
            <TouchableOpacity style={styles.modalCancelBtn} onPress={onCancel}>
              <Text style={styles.modalCancelText}>Cancel</Text>
            </TouchableOpacity>
            <TouchableOpacity style={styles.modalSubmitBtn} onPress={onSubmit}>
              <Text style={styles.modalSubmitText}>{submitLabel}</Text>
            </TouchableOpacity>
          </View>
        </View>
      </View>
    </Modal>
  );
}

export default function OrderDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: order, isLoading } = useOrder(id);

  const confirmMutation = useConfirmOrder();
  const fulfillMutation = useFulfillOrder();
  const cancelMutation = useCancelOrder();
  const refundMutation = useRefundOrder();

  const [fulfillModalVisible, setFulfillModalVisible] = useState(false);
  const [refundModalVisible, setRefundModalVisible] = useState(false);
  const [trackingNumber, setTrackingNumber] = useState("");
  const [refundAmount, setRefundAmount] = useState("");

  const handleConfirm = useCallback(() => {
    Alert.alert("Confirm Order", "Mark this order as confirmed?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Confirm",
        onPress: () => confirmMutation.mutate(id),
      },
    ]);
  }, [id, confirmMutation]);

  const handleFulfill = useCallback(() => {
    if (!trackingNumber.trim()) return;
    fulfillMutation.mutate(
      { id, trackingNumber: trackingNumber.trim() },
      { onSuccess: () => setFulfillModalVisible(false) },
    );
    setTrackingNumber("");
  }, [id, trackingNumber, fulfillMutation]);

  const handleCancel = useCallback(() => {
    Alert.alert(
      "Cancel Order",
      "Are you sure you want to cancel this order? This cannot be undone.",
      [
        { text: "No", style: "cancel" },
        {
          text: "Cancel Order",
          style: "destructive",
          onPress: () => cancelMutation.mutate({ id }),
        },
      ],
    );
  }, [id, cancelMutation]);

  const handleRefund = useCallback(() => {
    const amount = parseFloat(refundAmount);
    if (isNaN(amount) || amount <= 0) return;
    refundMutation.mutate(
      { id, amount },
      { onSuccess: () => setRefundModalVisible(false) },
    );
    setRefundAmount("");
  }, [id, refundAmount, refundMutation]);

  if (isLoading || !order) {
    return (
      <View style={styles.loadingContainer}>
        <ActivityIndicator size="large" color="#0E0E0C" />
      </View>
    );
  }

  const isMutating =
    confirmMutation.isPending ||
    fulfillMutation.isPending ||
    cancelMutation.isPending ||
    refundMutation.isPending;

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      {/* Header */}
      <View style={styles.card}>
        <View style={styles.headerRow}>
          <Text style={styles.orderNumber}>{order.order_number}</Text>
          <StatusBadge status={order.status} />
        </View>
        <Text style={styles.date}>{formatDate(order.created_at)}</Text>
      </View>

      {/* Line items */}
      <View style={styles.card}>
        <SectionHeader title="Items" />
        {order.line_items.map((item) => (
          <LineItemRow key={item.id} item={item} />
        ))}
        <View style={styles.totalRow}>
          <Text style={styles.totalLabel}>Total</Text>
          <Text style={styles.totalValue}>
            {formatCurrency(order.grand_total)}
          </Text>
        </View>
      </View>

      {/* Customer */}
      <View style={styles.card}>
        <SectionHeader title="Customer" />
        <InfoRow label="Name" value={order.customer_name} />
        <InfoRow label="Email" value={order.customer_email} />
      </View>

      {/* Shipping */}
      <View style={styles.card}>
        <SectionHeader title="Shipping" />
        {order.shipping_address ? (
          <InfoRow label="Address" value={formatAddress(order.shipping_address)} />
        ) : (
          <Text style={styles.emptyText}>No shipping address</Text>
        )}
        <InfoRow label="Method" value={order.shipping_method} />
        <InfoRow label="Tracking" value={order.tracking_number} />
      </View>

      {/* Payment */}
      <View style={styles.card}>
        <SectionHeader title="Payment" />
        <InfoRow label="Method" value={order.payment_method} />
        <InfoRow label="Amount" value={formatCurrency(order.grand_total)} />
        <InfoRow label="Transaction" value={order.payment_transaction_id} />
      </View>

      {/* Actions */}
      {order.status !== "cancelled" && (
        <View style={styles.actionsSection}>
          {order.status === "pending" && (
            <TouchableOpacity
              style={styles.primaryBtn}
              onPress={handleConfirm}
              disabled={isMutating}
              accessibilityRole="button"
            >
              <Text style={styles.primaryBtnText}>
                {confirmMutation.isPending ? "Confirming..." : "Confirm Order"}
              </Text>
            </TouchableOpacity>
          )}

          {order.status === "confirmed" && (
            <TouchableOpacity
              style={styles.primaryBtn}
              onPress={() => setFulfillModalVisible(true)}
              disabled={isMutating}
              accessibilityRole="button"
            >
              <Text style={styles.primaryBtnText}>Mark Fulfilled</Text>
            </TouchableOpacity>
          )}

          {order.status === "fulfilled" && (
            <TouchableOpacity
              style={styles.secondaryBtn}
              onPress={() => setRefundModalVisible(true)}
              disabled={isMutating}
              accessibilityRole="button"
            >
              <Text style={styles.secondaryBtnText}>Refund</Text>
            </TouchableOpacity>
          )}

          <TouchableOpacity
            style={styles.dangerBtn}
            onPress={handleCancel}
            disabled={isMutating}
            accessibilityRole="button"
          >
            <Text style={styles.dangerBtnText}>
              {cancelMutation.isPending ? "Cancelling..." : "Cancel Order"}
            </Text>
          </TouchableOpacity>
        </View>
      )}

      {/* Fulfill modal */}
      <InputModal
        visible={fulfillModalVisible}
        title="Tracking Number"
        placeholder="Enter tracking number"
        value={trackingNumber}
        onChangeText={setTrackingNumber}
        onSubmit={handleFulfill}
        onCancel={() => {
          setFulfillModalVisible(false);
          setTrackingNumber("");
        }}
        submitLabel={fulfillMutation.isPending ? "Saving..." : "Mark Fulfilled"}
      />

      {/* Refund modal */}
      <InputModal
        visible={refundModalVisible}
        title="Refund Amount"
        placeholder="0.00"
        value={refundAmount}
        onChangeText={setRefundAmount}
        onSubmit={handleRefund}
        onCancel={() => {
          setRefundModalVisible(false);
          setRefundAmount("");
        }}
        submitLabel={refundMutation.isPending ? "Processing..." : "Refund"}
        keyboardType="decimal-pad"
      />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: "#F7F6F2",
  },
  content: {
    padding: 16,
    paddingBottom: 40,
    gap: 12,
  },
  loadingContainer: {
    flex: 1,
    backgroundColor: "#F7F6F2",
    justifyContent: "center",
    alignItems: "center",
  },
  card: {
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    padding: 14,
    borderWidth: 0.5,
    borderColor: "#0E0E0C10",
  },
  headerRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 4,
  },
  orderNumber: {
    fontSize: 18,
    fontWeight: "700",
    color: "#0E0E0C",
  },
  badge: {
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 4,
  },
  badgeText: {
    fontSize: 11,
    fontWeight: "600",
    color: "#FFFFFF",
    textTransform: "capitalize",
  },
  date: {
    fontSize: 13,
    color: "#0E0E0C",
    opacity: 0.5,
  },
  sectionHeader: {
    fontSize: 12,
    fontWeight: "600",
    color: "#0E0E0C",
    opacity: 0.5,
    textTransform: "uppercase",
    letterSpacing: 0.5,
    marginBottom: 10,
  },
  lineItem: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 8,
    borderBottomWidth: 0.5,
    borderBottomColor: "#0E0E0C10",
  },
  thumbnail: {
    width: 40,
    height: 40,
    borderRadius: 4,
    backgroundColor: "#F7F6F2",
    marginRight: 10,
  },
  lineItemInfo: {
    flex: 1,
  },
  lineItemName: {
    fontSize: 14,
    fontWeight: "500",
    color: "#0E0E0C",
  },
  lineItemVariant: {
    fontSize: 12,
    color: "#0E0E0C",
    opacity: 0.5,
    marginTop: 1,
  },
  lineItemQty: {
    fontSize: 12,
    color: "#0E0E0C",
    opacity: 0.6,
    marginTop: 2,
  },
  lineItemTotal: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
    marginLeft: 8,
  },
  totalRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingTop: 10,
    marginTop: 4,
  },
  totalLabel: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  totalValue: {
    fontSize: 16,
    fontWeight: "700",
    color: "#0E0E0C",
  },
  infoRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    paddingVertical: 6,
  },
  infoLabel: {
    fontSize: 13,
    color: "#0E0E0C",
    opacity: 0.5,
    flex: 1,
  },
  infoValue: {
    fontSize: 13,
    color: "#0E0E0C",
    fontWeight: "500",
    flex: 2,
    textAlign: "right",
  },
  emptyText: {
    fontSize: 13,
    color: "#0E0E0C",
    opacity: 0.4,
    fontStyle: "italic",
  },
  actionsSection: {
    gap: 10,
    marginTop: 4,
  },
  primaryBtn: {
    backgroundColor: "#2D4A2B",
    borderRadius: 6,
    paddingVertical: 14,
    alignItems: "center",
  },
  primaryBtnText: {
    fontSize: 15,
    fontWeight: "600",
    color: "#FFFFFF",
  },
  secondaryBtn: {
    backgroundColor: "#0E0E0C",
    borderRadius: 6,
    paddingVertical: 14,
    alignItems: "center",
  },
  secondaryBtnText: {
    fontSize: 15,
    fontWeight: "600",
    color: "#FFFFFF",
  },
  dangerBtn: {
    backgroundColor: "transparent",
    borderRadius: 6,
    paddingVertical: 14,
    alignItems: "center",
    borderWidth: 1,
    borderColor: "#8B2020",
  },
  dangerBtnText: {
    fontSize: 15,
    fontWeight: "600",
    color: "#8B2020",
  },
  modalOverlay: {
    flex: 1,
    backgroundColor: "rgba(0, 0, 0, 0.4)",
    justifyContent: "center",
    alignItems: "center",
    padding: 24,
  },
  modalContent: {
    backgroundColor: "#FFFFFF",
    borderRadius: 10,
    padding: 20,
    width: "100%",
    maxWidth: 340,
  },
  modalTitle: {
    fontSize: 16,
    fontWeight: "700",
    color: "#0E0E0C",
    marginBottom: 14,
  },
  modalInput: {
    backgroundColor: "#F7F6F2",
    borderRadius: 6,
    paddingHorizontal: 14,
    paddingVertical: 10,
    fontSize: 14,
    color: "#0E0E0C",
    borderWidth: 0.5,
    borderColor: "#0E0E0C10",
    marginBottom: 16,
  },
  modalActions: {
    flexDirection: "row",
    justifyContent: "flex-end",
    gap: 10,
  },
  modalCancelBtn: {
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderRadius: 6,
  },
  modalCancelText: {
    fontSize: 14,
    fontWeight: "500",
    color: "#0E0E0C",
    opacity: 0.6,
  },
  modalSubmitBtn: {
    backgroundColor: "#2D4A2B",
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderRadius: 6,
  },
  modalSubmitText: {
    fontSize: 14,
    fontWeight: "600",
    color: "#FFFFFF",
  },
});
