import { Text, StyleSheet, Pressable, ActivityIndicator } from "react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useRouter } from "expo-router";
import { useNotifyMe } from "@/lib/hooks/use-notify-me";

interface NotifyMeButtonProps {
  productId: string;
}

export function NotifyMeButton({ productId }: NotifyMeButtonProps) {
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
    >
      {subscribe.isPending ? (
        <ActivityIndicator size="small" color="#FFFFFF" />
      ) : subscribe.isSuccess ? (
        <Text style={styles.text}>You'll be notified</Text>
      ) : (
        <Text style={styles.text}>Notify me when available</Text>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  button: {
    backgroundColor: "#2D4A2B",
    paddingVertical: 14,
    paddingHorizontal: 24,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
  },
  text: {
    color: "#FFFFFF",
    fontSize: 14,
    fontWeight: "600",
  },
});
