import { forwardRef, useImperativeHandle, useRef, useState } from "react";
import { View, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import * as Haptics from "expo-haptics";
import { Text, FieldInput } from "@/components/ui";
import { theme } from "@/lib/theme";
import { randomId } from "@/lib/id";
import { formatMoney } from "@/lib/money";

export interface RefundSheetHandle {
  present: () => void;
  /** Called by the parent once the mutation resolves successfully — the
   *  sheet no longer dismisses itself optimistically on submit. */
  dismiss: () => void;
}

interface RefundSheetProps {
  /** amount omitted ⇒ full remaining balance; refundRequestId is the idempotency scope. */
  onSubmit: (args: { amount?: number; refundRequestId: string }) => void;
  isSubmitting?: boolean;
  /** Whether this order has a shipment — a full refund also cancels/returns
   *  it at the carrier server-side, so the sheet surfaces that impact. */
  hasShipment?: boolean;
  /** Grand total minus already-refunded — the max refundable balance. Shown
   *  as context near the amount field and used to validate the entered
   *  amount client-side (server is the real source of truth). */
  refundableAmount: number;
  currencyCode?: string;
  /** Latest submit error, surfaced inline. The sheet stays open on failure
   *  so the merchant can correct and retry without re-opening it. */
  error?: string | null;
}

/**
 * Refund composer. The backend requires a stable `refund_request_id`
 * (idempotency scope) per attempt — generated once when the sheet opens and
 * reused across submit retries within this session, so a timed-out resubmit
 * lands on the coordinator's already-done path instead of double-refunding.
 * Amount is optional: blank = full remaining balance.
 *
 * The sheet does NOT dismiss itself on submit — it stays open with the
 * submit button showing a spinner until the parent's mutation settles, then
 * calls `dismiss()` on success or passes `error` back in on failure.
 */
export const RefundSheet = forwardRef<RefundSheetHandle, RefundSheetProps>(
  function RefundSheet(
    { onSubmit, isSubmitting = false, hasShipment = false, refundableAmount, currencyCode, error = null },
    ref,
  ) {
    const modalRef = useRef<BottomSheetModal>(null);
    const requestIdRef = useRef<string>("");
    const [amount, setAmount] = useState("");

    useImperativeHandle(ref, () => ({
      present: () => {
        setAmount("");
        requestIdRef.current = randomId();
        void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Warning);
        modalRef.current?.present();
      },
      dismiss: () => modalRef.current?.dismiss(),
    }));

    const parsed = amount.trim() === "" ? undefined : Number(amount);
    const withinBalance = parsed === undefined || parsed <= refundableAmount;
    const amountValid = parsed === undefined || (Number.isFinite(parsed) && parsed > 0 && withinBalance);
    const canSubmit = amountValid && !isSubmitting;
    // Only a FULL refund (blank amount) auto-cancels/returns the shipment
    // server-side — a partial refund leaves it untouched.
    const isFullRefund = amount.trim() === "";

    const handleSubmit = () => {
      if (!amountValid) return;
      onSubmit({ amount: parsed, refundRequestId: requestIdRef.current });
    };

    return (
      <BottomSheetModal
        ref={modalRef}
        snapPoints={["52%"]}
        enablePanDownToClose
        enableDynamicSizing={false}
        keyboardBehavior="interactive"
        keyboardBlurBehavior="restore"
      >
        <View style={styles.root}>
          <Text preset="h3" color="text">
            Refund order
          </Text>
          <Text preset="body" color="textSecondary">
            This can&apos;t be undone. Leave the amount blank to refund the full remaining balance.
          </Text>
          <Text preset="caption" color="textTertiary">
            Refundable: {formatMoney(refundableAmount, currencyCode)}
          </Text>
          <FieldInput
            label="Amount"
            value={amount}
            onChangeText={setAmount}
            placeholder="Full remaining balance"
            accessibilityLabel="Refund amount"
            keyboardType="decimal-pad"
            autoFocus
            editable={!isSubmitting}
          />
          {parsed !== undefined && !withinBalance ? (
            <Text preset="caption" color="danger">
              Amount exceeds the refundable balance of {formatMoney(refundableAmount, currencyCode)}.
            </Text>
          ) : null}
          {hasShipment && isFullRefund ? (
            <View style={styles.shipmentNote}>
              <Text preset="caption" color={theme.colors.warningInk}>
                A full refund will also cancel or return this shipment with the carrier.
              </Text>
            </View>
          ) : null}
          {error ? (
            <Text preset="caption" color="danger">
              {error}
            </Text>
          ) : null}
          <View style={styles.actions}>
            <Pressable
              style={[styles.cancelBtn, isSubmitting && styles.disabled]}
              onPress={() => modalRef.current?.dismiss()}
              disabled={isSubmitting}
              accessibilityRole="button"
              accessibilityLabel="Cancel"
            >
              <Text preset="bodyEmphasis" color="text">
                Cancel
              </Text>
            </Pressable>
            <Pressable
              style={[styles.refundBtn, !canSubmit && styles.disabled]}
              onPress={handleSubmit}
              disabled={!canSubmit}
              accessibilityRole="button"
              accessibilityLabel="Issue refund"
            >
              {isSubmitting ? (
                <ActivityIndicator size="small" color={theme.colors.inverse} />
              ) : (
                <Text preset="bodyEmphasis" color="inverse">
                  Issue Refund
                </Text>
              )}
            </Pressable>
          </View>
        </View>
      </BottomSheetModal>
    );
  },
);

const styles = StyleSheet.create({
  root: { flex: 1, padding: theme.spacing.lg, gap: theme.spacing.md },
  shipmentNote: {
    backgroundColor: theme.colors.warningTint,
    borderRadius: theme.radii.sm,
    padding: theme.spacing.sm,
  },
  actions: { flexDirection: "row", gap: theme.spacing.md, marginTop: theme.spacing.sm },
  cancelBtn: {
    flex: 1,
    height: 48,
    borderRadius: theme.radii.md,
    borderWidth: 1,
    borderColor: theme.colors.border,
    alignItems: "center",
    justifyContent: "center",
  },
  refundBtn: {
    flex: 1,
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
  disabled: { opacity: 0.4 },
});
