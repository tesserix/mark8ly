import { useCallback, useState, type ReactNode } from "react";
import {
  Platform,
  Pressable,
  StyleSheet,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import { theme } from "@/lib/theme";

/**
 * The one row press surface in the app.
 *
 * Replaces the legacy touchable-with-opacity-fade pattern: a whole-row 60%
 * fade is a web-styled-RN signature, not native press feedback. iOS shifts the row
 * background to the sink surface while held; Android draws a ripple.
 *
 * Owns press feedback and density ONLY. Callers supply all content and all
 * handlers — no business logic lives here.
 */
export interface PressableRowProps {
  children: ReactNode;
  onPress: () => void;
  onLongPress?: () => void;
  /** 1 for a single-line row (64pt), 2 for the primary+secondary stack (88pt). */
  lines?: 1 | 2;
  style?: StyleProp<ViewStyle>;
  accessibilityLabel: string;
  testID?: string;
}

export function PressableRow({
  children,
  onPress,
  onLongPress,
  lines = 1,
  style,
  accessibilityLabel,
  testID,
}: PressableRowProps) {
  // Press state is tracked explicitly rather than via Pressable's
  // `style={({pressed}) => …}` callback form. Under NativeWind's JSX interop a
  // FUNCTION style prop is not resolved the way a plain array is, and the base
  // styles were silently dropped at runtime — rows rendered with no padding
  // and `flexDirection` falling back to `column`, so every list row stacked
  // vertically. Nothing caught it: RNTL renders without the NativeWind runtime,
  // so the unit tests (which assert on the resolved style array) all passed.
  // Keep this an ARRAY. Do not "simplify" it back to the callback form.
  const [pressed, setPressed] = useState(false);
  const handlePressIn = useCallback(() => setPressed(true), []);
  const handlePressOut = useCallback(() => setPressed(false), []);

  return (
    <Pressable
      onPress={onPress}
      onLongPress={onLongPress}
      onPressIn={handlePressIn}
      onPressOut={handlePressOut}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
      testID={testID}
      android_ripple={theme.press.rippleInk}
      style={[
        styles.base,
        lines === 2 ? styles.twoLine : styles.oneLine,
        style,
        // `pressed` MUST be last: RN flattens the array later-wins, and every
        // row caller that needs an explicit `backgroundColor` (to match a
        // parent Card/sheet surface instead of inheriting `base`'s paper) was
        // passing it via `style` — which, before this fix, was placed after
        // `pressed` and silently killed the iOS press feedback on every one
        // of those rows (Android still rippled, masking it on emulator).
        // `styles.pressed` contains ONLY `backgroundColor`, so it can safely
        // win last without clobbering a caller's other overrides (e.g.
        // OrderRow's `flexDirection: "column"`) — those keys aren't in this
        // object, so array-merge leaves them untouched.
        pressed && Platform.OS === "ios" ? styles.pressed : null,
      ]}
    >

      {children}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.row.gap,
    paddingHorizontal: theme.row.paddingH,
    paddingVertical: theme.row.paddingV,
    backgroundColor: theme.colors.background,
  },
  oneLine: { minHeight: theme.row.minHeightSingle },
  twoLine: { minHeight: theme.row.minHeightDouble },
  pressed: { backgroundColor: theme.colors.sink },
});
