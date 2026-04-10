import { View, Text, TouchableOpacity, Linking, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useNotifications } from "../../../lib/hooks/use-notifications";

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
    backgroundColor: "#F7F6F2",
    paddingTop: 16,
  },
  menu: {
    backgroundColor: "#FFFFFF",
    marginHorizontal: 16,
    borderRadius: 6,
    borderWidth: 0.5,
    borderColor: "#0E0E0C10",
  },
  menuItem: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingVertical: 16,
  },
  menuLabel: {
    flex: 1,
    fontSize: 15,
    fontWeight: "500",
    color: "#0E0E0C",
  },
  badge: {
    backgroundColor: "#8B2020",
    borderRadius: 10,
    minWidth: 20,
    height: 20,
    paddingHorizontal: 6,
    justifyContent: "center",
    alignItems: "center",
    marginRight: 8,
  },
  badgeText: {
    fontSize: 11,
    fontWeight: "700",
    color: "#FFFFFF",
  },
  chevron: {
    fontSize: 20,
    color: "#0E0E0C",
    opacity: 0.3,
  },
  separator: {
    height: 0.5,
    backgroundColor: "#0E0E0C10",
    marginLeft: 16,
  },
  version: {
    textAlign: "center",
    fontSize: 12,
    color: "#0E0E0C",
    opacity: 0.3,
    marginTop: 24,
  },
});
