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
  // Tint, not a solid accent fill — keeps the one moss accent reserved for
  // primary actions instead of spending it on every success chip. Moss on
  // this tint is ~8.35:1 (see fix-batch-1-report.md).
  success: { bg: theme.colors.accentTint, fg: theme.colors.accent },
  // Amber TINT with deep-bronze text (~6:1) — a tint like `success`, not a
  // solid fill. Ink on the saturated solid amber was AA-passing (~5:1) but
  // perceptually marginal; white/inverse on amber (~2.98:1) failed outright.
  warning: { bg: theme.colors.warningTint, fg: theme.colors.warningInk },
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
  // fontSize was a literal 11 (the old eyebrow scale) — orphaned by the type
  // rescale. theme.text.eyebrow.fontSize stays anchored to the current
  // scale; the two badge-counter pills (10, 9 elsewhere in the app) are
  // deliberately smaller still and are left alone.
  label: {
    fontSize: theme.text.eyebrow.fontSize,
    fontWeight: "600",
    letterSpacing: 0.4,
    textTransform: "capitalize",
  },
});
