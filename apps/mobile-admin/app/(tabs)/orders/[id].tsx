import { useState, useCallback } from "react";
import {
  View,
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
import {
  BackHeader,
  Card,
  Eyebrow,
  Hairline,
  Screen,
  StatusBadge,
  Text,
  type StatusTone,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import type { LineItem, Address } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

const STATUS_TONE: Record<string, StatusTone> = {
  pending: "warning",
  confirmed: "success",
  fulfilled: "neutral",
  cancelled: "danger",
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

function InfoRow({ label, value }: { label: string; value: string | null | undefined }) {
  if (!value) return null;
  return (
    <View style={styles.infoRow}>
      <Text preset="caption" color="textTertiary" style={styles.infoLabel}>
        {label}
      </Text>
      <Text preset="body" color="text" style={styles.infoValue}>
        {value}
      </Text>
    </View>
  );
}

function LineItemRow({ item }: { item: LineItem }) {
  const total = item.quantity * item.unit_price;
  return (
    <View style={styles.lineItem}>
      <View style={styles.lineThumb} />
      <View style={styles.lineInfo}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
          {item.product_name}
        </Text>
        {item.variant_name ? (
          <Text preset="caption" color="textTertiary">
            {item.variant_name}
          </Text>
        ) : null}
        <Text preset="caption" color="textSecondary">
          {item.quantity} × {formatCurrency(item.unit_price)}
        </Text>
      </View>
      <Text preset="bodyEmphasis" color="text">
        {formatCurrency(total)}
      </Text>
    </View>
  );
}

function InputModal({
  visible, title, placeholder, value, onChangeText, onSubmit, onCancel, submitLabel, keyboardType,
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
          <Text preset="h3" color="text" style={styles.modalTitle}>
            {title}
          </Text>
          <TextInput
            style={styles.modalInput}
            placeholder={placeholder}
            placeholderTextColor={theme.colors.textTertiary}
            value={value}
            onChangeText={onChangeText}
            keyboardType={keyboardType ?? "default"}
            autoFocus
            accessibilityLabel={title}
          />
          <View style={styles.modalActions}>
            <TouchableOpacity
              style={styles.modalCancelBtn}
              onPress={onCancel}
              accessibilityRole="button"
              accessibilityLabel="Cancel"
            >
              <Text preset="bodyEmphasis" color="textSecondary">
                Cancel
              </Text>
            </TouchableOpacity>
            <TouchableOpacity
              style={styles.modalSubmitBtn}
              onPress={onSubmit}
              accessibilityRole="button"
              accessibilityLabel={submitLabel}
            >
              <Text preset="bodyEmphasis" color="inverse">
                {submitLabel}
              </Text>
            </TouchableOpacity>
          </View>
        </View>
      </View>
    </Modal>
  );
}

export default function OrderDetailScreen() {
  const dockPad = useDockClearance();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: order, isLoading } = useOrder(id);

  const confirmMutation = useConfirmOrder();
  const fulfillMutation = useFulfillOrder();
  const cancelMutation = useCancelOrder();
  const refundMutation = useRefundOrder();

  const [fulfillVisible, setFulfillVisible] = useState(false);
  const [refundVisible, setRefundVisible] = useState(false);
  const [trackingNumber, setTrackingNumber] = useState("");
  const [refundAmount, setRefundAmount] = useState("");

  const handleConfirm = useCallback(() => {
    Alert.alert("Confirm Order", "Mark this order as confirmed?", [
      { text: "Cancel", style: "cancel" },
      { text: "Confirm", onPress: () => confirmMutation.mutate(id) },
    ]);
  }, [id, confirmMutation]);

  const handleFulfill = useCallback(() => {
    if (!trackingNumber.trim()) return;
    fulfillMutation.mutate(
      { id, trackingNumber: trackingNumber.trim() },
      { onSuccess: () => setFulfillVisible(false) },
    );
    setTrackingNumber("");
  }, [id, trackingNumber, fulfillMutation]);

  const handleCancel = useCallback(() => {
    Alert.alert(
      "Cancel Order",
      "Are you sure? This cannot be undone.",
      [
        { text: "No", style: "cancel" },
        { text: "Cancel Order", style: "destructive", onPress: () => cancelMutation.mutate({ id }) },
      ],
    );
  }, [id, cancelMutation]);

  const handleRefund = useCallback(() => {
    const amount = parseFloat(refundAmount);
    if (isNaN(amount) || amount <= 0) return;
    refundMutation.mutate(
      { id, amount },
      { onSuccess: () => setRefundVisible(false) },
    );
    setRefundAmount("");
  }, [id, refundAmount, refundMutation]);

  if (isLoading || !order) {
    return (
      <Screen>
        <BackHeader eyebrow="ORDER" title="Loading…" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  const tone = STATUS_TONE[order.status] ?? "neutral";
  const isMutating =
    confirmMutation.isPending ||
    fulfillMutation.isPending ||
    cancelMutation.isPending ||
    refundMutation.isPending;

  return (
    <Screen>
      <BackHeader eyebrow="ORDER" title={`#${order.order_number}`} />
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
        <View style={styles.heading}>
          <Text preset="h1" color="text">
            #{order.order_number}
          </Text>
          <View style={styles.headingMeta}>
            <StatusBadge label={order.status} tone={tone} />
            <Text preset="caption" color="textTertiary">
              {formatDate(order.created_at)}
            </Text>
          </View>
        </View>

        <Eyebrow label="Items" />
        <Card padding={0} style={styles.card}>
          {order.line_items.map((item, i) => (
            <View key={item.id}>
              {i > 0 ? <Hairline inset={theme.spacing.lg} /> : null}
              <LineItemRow item={item} />
            </View>
          ))}
          <Hairline />
          <View style={styles.totalRow}>
            <Text preset="body" color="textSecondary">
              Total
            </Text>
            <Text preset="h3" color="text">
              {formatCurrency(order.grand_total)}
            </Text>
          </View>
        </Card>

        <Eyebrow label="Customer" />
        <Card style={styles.card}>
          <InfoRow label="Name" value={order.customer_name} />
          <InfoRow label="Email" value={order.customer_email} />
        </Card>

        <Eyebrow label="Shipping" />
        <Card style={styles.card}>
          {order.shipping_address ? (
            <InfoRow label="Address" value={formatAddress(order.shipping_address)} />
          ) : (
            <Text preset="caption" color="textTertiary">
              No shipping address.
            </Text>
          )}
          <InfoRow label="Method" value={order.shipping_method} />
          <InfoRow label="Tracking" value={order.tracking_number} />
        </Card>

        <Eyebrow label="Payment" />
        <Card style={styles.card}>
          <InfoRow label="Method" value={order.payment_method} />
          <InfoRow label="Amount" value={formatCurrency(order.grand_total)} />
          <InfoRow label="Transaction" value={order.payment_transaction_id} />
        </Card>

        {order.status !== "cancelled" ? (
          <View style={styles.actions}>
            {order.status === "pending" ? (
              <PrimaryButton
                label={confirmMutation.isPending ? "Confirming…" : "Confirm Order"}
                onPress={handleConfirm}
                disabled={isMutating}
              />
            ) : null}
            {order.status === "confirmed" ? (
              <PrimaryButton
                label="Mark Fulfilled"
                onPress={() => setFulfillVisible(true)}
                disabled={isMutating}
              />
            ) : null}
            {order.status === "fulfilled" ? (
              <SecondaryButton
                label="Refund"
                onPress={() => setRefundVisible(true)}
                disabled={isMutating}
              />
            ) : null}
            <DangerButton
              label={cancelMutation.isPending ? "Cancelling…" : "Cancel Order"}
              onPress={handleCancel}
              disabled={isMutating}
            />
          </View>
        ) : null}
      </ScrollView>

      <InputModal
        visible={fulfillVisible}
        title="Tracking Number"
        placeholder="Enter tracking number"
        value={trackingNumber}
        onChangeText={setTrackingNumber}
        onSubmit={handleFulfill}
        onCancel={() => {
          setFulfillVisible(false);
          setTrackingNumber("");
        }}
        submitLabel={fulfillMutation.isPending ? "Saving…" : "Mark Fulfilled"}
      />

      <InputModal
        visible={refundVisible}
        title="Refund Amount"
        placeholder="0.00"
        value={refundAmount}
        onChangeText={setRefundAmount}
        onSubmit={handleRefund}
        onCancel={() => {
          setRefundVisible(false);
          setRefundAmount("");
        }}
        submitLabel={refundMutation.isPending ? "Processing…" : "Refund"}
        keyboardType="decimal-pad"
      />
    </Screen>
  );
}

function PrimaryButton({ label, onPress, disabled }: { label: string; onPress: () => void; disabled?: boolean }) {
  return (
    <TouchableOpacity
      style={[styles.btnPrimary, disabled && styles.btnDisabled]}
      onPress={onPress}
      disabled={disabled}
      activeOpacity={0.85}
      accessibilityRole="button"
      accessibilityLabel={label}
    >
      <Text preset="bodyEmphasis" color="inverse">{label}</Text>
    </TouchableOpacity>
  );
}

function SecondaryButton({ label, onPress, disabled }: { label: string; onPress: () => void; disabled?: boolean }) {
  return (
    <TouchableOpacity
      style={[styles.btnSecondary, disabled && styles.btnDisabled]}
      onPress={onPress}
      disabled={disabled}
      activeOpacity={0.85}
      accessibilityRole="button"
      accessibilityLabel={label}
    >
      <Text preset="bodyEmphasis" color="text">{label}</Text>
    </TouchableOpacity>
  );
}

function DangerButton({ label, onPress, disabled }: { label: string; onPress: () => void; disabled?: boolean }) {
  return (
    <TouchableOpacity
      style={[styles.btnDanger, disabled && styles.btnDisabled]}
      onPress={onPress}
      disabled={disabled}
      activeOpacity={0.85}
      accessibilityRole="button"
      accessibilityLabel={label}
    >
      <Text preset="bodyEmphasis" color="danger">{label}</Text>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  scroll: {
    paddingBottom: theme.spacing.huge,
  },
  heading: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.lg,
    paddingBottom: theme.spacing.sm,
    gap: theme.spacing.sm,
  },
  headingMeta: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.md,
  },
  card: {
    marginHorizontal: theme.spacing.lg,
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
  lineItem: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.md,
    gap: theme.spacing.md,
  },
  lineThumb: {
    width: 40,
    height: 40,
    borderRadius: theme.radii.sm,
    backgroundColor: theme.colors.background,
  },
  lineInfo: { flex: 1, gap: 2 },
  totalRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.md,
  },
  infoRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "flex-start",
    paddingVertical: theme.spacing.sm,
    gap: theme.spacing.md,
  },
  infoLabel: { flex: 1 },
  infoValue: { flex: 2, textAlign: "right" },
  actions: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.xl,
    gap: theme.spacing.sm,
  },
  btnPrimary: {
    backgroundColor: theme.colors.accent,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnSecondary: {
    backgroundColor: theme.colors.elevated,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnDanger: {
    backgroundColor: "transparent",
    borderWidth: 1,
    borderColor: theme.colors.danger,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnDisabled: { opacity: 0.5 },
  modalOverlay: {
    flex: 1,
    backgroundColor: theme.colors.overlay,
    justifyContent: "center",
    alignItems: "center",
    padding: theme.spacing.xl,
  },
  modalContent: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radii.lg,
    padding: theme.spacing.xl,
    width: "100%",
    maxWidth: 360,
    gap: theme.spacing.md,
  },
  modalTitle: { marginBottom: theme.spacing.xs },
  modalInput: {
    backgroundColor: theme.colors.surfaceAlt,
    borderRadius: theme.radii.md,
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.sm,
    fontFamily: theme.fonts.sans,
    fontSize: 15,
    color: theme.colors.text,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    minHeight: 44,
  },
  modalActions: {
    flexDirection: "row",
    justifyContent: "flex-end",
    gap: theme.spacing.sm,
    marginTop: theme.spacing.sm,
  },
  modalCancelBtn: {
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.sm,
    minHeight: 44,
    justifyContent: "center",
  },
  modalSubmitBtn: {
    backgroundColor: theme.colors.accent,
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.sm,
    borderRadius: theme.radii.md,
    minHeight: 44,
    justifyContent: "center",
  },
});
