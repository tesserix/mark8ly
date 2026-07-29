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
  /**
   * Fires once per close, whatever caused it — "Cancel", a backdrop tap, a
   * swipe-down, or the parent's own `dismiss()` after a successful refund.
   * Sourced solely from `BottomSheetModal`'s own `onDismiss`.
   *
   * See `CancelReasonSheet.onDismiss` for why a sheet that cannot report its
   * own dismissal leaves the parent's target state stale.
   */
  onDismiss?: () => void;
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
 * Refund composer. The backend requires a stable `refund_request_id`
 * (idempotency scope) per attempt — generated once when the sheet opens and
 * reused across submit retries within this session, so a timed-out resubmit
 * lands on the coordinator's already-done path instead of double-refunding.
 * Amount is optional: blank = full remaining balance.
 *
 * The sheet does NOT dismiss itself on submit — it stays open with the
 * submit button showing a spinner until the parent's mutation settles, then
 * calls `dismiss()` on success or passes `error` back in on failure.
 *
 * A backdrop (`pressBehavior="close"`) is present because gorhom's hosting
 * container is `pointerEvents: "box-none"` and `BottomSheetModal` has no
 * default backdrop: without one the area above the sheet stays live, so over
 * the Orders LIST a mis-tap lands on an order row and navigates away
 * mid-composition, throwing the typed amount out. It is a flat, low-opacity
 * ink scrim (the same token `ActionSheet`, `StoreSelector` and
 * `BlockReasonSheet` use) — never a blur; this design system bans
 * glassmorphism.
 *
 * EVERY dismissal route is gated on `isSubmitting`, not just the "Cancel"
 * button — and on a sheet that moves real money the inconsistency was worst
 * here. `requestIdRef` is regenerated on `present()` and deliberately NOT on
 * dismissal, so a sheet that closed itself mid-flight and was reopened would
 * have started a NEW idempotency scope for what the merchant experiences as
 * the same refund. Gating the swipe and the backdrop keeps the in-flight
 * refund's scope on screen until it settles. The backdrop drops to
 * `pressBehavior="none"` rather than being unmounted: gorhom only attaches
 * its tap gesture when `pressBehavior !== "none"`, but the scrim keeps
 * `pointerEvents: "auto"` either way, so the tap-through shield stays up.
 *
 * There is no Android hardware-back route to gate: @gorhom/bottom-sheet 5.x
 * registers no `BackHandler` and `BottomSheetModal` renders through a portal,
 * not a react-native `Modal`, so it has no `onRequestClose` either.
 */
export const RefundSheet = forwardRef<RefundSheetHandle, RefundSheetProps>(
  function RefundSheet(
    {
      onSubmit,
      isSubmitting = false,
      hasShipment = false,
      refundableAmount,
      currencyCode,
      error = null,
      onDismiss,
    },
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
        snapPoints={["52%"]}
        enablePanDownToClose={!isSubmitting}
        enableDynamicSizing={false}
        keyboardBehavior="interactive"
        keyboardBlurBehavior="restore"
        onDismiss={onDismiss}
        backdropComponent={renderBackdrop}
      >
        {/* A ScrollView, NOT a plain `View` — a fixed-percentage snap point
            paired with non-scrolling content is a silent-clipping trap, and
            `BlockReasonSheet` (which copied this file's `52%`) walked
            straight into it: at `content_size accessibility-large` the body
            copy alone grew past 52% of the screen and BOTH buttons were cut
            off by the sheet's own bounds, with no gesture that could reveal
            them — i.e. a merchant at accessibility text sizes could not issue
            a refund at all. This sheet is the WORST case of the three: it
            carries an extra "Refundable:" line, a conditional
            over-balance error and a conditional carrier warning, so at AX
            sizes it overflows sooner than the others. `52%` cannot track
            Dynamic Type; scrolling inside the bounded sheet can. */}
        <ScrollBody contentContainerStyle={styles.root} testID="refund-sheet-body">
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
              // "Cancel" is also the label of the row-level swipe action, so
              // the dismiss control carries an id a test can name directly
              // rather than being picked out of a label collision.
              testID="refund-sheet-dismiss"
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
  backdrop: {
    // Flat, low-opacity ink scrim — never a blur/glassmorphism.
    backgroundColor: theme.colors.overlay,
  },
});
