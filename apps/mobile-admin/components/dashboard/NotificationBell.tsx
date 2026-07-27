import { Platform, Pressable, View, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { Bell } from "lucide-react-native";
import { useNotifications } from "@/lib/hooks/use-notifications";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";

/**
 * Header action for the dashboard: a bell that opens the notifications inbox
 * and carries an unread badge. Unread is derived from the first page of the
 * notifications list — enough for a "you have some" signal; the inbox shows
 * the exact list.
 */
export function NotificationBell() {
  const router = useRouter();
  const { data } = useNotifications();
  const unread = data?.notifications.filter((n) => !n.is_read).length ?? 0;

  return (
    <Pressable
      onPress={() => router.push("/notifications")}
      hitSlop={10}
      accessibilityRole="button"
      accessibilityLabel={
        unread > 0 ? `Notifications, ${unread} unread` : "Notifications"
      }
      android_ripple={{ ...theme.press.rippleInk, borderless: true }}
      style={({ pressed }) => [
        styles.btn,
        pressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
      ]}
    >
      <Bell size={22} color={theme.colors.text} strokeWidth={1.75} />
      {unread > 0 ? (
        <View style={styles.badge}>
          <Text preset="caption" color="inverse" style={styles.badgeText}>
            {unread > 9 ? "9+" : String(unread)}
          </Text>
        </View>
      ) : null}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  btn: {
    width: 40,
    height: 40,
    alignItems: "center",
    justifyContent: "center",
  },
  badge: {
    position: "absolute",
    top: 2,
    right: 2,
    minWidth: 16,
    height: 16,
    paddingHorizontal: 4,
    borderRadius: theme.radii.pill,
    backgroundColor: theme.colors.accent,
    alignItems: "center",
    justifyContent: "center",
    borderWidth: 1.5,
    borderColor: theme.colors.background,
  },
  badgeText: {
    fontSize: 9,
    lineHeight: 12,
    fontWeight: "700",
  },
});
