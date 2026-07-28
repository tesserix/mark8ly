import { useCallback, useState } from "react";
import {
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  useWindowDimensions,
  View,
  type ViewStyle,
} from "react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

export interface FilterChip<T extends string> {
  key: T;
  label: string;
}

export interface FilterChipsProps<T extends string> {
  chips: FilterChip<T>[];
  value: T;
  /** Fired ONLY when the selection actually changes — see `handlePress`. */
  onChange: (key: T) => void;
  /**
   * Applied to the INNER row (the `ScrollView`'s `contentContainerStyle`),
   * not to the scroll box itself — so padding here insets the chips while
   * the strip still scrolls edge to edge.
   *
   * Named for where it lands. It used to be `style`, which read as "the
   * container's style" and silently wasn't: a caller passing a real
   * container style (a background, a margin) would have got it painted on
   * the inner row inside a fixed-height box instead, with nothing failing.
   * The scroll box's own geometry is not caller-tunable on purpose — it is
   * `flexGrow/flexShrink: 0` plus a computed height, and letting a caller
   * override either re-opens the ~110pt dead-space bug (see below).
   */
  contentContainerStyle?: ViewStyle;
}

/** Visible pill height at the default text size. */
const PILL_HEIGHT = 40;

/**
 * Cap on the Dynamic Type multiplier applied to the chip label. 2.0 honours
 * WCAG 2.1 SC 1.4.4 (text resizable to 200% without loss of content) while
 * stopping iOS's accessibility sizes — which reach 3.1× — from turning a
 * filter row into a third of the screen. Same value and same reasoning as
 * `CollapsingHeader.MAX_FONT_SCALE`.
 */
export const MAX_FONT_SCALE = 2;

/**
 * Pill and touch-target heights for a given device font scale.
 *
 * The pill has a FIXED height (a pill whose height is derived from its
 * content isn't a pill — its radius and its neighbours' baselines both
 * drift), which makes it exactly the shape that clips when the label grows.
 * So the BOX scales by the same clamped multiplier the LABEL is capped at:
 * content height is `lineHeight × s`, the box is `PILL_HEIGHT × s` for the
 * same `s`, and `PILL_HEIGHT (40) > label lineHeight (20)` holds at s = 1 —
 * multiplying both sides by the same positive `s` preserves it at every
 * scale. Structural, not empirical. Identical argument to
 * `CollapsingHeader.headerHeightsFor`.
 *
 * `target` is the surrounding press box. It never drops below
 * `theme.touchTarget` (44) — a REAL minHeight, not `hitSlop`, which extends
 * the responder region without extending anything a user can see or an
 * accessibility audit can measure.
 *
 * Exported so the arithmetic is testable without mocking RN's Dimensions.
 */
export function chipHeightsFor(fontScale: number): { pill: number; target: number } {
  const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
  const pill = PILL_HEIGHT * scale;
  return { pill, target: Math.max(theme.touchTarget, pill + theme.spacing.xs) };
}

/**
 * Horizontally scrolling row of pill filter chips.
 *
 * The Orders replacement for `SegmentedControl`'s underlined tabs.
 * `SegmentedControl` is deliberately left alone — it still has eight other
 * call sites, and re-skinning a shared primitive to suit one screen is the
 * ~15-call-site ripple this increment already paid for once. This is an
 * additive sibling, not a redesign of that one.
 *
 * Horizontal scroll, not wrapping: a wrapping row changes height when a
 * filter set grows, which moves the list beneath it. Scrolling keeps the row
 * one pill tall for any number of filters.
 *
 * The active chip is a solid INK fill with paper text — never moss. On
 * Orders the single accent is spent on the Approve swipe action and nothing
 * else, and ink-as-selected is the language the Ink dock already established
 * app-wide.
 */
export function FilterChips<T extends string>({
  chips,
  value,
  onChange,
  contentContainerStyle,
}: FilterChipsProps<T>) {
  // `useWindowDimensions` (not `PixelRatio.getFontScale()`) so the row
  // re-renders when the merchant changes their text size with the app
  // foregrounded — a static read would leave the pills at the old height.
  const { fontScale } = useWindowDimensions();
  const heights = chipHeightsFor(fontScale);

  const handlePress = useCallback(
    (key: T) => {
      // Re-tapping the active chip is not a change: no haptic (a buzz that
      // reports nothing is noise) and no `onChange` (which would push a
      // redundant state update and re-render the list for nothing).
      if (key === value) return;
      void adminHaptics.selectionChanged();
      onChange(key);
    },
    [value, onChange],
  );

  return (
    <ScrollView
      horizontal
      showsHorizontalScrollIndicator={false}
      // `flexGrow/flexShrink: 0` + an explicit height: a ScrollView placed in
      // a flex COLUMN otherwise stretches to fill the space left over by its
      // siblings, and `styles.row`'s `alignItems: "center"` then centres the
      // chips inside that tall box — which on device rendered ~110pt of dead
      // paper between the header and the pills, and as much again beneath
      // them. Found on device; the unit tests render the row in isolation
      // where nothing stretches, so they were all green.
      style={[styles.scroll, { height: heights.target }]}
      contentContainerStyle={[styles.row, contentContainerStyle]}
      // The row is a control strip, not content: let a horizontal drag that
      // starts on a chip scroll the strip rather than arm a press.
      keyboardShouldPersistTaps="handled"
    >
      {chips.map((chip) => (
        <Chip
          key={chip.key}
          chip={chip}
          active={chip.key === value}
          heights={heights}
          onPress={() => handlePress(chip.key)}
        />
      ))}
    </ScrollView>
  );
}

interface ChipProps<T extends string> {
  chip: FilterChip<T>;
  active: boolean;
  heights: { pill: number; target: number };
  onPress: () => void;
}

// Extracted so press state can live in `useState`: this renders once per
// chip inside `chips.map()`, and hooks can't be called from a `.map()`
// callback — each chip needs its own component instance.
function Chip<T extends string>({ chip, active, heights, onPress }: ChipProps<T>) {
  // Explicit press tracking merged into a plain style ARRAY. A FUNCTION
  // `style` prop on `Pressable` is NOT resolved by NativeWind 4.2.5's JSX
  // interop and the styles are silently dropped at runtime — see
  // PressableRow's doc comment for the 24-call-site incident. Do not
  // "simplify" this back to `style={({ pressed }) => …}`.
  const [pressed, setPressed] = useState(false);
  const handlePressIn = useCallback(() => setPressed(true), []);
  const handlePressOut = useCallback(() => setPressed(false), []);

  return (
    <Pressable
      onPress={onPress}
      onPressIn={handlePressIn}
      onPressOut={handlePressOut}
      accessibilityRole="tab"
      accessibilityState={{ selected: active }}
      accessibilityLabel={`Filter: ${chip.label}`}
      android_ripple={active ? theme.press.rippleOnDark : theme.press.rippleInk}
      testID={`filter-chip-${chip.key}-target`}
      style={[styles.target, { minHeight: heights.target }]}
    >
      <View
        testID={`filter-chip-${chip.key}`}
        style={[
          styles.pill,
          { height: heights.pill },
          active ? styles.pillActive : styles.pillInactive,
          pressed && Platform.OS === "ios"
            ? { opacity: active ? theme.press.opacitySolidFill : theme.press.opacityStandard }
            : null,
        ]}
      >
        <Text
          preset="label"
          color={active ? "inverse" : "textSecondary"}
          numberOfLines={1}
          maxFontSizeMultiplier={MAX_FONT_SCALE}
          style={active ? styles.labelActive : undefined}
        >
          {chip.label}
        </Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  // Hug the content vertically — see the `style` prop's comment above.
  scroll: { flexGrow: 0, flexShrink: 0 },
  row: {
    flexDirection: "row",
    alignItems: "center",
    // Screen gutter: theme.spacing.xl (20) — the SAME left edge the
    // CollapsingHeader's eyebrow/title and every row's paddingH sit on. This
    // is `contentContainerStyle`, not `style`, so the first chip starts on
    // the gutter while the row itself still scrolls edge to edge.
    paddingHorizontal: theme.spacing.xl,
    gap: theme.spacing.sm,
  },
  target: {
    justifyContent: "center",
    // A real box, never hitSlop.
    minWidth: theme.touchTarget,
    alignItems: "center",
  },
  pill: {
    justifyContent: "center",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    borderRadius: theme.radii.pill,
    borderWidth: theme.hairline,
  },
  pillActive: {
    backgroundColor: theme.colors.text,
    borderColor: theme.colors.text,
  },
  pillInactive: {
    backgroundColor: theme.colors.background,
    borderColor: theme.colors.border,
  },
  // Weight, not colour, carries the second half of the active signal — the
  // fill already carries the first.
  labelActive: { fontWeight: "600" },
});
