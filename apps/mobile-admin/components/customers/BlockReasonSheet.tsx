import { forwardRef, useImperativeHandle, useRef, useState } from "react";
import { View, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import { Text, FieldInput } from "@/components/ui";
import { theme } from "@/lib/theme";

export interface BlockReasonSheetHandle {
  present: () => void;
}

interface BlockReasonSheetProps {
  /** Called with the trimmed, non-empty reason when the merchant confirms. */
  onSubmit: (reason: string) => void;
  /** A block request is in flight — disables the control and shows a spinner. */
  isSubmitting?: boolean;
}

/**
 * The web admin requires a typed reason before blocking a customer
 * (CustomerActionsBar). Mobile used to hardcode "Blocked from mobile admin";
 * this sheet lets the merchant record why, matching web. Reason is required —
 * the confirm button stays disabled until it's non-empty.
 */
export const BlockReasonSheet = forwardRef<BlockReasonSheetHandle, BlockReasonSheetProps>(
  function BlockReasonSheet({ onSubmit, isSubmitting = false }, ref) {
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
            Block customer
          </Text>
          <Text preset="body" color="textSecondary">
            They won&apos;t be able to place new orders. Add a reason for your records.
          </Text>

          <FieldInput
            label="Reason"
            value={reason}
            onChangeText={setReason}
            onSubmitEditing={handleSubmit}
            placeholder="e.g. Repeated chargebacks"
            accessibilityLabel="Block reason"
            autoFocus
            returnKeyType="done"
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
              style={[styles.blockBtn, !canSubmit && styles.blockBtnDisabled]}
              onPress={handleSubmit}
              disabled={!canSubmit}
              accessibilityRole="button"
              accessibilityLabel="Block customer"
            >
              {isSubmitting ? (
                <ActivityIndicator size="small" color={theme.colors.inverse} />
              ) : (
                <Text preset="bodyEmphasis" color="inverse">
                  Block Customer
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
  blockBtn: {
    flex: 1,
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.danger,
    alignItems: "center",
    justifyContent: "center",
  },
  blockBtnDisabled: { opacity: 0.4 },
});
