import { forwardRef, useImperativeHandle, useRef, useState } from "react";
import { View, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import { Text, FieldInput } from "@/components/ui";
import { theme } from "@/lib/theme";

export interface EmailLabelSheetHandle {
  present: () => void;
}

interface EmailLabelSheetProps {
  onSubmit: (recipient: string) => void;
  isSubmitting?: boolean;
}

// Minimal email shape check — the backend validates `binding:"email"` and is
// the real authority; this only stops an obviously blank/garbage submit.
function looksLikeEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());
}

/**
 * Recipient composer for "Email label". The label PDF is emailed as an
 * attachment to any nominated address (e.g. a 3PL warehouse that prints
 * labels). Mirrors the RefundSheet interaction so the order detail's sheets
 * stay consistent.
 */
export const EmailLabelSheet = forwardRef<EmailLabelSheetHandle, EmailLabelSheetProps>(
  function EmailLabelSheet({ onSubmit, isSubmitting = false }, ref) {
    const modalRef = useRef<BottomSheetModal>(null);
    const [recipient, setRecipient] = useState("");

    useImperativeHandle(ref, () => ({
      present: () => {
        setRecipient("");
        modalRef.current?.present();
      },
    }));

    const canSubmit = looksLikeEmail(recipient) && !isSubmitting;

    const handleSubmit = () => {
      if (!looksLikeEmail(recipient)) return;
      onSubmit(recipient.trim());
      modalRef.current?.dismiss();
    };

    return (
      <BottomSheetModal
        ref={modalRef}
        snapPoints={["44%"]}
        enablePanDownToClose
        enableDynamicSizing={false}
        keyboardBehavior="interactive"
        keyboardBlurBehavior="restore"
      >
        <View style={styles.root}>
          <Text preset="h3" color="text">
            Email shipping label
          </Text>
          <Text preset="body" color="textSecondary">
            Send the label PDF as an attachment — e.g. to the warehouse that
            prints it.
          </Text>
          <FieldInput
            label="Send to"
            value={recipient}
            onChangeText={setRecipient}
            placeholder="warehouse@example.com"
            accessibilityLabel="Recipient email"
            keyboardType="email-address"
            autoCapitalize="none"
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
              style={[styles.sendBtn, !canSubmit && styles.disabled]}
              onPress={handleSubmit}
              disabled={!canSubmit}
              accessibilityRole="button"
              accessibilityLabel="Send label"
            >
              {isSubmitting ? (
                <ActivityIndicator size="small" color={theme.colors.inverse} />
              ) : (
                <Text preset="bodyEmphasis" color="inverse">
                  Send label
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
  sendBtn: {
    flex: 1,
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
  disabled: { opacity: 0.4 },
});
