import { useCallback, type ReactNode } from "react";
import {
  StyleSheet,
  View,
  useWindowDimensions,
  type LayoutChangeEvent,
} from "react-native";
import { MAX_FONT_SCALE } from "./Text";
import { theme } from "@/lib/theme";

/**
 * A full-bleed bar pinned to the bottom of a screen, holding that screen's
 * primary action(s).
 *
 * Extracted verbatim from the one hand-rolled sticky footer that already
 * existed (`products/new.tsx`) — a hairline top rule on a solid Paper
 * background, NOT a floating pill and never a blur; this design system bans
 * glassmorphism.
 *
 * Two rules this component exists to hold:
 *
 *  1. The bar's height does not change with the host screen's STATE. A bar
 *     that appears and disappears reflows the whole screen under the
 *     merchant's thumb, and a `paddingBottom` that varies with state is the
 *     same class as the silent-clipping bugs this programme keeps shipping.
 *     A caller with a conditional action puts a caption in the same box, not
 *     nothing.
 *  2. The bar's height DOES change with Dynamic Type, and the host's scroll
 *     padding has to follow it or the last row of content hides behind the
 *     bar. `useStickyBarHeight()` is the number to pad by; `onHeightChange`
 *     reports the real measured height for callers that would rather not
 *     trust arithmetic (recommended — see below).
 */

/** Vertical padding above and below the bar's content, per side. */
const BAR_PADDING_V = theme.spacing.md;

/**
 * The content row's height at fontScale 1 — a 48pt primary control, the same
 * box every primary button in this app uses.
 */
export const STICKY_BAR_CONTENT_HEIGHT = 48;

/**
 * The bar's own height excluding `bottom`, at fontScale 1.
 *
 * Prefer `useStickyBarHeight()` for scroll padding: this constant is the 1×
 * value and a merchant at 2× text needs roughly twice the content row.
 */
export const STICKY_BAR_HEIGHT = STICKY_BAR_CONTENT_HEIGHT + BAR_PADDING_V * 2;

function clampFontScale(fontScale: number): number {
  return Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
}

/** The bar's height at a given device font scale. Clamped to `MAX_FONT_SCALE`. */
export function stickyBarHeightFor(fontScale: number): number {
  return STICKY_BAR_CONTENT_HEIGHT * clampFontScale(fontScale) + BAR_PADDING_V * 2;
}

/**
 * The bar's height on this device right now — what a host screen should add
 * to its `paddingBottom`, on top of whatever it passed as `bottom`.
 */
export function useStickyBarHeight(): number {
  const { fontScale } = useWindowDimensions();
  return stickyBarHeightFor(fontScale);
}

export interface StickyActionBarProps {
  children: ReactNode;
  /**
   * Distance from the bottom of the SCREEN to the bottom of the bar.
   *
   * The caller owns this because the two callers sit in different worlds:
   *   - a `(tabs)` screen must clear the floating dock → `insets.bottom +
   *     DOCK_BOTTOM_GAP + DOCK_HEIGHT + a gap`
   *   - a `presentation: "modal"` screen COVERS the dock → `insets.bottom`
   *
   * Getting this wrong is the modal-sheet trap in a different costume.
   */
  bottom: number;
  /**
   * Fires with the bar's real measured height whenever it changes.
   *
   * `stickyBarHeightFor` is an estimate built on "the content row is one
   * 48pt-scaled box". It is right for the two callers today, but a label that
   * wraps to two lines at 2× text makes the real bar taller than the estimate
   * — and an under-estimate hides the last row of scroll content behind the
   * bar, silently. Callers should pad by `Math.max(estimate, measured)`.
   */
  onHeightChange?: (height: number) => void;
  testID?: string;
}

export function StickyActionBar({
  children,
  bottom,
  onHeightChange,
  testID,
}: StickyActionBarProps) {
  const { fontScale } = useWindowDimensions();

  const handleLayout = useCallback(
    (e: LayoutChangeEvent) => onHeightChange?.(e.nativeEvent.layout.height),
    [onHeightChange],
  );

  return (
    <View
      testID={testID}
      onLayout={onHeightChange ? handleLayout : undefined}
      // A plain ARRAY, never a function: NativeWind's JSX interop resolves an
      // array `style` but silently drops a function one.
      style={[
        styles.bar,
        { bottom, minHeight: stickyBarHeightFor(fontScale) },
      ]}
    >
      {children}
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {
    position: "absolute",
    left: 0,
    right: 0,
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.sm,
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: BAR_PADDING_V,
    backgroundColor: theme.colors.background,
    borderTopWidth: theme.hairline,
    borderTopColor: theme.colors.hairline,
  },
});
