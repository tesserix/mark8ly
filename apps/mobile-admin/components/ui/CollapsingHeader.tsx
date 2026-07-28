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
 * Deliberately an ADDITIVE opt-in, not a changed default. Not because of call
 * volume — `CollapsingHeader` has ONE caller today (the Dashboard) and gains a
 * second with Orders — but because the uppercase small-caps eyebrow is the
 * primitive's designed identity, and "the Dashboard's dateline wants sentence
 * case" is a local need. A caller that wants the other typography asks for it.
 * (The ~15-call-site ripple this comment used to cite happened to `Eyebrow`,
 * a different primitive, earlier in this increment.)
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

/**
 * Line allowance for the EXPANDED title. Two, not one.
 *
 * A merchant's shop name is the one string on this screen we don't control,
 * and at h1 (30pt serif) roughly 20 characters fit on an iPhone line — so a
 * one-line cap rendered `Northside Coffee Roasters` as `Northside Coffee Ro…`
 * at the DEFAULT text size, and truncated even a short name like
 * `Bondi Beach Co.` once Dynamic Type was raised. Losing content at 200%
 * resize is what WCAG 2.1 SC 1.4.4 forbids, and WCAG 2.1 AA is a project
 * baseline. Two lines fit the box (see EXPANDED_HEIGHT); a third would not,
 * so the ellipsis is pushed out to names no phone could show anyway.
 *
 * The COLLAPSED layer stays at one line deliberately: it is a 56pt bar the
 * merchant scrolls past, not the place they read their own shop name.
 */
export const EXPANDED_TITLE_LINES = 2;

/**
 * `18` (caption, the taller of the two eyebrow presets) `+ 4` margin
 * `+ 36 × 2` (two h1 lines) `= 94`, plus 2pt of slack.
 */
const EXPANDED_HEIGHT = 96;
/**
 * The subtitle's own box on top of that: `94 + 4` margin `+ 24` (body)
 * `= 122`, plus the same 2pt slack. The expanded base MUST grow for it — 122
 * of content inside a 96pt `overflow: "hidden"` container is the clip this
 * primitive exists to have already solved. No caller passes `subtitle` today;
 * the height is here so the first one that does isn't the one who finds out.
 */
const EXPANDED_HEIGHT_WITH_SUBTITLE = 124;
/** `26` (h3) `+ 2` margin `+ 18` (caption) `= 46`, comfortably inside 56. */
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
 * the multiplier, so a two-line h1 (72pt of line boxes) needs 137pt at 1.9×
 * and the eyebrow+title stack needs ~179pt — far past `EXPANDED_HEIGHT` 96. A
 * merchant who bumped their text size lost the ascenders of their own shop
 * name.
 *
 * Scaling the CONTAINER by the same clamped multiplier the text is capped at
 * makes non-clipping structural rather than empirical: content height is
 * `C × s` and the box is `H × s` for the same `s`, and `H > C` holds at s = 1
 * for BOTH subtitle cases (expanded 18 + 4 + 72 = 94 < 96; with a subtitle
 * 94 + 4 + 24 = 122 < 124; collapsed 26 + 2 + 18 = 46 < 56). Multiplying both
 * sides by the same positive `s` preserves the inequality for every scale.
 *
 * `hasSubtitle` is a parameter rather than a constant because the subtitle's
 * box is the one part of the content height a caller can turn on — folding it
 * into a single fixed height would either clip the callers that pass one or
 * leave 28pt of dead air above every caller that doesn't.
 *
 * Exported so the arithmetic is testable without mocking the RN Dimensions
 * module.
 */
export function headerHeightsFor(
  fontScale: number,
  hasSubtitle = false,
): {
  expanded: number;
  collapsed: number;
} {
  const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
  const expandedBase = hasSubtitle ? EXPANDED_HEIGHT_WITH_SUBTITLE : EXPANDED_HEIGHT;
  return {
    expanded: expandedBase * scale,
    collapsed: COLLAPSED_HEIGHT * scale,
  };
}

/**
 * Scroll-driven serif header: a tall editorial block (eyebrow + h1 + subtitle)
 * that crossfades into a compact bar (h3 + caption + hairline) as the owning
 * scroll view moves past `COLLAPSE_DISTANCE`.
 *
 * Dynamic Type safe: every line is capped at `MAX_FONT_SCALE`, each element
 * gets a KNOWN line allowance (the expanded title two, everything else one —
 * see `EXPANDED_TITLE_LINES`), and the container's own height scales by the
 * same clamped multiplier (see `headerHeightsFor`) so nothing clips against
 * `overflow: "hidden"`.
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
  // `Boolean(subtitle)`, not `subtitle !== undefined` — the render below
  // treats an empty string as absent too, so the height must agree with it.
  const heights = headerHeightsFor(fontScale, Boolean(subtitle));

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
                just the title: the height math above assumes a KNOWN number
                of line boxes per element, capped at MAX_FONT_SCALE. Dropping
                either reintroduces the clipping — and lowering the title's
                allowance to 1 reintroduces the truncated shop name. */}
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
              numberOfLines={EXPANDED_TITLE_LINES}
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
            {/* One line here, unlike the expanded layer: this is the compact
                bar the merchant scrolls past, and a second line would double
                the height of a state whose whole job is to get out of the
                way. See EXPANDED_TITLE_LINES. */}
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
