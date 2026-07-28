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
  /** Additive: lets a caller assert on the badge without matching its copy. */
  testID?: string;
}

/**
 * `success`, `warning`, `danger` and `muted` share ONE construction: a filled
 * TINT field with its own darker text, no border. That uniformity is the
 * point — a solid fill among tints reads as an alert, and on a calm Paper
 * screen the loudest chip wins attention it hasn't earned. `neutral` (solid
 * ink) and `info` (hairline-bordered surface) are the two deliberate
 * exceptions: neutral is the emphatic state, info is the quietest.
 *
 * Every text/field pair below is COMPUTED against WCAG AA 4.5:1, never
 * eyeballed. Ratios recorded in inc2-task-8-report.md, "Fix round 1".
 */
const TONE: Record<StatusTone, { bg: string; fg: string; border?: string }> = {
  neutral: { bg: theme.colors.text, fg: theme.colors.inverse },
  info: { bg: theme.colors.surfaceAlt, fg: theme.colors.text, border: theme.colors.hairline },
  // Moss on moss tint — 8.35:1. Keeps the one moss accent reserved for
  // primary actions instead of spending it on every success chip.
  success: { bg: theme.colors.accentTint, fg: theme.colors.accent },
  // Deep bronze on amber tint — 6.05:1. Ink on the saturated solid amber was
  // AA-passing (~5:1) but perceptually marginal; white/inverse on amber
  // (~2.98:1) failed outright.
  warning: { bg: theme.colors.warningTint, fg: theme.colors.warningInk },
  // Oxblood on blood tint — 6.82:1. Was a SOLID crimson fill with inverse
  // text: the only non-tint among the four status tones, and on the Dashboard
  // it made a single "Low stock" chip the loudest element on the screen.
  // User-approved app-wide change; every danger badge in the app is a tint now
  // (orders, reviews, tickets, shipments, audit logs, campaigns, customers).
  danger: { bg: theme.colors.dangerTint, fg: theme.colors.danger },
  // Tertiary ink on the sink surface — 5.80:1. Was a TRANSPARENT chip with a
  // hairline, which made it the only tone whose field didn't exist; filled to
  // match the other three (and the mockup's `b-mute` #ECEAE3 chip).
  muted: { bg: theme.colors.sink, fg: theme.colors.textTertiary },
};

export function StatusBadge({ label, tone = "neutral", style, testID }: StatusBadgeProps) {
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
      testID={testID}
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
