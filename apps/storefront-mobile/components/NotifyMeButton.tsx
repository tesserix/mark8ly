import { useMemo } from "react";
import { Text, StyleSheet, Pressable, ActivityIndicator } from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useRouter } from "expo-router";
import { useNotifyMe } from "@/lib/hooks/use-notify-me";

interface NotifyMeButtonProps {
  productId: string;
}

export function NotifyMeButton({ productId }: NotifyMeButtonProps) {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  const { user } = useAuth();
  const router = useRouter();
  const { subscribe } = useNotifyMe({ productId });

  const handlePress = () => {
    if (!user) {
      router.push("/(auth)/login");
      return;
    }
    subscribe.mutate();
  };

  return (
    <Pressable
      style={styles.button}
      onPress={handlePress}
      disabled={subscribe.isPending || subscribe.isSuccess}
     accessibilityRole="button">
      {subscribe.isPending ? (
        <ActivityIndicator size="small" color={theme.elevated} />
      ) : subscribe.isSuccess ? (
        <Text style={styles.text}>You'll be notified</Text>
      ) : (
        <Text style={styles.text}>Notify me when available</Text>
      )}
    </Pressable>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  button: {
    backgroundColor: theme.accent,
    paddingVertical: 14,
    paddingHorizontal: 24,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
  },
  text: {
    color: theme.elevated,
    fontSize: 14,
    fontWeight: "600",
  },
});
}
