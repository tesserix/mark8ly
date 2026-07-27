import type { ReactNode } from "react";
import { StyleSheet, View } from "react-native";
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

export interface CollapsingHeaderProps {
  /** Small caps label above the title, shown only in the expanded state. */
  eyebrow?: string;
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
 * Scroll-driven serif header: a tall editorial block (eyebrow + h1 + subtitle)
 * that crossfades into a compact bar (h3 + caption + hairline) as the owning
 * scroll view moves past `COLLAPSE_DISTANCE`.
 *
 * Both layers are always mounted and cross-faded via animated `opacity` —
 * driven entirely by `useDerivedValue`/`useAnimatedStyle` off the caller's
 * `scrollY`, never by a React re-render on scroll. Surface is solid Paper
 * with a hairline; never a blur — that is a design-system rule, not a
 * preference, so don't reach for `expo-blur` here later.
 */
export function CollapsingHeader({
  eyebrow,
  title,
  subtitle,
  rightSlot,
  scrollY,
}: CollapsingHeaderProps) {
  const reduceMotion = useReducedMotion();

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

  const containerStyle = useAnimatedStyle(() => ({
    height: interpolate(
      progress.value,
      [0, 1],
      [EXPANDED_HEIGHT, COLLAPSED_HEIGHT],
      Extrapolation.CLAMP,
    ),
  }));

  const expandedStyle = useAnimatedStyle(() => ({ opacity: 1 - progress.value }));
  const collapsedStyle = useAnimatedStyle(() => ({ opacity: progress.value }));

  return (
    <Animated.View style={[styles.container, containerStyle]}>
      <View style={styles.row}>
        <View style={styles.left}>
          <Animated.View
            style={[styles.block, expandedStyle]}
            pointerEvents="none"
            testID="collapsing-header-expanded"
          >
            {eyebrow ? (
              <Text preset="eyebrow" color="textTertiary" style={styles.eyebrow}>
                {eyebrow}
              </Text>
            ) : null}
            <Text preset="h1" color="text">
              {title}
            </Text>
            {subtitle ? (
              <Text preset="body" color="textSecondary" style={styles.expandedSubtitle}>
                {subtitle}
              </Text>
            ) : null}
          </Animated.View>
          <Animated.View
            style={[styles.block, collapsedStyle]}
            pointerEvents="none"
            testID="collapsing-header-collapsed"
          >
            <Text preset="h3" color="text">
              {title}
            </Text>
            {subtitle ? (
              <Text preset="caption" color="textSecondary" style={styles.collapsedSubtitle}>
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
