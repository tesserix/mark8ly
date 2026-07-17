import { forwardRef, useImperativeHandle, useRef, useState } from "react";
import { View, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import { Text, FieldInput } from "@/components/ui";
import { theme } from "@/lib/theme";
import { randomId } from "@/lib/id";

export interface RefundSheetHandle {
  present: () => void;
}

interface RefundSheetProps {
  /** amount omitted ⇒ full remaining balance; refundRequestId is the idempotency scope. */
  onSubmit: (args: { amount?: number; refundRequestId: string }) => void;
  isSubmitting?: boolean;
}

/**
 * Refund composer. The backend requires a stable `refund_request_id`
 * (idempotency scope) per attempt — generated once when the sheet opens and
 * reused across submit retries within this session, so a timed-out resubmit
 * lands on the coordinator's already-done path instead of double-refunding.
 * Amount is optional: blank = full remaining balance.
 */
export const RefundSheet = forwardRef<RefundSheetHandle, RefundSheetProps>(
  function RefundSheet({ onSubmit, isSubmitting = false }, ref) {
    const modalRef = useRef<BottomSheetModal>(null);
    const requestIdRef = useRef<string>("");
    const [amount, setAmount] = useState("");

    useImperativeHandle(ref, () => ({
      present: () => {
        setAmount("");
        requestIdRef.current = randomId();
        modalRef.current?.present();
      },
    }));

    const parsed = amount.trim() === "" ? undefined : Number(amount);
    const amountValid = parsed === undefined || (Number.isFinite(parsed) && parsed > 0);
    const canSubmit = amountValid && !isSubmitting;

    const handleSubmit = () => {
      if (!amountValid) return;
      onSubmit({ amount: parsed, refundRequestId: requestIdRef.current });
      modalRef.current?.dismiss();
    };

    return (
      <BottomSheetModal
        ref={modalRef}
        snapPoints={["48%"]}
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
            Leave the amount blank to refund the full remaining balance.
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
          <View style={styles.actions}>
            <Pressable
              style={styles.cancelBtn}
              onPress={() => modalRef.current?.dismiss()}
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
