import {
  forwardRef,
  useCallback,
  useImperativeHandle,
  useRef,
  useState,
  type ComponentType,
  type ReactNode,
} from "react";
import { StyleSheet, type StyleProp, type ViewStyle } from "react-native";
import {
  BottomSheetBackdrop,
  BottomSheetModal,
  BottomSheetScrollView,
  type BottomSheetBackdropProps,
} from "@gorhom/bottom-sheet";
import * as Haptics from "expo-haptics";
import { Text, FieldInput, SheetActions, SheetActionButton } from "@/components/ui";
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
 * Cancelling an order requires a reason — CancelOrderRequest.Reason is
 * `binding:"required"`, so the old reason-less mobile cancel was an
 * unconditional 400. Mirror BlockReasonSheet.
 *
 * The sheet does NOT dismiss itself on submit — it stays open with the
 * submit button showing a spinner until the parent's mutation settles, then
 * calls `dismiss()` on success or passes `error` back in on failure.
 *
 * A backdrop (`pressBehavior="close"`) is present because gorhom's hosting
 * container is `pointerEvents: "box-none"` and `BottomSheetModal` has no
 * default backdrop: without one the area above the sheet stays live, so over
 * the Orders LIST a mis-tap lands on an order row and navigates away
 * mid-sentence, throwing the typed reason out. It is a flat, low-opacity ink
 * scrim (the same token `ActionSheet`, `StoreSelector` and `BlockReasonSheet`
 * use) — never a blur; this design system bans glassmorphism.
 *
 * EVERY dismissal route is gated on `isSubmitting`, not just the "Keep order"
 * button. Telling the merchant "you can't back out right now" by disabling
 * one control while a swipe-down or a backdrop tap still closes the sheet is
 * an inconsistency, and it lands the mutation's `onSuccess`/`onError` on a
 * sheet whose target the parent has already released. The backdrop drops to
 * `pressBehavior="none"` rather than being unmounted: gorhom only attaches
 * its tap gesture when `pressBehavior !== "none"`, but the scrim keeps
 * `pointerEvents: "auto"` either way — so the tap-through shield that is the
 * whole reason for the backdrop stays up while the cancel is in flight.
 *
 * There is no Android hardware-back route to gate: @gorhom/bottom-sheet 5.x
 * registers no `BackHandler` and `BottomSheetModal` renders through a portal,
 * not a react-native `Modal`, so it has no `onRequestClose` either.
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
            `BlockReasonSheet` (modelled on this file) walked straight into
            it: at `content_size accessibility-large` the body copy alone grew
            past 52% of the screen and BOTH buttons were cut off by the
            sheet's own bounds, with no gesture that could reveal them —
            i.e. a merchant at accessibility text sizes could not cancel an
            order at all. `52%` cannot track Dynamic Type; scrolling inside
            the bounded sheet can. */}
        <ScrollBody contentContainerStyle={styles.root} testID="cancel-sheet-body">
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
          <SheetActions>
            <SheetActionButton
              label="Keep order"
              onPress={() => modalRef.current?.dismiss()}
              disabled={isSubmitting}
            />
            <SheetActionButton
              label="Cancel Order"
              accessibilityLabel="Cancel order"
              tone="danger"
              onPress={handleSubmit}
              disabled={!canSubmit}
              busy={isSubmitting}
            />
          </SheetActions>
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
  backdrop: {
    // Flat, low-opacity ink scrim — never a blur/glassmorphism.
    backgroundColor: theme.colors.overlay,
  },
});
