import type { ReactNode } from "react";
import { View, StyleSheet, type ViewStyle } from "react-native";
import { theme } from "@/lib/theme";

interface CardProps {
  children: ReactNode;
  style?: ViewStyle;
  /** "outline" = hairline border, "ghost" = borderless on Paper. */
  variant?: "outline" | "ghost";
  padding?: keyof typeof theme.spacing | 0;
}

export function Card({
  children,
  style,
  variant = "outline",
  padding = "lg",
}: CardProps) {
  const padValue = padding === 0 ? 0 : theme.spacing[padding];
  return (
    <View
      style={[
        styles.base,
        variant === "outline" ? styles.outline : styles.ghost,
        { padding: padValue },
        style,
      ]}
    >
      {children}
    </View>
  );
}

const styles = StyleSheet.create({
  base: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radii.md,
  },
  outline: {
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
  },
  ghost: {
    backgroundColor: "transparent",
  },
});
