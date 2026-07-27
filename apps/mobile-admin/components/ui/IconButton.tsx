import { useCallback, useState, type ReactNode } from "react";
import {
  Platform,
  Pressable,
  StyleSheet,
  type AccessibilityRole,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import { theme } from "@/lib/theme";

/**
 * The one icon-button press surface in the app.
 *
 * Replaces the copy-pasted `Pressable` + `hitSlop` + iOS-opacity + ripple
 * idiom that lived at ~19 call sites. The touch target is a REAL
 * `minWidth`/`minHeight` of `theme.touchTarget` on the pressable itself, not
 * `hitSlop` — several pre-migration sites used a `hitSlop` wide enough to
 * bleed onto adjacent content (an invisible overlay that can steal taps from
 * a sibling), which a real sized box can't do.
 *
 * `tone` selects BOTH the Android ripple colour and the iOS press-opacity
 * value, paired the same way every pre-migration call site paired them:
 * - "ink" / "danger" / "accent" sit on a transparent glyph with no
 *   background to shift, so the iOS dim is `opacityStandard` (a visible
 *   45% fade reads correctly when there's nothing solid underneath).
 * - "onDark" is for a glyph sitting on a solid ink/moss fill (the products
 *   FAB, the media-picker remove badge) — `opacityStandard`'s 45% fade
 *   "looks broken, not pressed" on a filled surface per theme.ts, so this
 *   tone pairs with the gentler `opacitySolidFill` instead.
 */
export interface IconButtonProps {
  /** The glyph. Usually a single lucide-react-native icon. */
  children: ReactNode;
  onPress: () => void;
  accessibilityLabel: string;
  /** Defaults to "button". */
  accessibilityRole?: AccessibilityRole;
  /** Selects the `theme.press.ripple*` token (and its paired iOS opacity). Defaults to "ink". */
  tone?: "ink" | "onDark" | "danger" | "accent";
  /**
   * Disables the button: no `onPress`, no press feedback (ripple or iOS
   * dim), and `accessibilityState.disabled` is forced true so TalkBack/
   * VoiceOver announce it correctly. Sets both `Pressable`'s native
   * `disabled` prop AND the accessibility state — the two are independent in
   * RN and both must be set for a control to read as truly non-interactive.
   */
  disabled?: boolean;
  style?: StyleProp<ViewStyle>;
  testID?: string;
}

const TONE_RIPPLE = {
  ink: theme.press.rippleInk,
  onDark: theme.press.rippleOnDark,
  danger: theme.press.rippleDanger,
  accent: theme.press.rippleAccent,
} as const;

const TONE_OPACITY = {
  ink: theme.press.opacityStandard,
  onDark: theme.press.opacitySolidFill,
  danger: theme.press.opacityStandard,
  accent: theme.press.opacityStandard,
} as const;

export function IconButton({
  children,
  onPress,
  accessibilityLabel,
  accessibilityRole = "button",
  tone = "ink",
  disabled = false,
  style,
  testID,
}: IconButtonProps) {
  // Press state is tracked explicitly rather than via Pressable's
  // `style={({pressed}) => …}` callback form. Under NativeWind's JSX interop
  // a FUNCTION style prop is not resolved the way a plain array is, and the
  // base styles are silently dropped at runtime — this shipped in increment
  // 1 across 41 call sites, 24 of which rendered with zero styles. Nothing
  // caught it: RNTL renders without the NativeWind runtime, so the unit
  // tests (which assert on the resolved style array) all passed. Keep this
  // an ARRAY. Do not "simplify" it back to the callback form.
  const [pressed, setPressed] = useState(false);
  // Guard explicitly rather than relying solely on RN's native `disabled`
  // handling on Pressable — belt-and-suspenders so a disabled button can
  // never visually engage its press state even if the responder system
  // changes.
  const handlePressIn = useCallback(() => {
    if (disabled) return;
    setPressed(true);
  }, [disabled]);
  const handlePressOut = useCallback(() => {
    if (disabled) return;
    setPressed(false);
  }, [disabled]);

  return (
    <Pressable
      onPress={onPress}
      onPressIn={handlePressIn}
      onPressOut={handlePressOut}
      disabled={disabled}
      accessibilityRole={accessibilityRole}
      accessibilityLabel={accessibilityLabel}
      // `Pressable` itself merges its own `disabled` prop into the final
      // accessibilityState it hands to the host node, so `disabled: true`
      // reaches TalkBack/VoiceOver without this component duplicating that
      // merge itself.
      accessibilityState={disabled ? { disabled: true } : undefined}
      testID={testID}
      android_ripple={disabled ? undefined : { ...TONE_RIPPLE[tone], borderless: true }}
      style={[
        styles.base,
        style,
        pressed && Platform.OS === "ios" ? { opacity: TONE_OPACITY[tone] } : null,
      ]}
    >
      {children}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    minWidth: theme.touchTarget,
    minHeight: theme.touchTarget,
    alignItems: "center",
    justifyContent: "center",
  },
});
