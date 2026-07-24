import type { ReactNode } from "react";
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
}

export function EmptyState({ title, message, icon, action }: EmptyStateProps) {
  return (
    <View style={styles.container}>
      {icon ? <View style={styles.icon}>{icon}</View> : null}
      <Text preset="h3" color="text" align="center">
        {title}
      </Text>
      {message ? (
        <Text preset="body" color="textTertiary" align="center" style={styles.message}>
          {message}
        </Text>
      ) : null}
      {action ? (
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={action.label}
          onPress={action.onPress}
          style={({ pressed }) => [styles.action, pressed && styles.actionPressed]}
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
