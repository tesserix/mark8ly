import { useEffect, useMemo, useRef, type ReactNode } from "react";
import { StyleSheet, View } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
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
  onDismiss: () => void;
}

// Eyebrow's own padding (theme.spacing.lg top, theme.spacing.sm bottom)
// plus its `eyebrow` text preset's 16pt line height — matches its rendered
// height closely enough for a snap-point estimate (a few px of slack is
// invisible against the sheet's own top handle/grabber padding).
const TITLE_BLOCK_HEIGHT = theme.spacing.lg + 16 + theme.spacing.sm;
// gorhom's built-in handle/grabber area above the content, plus this
// component's own `root` bottom padding.
const CHROME_HEIGHT = 24 + theme.spacing.xl;

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
 * open haptic.
 *
 * Rows reuse `PressableRow` unmodified — its default `lines={1}` already
 * gives the required 64pt `minHeight` box, real press feedback (no function
 * `style` prop; see PressableRow's own doc comment on the NativeWind
 * landmine), and a real 44pt+ touch target. Nothing here hand-rolls a
 * near-copy of it.
 *
 * Tapping a row fires that item's `onPress` and then the sheet's own
 * `onDismiss` directly — it does NOT wait for `BottomSheetModal`'s own
 * `onDismiss` callback (which only fires once the close animation actually
 * completes, and only for a user-driven gesture/backdrop dismiss). The
 * parent is expected to flip `visible` to false in response; the effect
 * above then calls the real `modalRef.current?.dismiss()` to animate the
 * sheet shut. `BottomSheetModal`'s own `onDismiss` prop is still wired to
 * `onDismiss` so a user's swipe-down-to-close (which this component never
 * initiates itself) also syncs back out to the parent's `visible` state.
 *
 * Content sits in a plain `View`, not `BottomSheetView` — @gorhom/bottom-sheet
 * ships its own copy of @types/react whose `ReactNode` includes `bigint`,
 * which this project's doesn't, so `BottomSheetView` trips TS2786 ("cannot
 * be used as a JSX component"). Same dodge as CancelReasonSheet/RefundSheet/
 * EmailLabelSheet (non-scrolling content); see OptionBuilderSheet's
 * `ScrollBody` cast for the scrolling-content equivalent.
 *
 * That dodge is also why this sheet does NOT use `enableDynamicSizing`
 * (unlike its doc-comment's first draft, which enabled it): dynamic sizing
 * measures content height through `BottomSheetView`'s own internal
 * onLayout→context wiring, which a plain `View` doesn't participate in — the
 * sheet's `present()` call succeeds (confirmed on-device: the modal opens)
 * but renders at zero measured height, i.e. invisible. No sheet anywhere
 * else in this codebase uses dynamic sizing either, for the same reason —
 * they all pair a plain `View` with a fixed `snapPoints`
 * (CancelReasonSheet/RefundSheet/EmailLabelSheet/OptionBuilderSheet). This
 * component follows suit with a snap point computed from the known,
 * fixed-height pieces (`PressableRow`'s 64pt rows, the optional `Eyebrow`
 * title, hairlines, chrome) rather than a hand-picked percentage — item
 * count is caller-controlled and a fixed "50%" would clip a 6-item menu or
 * leave a 2-item one floating with empty space below it.
 */
export function ActionSheet({ title, items, visible, onDismiss }: ActionSheetProps) {
  const modalRef = useRef<BottomSheetModal>(null);
  const wasVisible = useRef(false);

  const snapPoints = useMemo(() => {
    const rowsHeight = items.length * theme.row.minHeightSingle;
    const titleHeight = title ? TITLE_BLOCK_HEIGHT : 0;
    return [rowsHeight + titleHeight + CHROME_HEIGHT];
  }, [items.length, title]);

  useEffect(() => {
    if (visible && !wasVisible.current) {
      void adminHaptics.menuOpen();
      modalRef.current?.present();
    } else if (!visible && wasVisible.current) {
      modalRef.current?.dismiss();
    }
    wasVisible.current = visible;
  }, [visible]);

  const handlePress = (item: ActionSheetItem) => {
    item.onPress();
    onDismiss();
  };

  return (
    <BottomSheetModal
      ref={modalRef}
      snapPoints={snapPoints}
      enableDynamicSizing={false}
      enablePanDownToClose
      onDismiss={onDismiss}
      backgroundStyle={styles.background}
      handleIndicatorStyle={styles.handleIndicator}
    >
      <View style={styles.root}>
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
                color={item.tone === "danger" ? theme.colors.danger : theme.colors.text}
              >
                {item.label}
              </Text>
            </PressableRow>
          </View>
        ))}
      </View>
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
  root: {
    paddingBottom: theme.spacing.xl,
  },
});
