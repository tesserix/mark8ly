import { useState, type ReactNode } from "react";
import { StyleSheet, View, useWindowDimensions } from "react-native";
import Animated, {
  Extrapolation,
  interpolate,
  useAnimatedStyle,
  useDerivedValue,
  useReducedMotion,
  type SharedValue,
} from "react-native-reanimated";
import { ChevronLeft } from "lucide-react-native";
import { Text, MAX_FONT_SCALE as TEXT_MAX_FONT_SCALE } from "./Text";
import { Hairline } from "./Hairline";
import { IconButton } from "./IconButton";
import { theme } from "@/lib/theme";

/**
 * Typography for the eyebrow line. `"eyebrow"` (the DEFAULT, and what every
 * existing call site gets) is the uppercase, letterspaced small-caps label.
 * `"caption"` is sentence case at the same tertiary ink — for datelines and
 * other running prose where SHOUTING IN CAPS is wrong ("Monday, 27 July",
 * not "MONDAY, 27 JULY").
 *
 * Deliberately an ADDITIVE opt-in, not a changed default — and NOT because of
 * call volume. This primitive has exactly TWO callers (the Dashboard and
 * Orders); the ~15-call-site ripple that argument was borrowed from belongs
 * to `Eyebrow`, a different primitive changed earlier in this increment. The
 * real reason is that the uppercase small-caps eyebrow is this component's
 * designed identity, and "the Dashboard's dateline wants sentence case" is a
 * local need. A caller that wants the other typography asks for it.
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
  /**
   * Trailing content, vertically centred against both header states.
   *
   * If it contains TEXT, cap it at `MAX_FONT_SCALE` like every line this
   * primitive draws itself — the container's height is computed from that
   * multiplier (see `headerHeightsFor`), so an uncapped slot is measured
   * against a box that was never sized for it.
   */
  rightSlot?: ReactNode;
  /**
   * Renders a back chevron at the leading edge in BOTH states, vertically
   * centred like `rightSlot`.
   *
   * ADDITIVE, and that is the point: six of the eight list screens increment
   * 3 rolls this primitive across are NESTED routes that used `BackHeader`,
   * and this component had no back affordance at all — so "every screen gets
   * a collapsing header" was impossible without it. Omitted on the two
   * tab-root screens that already use this primitive (Dashboard, Orders), so
   * their rendering stays bit-identical.
   *
   * It is a FIXED-SIZE 44pt control containing no text, so it does NOT
   * participate in `headerHeightsFor`: the collapsed bar is 56pt at 1× and
   * 112pt at the cap, both of which contain 44 comfortably. What it does cost
   * is HORIZONTAL — the title block loses `44 + theme.spacing.sm` — and both
   * title layers already wrap to two lines and then shrink to a 13pt floor
   * (see `TITLE_MIN_FONT_SIZE`), so a long title gets smaller rather than
   * clipped.
   */
  onBack?: () => void;
  /**
   * Escape hatch for a NON-back leading control (a Close on a modal-ish
   * route, say). Ignored when `onBack` is set — `onBack` is the common case
   * and the one the rollout uses, so a screen that passes both gets the back
   * chevron rather than two stacked leading controls.
   */
  leadingSlot?: ReactNode;
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
 * The COLLAPSED layer gets the same allowance, for the same reason — see
 * COLLAPSED_TITLE_LINES.
 */
export const EXPANDED_TITLE_LINES = 2;

/**
 * Line allowance for the COLLAPSED title. Also two, and this one is measured.
 *
 * This layer used to be pinned at one line, on the argument that the compact
 * bar is "a 56pt bar the merchant scrolls past, not the place they read their
 * own shop name". On device that argument bought a truncated shop name in
 * exactly the place the app states the merchant's identity:
 *
 *   - `The Bondi Store` → `The Bondi Sto…` at `accessibility-large`
 *   - `Northside Coffee Roasters & Bakehouse` → `Northside Coffee Roasters & B…`
 *     at the DEFAULT text size, and `Northside Co…` at `accessibility-large`
 *
 * Note the second row: the collapsed bar truncates a long enough name at
 * EVERY text size, so this is not a Dynamic Type bug with a Dynamic Type fix.
 * The cause is horizontal, not vertical — `headerHeightsFor` was already
 * growing the bar correctly (a measured ~112pt at the capped 2×, glyphs with
 * full ascender room), and the trailing slot was not squeezing anything: the
 * Dashboard's is a fixed 40pt monogram. The title simply had one line and a
 * scaled font, and 200%-resized serif does not fit 15 characters in the ~306pt
 * the row leaves it.
 *
 * "A `numberOfLines` that ellipsises a merchant's shop name is a different
 * bug, not a fix" was the ruling when the EXPANDED title was given two lines.
 * It applies here unchanged; WCAG 2.1 SC 1.4.4 (no loss of content at 200%)
 * does not exempt a header because it is the short one.
 *
 * Two lines is where the LINE allowance stops — a third costs
 * `26 × 2 = 52pt` on top of the 56pt base, which at any scale puts the
 * collapsed bar past the 96pt expanded one and inverts the point of
 * collapsing. Everything past two lines is bought horizontally instead; see
 * `TITLE_MIN_FONT_SIZE`.
 */
export const COLLAPSED_TITLE_LINES = 2;

/**
 * Floor for the title's shrink-to-fit, in unscaled points.
 *
 * Two lines was not enough. Measured on an iPhone 17 Pro (402pt wide, so the
 * title column is `402 − 20 − 20 − 16 gap − 44 slot = 302pt`) against the
 * name this file's own comments use, `Northside Coffee Roasters & Bakehouse`
 * (37 characters):
 *
 *   | layer          | text size           | rendered                               |
 *   |----------------|---------------------|----------------------------------------|
 *   | expanded (h1)  | large (1.0×)        | `Northside Coffee Roasters & Bakehou…`  |
 *   | expanded (h1)  | accessibility-large | `Northside Coffee R…`                   |
 *   | collapsed (h3) | large (1.0×)        | full name, two lines                    |
 *   | collapsed (h3) | accessibility-large | `Northside Coffee Roaste…`              |
 *
 * Three things that reading kills:
 *
 *  1. It is a WIDTH problem, not a Dynamic Type one — the EXPANDED title
 *     truncates at the DEFAULT text size, by about 14pt of line. Any fix
 *     keyed on `fontScale` would have missed that row entirely.
 *  2. It is not the collapsed layer's problem — the expanded layer, the one
 *     the merchant reads at rest, loses MORE of the name (18 of 37 vs 22 of
 *     37) at `accessibility-large`. Fixing only the compact bar would have
 *     left the collapsed state showing more of the shop name than the
 *     expanded state, which is worse than the bug.
 *  3. Line allowance cannot buy the rest: the collapsed bar is capped at two
 *     lines by `COLLAPSED_HEIGHT` vs `EXPANDED_HEIGHT` (above).
 *
 * So the remaining room is horizontal, and shrink-to-fit is the lever —
 * `MetricsCard`'s hero numeral pattern, which this file previously ruled out
 * on the grounds that "money has to read as a single token, so it can only
 * get smaller, while a shop name is prose and can legitimately wrap". That
 * argument was about choosing WRAPPING OVER SHRINKING; it says nothing about
 * what to do once wrapping is spent, which is where both layers now are.
 * `adjustsFontSizeToFit` picks the LARGEST size that fits, so a name that
 * already fits — every name at `The Bondi Store`'s length, at every text
 * size — is not shrunk by a single point.
 *
 * The floor is `caption`, the smallest type the design system ships as
 * running copy (13pt; it is what this header's own subtitle draws in). Not a
 * bare fraction: `MetricsCard` can express its floor as a constant 0.5
 * because the hero is only ever squeezed at raised text sizes, and half of
 * the 2×-scaled 88pt is exactly its default 44pt. This title is squeezed at
 * the DEFAULT size too, where the same 0.5 would authorise a 10pt collapsed
 * shop name — below iOS's 11pt legibility guidance and squarely inside the
 * "never shrink below an accessible size" line. Pinning the floor to a real
 * design-system size instead means the shop name can never render smaller
 * than the caption sitting next to it, at any text size: `minimumFontScale`
 * is a fraction of the ALREADY-Dynamic-Type-scaled size, so `13 / fontSize`
 * holds the floor at `13 × fontScale` for every scale without plumbing
 * `fontScale` into it.
 *
 * Cross-platform despite the typings: `adjustsFontSizeToFit` is declared in
 * RN's `TextPropsIOS` for historical reasons, but Android implements it too
 * (`ReactTextViewManager.kt` `setAdjustFontSizeToFit`, and
 * `TextLayoutManager.kt`'s `adjustFontSizeToFit` measurement path).
 */
export const TITLE_MIN_FONT_SIZE = theme.text.caption.fontSize;

/**
 * `minimumFontScale` for a title drawn at `styledFontSize`, i.e. the fraction
 * of its own size at which it stops shrinking. Capped at 1 so a preset that
 * is ALREADY at or below the floor is never told it may grow.
 */
export function titleMinimumFontScale(styledFontSize: number): number {
  return Math.min(1, TITLE_MIN_FONT_SIZE / styledFontSize);
}

/** h1 → 13/30. */
export const EXPANDED_TITLE_MIN_SCALE = titleMinimumFontScale(theme.text.h1.fontSize);
/** h3 → 13/20. */
export const COLLAPSED_TITLE_MIN_SCALE = titleMinimumFontScale(theme.text.h3.fontSize);

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
 * One h3 line box — what a SECOND collapsed title line costs the bar.
 *
 * Read off the theme rather than written as a literal `26`: this number and
 * the line the collapsed title actually draws must be the same number, and a
 * hand-copied constant is how they drift. With it, the two-line collapsed
 * content is `26 × 2 + 2 + 18 = 72` inside `56 + 26 = 82` — the same ~10pt of
 * slack the one-line case has, preserved rather than coincidental.
 */
const COLLAPSED_TITLE_LINE = theme.text.h3.lineHeight;

/**
 * Cap on the iOS Dynamic Type / Android font-scale multiplier applied to
 * every line in this header, and the multiplier `headerHeightsFor` scales the
 * CONTAINER by. The two must be the same number or the height math below is
 * measuring a different box than the one the text draws into — so this is now
 * an alias of the app-wide default in `Text.tsx` rather than a second,
 * independently-maintained `2`. Re-exported because the height arithmetic is
 * part of this primitive's public contract and its tests assert against it.
 */
export const MAX_FONT_SCALE = TEXT_MAX_FONT_SCALE;

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
 * `collapsedTitleLines` is a parameter for the same reason, and it is why the
 * collapsed title can now wrap at all: `COLLAPSED_TITLE_LINES` alone would
 * have let a two-line title draw 72pt of content into a 56pt `overflow:
 * "hidden"` box and clip silently — the exact failure this function exists to
 * make structural. It defaults to `1`, so every existing call keeps its
 * current answer and only a caller that has MEASURED two lines pays for the
 * second. Scaling it by `fontScale` rather than thresholding on it is
 * deliberate: the truncation reproduces at the DEFAULT text size for a long
 * enough name, so a scale-keyed rule would not have covered it.
 *
 * The titles' shrink-to-fit (see `TITLE_MIN_FONT_SIZE`) does NOT enter this
 * arithmetic, and that is a property worth stating rather than an oversight.
 * `adjustsFontSizeToFit` only ever picks a font size at or BELOW the styled
 * one, and a line box never grows when its font shrinks — so the content
 * height computed here stays an upper bound on the content actually drawn,
 * whatever the shrink lands on. The inequalities above are the ones that must
 * hold, and shrinking can only widen them. (Nor can it feed back into the
 * measured `collapsedTitleLines`: the shrink is bound by the title's WIDTH,
 * which this height does not touch.)
 *
 * Exported so the arithmetic is testable without mocking the RN Dimensions
 * module.
 */
export function headerHeightsFor(
  fontScale: number,
  hasSubtitle = false,
  collapsedTitleLines = 1,
): {
  expanded: number;
  collapsed: number;
} {
  const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
  const expandedBase = hasSubtitle ? EXPANDED_HEIGHT_WITH_SUBTITLE : EXPANDED_HEIGHT;
  // Clamped, not trusted: this is fed from a native layout callback, and a 0
  // (or a 3 from some future allowance bump) must not silently resize the box
  // to something the title's own `numberOfLines` will never draw into.
  const titleLines = Math.min(
    Math.max(Math.round(collapsedTitleLines), 1),
    COLLAPSED_TITLE_LINES,
  );
  return {
    expanded: expandedBase * scale,
    collapsed: (COLLAPSED_HEIGHT + COLLAPSED_TITLE_LINE * (titleLines - 1)) * scale,
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
  onBack,
  leadingSlot,
  scrollY,
}: CollapsingHeaderProps) {
  const reduceMotion = useReducedMotion();
  // `useWindowDimensions` (not `PixelRatio.getFontScale()`) because it
  // re-renders when the user changes their text size while the app is
  // foregrounded — the static read would leave the header at the old height.
  const { fontScale } = useWindowDimensions();
  /**
   * How many lines the COLLAPSED title actually occupies, MEASURED via
   * `onTextLayout` rather than predicted.
   *
   * Whether a shop name wraps depends on the name, the font scale, the device
   * width and what the trailing slot leaves behind — there is no expression of
   * `fontScale` that answers it, which is why the truncation reproduced at the
   * default text size as well as the accessibility ones. Asking the layout is
   * the only answer that is right for every name at every size.
   *
   * Seeded at the MAXIMUM allowance, not at 1, so the box is never briefly
   * smaller than its content: the first frame is sized for two lines and the
   * measurement only ever shrinks it, so a wrapping title cannot flash clipped
   * against `overflow: "hidden"`. Nothing is visible either way — the header
   * mounts expanded, and this height only applies once the user scrolls.
   *
   * This cannot oscillate: the collapsed height feeds the container's HEIGHT,
   * and the title's available WIDTH is independent of it, so a re-measure
   * after the height changes returns the same line count.
   */
  const [collapsedTitleLines, setCollapsedTitleLines] = useState(COLLAPSED_TITLE_LINES);
  // `Boolean(subtitle)`, not `subtitle !== undefined` — the render below
  // treats an empty string as absent too, so the height must agree with it.
  const heights = headerHeightsFor(fontScale, Boolean(subtitle), collapsedTitleLines);

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

  // `onBack` wins over `leadingSlot` — see the prop docs. Resolved once here
  // rather than branched in the JSX so there is exactly one leading node and
  // the precedence can't be re-derived differently in a later edit.
  const leading = onBack ? (
    <IconButton onPress={onBack} accessibilityLabel="Go back" tone="ink">
      <ChevronLeft size={22} color={theme.colors.text} strokeWidth={1.75} />
    </IconButton>
  ) : (
    leadingSlot
  );

  return (
    <Animated.View style={[styles.container, containerStyle]} testID="collapsing-header">
      <View style={styles.row}>
        {/* Outside the cross-faded blocks, so it is present and tappable in
            BOTH states rather than fading with either one. The blocks are
            `pointerEvents="none"`, so nothing here can be occluded by them. */}
        {leading ? (
          <View style={styles.leading} testID="collapsing-header-leading">
            {leading}
          </View>
        ) : null}
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
            {/* Wrap first, shrink second, ellipsise never — see
                TITLE_MIN_FONT_SIZE. This layer truncated a 37-character shop
                name at the DEFAULT text size, so the pair is not an
                accessibility-only concern and must not be reduced to one. */}
            <Text
              preset="h1"
              color="text"
              numberOfLines={EXPANDED_TITLE_LINES}
              maxFontSizeMultiplier={MAX_FONT_SCALE}
              adjustsFontSizeToFit
              minimumFontScale={EXPANDED_TITLE_MIN_SCALE}
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
            {/* Two lines here, as in the expanded layer, and the bar grows to
                hold the second ONLY when the title actually takes it — see
                COLLAPSED_TITLE_LINES. The compact bar is still compact for
                every name that fits; it stops being compact exactly when the
                alternative is truncating the merchant's shop name, and
                correctness wins that trade. Do NOT put this back to 1 without
                also shrinking the box in `headerHeightsFor`, and do not shrink
                the box without putting this back — they are one contract. */}
            <Text
              preset="h3"
              color="text"
              numberOfLines={COLLAPSED_TITLE_LINES}
              maxFontSizeMultiplier={MAX_FONT_SCALE}
              // The second half of the same contract as the expanded title:
              // two lines, then shrink to the caption floor, and only then
              // give up. `headerHeightsFor` is unaffected — shrinking can
              // only make the content shorter than the box already allows.
              adjustsFontSizeToFit
              minimumFontScale={COLLAPSED_TITLE_MIN_SCALE}
              onTextLayout={(event) => {
                // `lines` is OPTIONAL, not guaranteed. With
                // `adjustsFontSizeToFit` on, iOS fires `onTextLayout` for its
                // font-fitting passes with a payload that carries no `lines`
                // array at all — reading `.length` off it threw
                // "Cannot read property 'lines' of null" and took the whole
                // Dashboard down to a red screen the moment the header
                // collapsed. Found on device; every test was green, because
                // the tests hand-build the event.
                const measured = event.nativeEvent?.lines?.length;
                if (!measured) return;
                setCollapsedTitleLines((previous) =>
                  // Same-value guard: `onTextLayout` fires on every layout
                  // pass, and returning the identical number lets React bail
                  // out instead of re-rendering the header on each one.
                  previous === measured ? previous : measured,
                );
              }}
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
        {rightSlot ? (
          <View style={styles.right} testID="collapsing-header-right">
            {rightSlot}
          </View>
        ) : null}
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
    // Spacing lives on the SLOTS, not as a row `gap`, because the two sides
    // want different numbers: the trailing slot keeps the `md` it has always
    // had (so the two existing tab-root callers lay out identically), while
    // the leading control takes the tighter `sm` — every point it borrows
    // comes straight out of the merchant's shop name.
  },
  left: { flex: 1, position: "relative" },
  leading: {
    minWidth: theme.touchTarget,
    minHeight: theme.touchTarget,
    alignItems: "center",
    justifyContent: "center",
    marginRight: theme.spacing.sm,
    // NOT `flexShrink: 1` (which `right` needs): this slot is a fixed-size
    // glyph button with no text in it, so there is nothing for a raised text
    // size to grow — and shrinking it below 44 would break the touch target.
    flexShrink: 0,
  },
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
    // RN's flex default is `flexShrink: 0`, which is right for the Dashboard's
    // fixed 40pt monogram and WRONG for a `rightSlot` made of scalable text.
    // `left` is `flex: 1` — i.e. `flexBasis: 0` — so it has no basis to shrink
    // FROM: an unshrinkable right slot that grows past the row simply drives
    // left's resolved width to zero, and Orders' "Inbox" title vanished while
    // its "3 pending" caption kept every pixel it asked for. With a shrink
    // factor here the overflow is absorbed by the slot that caused it, down to
    // the 44pt touch-target floor above; a text slot then truncates on its own
    // `numberOfLines`, which is the caller's choice to make.
    flexShrink: 1,
    // The `md` that used to be the row's `gap` — see `styles.row`. Identical
    // geometry for every existing caller (the row has only these two children
    // when no leading control is present).
    marginLeft: theme.spacing.md,
  },
  hairline: {
    position: "absolute",
    left: 0,
    right: 0,
    bottom: 0,
  },
});
