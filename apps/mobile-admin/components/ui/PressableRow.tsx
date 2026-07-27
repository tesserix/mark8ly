import type { ReactNode } from "react";
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
 * Replaces `TouchableOpacity` + `activeOpacity`: a whole-row 60% fade is a
 * web-styled-RN signature, not native press feedback. iOS shifts the row
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

const RIPPLE = { color: "rgba(14, 14, 12, 0.12)" } as const;

export function PressableRow({
  children,
  onPress,
  onLongPress,
  lines = 1,
  style,
  accessibilityLabel,
  testID,
}: PressableRowProps) {
  return (
    <Pressable
      onPress={onPress}
      onLongPress={onLongPress}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
      testID={testID}
      android_ripple={RIPPLE}
      style={({ pressed }) => [
        styles.base,
        lines === 2 ? styles.twoLine : styles.oneLine,
        // Android draws its own ripple; only iOS needs the background shift.
        pressed && Platform.OS === "ios" ? styles.pressed : null,
        style,
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
