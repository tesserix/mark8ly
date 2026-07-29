import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  type ComponentType,
  type ReactNode,
} from "react";
import {
  StyleSheet,
  useWindowDimensions,
  View,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import {
  BottomSheetBackdrop,
  BottomSheetModal,
  BottomSheetScrollView,
  type BottomSheetBackdropProps,
} from "@gorhom/bottom-sheet";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { Eyebrow } from "./Eyebrow";
import { Hairline } from "./Hairline";
import { PressableRow } from "./PressableRow";
import { MAX_FONT_SCALE, Text } from "./Text";
import { theme } from "@/lib/theme";

export interface ActionSheetItem {
  key: string;
  label: string;
  icon?: ReactNode;
  tone?: "default" | "danger";
  /**
   * Renders the row as present-but-unavailable: no `onPress`, no press
   * feedback, tertiary ink instead of its normal colour, and
   * `accessibilityState.disabled` for VoiceOver/TalkBack.
   *
   * Exists so a caller whose action set is CONDITIONAL can keep
   * `items.length` constant. Orders' long-press menu shows the same four
   * actions on every order, but only some are legal for a given one
   * (fulfilling a cancelled order is a guaranteed 409; "Email label" needs a
   * shipment that is fetched lazily and may not exist). Dropping the illegal
   * ones from `items` instead would change `items.length`, which recomputes
   * `snapPoints` — resizing the sheet under the merchant's thumb, and for
   * the lazily-fetched shipment resizing it AFTER it has already opened.
   *
   * Additive and defaulted off: every existing call site is unaffected.
   */
  disabled?: boolean;
  /**
   * A second line under the label, for the VALUE a merchant is choosing by.
   *
   * Products' variant picker is the case that forced it: "Adjust stock" asked
   * which variant to restock while showing no quantities, and the product row
   * above it shows only the total — so finding the low variant cost five
   * open-read-back-out cycles. On this store the variants carry no option
   * values either, so the rows were bare SKUs with nothing to choose between.
   *
   * It is a SEPARATE LINE, not appended to `label`, and that is the whole
   * point: a suffix would be the first thing an ellipsis ate on a narrow
   * device at accessibility text sizes — the value would be truncated away
   * and the row would be no better than before. Its own line is short enough
   * to survive, and the height budget below reserves it (`detailCount`), so
   * adding one cannot push the last row below the fold.
   *
   * Additive and defaulted off: a row without one is the single-line row it
   * always was, at exactly the height it always was.
   */
  detail?: string;
  onPress: () => void;
}

export interface ActionSheetProps {
  /**
   * Optional identifying line above the items. ARBITRARY caller text is
   * safe here — a merchant's product title, a customer's name, a ticket
   * subject: it is clamped to `TITLE_LINES` and the height budget reserves
   * exactly that many lines, so a long title truncates rather than pushing
   * the last item below the fold.
   */
  title?: string;
  items: ActionSheetItem[];
  visible: boolean;
  /**
   * Fires exactly once per close — whether the close was triggered by a row
   * tap, the backdrop tap, or a swipe-down — because it is sourced solely
   * from `BottomSheetModal`'s own `onDismiss` callback (see the component
   * doc comment below). The parent is expected to flip `visible` to false
   * in response. Until it does, `visible={true}` is a no-op: `wasVisible`
   * only re-arms the false→true edge once the prop actually goes false, so
   * re-passing `visible={true}` without that round-trip will NOT re-present
   * an already-dismissed sheet.
   */
  onDismiss: () => void;
}

/**
 * How many line boxes the title may occupy — and the number the height
 * budget below is allowed to assume.
 *
 * ONE, and the `Eyebrow` below is clamped to the same constant so the two
 * cannot drift. The budget used to assume a single line while rendering an
 * unclamped `Eyebrow`, which held only because the first caller passed
 * `Order #1042`. Products passes a merchant's own product title: "Insulated
 * Stainless Steel Sport Water Bottle 1L — Bondi Edition" wrapped to three
 * uppercased, letter-spaced lines, the snap point came up ~32pt short, and
 * the menu's last item fell below the fold. Every screen after this one
 * passes arbitrary text too (customer names, review bodies, campaign names,
 * ticket subjects).
 *
 * Raising it is a real design change, not a knob: it makes EVERY sheet taller
 * (a one-line title would then reserve two lines' worth of empty space), so
 * the title truncates instead. A menu's title identifies the target; it is
 * not the place to read the whole of it.
 */
const TITLE_LINES = 1;
// Read off the same tokens the rendered text uses, so a type-scale change
// moves the budget with it rather than orphaning a literal.
const EYEBROW_LINE_HEIGHT = theme.text.eyebrow.lineHeight ?? 16;
const BODY_LINE_HEIGHT = theme.text.body.lineHeight ?? 24;
// The `detail` second line, and the gap above it — same 13/18 caption and
// same 4pt stack gap every two-line row in the app uses (see `SegmentRow`).
const CAPTION_LINE_HEIGHT = theme.text.caption.lineHeight ?? 18;
const DETAIL_STACK_GAP = theme.spacing.xs;
// gorhom's built-in handle/grabber area above the content.
const HANDLE_HEIGHT = 24;
// Minimum gap kept between the sheet's top edge and the screen's safe-area
// top inset when a long item list would otherwise push the snap point tall
// enough to reach (or clip under) the notch.
const TOP_CLEARANCE = theme.spacing.xxl;

export interface ActionSheetHeightInput {
  itemCount: number;
  /**
   * How many of those items carry a `detail` second line. Optional and
   * defaulted to 0, so every pre-existing call site computes exactly the
   * height it did before. A detail row is a two-line stack, not a one-line
   * row, and budgeting it as one line is the same one-line under-budget that
   * put a four-item menu's last row below the fold at AX sizes.
   */
  detailCount?: number;
  hasTitle: boolean;
  /**
   * The OS Dynamic Type / font-scale multiplier, RAW. Capped internally at
   * `MAX_FONT_SCALE` because `Text` caps every label it renders at exactly
   * that — budgeting against the uncapped OS value would reserve space for
   * text that can never be drawn.
   */
  fontScale: number;
  bottomPadding: number;
  windowHeight: number;
  topInset: number;
}

/**
 * The sheet's snap-point height, as pure arithmetic over its known pieces.
 *
 * Exported (and unit-tested at explicit `fontScale` values) because this is
 * the part that cannot be seen in a screenshot: an under-budget of one line
 * parks the last row just below the fold, where it is reachable only by
 * scrolling inside a menu that gives no hint it scrolls.
 *
 * Dynamic Type moves BOTH terms, which the first version of this arithmetic
 * missed — it measured everything at 1×. At the accessibility text sizes a
 * title line is 32pt not 16, and a row's single 17/24 label needs
 * 24×2 + 14×2 = 76pt, past `theme.row.minHeightSingle`'s 64. A four-item menu
 * was therefore ~64pt short at AX sizes (recorded on device against Products
 * AND Orders, whose menu is also four items).
 */
export function actionSheetHeight({
  itemCount,
  detailCount = 0,
  hasTitle,
  fontScale,
  bottomPadding,
  windowHeight,
  topInset,
}: ActionSheetHeightInput): number {
  const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
  // `theme.row.minHeightSingle` is a FLOOR on `PressableRow`, not its height:
  // above ~1.3× the label's own line box plus the row's fixed vertical
  // padding is what actually sizes the row.
  const rowHeight = Math.max(
    theme.row.minHeightSingle,
    BODY_LINE_HEIGHT * scale + theme.row.paddingV * 2,
  );
  // Eyebrow's own padding (theme.spacing.lg top, theme.spacing.sm bottom) —
  // fixed, it does not scale — plus its clamped line boxes, which do.
  const titleHeight = hasTitle
    ? theme.spacing.lg + EYEBROW_LINE_HEIGHT * TITLE_LINES * scale + theme.spacing.sm
    : 0;
  // A `detail` row carries a 17/24 label AND a 13/18 caption, both scaling,
  // with the 4pt stack gap between them — floored at the app's two-line row
  // density, exactly as the single-line case is floored at the one-line one.
  const detailRowHeight = Math.max(
    theme.row.minHeightDouble,
    (BODY_LINE_HEIGHT + CAPTION_LINE_HEIGHT) * scale + DETAIL_STACK_GAP + theme.row.paddingV * 2,
  );
  const details = Math.min(Math.max(detailCount, 0), itemCount);
  const chromeHeight = HANDLE_HEIGHT + bottomPadding;
  const computedHeight =
    (itemCount - details) * rowHeight +
    details * detailRowHeight +
    titleHeight +
    chromeHeight;
  // Clamp so a long item list can't pin the sheet to y=0 under the notch —
  // content sits in `ScrollBody` (see doc comment), so anything past the
  // clamp is still reachable by scrolling, not silently truncated.
  const maxHeight = windowHeight - topInset - TOP_CLEARANCE;
  // Floor at the sheet's own chrome height. Without this, a degenerate
  // `windowHeight` (e.g. 0 before layout settles) drives `maxHeight`
  // negative, and `normalizeSnapPoint` would then position the sheet BELOW
  // the container — `present()` still reports success, but the sheet is
  // invisible off-screen. That's the exact zero-height failure mode the
  // dynamic-sizing dodge already fixed once; this floor closes the same
  // failure mode for the clamp path.
  return Math.max(Math.min(computedHeight, maxHeight), chromeHeight);
}

// @gorhom/bottom-sheet ships its own copy of @types/react, whose `ReactNode`
// includes `bigint`; this project's doesn't, so its components trip TS2786
// ("cannot be used as a JSX component") — same issue as `BottomSheetView`
// (see doc comment below) and `BottomSheetScrollView`
// (OptionBuilderSheet's `ScrollBody` cast). Re-typed through this project's
// React to the props actually passed; runtime is unaffected.
//
// `appearsOnIndex`/`disappearsOnIndex`/`pressBehavior`/`opacity` live on
// gorhom's internal `BottomSheetDefaultBackdropProps`, which — unlike the
// base `BottomSheetBackdropProps` the package DOES export — isn't exported
// from the package at all, so they're declared here directly instead.
const Backdrop = BottomSheetBackdrop as unknown as ComponentType<
  BottomSheetBackdropProps & {
    appearsOnIndex?: number;
    disappearsOnIndex?: number;
    pressBehavior?: "none" | "close" | "collapse" | number;
    opacity?: number;
  }
>;

// Same TS2786 dodge as `Backdrop` above, for the scrolling-content
// equivalent of the `BottomSheetView` cast this component used to need —
// see `OptionBuilderSheet`'s own `ScrollBody` cast, which this follows
// exactly (same prop surface: content + a style for the inner container).
const ScrollBody = BottomSheetScrollView as unknown as ComponentType<{
  contentContainerStyle?: StyleProp<ViewStyle>;
  children?: ReactNode;
}>;

/**
 * Long-press action menu — the zero-dependency stand-in for a native context
 * menu (Orders' Fulfil / Email label / Refund / Cancel). Wraps
 * @gorhom/bottom-sheet as a SOLID Paper surface with hairline rules between
 * rows; no backdrop blur or translucent material — matches the sheets in
 * components/orders/.
 *
 * Controlled by a `visible` boolean rather than the imperative
 * present()/dismiss() handle CancelReasonSheet/RefundSheet expose. An effect
 * bridges that boolean to BottomSheetModal's own imperative API: it calls
 * `present()`/`dismiss()` only on the false→true / true→false EDGE (guarded
 * by `wasVisible`), so a re-render while already visible (e.g. the parent
 * passing a new `items` array) never re-presents the sheet or re-fires the
 * open haptic. See `ActionSheetProps.onDismiss` for the prop-contract
 * implication of that edge guard.
 *
 * Rows reuse `PressableRow` unmodified — its default `lines={1}` already
 * gives the required 64pt `minHeight` box, real press feedback (no function
 * `style` prop; see PressableRow's own doc comment on the NativeWind
 * landmine), and a real 44pt+ touch target. Nothing here hand-rolls a
 * near-copy of it.
 *
 * Tapping a row fires that item's `onPress` and then closes the sheet via
 * `modalRef.current?.dismiss()` — it does NOT call the `onDismiss` prop
 * directly. `onDismiss` is wired ONLY to `BottomSheetModal`'s own
 * `onDismiss` callback, which fires once for every close, whatever
 * triggered it (a row tap's programmatic `dismiss()`, a backdrop tap, or a
 * user's swipe-down). That single source of truth is deliberate: calling
 * `onDismiss()` from `handlePress` AND wiring it to `BottomSheetModal`'s own
 * `onDismiss` double-fires it for a row tap once the parent's resulting
 * `visible={false}` round-trips back through the effect above (which then
 * also calls `modalRef.current?.dismiss()`) — see the "fires onDismiss
 * exactly once" tests.
 *
 * A backdrop (`BottomSheetBackdrop`, `pressBehavior="close"`) is required,
 * not optional, for a context menu: gorhom's hosting container is
 * `pointerEvents: 'box-none'` and `BottomSheetModal` has no default
 * backdrop, so without one the area above the sheet is live — a tap meant
 * to cancel the menu instead lands on whatever is underneath it. It's a
 * flat, low-opacity `theme.colors.overlay` ink scrim (same token
 * StoreSelector uses) — never a blur; this design system bans
 * glassmorphism.
 *
 * Content sits in `BottomSheetScrollView` (via the `ScrollBody` cast — same
 * TS2786 dodge as `Backdrop` below, and the same cast `OptionBuilderSheet`
 * uses), not a plain `View`. A fixed-height snap point paired with
 * non-scrolling content is a silent-clipping trap: on a short device (iPhone
 * SE) or a long item list, rows past the clamped height would be clipped by
 * the sheet's own bounds and permanently unreachable — no scroll gesture
 * could ever reveal them, and nothing about that failure is visible in a
 * screenshot taken above the fold. `BottomSheetScrollView` keeps the sheet
 * itself bounded by `snapPoints` while making every row reachable by
 * scrolling inside it, however many items the caller passes.
 *
 * This sheet does NOT use `enableDynamicSizing` (unlike its doc-comment's
 * first draft, which enabled it): dynamic sizing measures content height
 * through `BottomSheetView`'s own internal onLayout→context wiring, which
 * neither the plain-`View` dodge nor the `ScrollBody` dodge participates in
 * — the sheet's `present()` call succeeds (confirmed on-device: the modal
 * opens) but renders at zero measured height, i.e. invisible. No sheet
 * anywhere else in this codebase uses dynamic sizing either, for the same
 * reason — they all pair fixed `snapPoints` with either a plain `View`
 * (CancelReasonSheet/RefundSheet/EmailLabelSheet) or `ScrollBody`
 * (OptionBuilderSheet). This component follows suit with a snap point
 * computed from the known, fixed-height pieces (`PressableRow`'s 64pt rows,
 * the optional `Eyebrow` title, hairlines, chrome) rather than a hand-picked
 * percentage — item count is caller-controlled and a fixed "50%" would clip
 * a 6-item menu or leave a 2-item one floating with empty space below it.
 * Unlike OptionBuilderSheet's `["60%"]`, this component's height is
 * content-derived and clamped/floored (see the `snapPoints` `useMemo`
 * below), so `ScrollBody` here is a reachability guarantee for the clamp
 * case, not the primary sizing mechanism.
 */
export function ActionSheet({ title, items, visible, onDismiss }: ActionSheetProps) {
  const modalRef = useRef<BottomSheetModal>(null);
  const wasVisible = useRef(false);
  const insets = useSafeAreaInsets();
  const { height: windowHeight, fontScale } = useWindowDimensions();
  const hasItems = items.length > 0;

  // At least the home-indicator inset, but never less than the sheet's
  // previous fixed bottom padding — devices with no home indicator (no
  // bottom inset) still get real breathing room under the last row.
  const bottomPadding = Math.max(insets.bottom, theme.spacing.xl);

  // By COUNT, not by identity: the budget cares how many rows are two lines
  // tall, never what they say.
  const detailCount = items.reduce((total, item) => (item.detail ? total + 1 : total), 0);

  const snapPoints = useMemo(
    () => [
      actionSheetHeight({
        itemCount: items.length,
        detailCount,
        hasTitle: Boolean(title),
        fontScale,
        bottomPadding,
        windowHeight,
        topInset: insets.top,
      }),
    ],
    // `title` by PRESENCE only — the budget reserves the same clamped block
    // whatever the copy is, which is the whole point of the clamp.
    [items.length, detailCount, title, fontScale, bottomPadding, windowHeight, insets.top],
  );

  useEffect(() => {
    if (visible && hasItems && !wasVisible.current) {
      void adminHaptics.menuOpen();
      modalRef.current?.present();
      wasVisible.current = true;
    } else if ((!visible || !hasItems) && wasVisible.current) {
      modalRef.current?.dismiss();
      wasVisible.current = false;
    }
  }, [visible, hasItems]);

  // No `if (item.disabled) return` guard here. `PressableRow` below receives
  // `disabled={item.disabled}` and holds the only guard that is reachable:
  // it never calls its `onPress` when disabled, so this function is never
  // entered for a disabled item and a guard here could not be exercised by
  // any test. Deleting `disabled={item.disabled}` reddens three tests;
  // deleting the guard that used to sit here reddened none — belt-and-braces
  // that no test can load-bear is just unverifiable code. The disabled path
  // is covered against `PressableRow` (see action-sheet.test.tsx, "disabled
  // items", and orders-screen.test.tsx's illegal-action cases).
  const handlePress = (item: ActionSheetItem) => {
    item.onPress();
    modalRef.current?.dismiss();
  };

  // `BottomSheetModal`'s own `onDismiss` already reflects the sheet as
  // closed by the time it fires (gorhom has finished its own close
  // animation/state transition) — clearing `wasVisible` here, rather than
  // leaving it to the present/dismiss effect above, means that effect's
  // `!visible` branch finds `wasVisible.current` already `false` once the
  // parent's resulting `visible={false}` round-trips back through it, so it
  // never issues a second, redundant `modalRef.current?.dismiss()` call on
  // a sheet gorhom has already closed. See the "calls dismiss() exactly
  // once" test.
  const handleSheetDismissed = useCallback(() => {
    wasVisible.current = false;
    onDismiss();
  }, [onDismiss]);

  const renderBackdrop = useCallback(
    (props: BottomSheetBackdropProps) => (
      <Backdrop
        {...props}
        appearsOnIndex={0}
        disappearsOnIndex={-1}
        pressBehavior="close"
        opacity={1}
        style={styles.backdrop}
      />
    ),
    [],
  );

  return (
    <BottomSheetModal
      ref={modalRef}
      snapPoints={snapPoints}
      enableDynamicSizing={false}
      enablePanDownToClose
      onDismiss={handleSheetDismissed}
      backdropComponent={renderBackdrop}
      backgroundStyle={styles.background}
      handleIndicatorStyle={styles.handleIndicator}
    >
      <ScrollBody contentContainerStyle={{ paddingBottom: bottomPadding }}>
        {title ? <Eyebrow label={title} numberOfLines={TITLE_LINES} /> : null}
        {items.map((item, index) => (
          <View key={item.key}>
            {index > 0 ? <Hairline inset={theme.row.paddingH} /> : null}
            <PressableRow
              onPress={() => handlePress(item)}
              disabled={item.disabled}
              lines={item.detail ? 2 : 1}
              // The detail is CONTENT, not a hint — a merchant using
              // VoiceOver picks a variant by its stock level exactly as a
              // sighted one does, so it is announced with the label rather
              // than left to be discovered.
              accessibilityLabel={item.detail ? `${item.label}, ${item.detail}` : item.label}
              testID={`action-sheet-item-${item.key}`}
              ripple={item.tone === "danger" ? theme.press.rippleDanger : theme.press.rippleInk}
            >
              {item.icon}
              <View style={styles.body}>
                <Text
                  preset="body"
                  numberOfLines={1}
                  // Disabled wins over tone: a disabled DANGER row painted
                  // oxblood still reads as an armed destructive action.
                  // Tertiary (#5C5953) is the AA-passing muted ink — never the
                  // banned rgba(14,14,12,0.5).
                  color={
                    item.disabled
                      ? theme.colors.textTertiary
                      : item.tone === "danger"
                        ? theme.colors.danger
                        : undefined
                  }
                >
                  {item.label}
                </Text>
                {item.detail ? (
                  <Text
                    preset="caption"
                    numberOfLines={1}
                    color={
                      item.disabled ? theme.colors.textTertiary : theme.colors.textSecondary
                    }
                    testID={`action-sheet-detail-${item.key}`}
                  >
                    {item.detail}
                  </Text>
                ) : null}
              </View>
            </PressableRow>
          </View>
        ))}
      </ScrollBody>
    </BottomSheetModal>
  );
}

const styles = StyleSheet.create({
  // The label (and its optional detail line) as one column, so a two-line
  // row stacks rather than trying to fit both on the row's single baseline.
  // `flex: 1` claims the space left of nothing — the row has no trailing
  // slot — so the label's own `numberOfLines={1}` truncation point is
  // unchanged from before this stack existed.
  body: { flex: 1, gap: theme.spacing.xs },
  background: {
    backgroundColor: theme.colors.background,
  },
  handleIndicator: {
    backgroundColor: theme.colors.border,
  },
  backdrop: {
    // Flat, low-opacity ink scrim — never a blur/glassmorphism. Same token
    // StoreSelector uses for its own full-screen overlay.
    backgroundColor: theme.colors.overlay,
  },
});
