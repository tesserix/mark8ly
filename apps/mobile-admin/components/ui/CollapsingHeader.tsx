import type { ReactNode } from "react";
import { StyleSheet, View, useWindowDimensions } from "react-native";
import Animated, {
  Extrapolation,
  interpolate,
  useAnimatedStyle,
  useDerivedValue,
  useReducedMotion,
  type SharedValue,
} from "react-native-reanimated";
import { Text } from "./Text";
import { Hairline } from "./Hairline";
import { theme } from "@/lib/theme";

/**
 * Typography for the eyebrow line. `"eyebrow"` (the DEFAULT, and what every
 * existing call site gets) is the uppercase, letterspaced small-caps label.
 * `"caption"` is sentence case at the same tertiary ink — for datelines and
 * other running prose where SHOUTING IN CAPS is wrong ("Monday, 27 July",
 * not "MONDAY, 27 JULY").
 *
 * Deliberately an ADDITIVE opt-in, not a changed default: this primitive has
 * ~15 call sites and flipping a shared default rippled through all of them
 * earlier in this increment.
 */
export type EyebrowPreset = "eyebrow" | "caption";

export interface CollapsingHeaderProps {
  /** Label above the title, shown only in the expanded state. */
  eyebrow?: string;
  /** Typography for `eyebrow`. Defaults to the uppercase small-caps preset. */
  eyebrowPreset?: EyebrowPreset;
  /** Serif title — h1 expanded, h3 collapsed. */
  title: string;
  /** Optional one-line caption — body under the title expanded, caption collapsed. */
  subtitle?: string;
  rightSlot?: ReactNode;
  /**
   * Owned by the caller and wired to their scroll view's
   * `useAnimatedScrollHandler`. This component only reads it — it never
   * creates its own scroll handler, so the same shared value can drive other
   * scroll-linked UI (e.g. a search field reveal) in the same screen.
   */
  scrollY: SharedValue<number>;
}

/** Scroll offset (px) at which the header reaches its fully collapsed state. */
export const COLLAPSE_DISTANCE = 64;

const EXPANDED_HEIGHT = 96;
const COLLAPSED_HEIGHT = 56;

/**
 * Cap on the iOS Dynamic Type / Android font-scale multiplier applied to
 * every line in this header. 2.0 honours WCAG 2.1 SC 1.4.4 (text resizable to
 * 200% without loss of content) while stopping the accessibility sizes above
 * it — iOS reaches 3.1× — from turning a header into most of the screen.
 */
export const MAX_FONT_SCALE = 2;

/**
 * Header heights for a given device font scale.
 *
 * `styles.block` is `position: absolute; top: 0; bottom: 0` inside a
 * container with `overflow: "hidden"`, so a FIXED height clips as soon as the
 * scaled line boxes exceed it. RN scales BOTH `fontSize` and `lineHeight` by
 * the multiplier, so a one-line h1 (36pt line box) needs 68pt at 1.9× and the
 * eyebrow+title stack needs ~99pt — past `EXPANDED_HEIGHT` 96. A merchant who
 * bumped their text size lost the ascenders of their own shop name.
 *
 * Scaling the CONTAINER by the same clamped multiplier the text is capped at
 * makes non-clipping structural rather than empirical: content height is
 * `C × s` and the box is `H × s` for the same `s`, and `H > C` holds at every
 * combination of eyebrow/subtitle presence at s = 1 (expanded: 20 + 36 + 28 =
 * 84 < 96; collapsed: 26 + 20 = 46 < 56). Multiplying both sides by the same
 * positive `s` preserves the inequality for every scale.
 *
 * Exported so the arithmetic is testable without mocking the RN Dimensions
 * module.
 */
export function headerHeightsFor(fontScale: number): {
  expanded: number;
  collapsed: number;
} {
  const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
  return {
    expanded: EXPANDED_HEIGHT * scale,
    collapsed: COLLAPSED_HEIGHT * scale,
  };
}

/**
 * Scroll-driven serif header: a tall editorial block (eyebrow + h1 + subtitle)
 * that crossfades into a compact bar (h3 + caption + hairline) as the owning
 * scroll view moves past `COLLAPSE_DISTANCE`.
 *
 * Dynamic Type safe: every line is capped at `MAX_FONT_SCALE` and held to one
 * line, and the container's own height scales by the same clamped multiplier
 * (see `headerHeightsFor`) so nothing clips against `overflow: "hidden"`.
 *
 * Both layers are always mounted and cross-faded via animated `opacity` —
 * driven entirely by `useDerivedValue`/`useAnimatedStyle` off the caller's
 * `scrollY`, never by a React re-render on scroll. Surface is solid Paper
 * with a hairline; never a blur — that is a design-system rule, not a
 * preference, so don't reach for `expo-blur` here later.
 */
export function CollapsingHeader({
  eyebrow,
  eyebrowPreset = "eyebrow",
  title,
  subtitle,
  rightSlot,
  scrollY,
}: CollapsingHeaderProps) {
  const reduceMotion = useReducedMotion();
  // `useWindowDimensions` (not `PixelRatio.getFontScale()`) because it
  // re-renders when the user changes their text size while the app is
  // foregrounded — the static read would leave the header at the old height.
  const { fontScale } = useWindowDimensions();
  const heights = headerHeightsFor(fontScale);

  // Single source of truth for collapse progress (0 expanded → 1 collapsed).
  // Reduced motion bypasses the interpolation entirely: any non-zero offset
  // snaps straight to the collapsed state instead of easing through it.
  const progress = useDerivedValue(() => {
    "worklet";
    if (reduceMotion) {
      return scrollY.value > 0 ? 1 : 0;
    }
    return interpolate(scrollY.value, [0, COLLAPSE_DISTANCE], [0, 1], Extrapolation.CLAMP);
  }, [reduceMotion]);

  const containerStyle = useAnimatedStyle(
    () => ({
      height: interpolate(
        progress.value,
        [0, 1],
        [heights.expanded, heights.collapsed],
        Extrapolation.CLAMP,
      ),
    }),
    [heights.expanded, heights.collapsed],
  );

  const expandedStyle = useAnimatedStyle(() => ({ opacity: 1 - progress.value }));
  const collapsedStyle = useAnimatedStyle(() => ({ opacity: progress.value }));

  return (
    <Animated.View style={[styles.container, containerStyle]} testID="collapsing-header">
      <View style={styles.row}>
        <View style={styles.left}>
          <Animated.View
            style={[styles.block, expandedStyle]}
            pointerEvents="none"
            testID="collapsing-header-expanded"
          >
            {/* `maxFontSizeMultiplier` + `numberOfLines` on every line, not
                just the title: the height math above assumes exactly one line
                box per element, capped at MAX_FONT_SCALE. Either omission
                reintroduces the clipping. */}
            {eyebrow ? (
              <Text
                preset={eyebrowPreset}
                color="textTertiary"
                style={styles.eyebrow}
                numberOfLines={1}
                maxFontSizeMultiplier={MAX_FONT_SCALE}
              >
                {eyebrow}
              </Text>
            ) : null}
            <Text
              preset="h1"
              color="text"
              numberOfLines={1}
              maxFontSizeMultiplier={MAX_FONT_SCALE}
            >
              {title}
            </Text>
            {subtitle ? (
              <Text
                preset="body"
                color="textSecondary"
                style={styles.expandedSubtitle}
                numberOfLines={1}
                maxFontSizeMultiplier={MAX_FONT_SCALE}
              >
                {subtitle}
              </Text>
            ) : null}
          </Animated.View>
          <Animated.View
            style={[styles.block, collapsedStyle]}
            pointerEvents="none"
            testID="collapsing-header-collapsed"
          >
            <Text
              preset="h3"
              color="text"
              numberOfLines={1}
              maxFontSizeMultiplier={MAX_FONT_SCALE}
            >
              {title}
            </Text>
            {subtitle ? (
              <Text
                preset="caption"
                color="textSecondary"
                style={styles.collapsedSubtitle}
                numberOfLines={1}
                maxFontSizeMultiplier={MAX_FONT_SCALE}
              >
                {subtitle}
              </Text>
            ) : null}
          </Animated.View>
        </View>
        {rightSlot ? <View style={styles.right}>{rightSlot}</View> : null}
      </View>
      <Animated.View style={[styles.hairline, collapsedStyle]} pointerEvents="none">
        <Hairline />
      </Animated.View>
    </Animated.View>
  );
}

const styles = StyleSheet.create({
  container: {
    backgroundColor: theme.colors.background,
    overflow: "hidden",
  },
  row: {
    flex: 1,
    flexDirection: "row",
    // NOT "center": `left`'s only children are `position: "absolute"` (the
    // cross-faded expanded/collapsed blocks), so it has no intrinsic content
    // height of its own. `alignItems: "center"` would size it to that
    // (zero) intrinsic height instead of stretching it to the row's actual
    // height — and the blocks' `top: 0; bottom: 0` anchors then resolve
    // against a zero-height box, rendering the title with zero height
    // (invisible, confirmed on-device: a blank header with correct spacing
    // but no visible text). "stretch" (the flex default, spelled out here
    // so the reason isn't silently reintroduced) gives `left` a real height
    // to anchor against.
    alignItems: "stretch",
    // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH and
    // PageHeader so the eyebrow/title share one left edge with the rows
    // beneath, per the Task 1 layout invariant.
    paddingHorizontal: theme.spacing.xl,
    gap: theme.spacing.md,
  },
  left: { flex: 1, position: "relative" },
  block: {
    position: "absolute",
    top: 0,
    bottom: 0,
    left: 0,
    right: 0,
    justifyContent: "center",
  },
  eyebrow: { marginBottom: 4 },
  expandedSubtitle: { marginTop: 4 },
  collapsedSubtitle: { marginTop: 2 },
  right: {
    minWidth: theme.touchTarget,
    minHeight: theme.touchTarget,
    alignItems: "center",
    justifyContent: "center",
  },
  hairline: {
    position: "absolute",
    left: 0,
    right: 0,
    bottom: 0,
  },
});
