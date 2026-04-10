import { View, Text, TouchableOpacity, Linking, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useNotifications } from "../../../lib/hooks/use-notifications";
import { theme } from "@/lib/theme";

const APP_VERSION = "1.0.0";

export default function MoreScreen() {
  const router = useRouter();
  const { data: notifications } = useNotifications();

  const unreadCount = notifications?.items.filter((n) => !n.read).length ?? 0;

  return (
    <View style={styles.screen}>
      <View style={styles.menu}>
        <TouchableOpacity
          style={styles.menuItem}
          onPress={() => router.push("/(tabs)/more/notifications")}
          activeOpacity={0.7}
          accessibilityRole="button"
          accessibilityLabel={`Notifications${unreadCount > 0 ? `, ${unreadCount} unread` : ""}`}
        >
          <Text style={styles.menuLabel}>Notifications</Text>
          {unreadCount > 0 && (
            <View style={styles.badge}>
              <Text style={styles.badgeText}>
                {unreadCount > 99 ? "99+" : String(unreadCount)}
              </Text>
            </View>
          )}
          <Text style={styles.chevron}>&#x203A;</Text>
        </TouchableOpacity>

        <View style={styles.separator} />

        <TouchableOpacity
          style={styles.menuItem}
          onPress={() => router.push("/(tabs)/more/account")}
          activeOpacity={0.7}
          accessibilityRole="button"
          accessibilityLabel="Account settings"
        >
          <Text style={styles.menuLabel}>Account</Text>
          <Text style={styles.chevron}>&#x203A;</Text>
        </TouchableOpacity>

        <View style={styles.separator} />

        <TouchableOpacity
          style={styles.menuItem}
          onPress={() => Linking.openURL("https://admin.mark8ly.com")}
          activeOpacity={0.7}
          accessibilityRole="link"
          accessibilityLabel="Open admin dashboard in browser"
        >
          <Text style={styles.menuLabel}>Open in Browser</Text>
          <Text style={styles.chevron}>&#x203A;</Text>
        </TouchableOpacity>
      </View>

      <Text style={styles.version}>Version {APP_VERSION}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: theme.colors.background,
    paddingTop: theme.spacing.lg,
  },
  menu: {
    backgroundColor: theme.colors.elevated,
    marginHorizontal: theme.spacing.lg,
    borderRadius: theme.radius,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  menuItem: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.lg,
    minHeight: 48,
  },
  menuLabel: {
    flex: 1,
    fontSize: 15,
    fontWeight: "500",
    color: theme.colors.text,
  },
  badge: {
    backgroundColor: theme.colors.danger,
    borderRadius: 10,
    minWidth: 20,
    height: 20,
    paddingHorizontal: theme.spacing.sm,
    justifyContent: "center",
    alignItems: "center",
    marginRight: theme.spacing.sm,
  },
  badgeText: {
    fontSize: 11,
    fontWeight: "700",
    color: theme.colors.elevated,
  },
  chevron: {
    fontSize: 20,
    color: theme.colors.text,
    opacity: 0.3,
  },
  separator: {
    height: 0.5,
    backgroundColor: `${theme.colors.text}10`,
    marginLeft: theme.spacing.lg,
  },
  version: {
    textAlign: "center",
    fontSize: 12,
    color: theme.colors.text,
    opacity: 0.3,
    marginTop: theme.spacing.xxl,
  },
});
