import type { ReactNode } from "react";
import { View, StyleSheet } from "react-native";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

interface EmptyStateProps {
  title: string;
  message?: string;
  icon?: ReactNode;
}

export function EmptyState({ title, message, icon }: EmptyStateProps) {
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
});
