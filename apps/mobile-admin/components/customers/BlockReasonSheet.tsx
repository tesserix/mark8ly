import {
  forwardRef,
  useCallback,
  useImperativeHandle,
  useRef,
  useState,
  type ComponentType,
  type ReactNode,
} from "react";
import { View, Pressable, ActivityIndicator, StyleSheet, type StyleProp, type ViewStyle } from "react-native";
import {
  BottomSheetBackdrop,
  BottomSheetModal,
  BottomSheetScrollView,
  type BottomSheetBackdropProps,
} from "@gorhom/bottom-sheet";
import * as Haptics from "expo-haptics";
import { Text, FieldInput, titleMinimumFontScale } from "@/components/ui";
import { theme } from "@/lib/theme";

export interface BlockReasonSheetHandle {
  present: () => void;
  /** Called by the parent once its mutation resolves successfully — the sheet
   *  no longer dismisses itself optimistically on submit. */
  dismiss: () => void;
}

export interface BlockReasonSheetProps {
  /**
   * WHO is being blocked, stated on its own line.
   *
   * Comes from `lib/customer-identity.ts`, so for a customer with no name it
   * is their email — see the identity line below for why that string cannot
   * be dropped into running copy.
   *
   * Optional because the sheet is always mounted: with no target armed there
   * is nobody to name, and the line is absent rather than blank.
   */
  customerLabel?: string;
  onSubmit: (reason: string) => void;
  /** A block request is in flight — locks the field and the submit. */
  isSubmitting: boolean;
  /**
   * Latest submit error, surfaced inline. LOCAL parent state, never
   * `mutation.error`: react-query never resets a mutation error, so a sheet
   * bound to one greets the merchant with the previous customer's failure.
   */
  error: string | null;
  /**
   * Fires once per close, whatever caused it — "Cancel", a backdrop tap, a
   * swipe-down, or the parent's own `dismiss()` after a successful block.
   * Sourced solely from `BottomSheetModal`'s own `onDismiss`, so there is one
   * path, not three.
   *
   * Exists because the parent's "which customer is this sheet about?" state
   * otherwise outlives the sheet. This sheet renders through a portal and is
   * ALWAYS mounted, so a target left pinned by a backed-out sheet is a live,
   * armed submit for a customer the merchant explicitly walked away from —
   * the same bug Orders shipped with `CancelReasonSheet`.
   */
  onDismiss: () => void;
}

/**
 * Floor for the identity line's shrink-to-fit, as a fraction of `bodyEmphasis`
 * — the same 13pt caption floor `CollapsingHeader` and the customer detail
 * screen use, via the same helper rather than a third hand-written ratio.
 */
const IDENTITY_MIN_SCALE = titleMinimumFontScale(theme.text.bodyEmphasis.fontSize);

// @gorhom/bottom-sheet ships its own copy of @types/react, whose `ReactNode`
// includes `bigint`; this project's doesn't, so its components trip TS2786.
// `appearsOnIndex`/`disappearsOnIndex`/`pressBehavior`/`opacity` live on
// gorhom's internal `BottomSheetDefaultBackdropProps`, which isn't exported,
// so they're declared here directly. Identical dodge to `ActionSheet`'s.
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
// cast `ActionSheet` and `OptionBuilderSheet` use.
const ScrollBody = BottomSheetScrollView as unknown as ComponentType<{
  contentContainerStyle?: StyleProp<ViewStyle>;
  children?: ReactNode;
  testID?: string;
}>;

/**
 * Blocking a customer requires a reason — `BlockCustomerRequest.Reason` is
 * `binding:"required"`, so a reason-less block is an unconditional 400, and a
 * 400 the merchant could have been shown inline is a failure of the client.
 * Submit therefore stays disabled until the reason is non-empty.
 *
 * The sheet does NOT dismiss itself on submit. It stays open with the submit
 * showing a spinner until the parent's mutation settles, then the parent calls
 * `dismiss()` on success or passes `error` back in on failure — so a failed
 * block keeps the typed reason and costs no retyping. Mirrors
 * `components/orders/CancelReasonSheet.tsx`, which this is otherwise modelled
 * on line for line.
 *
 * A backdrop (`pressBehavior="close"`) is present and is the ONE deliberate
 * divergence from that model. gorhom's hosting container is
 * `pointerEvents: "box-none"` and `BottomSheetModal` has no default backdrop,
 * so without one the area above the sheet stays live: over a LIST screen a
 * mis-tap lands on a customer row and navigates away mid-sentence, throwing
 * the typed reason out. It is a flat, low-opacity ink scrim (the same token
 * `ActionSheet` and `StoreSelector` use) — never a blur; this design system
 * bans glassmorphism. The other three reason sheets predate this and now
 * follow it.
 *
 * EVERY dismissal route is gated on `isSubmitting`, not just the "Cancel"
 * button. Telling the merchant "you can't back out right now" by disabling
 * one control while a swipe-down or a backdrop tap still closes the sheet is
 * an inconsistency, and it lands the mutation's `onSuccess`/`onError` on a
 * sheet whose target the parent has already released. The backdrop drops to
 * `pressBehavior="none"` rather than being unmounted: gorhom only attaches
 * its tap gesture when `pressBehavior !== "none"`, but the scrim keeps
 * `pointerEvents: "auto"` either way — so the tap-through shield that is the
 * whole reason for the backdrop stays up while the block is in flight.
 *
 * There is no Android hardware-back route to gate: @gorhom/bottom-sheet 5.x
 * registers no `BackHandler` and `BottomSheetModal` renders through a portal,
 * not a react-native `Modal`, so it has no `onRequestClose` either.
 */
export const BlockReasonSheet = forwardRef<BlockReasonSheetHandle, BlockReasonSheetProps>(
  function BlockReasonSheet({ customerLabel, onSubmit, isSubmitting, error, onDismiss }, ref) {
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

    const handleSubmit = () => {
      // Not merely `!== ""`: Go's `binding:"required"` accepts "   ", which
      // would store three spaces as the record of WHY a customer was blocked.
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
            this sheet walked straight into it: at `content_size
            accessibility-large` the body copy alone grew past 52% of the
            screen and BOTH buttons were cut off by the sheet's own bounds,
            with no gesture that could reveal them. `52%` cannot track Dynamic
            Type; scrolling inside the bounded sheet can, which is exactly the
            reachability guarantee `ActionSheet`'s own doc comment sets out.
            The other three reason sheets carry the same latent trap. */}
        <ScrollBody contentContainerStyle={styles.root} testID="block-sheet-body">
          <Text preset="h3" color="text">
            Block customer
          </Text>
          {/* The identity on its OWN line, clamped to one and allowed to
              shrink — never interpolated into the sentence below. A customer
              with no name is identified by their email, and an email is a
              single indivisible token: inside running copy
              `mahesh.sangawar@gmail.com` breaks as `mahesh.sangawar@gmai` /
              `l.com`, which reads as if the address ended at the line break.
              That exact break is what the customer detail screen's identity
              title had to fix once already. */}
          {customerLabel ? (
            <Text
              preset="bodyEmphasis"
              color="text"
              numberOfLines={1}
              adjustsFontSizeToFit
              minimumFontScale={IDENTITY_MIN_SCALE}
              testID="block-sheet-customer"
            >
              {customerLabel}
            </Text>
          ) : null}
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
              style={[styles.blockBtn, !canSubmit && styles.disabled]}
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
  blockBtn: {
    flex: 1,
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.danger,
    alignItems: "center",
    justifyContent: "center",
  },
  disabled: { opacity: 0.4 },
  backdrop: {
    // Flat, low-opacity ink scrim — never a blur/glassmorphism.
    backgroundColor: theme.colors.overlay,
  },
});
