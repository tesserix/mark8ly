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
import { Text } from "./Text";
import { theme } from "@/lib/theme";

export interface ActionSheetItem {
  key: string;
  label: string;
  icon?: ReactNode;
  tone?: "default" | "danger";
  onPress: () => void;
}

export interface ActionSheetProps {
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

// Eyebrow's own padding (theme.spacing.lg top, theme.spacing.sm bottom)
// plus its `eyebrow` text preset's 16pt line height — matches its rendered
// height closely enough for a snap-point estimate (a few px of slack is
// invisible against the sheet's own top handle/grabber padding).
const TITLE_BLOCK_HEIGHT = theme.spacing.lg + 16 + theme.spacing.sm;
// gorhom's built-in handle/grabber area above the content.
const HANDLE_HEIGHT = 24;
// Minimum gap kept between the sheet's top edge and the screen's safe-area
// top inset when a long item list would otherwise push the snap point tall
// enough to reach (or clip under) the notch.
const TOP_CLEARANCE = theme.spacing.xxl;

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
  const { height: windowHeight } = useWindowDimensions();
  const hasItems = items.length > 0;

  // At least the home-indicator inset, but never less than the sheet's
  // previous fixed bottom padding — devices with no home indicator (no
  // bottom inset) still get real breathing room under the last row.
  const bottomPadding = Math.max(insets.bottom, theme.spacing.xl);

  const snapPoints = useMemo(() => {
    const rowsHeight = items.length * theme.row.minHeightSingle;
    const titleHeight = title ? TITLE_BLOCK_HEIGHT : 0;
    const chromeHeight = HANDLE_HEIGHT + bottomPadding;
    const computedHeight = rowsHeight + titleHeight + chromeHeight;
    // Clamp so a long item list can't pin the sheet to y=0 under the notch —
    // content now sits in `ScrollBody` (see doc comment), so anything past
    // the clamp is still reachable by scrolling, not silently truncated.
    const maxHeight = windowHeight - insets.top - TOP_CLEARANCE;
    // Floor at the sheet's own chrome height. Without this, a degenerate
    // `windowHeight` (e.g. 0 before layout settles) drives `maxHeight`
    // negative, and `normalizeSnapPoint` would then position the sheet
    // BELOW the container — `present()` still reports success, but the
    // sheet is invisible off-screen. That's the exact zero-height failure
    // mode the dynamic-sizing dodge above already fixed once; this floor
    // closes the same failure mode for the clamp path.
    const minHeight = HANDLE_HEIGHT + bottomPadding;
    return [Math.max(Math.min(computedHeight, maxHeight), minHeight)];
  }, [items.length, title, bottomPadding, windowHeight, insets.top]);

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
        {title ? <Eyebrow label={title} /> : null}
        {items.map((item, index) => (
          <View key={item.key}>
            {index > 0 ? <Hairline inset={theme.row.paddingH} /> : null}
            <PressableRow
              onPress={() => handlePress(item)}
              accessibilityLabel={item.label}
              testID={`action-sheet-item-${item.key}`}
              ripple={item.tone === "danger" ? theme.press.rippleDanger : theme.press.rippleInk}
            >
              {item.icon}
              <Text
                preset="body"
                numberOfLines={1}
                color={item.tone === "danger" ? theme.colors.danger : undefined}
              >
                {item.label}
              </Text>
            </PressableRow>
          </View>
        ))}
      </ScrollBody>
    </BottomSheetModal>
  );
}

const styles = StyleSheet.create({
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
