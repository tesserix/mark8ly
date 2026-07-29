import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ComponentType,
  type ReactNode,
} from "react";
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  View,
  type NativeSyntheticEvent,
  type StyleProp,
  type TextInputSelectionChangeEventData,
  type ViewStyle,
} from "react-native";
import {
  BottomSheetBackdrop,
  BottomSheetModal,
  BottomSheetScrollView,
  type BottomSheetBackdropProps,
} from "@gorhom/bottom-sheet";
import { FieldInput, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import type { Product, ProductVariant } from "@repo/mobile-shared/api/types";
import type { VariantEditField } from "@/lib/admin-api/variant-quick-edit";
import { FIELD_INPUT_LABEL, FIELD_TITLE } from "./variant-edit-copy";
import { variantLabel } from "./variant-identity";

export interface VariantEditTarget {
  product: Product;
  variant: ProductVariant;
  field: VariantEditField;
}

export interface VariantValueResult {
  /** The number to send, or null when the input cannot be sent as it stands. */
  value: number | null;
  /** Merchant-readable reason, or null when there is nothing to say yet. */
  error: string | null;
}

/**
 * Empty is not an error — it is a merchant mid-overtype who has cleared the
 * field. It disables submit and says nothing, rather than turning the sheet
 * red the instant they select-all and start typing.
 */
const EMPTY: VariantValueResult = { value: null, error: null };

/**
 * A decimal-pad in a comma-decimal locale emits "24,50" and means 24.5.
 * Applied only when there is exactly one comma and no dot, so a
 * thousands-separated "1,200.00" is still rejected outright — the wire field
 * is a shopspring decimal, not a display string, and a formatted amount must
 * never reach it.
 */
function normalizeDecimal(raw: string): string {
  const trimmed = raw.trim();
  const commas = trimmed.match(/,/g)?.length ?? 0;
  if (commas === 1 && !trimmed.includes(".")) return trimmed.replace(",", ".");
  return trimmed;
}

/**
 * What the merchant typed, as something sendable — or the reason it is not.
 *
 * The server validates all of this too. That is not a reason to skip it: a
 * round trip to be told what the client already knew costs a merchant a
 * second of their thumb and tells them nothing the field could not have said
 * immediately. Exported and unit-tested because it is the half of this
 * component that cannot be seen in a screenshot.
 */
export function validateVariantValue(field: VariantEditField, raw: string): VariantValueResult {
  const text = field === "price" ? normalizeDecimal(raw) : raw.trim();
  if (text.length === 0) return EMPTY;

  // NOT parseFloat: it happily reads "49abc" as 49 and "$49" as NaN only by
  // accident of position. Number() rejects the whole string or nothing.
  const parsed = Number(text);
  if (!Number.isFinite(parsed)) {
    return {
      value: null,
      error:
        field === "price"
          ? "Enter a price as a plain number, like 24.50."
          : "Enter a quantity as a whole number.",
    };
  }
  if (parsed < 0) {
    return {
      value: null,
      error: field === "price" ? "A price can't be negative." : "Stock can't be negative.",
    };
  }
  if (field === "inventory_quantity" && !Number.isInteger(parsed)) {
    return { value: null, error: "Stock has to be a whole number." };
  }
  return { value: parsed, error: null };
}

// @gorhom/bottom-sheet ships its own copy of @types/react, whose `ReactNode`
// includes `bigint`; this project's doesn't, so its components trip TS2786.
// `appearsOnIndex`/`disappearsOnIndex`/`pressBehavior`/`opacity` live on
// gorhom's internal `BottomSheetDefaultBackdropProps`, which isn't exported.
// Identical dodge to `ActionSheet`'s and `RefundSheet`'s.
const Backdrop = BottomSheetBackdrop as unknown as ComponentType<
  BottomSheetBackdropProps & {
    appearsOnIndex?: number;
    disappearsOnIndex?: number;
    pressBehavior?: "none" | "close" | "collapse" | number;
    opacity?: number;
    style?: StyleProp<ViewStyle>;
  }
>;

const ScrollBody = BottomSheetScrollView as unknown as ComponentType<{
  contentContainerStyle?: StyleProp<ViewStyle>;
  children?: ReactNode;
  testID?: string;
}>;

export interface VariantValueSheetProps {
  /**
   * What is being edited, or null when the sheet is closed. Null is the ONLY
   * closed state — the parent must clear it in `onDismiss`, or a sheet backed
   * out of without submitting keeps its target and the next one opens on the
   * previous row's variant (a bug Orders shipped).
   */
  target: VariantEditTarget | null;
  isSubmitting?: boolean;
  /**
   * The last submit failure, as LOCAL parent state — never `mutation.error`.
   * react-query never resets a mutation error, so a sheet bound to it greets
   * the merchant with one stale failure on every subsequent open, on every
   * other product, for the life of the screen.
   */
  error?: string | null;
  /** Hands the target back rather than making the parent re-narrow its own
   *  state — there is then no unreachable "no target" branch on either side. */
  onSubmit: (target: VariantEditTarget, value: number) => void;
  onDismiss: () => void;
}

/**
 * The numeric-entry sheet behind Products' Edit price / Adjust stock.
 *
 * Controlled by a nullable `target` rather than an imperative handle, matching
 * `ActionSheet` (and unlike the older Orders sheets), because the target IS
 * the visibility: there is no state in which the sheet is open with nothing to
 * write to. An effect bridges that to `BottomSheetModal`'s present()/dismiss()
 * on the null→set / set→null EDGE only, so a re-render while open never
 * re-presents it.
 *
 * The body is mounted only while there is a target, and keyed on it, so every
 * open starts from the variant's real current value with no leftover draft —
 * "clear on present" as a structural property rather than a reset effect
 * someone can forget to extend.
 *
 * Content sits in `BottomSheetScrollView`, not a plain `View`: a fixed
 * percentage snap point cannot track Dynamic Type, and `BlockReasonSheet`
 * proved the failure mode by copying `RefundSheet`'s 52% and clipping BOTH
 * its buttons at `accessibility-large` with no gesture that could reveal them.
 */
export function VariantValueSheet({
  target,
  isSubmitting = false,
  error = null,
  onSubmit,
  onDismiss,
}: VariantValueSheetProps) {
  const modalRef = useRef<BottomSheetModal>(null);
  const wasVisible = useRef(false);
  const visible = target !== null;

  useEffect(() => {
    if (visible && !wasVisible.current) {
      modalRef.current?.present();
      wasVisible.current = true;
    } else if (!visible && wasVisible.current) {
      modalRef.current?.dismiss();
      wasVisible.current = false;
    }
  }, [visible]);

  // Cleared here rather than in the effect above so the effect's `!visible`
  // branch finds it already false once the parent's null round-trips back,
  // and never issues a second dismiss() on a sheet gorhom has already closed.
  const handleSheetDismissed = useCallback(() => {
    wasVisible.current = false;
    onDismiss();
  }, [onDismiss]);

  const renderBackdrop = useCallback(
    (props: BottomSheetBackdropProps): ReactNode => (
      <Backdrop
        {...props}
        appearsOnIndex={0}
        disappearsOnIndex={-1}
        // Gated while a write is open: gorhom keeps the scrim's
        // `pointerEvents: "auto"` either way, so the tap-through shield stays
        // up while only the dismiss gesture is withdrawn.
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
      snapPoints={SNAP_POINTS}
      enableDynamicSizing={false}
      enablePanDownToClose={!isSubmitting}
      keyboardBehavior="interactive"
      keyboardBlurBehavior="restore"
      onDismiss={handleSheetDismissed}
      backdropComponent={renderBackdrop}
      backgroundStyle={styles.background}
      handleIndicatorStyle={styles.handleIndicator}
    >
      <ScrollBody contentContainerStyle={styles.root} testID="variant-value-sheet-body">
        {target ? (
          <VariantValueForm
            key={`${target.variant.id}-${target.field}`}
            target={target}
            isSubmitting={isSubmitting}
            error={error}
            onSubmit={onSubmit}
            onCancel={() => modalRef.current?.dismiss()}
          />
        ) : null}
      </ScrollBody>
    </BottomSheetModal>
  );
}

/**
 * 52%, matching `RefundSheet` — the only other sheet in the app that pairs a
 * keyboard with a submit button, and the height that was settled on device
 * with the keypad up. The body scrolls (see the component doc), so this is a
 * resting height, not a ceiling on the content.
 */
const SNAP_POINTS = ["52%"];

interface VariantValueFormProps {
  target: VariantEditTarget;
  isSubmitting: boolean;
  error: string | null;
  onSubmit: (target: VariantEditTarget, value: number) => void;
  onCancel: () => void;
}

function VariantValueForm({
  target,
  isSubmitting,
  error,
  onSubmit,
  onCancel,
}: VariantValueFormProps) {
  const { product, variant, field } = target;
  const isPrice = field === "price";
  const current = isPrice ? variant.price : variant.inventory_quantity;
  // The raw decimal, never a formatted one: the field is what gets sent.
  const initial = String(current);
  const [text, setText] = useState(initial);

  /**
   * The pre-filled value, SELECTED, so the common case (replace it) is one
   * keystroke.
   *
   * `selectTextOnFocus` alone does not do this and never did: on iOS it acts
   * on a focus the USER causes, and this field is focused by `autoFocus`
   * during mount, before the native view has text to select. On device the
   * caret simply sat at the end of "89" and a merchant had to clear four
   * characters by hand — while the unit test asserting
   * `selectTextOnFocus === true` stayed green, because the prop really was
   * set. It is left set for the tap-back-in case; the controlled `selection`
   * below is what actually selects on open.
   *
   * Control is RELEASED the moment the merchant does anything — types, or
   * puts the caret anywhere other than the full-select this sets — because a
   * `selection` prop pinned across edits fights the keyboard for the caret.
   * The programmatic full-select echoes back through `onSelectionChange`
   * with exactly these values, so that echo is deliberately not treated as
   * the merchant moving the caret.
   */
  const [selection, setSelection] = useState<{ start: number; end: number } | undefined>(() => ({
    start: 0,
    end: initial.length,
  }));
  const releaseSelection = useCallback(() => setSelection(undefined), []);
  const handleSelectionChange = useCallback(
    (event: NativeSyntheticEvent<TextInputSelectionChangeEventData>) => {
      const { start, end } = event.nativeEvent.selection;
      if (start !== 0 || end !== initial.length) releaseSelection();
    },
    [initial.length, releaseSelection],
  );
  const handleChangeText = useCallback(
    (next: string) => {
      releaseSelection();
      setText(next);
    },
    [releaseSelection],
  );

  const parsed = validateVariantValue(field, text);
  const trimmed = text.trim();

  /**
   * Submit is armed by the TEXT changing, not by the parse succeeding.
   *
   * If an unparseable value disarmed the button, "-5" would grey it out and
   * the merchant would be left guessing which of empty / unchanged / invalid
   * they had hit. The inline message says which, and pressing the armed
   * button refuses the value without a request — the guard in `handleSubmit`
   * is the one that does the refusing, and it is reachable precisely because
   * of this.
   */
  const canSubmit = trimmed.length > 0 && trimmed !== initial.trim() && !isSubmitting;

  const handleSubmit = () => {
    if (parsed.value === null) return;
    onSubmit(target, parsed.value);
  };

  return (
    <>
      <Text preset="h3" color="text">
        {FIELD_TITLE[field]}
      </Text>
      {/* WHAT is being changed. On a single-variant product the picker is
          skipped entirely, so this line is the merchant's only confirmation
          that the sheet is aimed at the row they long-pressed. */}
      <Text preset="body" color="textSecondary" testID="variant-value-context">
        {`${product.title} · ${variantLabel(variant)}`}
      </Text>
      <Text preset="caption" color="textTertiary" testID="variant-value-current">
        {isPrice
          ? `Currently ${formatMoney(variant.price, variant.currency_code)}`
          : `Currently ${variant.inventory_quantity} in stock`}
      </Text>
      <FieldInput
        label={FIELD_INPUT_LABEL[field]}
        testID="variant-value-input"
        value={text}
        onChangeText={handleChangeText}
        selection={selection}
        onSelectionChange={handleSelectionChange}
        accessibilityLabel={`${FIELD_INPUT_LABEL[field]} for ${variantLabel(variant)}`}
        // decimal-pad for money, number-pad for a count: a keypad offering a
        // decimal point for a stock level invites the fractional value the
        // validator then has to refuse.
        keyboardType={isPrice ? "decimal-pad" : "number-pad"}
        autoFocus
        // Kept for a later tap back INTO the field; the controlled
        // `selection` above is what selects on open. See its doc comment —
        // this prop alone never worked with `autoFocus`.
        selectTextOnFocus
        editable={!isSubmitting}
      />
      {parsed.error ? (
        <Text preset="caption" color="danger" testID="variant-value-error">
          {parsed.error}
        </Text>
      ) : null}
      {error ? (
        <Text preset="caption" color="danger" testID="variant-value-submit-error">
          {error}
        </Text>
      ) : null}
      <View style={styles.actions}>
        <Pressable
          style={[styles.cancelBtn, isSubmitting && styles.disabled]}
          onPress={onCancel}
          disabled={isSubmitting}
          accessibilityRole="button"
          accessibilityLabel="Cancel"
          testID="variant-value-dismiss"
        >
          <Text preset="bodyEmphasis" color="text">
            Cancel
          </Text>
        </Pressable>
        <Pressable
          style={[styles.saveBtn, !canSubmit && styles.disabled]}
          onPress={handleSubmit}
          disabled={!canSubmit}
          accessibilityRole="button"
          accessibilityLabel={`Save ${FIELD_INPUT_LABEL[field].toLowerCase()}`}
          testID="variant-value-submit"
        >
          {isSubmitting ? (
            <ActivityIndicator size="small" color={theme.colors.inverse} />
          ) : (
            <Text preset="bodyEmphasis" color="inverse">
              Save
            </Text>
          )}
        </Pressable>
      </View>
    </>
  );
}

const styles = StyleSheet.create({
  // No `flex: 1`: this is a ScrollView CONTENT container, and a flexed one
  // pins the content to the viewport height — the very thing that clips
  // buttons at accessibility text sizes.
  root: {
    padding: theme.spacing.lg,
    gap: theme.spacing.md,
    paddingBottom: theme.spacing.xxl,
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
  saveBtn: {
    flex: 1,
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
  disabled: { opacity: 0.4 },
  background: { backgroundColor: theme.colors.background },
  handleIndicator: { backgroundColor: theme.colors.border },
  backdrop: {
    // Flat, low-opacity ink scrim — never a blur; this design system bans
    // glassmorphism.
    backgroundColor: theme.colors.overlay,
  },
});
