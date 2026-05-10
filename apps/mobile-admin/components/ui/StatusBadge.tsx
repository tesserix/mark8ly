import { View, StyleSheet, type ViewStyle } from "react-native";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

export type StatusTone =
  | "neutral"
  | "info"
  | "success"
  | "warning"
  | "danger"
  | "muted";

interface StatusBadgeProps {
  label: string;
  tone?: StatusTone;
  style?: ViewStyle;
}

const TONE: Record<StatusTone, { bg: string; fg: string; border?: string }> = {
  neutral: { bg: theme.colors.text, fg: theme.colors.inverse },
  info: { bg: theme.colors.surfaceAlt, fg: theme.colors.text, border: theme.colors.hairline },
  success: { bg: theme.colors.accent, fg: theme.colors.inverse },
  warning: { bg: theme.colors.warning, fg: theme.colors.inverse },
  danger: { bg: theme.colors.danger, fg: theme.colors.inverse },
  muted: { bg: "transparent", fg: theme.colors.textSecondary, border: theme.colors.hairline },
};

export function StatusBadge({ label, tone = "neutral", style }: StatusBadgeProps) {
  const t = TONE[tone];
  return (
    <View
      style={[
        styles.badge,
        {
          backgroundColor: t.bg,
          borderColor: t.border ?? "transparent",
          borderWidth: t.border ? theme.hairline : 0,
        },
        style,
      ]}
      accessible
      accessibilityLabel={`Status: ${label}`}
    >
      <Text preset="caption" color={t.fg} style={styles.label} numberOfLines={1}>
        {label}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  badge: {
    paddingHorizontal: theme.spacing.sm,
    paddingVertical: 3,
    borderRadius: 4,
    alignSelf: "flex-start",
  },
  label: {
    fontSize: 11,
    fontWeight: "600",
    letterSpacing: 0.4,
    textTransform: "capitalize",
  },
});
