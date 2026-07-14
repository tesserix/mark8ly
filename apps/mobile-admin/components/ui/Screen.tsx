import type { ReactNode } from "react";
import { View, StyleSheet, type ViewStyle } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { theme } from "@/lib/theme";

interface ScreenProps {
  children: ReactNode;
  style?: ViewStyle;
  variant?: "paper" | "warm";
  /**
   * Pad the top by the safe-area inset so content clears the status bar /
   * notch. On by default; pass false for screens that handle the inset
   * themselves (e.g. a full-bleed hero).
   */
  topInset?: boolean;
}

export function Screen({
  children,
  style,
  variant = "paper",
  topInset = true,
}: ScreenProps) {
  const insets = useSafeAreaInsets();
  const bg =
    variant === "warm" ? theme.colors.surfaceAlt : theme.colors.background;
  return (
    <View
      style={[
        styles.root,
        { backgroundColor: bg },
        topInset ? { paddingTop: insets.top } : null,
        style,
      ]}
    >
      {children}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1 },
});
