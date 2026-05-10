import {
  ActivityIndicator,
  StyleSheet,
  TouchableOpacity,
  type TouchableOpacityProps,
} from "react-native";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

interface ButtonProps extends Omit<TouchableOpacityProps, "children"> {
  label: string;
  variant?: "primary" | "secondary" | "ghost";
  loading?: boolean;
  fullWidth?: boolean;
}

export function Button({
  label,
  variant = "primary",
  loading = false,
  fullWidth,
  disabled,
  style,
  ...rest
}: ButtonProps) {
  const isPrimary = variant === "primary";
  const isSecondary = variant === "secondary";
  return (
    <TouchableOpacity
      activeOpacity={0.85}
      accessibilityRole="button"
      disabled={disabled || loading}
      {...rest}
      style={[
        styles.base,
        isPrimary && styles.primary,
        isSecondary && styles.secondary,
        variant === "ghost" && styles.ghost,
        fullWidth && styles.fullWidth,
        (disabled || loading) && styles.disabled,
        style,
      ]}
    >
      {loading ? (
        <ActivityIndicator
          size="small"
          color={isPrimary ? theme.colors.inverse : theme.colors.text}
        />
      ) : (
        <Text
          preset="bodyEmphasis"
          color={isPrimary ? "inverse" : "text"}
        >
          {label}
        </Text>
      )}
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  base: {
    height: 48,
    paddingHorizontal: theme.spacing.xl,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  primary: { backgroundColor: theme.colors.primary },
  secondary: {
    backgroundColor: "transparent",
    borderWidth: 1,
    borderColor: theme.colors.border,
  },
  ghost: { backgroundColor: "transparent" },
  fullWidth: { alignSelf: "stretch" },
  disabled: { opacity: 0.5 },
});
