import type { ReactNode } from "react";
import { View, StyleSheet, type ViewStyle } from "react-native";
import { theme } from "@/lib/theme";

interface ScreenProps {
  children: ReactNode;
  style?: ViewStyle;
  variant?: "paper" | "warm";
}

export function Screen({ children, style, variant = "paper" }: ScreenProps) {
  const bg = variant === "warm" ? theme.colors.surfaceAlt : theme.colors.background;
  return <View style={[styles.root, { backgroundColor: bg }, style]}>{children}</View>;
}

const styles = StyleSheet.create({
  root: { flex: 1 },
});
