import {
  forwardRef,
  useCallback,
  useImperativeHandle,
  useRef,
  useState,
  type ComponentType,
  type ReactNode,
} from "react";
import {
  View,
  Pressable,
  ActivityIndicator,
  StyleSheet,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import {
  BottomSheetBackdrop,
  BottomSheetModal,
  BottomSheetScrollView,
  type BottomSheetBackdropProps,
} from "@gorhom/bottom-sheet";
import { Text, FieldInput } from "@/components/ui";
import { theme } from "@/lib/theme";

export interface EmailLabelSheetHandle {
  present: () => void;
}

interface EmailLabelSheetProps {
  onSubmit: (recipient: string) => void;
  isSubmitting?: boolean;
  /**
   * Fires once per close, whatever caused it — "Cancel", a backdrop tap, a
   * swipe-down, or this sheet's own post-submit dismiss. Sourced solely from
   * `BottomSheetModal`'s own `onDismiss`, so there is one path, not three.
   *
   * Optional and additive. It exists for the same reason
   * `CancelReasonSheet.onDismiss` does: this sheet renders through a portal
   * and is ALWAYS mounted, so the parent's "which shipment is this sheet
   * about?" state otherwise outlives it. Backing out used to be possible only
   * via the explicit "Cancel" button or a swipe; a backdrop makes it a
   * one-tap accident, so the parent needs to hear about it.
   */
  onDismiss?: () => void;
}

// Minimal email shape check — the backend validates `binding:"email"` and is
// the real authority; this only stops an obviously blank/garbage submit.
function looksLikeEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());
}

// @gorhom/bottom-sheet ships its own copy of @types/react, whose `ReactNode`
// includes `bigint`; this project's doesn't, so its components trip TS2786.
// `appearsOnIndex`/`disappearsOnIndex`/`pressBehavior`/`opacity` live on
// gorhom's internal `BottomSheetDefaultBackdropProps`, which isn't exported,
// so they're declared here directly. Identical dodge to `BlockReasonSheet`'s.
const Backdrop = BottomSheetBackdrop as unknown as ComponentType<
  BottomSheetBackdropProps & {
    appearsOnIndex?: number;
    disappearsOnIndex?: number;
    pressBehavior?: "none" | "close" | "collapse" | number;
    opacity?: number;
    style?: StyleProp<ViewStyle>;
  }
>;

// Same TS2786 dodge as `Backdrop`, for the scrolling body — identical to the
// cast `BlockReasonSheet`, `ActionSheet` and `OptionBuilderSheet` use.
const ScrollBody = BottomSheetScrollView as unknown as ComponentType<{
  contentContainerStyle?: StyleProp<ViewStyle>;
  children?: ReactNode;
  testID?: string;
}>;

/**
 * Recipient composer for "Email label". The label PDF is emailed as an
 * attachment to any nominated address (e.g. a 3PL warehouse that prints
 * labels). Mirrors the RefundSheet interaction so the order detail's sheets
 * stay consistent.
 *
 * Unlike the cancel/refund sheets this one DOES dismiss itself on submit —
 * it has no `error` prop to keep it open for, and that behaviour is
 * unchanged here.
 *
 * A backdrop (`pressBehavior="close"`) and a scrolling body are present for
 * the same two reasons the other order sheets have them: gorhom's hosting
 * container is `pointerEvents: "box-none"` with no default backdrop, so a
 * mis-tap over the Orders list navigates away and throws the typed address
 * out; and a fixed `44%` snap point cannot track Dynamic Type, so at
 * `content_size accessibility-large` both buttons were clipped by the sheet's
 * own bounds with no gesture that could reveal them. The scrim is flat, low
 * opacity ink (`theme.colors.overlay`) — never a blur; this design system
 * bans glassmorphism.
 *
 * EVERY dismissal route is gated on `isSubmitting`, not just the "Cancel"
 * button, so the sheet cannot be swiped or backdrop-tapped away while the
 * send is in flight — the same gate the cancel and refund sheets carry. The
 * backdrop drops to `pressBehavior="none"` rather than being unmounted:
 * gorhom only attaches its tap gesture when `pressBehavior !== "none"`, but
 * the scrim keeps `pointerEvents: "auto"` either way, so the tap-through
 * shield stays up.
 *
 * There is no Android hardware-back route to gate: @gorhom/bottom-sheet 5.x
 * registers no `BackHandler` and `BottomSheetModal` renders through a portal,
 * not a react-native `Modal`, so it has no `onRequestClose` either.
 */
export const EmailLabelSheet = forwardRef<EmailLabelSheetHandle, EmailLabelSheetProps>(
  function EmailLabelSheet({ onSubmit, isSubmitting = false, onDismiss }, ref) {
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

    const renderBackdrop = useCallback(
      (props: BottomSheetBackdropProps): ReactNode => (
        <Backdrop
          {...props}
          appearsOnIndex={0}
          disappearsOnIndex={-1}
          pressBehavior={isSubmitting ? "none" : "close"}
          opacity={1}
          style={styles.backdrop}
        />
      ),
      [isSubmitting],
    );

    return (
      <BottomSheetModal
        ref={modalRef}
        snapPoints={["44%"]}
        enablePanDownToClose={!isSubmitting}
        enableDynamicSizing={false}
        keyboardBehavior="interactive"
        keyboardBlurBehavior="restore"
        onDismiss={onDismiss}
        backdropComponent={renderBackdrop}
      >
        {/* A ScrollView, NOT a plain `View` — a fixed-percentage snap point
            paired with non-scrolling content is a silent-clipping trap, and
            `BlockReasonSheet` (which shares this shape) walked straight into
            it: at `content_size accessibility-large` the body copy alone grew
            past the snap point and BOTH buttons were cut off by the sheet's
            own bounds, with no gesture that could reveal them. `44%` cannot
            track Dynamic Type; scrolling inside the bounded sheet can. */}
        <ScrollBody contentContainerStyle={styles.root} testID="email-label-sheet-body">
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
            {/* Gated on `isSubmitting` like the other two order sheets, so
                the button agrees with the swipe and the backdrop rather than
                being the one route left open. Unreachable in practice — this
                sheet dismisses ITSELF the moment `onSubmit` returns, so it is
                never on screen with a send in flight — but leaving the three
                routes disagreeing is how the inconsistency got here. */}
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
        </ScrollBody>
      </BottomSheetModal>
    );
  },
);

const styles = StyleSheet.create({
  // No `flex: 1`: this is a ScrollView CONTENT container now, and a flexed
  // content container pins the content to the viewport height — which is the
  // very thing that clipped the buttons.
  root: { padding: theme.spacing.lg, gap: theme.spacing.md, paddingBottom: theme.spacing.xxl },
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
  backdrop: {
    // Flat, low-opacity ink scrim — never a blur/glassmorphism.
    backgroundColor: theme.colors.overlay,
  },
});
