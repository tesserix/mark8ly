import { forwardRef, useImperativeHandle, useRef, useState } from "react";
import { View, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import * as Haptics from "expo-haptics";
import { Text, FieldInput } from "@/components/ui";
import { theme } from "@/lib/theme";

export interface CancelReasonSheetHandle {
  present: () => void;
  /** Called by the parent once the mutation resolves successfully — the
   *  sheet no longer dismisses itself optimistically on submit. */
  dismiss: () => void;
}

interface CancelReasonSheetProps {
  onSubmit: (reason: string) => void;
  isSubmitting?: boolean;
  /** Whether this order has a shipment — cancelling the order also cancels
   *  it with the carrier server-side, so the sheet surfaces that impact. */
  hasShipment?: boolean;
  /** Carrier label for the shipment-impact line (e.g. "Delhivery"). Falls
   *  back to a generic "carrier" when absent. */
  carrier?: string;
  /** Latest submit error, surfaced inline. The sheet stays open on failure
   *  so the merchant can retry without re-opening it. */
  error?: string | null;
  /**
   * Fires once per close, whatever caused it — "Keep order", a backdrop tap,
   * a swipe-down, or the parent's own `dismiss()` after a successful submit.
   * Sourced solely from `BottomSheetModal`'s own `onDismiss`, so there is one
   * path, not three.
   *
   * Exists because the parent's "which order is this sheet about?" state
   * otherwise outlives the sheet: backing out without submitting used to
   * leave that order pinned for the life of the screen, and on Orders that
   * state also drives the lazy shipment probe — so a dismissed cancel on a
   * SHIPPED order went on feeding its carrier warning into the next order's
   * refund sheet.
   */
  onDismiss?: () => void;
}

function titleize(value?: string): string | undefined {
  if (!value) return undefined;
  return value.charAt(0).toUpperCase() + value.slice(1);
}

/**
 * Cancelling an order requires a reason — CancelOrderRequest.Reason is
 * `binding:"required"`, so the old reason-less mobile cancel was an
 * unconditional 400. Mirror BlockReasonSheet.
 *
 * The sheet does NOT dismiss itself on submit — it stays open with the
 * submit button showing a spinner until the parent's mutation settles, then
 * calls `dismiss()` on success or passes `error` back in on failure.
 */
export const CancelReasonSheet = forwardRef<CancelReasonSheetHandle, CancelReasonSheetProps>(
  function CancelReasonSheet(
    { onSubmit, isSubmitting = false, hasShipment = false, carrier, error = null, onDismiss },
    ref,
  ) {
    const modalRef = useRef<BottomSheetModal>(null);
    const [reason, setReason] = useState("");

    useImperativeHandle(ref, () => ({
      present: () => {
        setReason("");
        void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Warning);
        modalRef.current?.present();
      },
      dismiss: () => modalRef.current?.dismiss(),
    }));

    const canSubmit = reason.trim() !== "" && !isSubmitting;
    const shipmentNote = hasShipment
      ? ` The ${titleize(carrier) ?? "carrier"} shipment will also be cancelled.`
      : "";

    const handleSubmit = () => {
      if (reason.trim() === "") return;
      onSubmit(reason.trim());
    };

    return (
      <BottomSheetModal
        ref={modalRef}
        snapPoints={["52%"]}
        enablePanDownToClose
        enableDynamicSizing={false}
        keyboardBehavior="interactive"
        keyboardBlurBehavior="restore"
        onDismiss={onDismiss}
      >
        <View style={styles.root}>
          <Text preset="h3" color="text">
            Cancel order
          </Text>
          <Text preset="body" color="textSecondary">
            This can&apos;t be undone. Add a reason for your records.{shipmentNote}
          </Text>
          <FieldInput
            label="Reason"
            value={reason}
            onChangeText={setReason}
            onSubmitEditing={handleSubmit}
            placeholder="e.g. Customer requested cancellation"
            accessibilityLabel="Cancellation reason"
            autoFocus
            returnKeyType="done"
            editable={!isSubmitting}
          />
          {error ? (
            <Text preset="caption" color="danger">
              {error}
            </Text>
          ) : null}
          <View style={styles.actions}>
            <Pressable
              style={[styles.keepBtn, isSubmitting && styles.disabled]}
              onPress={() => modalRef.current?.dismiss()}
              disabled={isSubmitting}
              accessibilityRole="button"
              accessibilityLabel="Keep order"
            >
              <Text preset="bodyEmphasis" color="text">
                Keep order
              </Text>
            </Pressable>
            <Pressable
              style={[styles.cancelBtn, !canSubmit && styles.disabled]}
              onPress={handleSubmit}
              disabled={!canSubmit}
              accessibilityRole="button"
              accessibilityLabel="Cancel order"
            >
              {isSubmitting ? (
                <ActivityIndicator size="small" color={theme.colors.inverse} />
              ) : (
                <Text preset="bodyEmphasis" color="inverse">
                  Cancel Order
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
  actions: { flexDirection: "row", gap: theme.spacing.md, marginTop: theme.spacing.sm },
  keepBtn: {
    flex: 1,
    height: 48,
    borderRadius: theme.radii.md,
    borderWidth: 1,
    borderColor: theme.colors.border,
    alignItems: "center",
    justifyContent: "center",
  },
  cancelBtn: {
    flex: 1,
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.danger,
    alignItems: "center",
    justifyContent: "center",
  },
  disabled: { opacity: 0.4 },
});
