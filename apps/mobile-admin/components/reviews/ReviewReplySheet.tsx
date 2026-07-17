import { forwardRef, useImperativeHandle, useRef, useState } from "react";
import { View, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import { Text, FieldInput } from "@/components/ui";
import { theme } from "@/lib/theme";

const MAX_LEN = 5000;

export interface ReviewReplySheetHandle {
  present: () => void;
}

interface ReviewReplySheetProps {
  /** Called with the trimmed, non-empty reply when the merchant confirms. */
  onSubmit: (content: string) => void;
  /** A reply request is in flight — disables the control and shows a spinner. */
  isSubmitting?: boolean;
}

/**
 * Public reply composer for a review. Mirrors BlockReasonSheet: a
 * BottomSheetModal with a required, length-capped field and a
 * submit-disabled-until-non-empty primary. 5000 chars matches the backend
 * (ReplyRequest `binding:"max=5000"`).
 */
export const ReviewReplySheet = forwardRef<ReviewReplySheetHandle, ReviewReplySheetProps>(
  function ReviewReplySheet({ onSubmit, isSubmitting = false }, ref) {
    const modalRef = useRef<BottomSheetModal>(null);
    const [content, setContent] = useState("");

    useImperativeHandle(ref, () => ({
      present: () => {
        setContent("");
        modalRef.current?.present();
      },
    }));

    const trimmed = content.trim();
    const canSubmit = trimmed !== "" && trimmed.length <= MAX_LEN && !isSubmitting;

    const handleSubmit = () => {
      if (trimmed === "" || trimmed.length > MAX_LEN) return;
      onSubmit(trimmed);
      modalRef.current?.dismiss();
    };

    return (
      <BottomSheetModal
        ref={modalRef}
        snapPoints={["55%"]}
        enablePanDownToClose
        enableDynamicSizing={false}
        keyboardBehavior="interactive"
        keyboardBlurBehavior="restore"
      >
        <View style={styles.root}>
          <Text preset="h3" color="text">
            Reply to review
          </Text>
          <Text preset="body" color="textSecondary">
            Your reply is public and appears under this review on the storefront.
          </Text>

          <FieldInput
            label="Reply"
            value={content}
            onChangeText={setContent}
            placeholder="Thanks for your feedback…"
            accessibilityLabel="Reply text"
            autoFocus
            multiline
            maxLength={MAX_LEN}
            editable={!isSubmitting}
            style={styles.input}
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
              style={[styles.replyBtn, !canSubmit && styles.replyBtnDisabled]}
              onPress={handleSubmit}
              disabled={!canSubmit}
              accessibilityRole="button"
              accessibilityLabel="Post reply"
            >
              {isSubmitting ? (
                <ActivityIndicator size="small" color={theme.colors.inverse} />
              ) : (
                <Text preset="bodyEmphasis" color="inverse">
                  Post Reply
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
  input: { minHeight: 96 },
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
  replyBtn: {
    flex: 1,
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
  replyBtnDisabled: { opacity: 0.4 },
});
