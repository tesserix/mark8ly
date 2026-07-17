import { forwardRef, useImperativeHandle, useRef, useState } from "react";
import { View, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import { Text, FieldInput } from "@/components/ui";
import { theme } from "@/lib/theme";

export interface CancelReasonSheetHandle {
  present: () => void;
}

interface CancelReasonSheetProps {
  onSubmit: (reason: string) => void;
  isSubmitting?: boolean;
}

/**
 * Cancelling an order requires a reason — CancelOrderRequest.Reason is
 * `binding:"required"`, so the old reason-less mobile cancel was an
 * unconditional 400. Mirror BlockReasonSheet.
 */
export const CancelReasonSheet = forwardRef<CancelReasonSheetHandle, CancelReasonSheetProps>(
  function CancelReasonSheet({ onSubmit, isSubmitting = false }, ref) {
    const modalRef = useRef<BottomSheetModal>(null);
    const [reason, setReason] = useState("");

    useImperativeHandle(ref, () => ({
      present: () => {
        setReason("");
        modalRef.current?.present();
      },
    }));

    const canSubmit = reason.trim() !== "" && !isSubmitting;

    const handleSubmit = () => {
      if (reason.trim() === "") return;
      onSubmit(reason.trim());
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
            Cancel order
          </Text>
          <Text preset="body" color="textSecondary">
            This can&apos;t be undone. Add a reason for your records.
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
          <View style={styles.actions}>
            <Pressable
              style={styles.keepBtn}
              onPress={() => modalRef.current?.dismiss()}
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
