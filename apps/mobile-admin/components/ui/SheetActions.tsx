import { useState, type ReactNode } from "react";
import {
  ActivityIndicator,
  Platform,
  Pressable,
  StyleSheet,
  View,
  useWindowDimensions,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import { MAX_FONT_SCALE, Text } from "./Text";
import { theme } from "@/lib/theme";

/**
 * The submit/dismiss button pair every reason sheet ends with.
 *
 * All four sheets — `RefundSheet`, `CancelReasonSheet`, `EmailLabelSheet`,
 * `BlockReasonSheet` — hand-rolled the SAME pair four times: a
 * `flexDirection: "row"` container holding two `flex: 1`, `height: 48`
 * `Pressable`s. There is no pre-existing shared button in this app to extend,
 * so the genuinely shared level is a NEW primitive the four adopt — which is
 * additive by construction: nothing else in the app changes behaviour.
 *
 * The defect it exists to fix: at accessibility text sizes the labels clipped
 * inside their own buttons ("Issue Refun|d", "Cancel Orde", "Block Custo|").
 * Two independent causes, both present:
 *
 *  1. `height: 48` is a FIXED box. `bodyEmphasis` is 16pt at fontScale 1 and
 *     32pt at the app's 2× cap, so a single line box is ~45pt and a wrapped
 *     two-line label ~90pt — both inside a 48pt box that cannot grow. This is
 *     the eighth instance of fixed-box-holding-scalable-text in this app, and
 *     the same family as the sheet-reachability fix in `bee7ee8e` one level
 *     further in: that commit made the buttons reachable, not their labels
 *     readable.
 *  2. `flexDirection: "row"` gives each button roughly half the sheet width.
 *     "Issue refund" at 32pt needs ~200pt on one line; half of a 390pt sheet
 *     minus padding and gap is ~163pt. Even with a growable box the label has
 *     to break mid-word to fit, which is what produced the ragged truncation.
 *
 * So the fix is two parts and BOTH are needed: the button's box grows
 * (`minHeight` + real vertical padding, never `height`), and the pair stacks
 * to a column once the font scale makes side-by-side impossible.
 */

/** The button's box height at fontScale 1 — unchanged from the four sheets. */
export const SHEET_BUTTON_MIN_HEIGHT = 48;

/**
 * Above this device font scale the pair stacks instead of sitting side by
 * side.
 *
 * 1.3 rather than "when it overflows" because RN gives no width-aware
 * breakpoint without measuring, and measuring here would resize the sheet
 * AFTER it has opened — the exact jitter `ActionSheet`'s constant item count
 * exists to avoid. 1.3 is where `bodyEmphasis` reaches ~21pt: "Issue refund"
 * needs ~130pt on one line and half a narrow (320pt) sheet's content width is
 * ~140pt, so it is the last scale at which the longest real label still fits
 * one line on the narrowest device this app supports.
 */
export const SHEET_ACTIONS_STACK_SCALE = 1.3;

function clampFontScale(fontScale: number): number {
  return Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
}

export interface SheetActionsProps {
  children: ReactNode;
  style?: StyleProp<ViewStyle>;
  testID?: string;
}

/**
 * The container for a sheet's action pair. Row at normal text sizes, column
 * once `SHEET_ACTIONS_STACK_SCALE` is passed.
 *
 * Reading order is preserved when it stacks (dismiss above submit) rather
 * than reversed for thumb reach: these sheets are opened deliberately and the
 * submit is the destructive one in three of the four, so putting it under the
 * thumb by default is the wrong trade.
 */
export function SheetActions({ children, style, testID }: SheetActionsProps) {
  const { fontScale } = useWindowDimensions();
  const stacked = fontScale > SHEET_ACTIONS_STACK_SCALE;
  return (
    <View
      testID={testID}
      style={[styles.actions, stacked ? styles.actionsStacked : styles.actionsRow, style]}
    >
      {children}
    </View>
  );
}

export type SheetActionTone = "neutral" | "ink" | "danger" | "accent";

export interface SheetActionButtonProps {
  /** The visible label. Wraps rather than truncating — never clamp it. */
  label: string;
  onPress: () => void;
  /**
   * "neutral" is the outlined dismiss; "ink", "danger" and "accent" are the
   * solid submit fills. One per fill the four sheets already rendered — no
   * sheet's appearance changes.
   */
  tone?: SheetActionTone;
  disabled?: boolean;
  /** Replaces the label with a spinner. Implies `disabled`. */
  busy?: boolean;
  /** Defaults to `label`. */
  accessibilityLabel?: string;
  testID?: string;
}

export function SheetActionButton({
  label,
  onPress,
  tone = "neutral",
  disabled = false,
  busy = false,
  accessibilityLabel,
  testID,
}: SheetActionButtonProps) {
  const { fontScale } = useWindowDimensions();
  // NativeWind's JSX interop doesn't resolve a FUNCTION `style` prop the way
  // it resolves a plain array, so press state is tracked explicitly — the
  // same idiom `IconButton` and the screens use.
  const [pressed, setPressed] = useState(false);
  const isDisabled = disabled || busy;

  const toneStyle =
    tone === "ink"
      ? styles.ink
      : tone === "danger"
        ? styles.danger
        : tone === "accent"
          ? styles.accent
          : styles.neutral;
  const ripple = tone === "neutral" ? theme.press.rippleInk : theme.press.rippleOnDark;
  const pressedOpacity =
    tone === "neutral" ? theme.press.opacityStandard : theme.press.opacitySolidFill;

  return (
    <Pressable
      onPress={onPress}
      onPressIn={() => setPressed(true)}
      onPressOut={() => setPressed(false)}
      disabled={isDisabled}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel ?? label}
      accessibilityState={{ disabled: isDisabled }}
      android_ripple={isDisabled ? undefined : ripple}
      testID={testID}
      style={[
        styles.button,
        // Derived from the same constant `SheetActions` uses rather than
        // passed down, so the two cannot disagree. `flex: 1` is a HALF-WIDTH
        // rule and belongs only to the row: in a column it resolves against
        // an indefinite main size and is a well-known Yoga foot-gun.
        fontScale > SHEET_ACTIONS_STACK_SCALE ? styles.buttonStacked : styles.buttonRow,
        toneStyle,
        // `minHeight`, NOT `height`: the box has to be able to grow past the
        // 48pt floor when the label wraps. Scaling the floor as well keeps
        // the button a comfortable target at large text instead of a
        // padding-tight strip.
        { minHeight: SHEET_BUTTON_MIN_HEIGHT * clampFontScale(fontScale) },
        isDisabled && styles.disabled,
        pressed && !isDisabled && Platform.OS === "ios" ? { opacity: pressedOpacity } : null,
      ]}
    >
      {busy ? (
        <ActivityIndicator
          size="small"
          color={tone === "neutral" ? theme.colors.text : theme.colors.inverse}
        />
      ) : (
        // No `numberOfLines`: a clamped label is the bug, not the fix. The
        // box above grows to whatever this wraps to.
        <Text preset="bodyEmphasis" color={tone === "neutral" ? "text" : "inverse"} align="center">
          {label}
        </Text>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  actions: { gap: theme.spacing.md, marginTop: theme.spacing.sm },
  actionsRow: { flexDirection: "row" },
  actionsStacked: { flexDirection: "column" },
  button: {
    // Real padding so a wrapped label is never flush against the border.
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.md,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  buttonRow: { flex: 1 },
  buttonStacked: { alignSelf: "stretch" },
  neutral: { borderWidth: 1, borderColor: theme.colors.border },
  ink: { backgroundColor: theme.colors.text },
  danger: { backgroundColor: theme.colors.danger },
  accent: { backgroundColor: theme.colors.accent },
  disabled: { opacity: 0.4 },
});
