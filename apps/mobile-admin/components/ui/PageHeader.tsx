import type { ReactNode } from "react";
import { View, StyleSheet } from "react-native";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

interface PageHeaderProps {
  /** Eyebrow above the title — typically the page name in caps. */
  eyebrow?: string;
  /** Serif title — the editorial heading. */
  title: string;
  /** Optional one-line caption under the title. */
  subtitle?: string;
  rightSlot?: ReactNode;
}

export function PageHeader({
  eyebrow,
  title,
  subtitle,
  rightSlot,
}: PageHeaderProps) {
  return (
    <View style={styles.wrap}>
      <View style={styles.row}>
        <View style={styles.left}>
          {eyebrow ? (
            <Text preset="eyebrow" color="textTertiary" style={styles.eyebrow}>
              {eyebrow}
            </Text>
          ) : null}
          <Text preset="h1" color="text" style={styles.title}>
            {title}
          </Text>
          {subtitle ? (
            <Text preset="body" color="textSecondary" style={styles.subtitle}>
              {subtitle}
            </Text>
          ) : null}
        </View>
        {rightSlot ? <View style={styles.right}>{rightSlot}</View> : null}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.lg,
    paddingBottom: theme.spacing.md,
  },
  row: {
    flexDirection: "row",
    alignItems: "flex-start",
    justifyContent: "space-between",
    gap: theme.spacing.md,
  },
  left: { flex: 1 },
  right: { paddingTop: theme.spacing.xs },
  eyebrow: { marginBottom: 4 },
  title: {},
  subtitle: { marginTop: 4 },
});
