import type { ReactNode } from "react";
import { StyleSheet, View, type ViewStyle } from "react-native";
import { theme } from "@/lib/theme";

interface CardProps {
  children: ReactNode;
  padding?: number | "sm" | "md" | "lg";
  style?: ViewStyle;
}

export function Card({ children, padding = "md", style }: CardProps) {
  const pad =
    typeof padding === "number"
      ? padding
      : padding === "sm"
        ? theme.spacing.sm
        : padding === "lg"
          ? theme.spacing.lg
          : theme.spacing.md;
  return <View style={[styles.card, { padding: pad }, style]}>{children}</View>;
}

const styles = StyleSheet.create({
  card: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radii.lg,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
  },
});
