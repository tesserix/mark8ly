import { useState, type ReactNode } from "react";
import { View, StyleSheet, Pressable } from "react-native";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

interface EmptyStateAction {
  label: string;
  onPress: () => void;
}

interface EmptyStateProps {
  title: string;
  message?: string;
  icon?: ReactNode;
  /**
   * Optional call-to-action. Used by list screens to offer a "Try again"
   * retry on an error state, which is what distinguishes a failed fetch from
   * a genuinely empty result.
   */
  action?: EmptyStateAction;
  /**
   * "center" (default) or "left". ADDITIVE — the default is unchanged, so
   * the ~30 pre-existing call sites keep the centred treatment they were
   * designed against and this prop changes nothing they render.
   *
   * "left" is the design system's position (no centred heroes; one left
   * gutter shared by eyebrow, title and rows) and is what the new
   * editorial screens use — `QueueEmptyState` on the Dashboard is
   * left-aligned by construction, and Orders now passes `align="left"` so
   * the two screens built in this increment treat the same moment the same
   * way. Migrate the older screens as they're touched; do NOT flip the
   * default, which would silently restyle every one of them at once.
   */
  align?: "center" | "left";
}

export function EmptyState({ title, message, icon, action, align = "center" }: EmptyStateProps) {
  const left = align === "left";
  // NativeWind's JSX interop doesn't resolve a function `style` prop the way
  // it resolves a plain array — press state is tracked explicitly instead.
  const [pressed, setPressed] = useState(false);

  return (
    <View style={[styles.container, left && styles.containerLeft]} testID="empty-state">
      {icon ? <View style={styles.icon}>{icon}</View> : null}
      <Text preset="h3" color="text" align={left ? "left" : "center"}>
        {title}
      </Text>
      {message ? (
        <Text
          preset="body"
          color="textTertiary"
          align={left ? "left" : "center"}
          style={styles.message}
        >
          {message}
        </Text>
      ) : null}
      {action ? (
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={action.label}
          onPress={action.onPress}
          onPressIn={() => setPressed(true)}
          onPressOut={() => setPressed(false)}
          style={[styles.action, pressed && styles.actionPressed]}
        >
          <Text preset="bodyEmphasis" color="text">
            {action.label}
          </Text>
        </Pressable>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: theme.spacing.xl,
    paddingVertical: theme.spacing.huge,
    gap: theme.spacing.sm,
  },
  // Only the cross-axis alignment changes. The gutter stays `spacing.xl` —
  // the same token `QueueEmptyState` and every row on these screens use — so
  // a left-aligned empty state lands on the screen's ONE left edge rather
  // than inventing a second one.
  containerLeft: {
    alignItems: "flex-start",
  },
  icon: {
    marginBottom: theme.spacing.md,
    opacity: 0.5,
  },
  message: {
    maxWidth: 280,
  },
  action: {
    marginTop: theme.spacing.lg,
    minHeight: theme.touchTarget,
    justifyContent: "center",
    paddingHorizontal: theme.spacing.xl,
    borderWidth: 1,
    borderColor: theme.colors.border,
    borderRadius: theme.radius,
    backgroundColor: theme.colors.elevated,
  },
  actionPressed: {
    opacity: 0.7,
  },
});
