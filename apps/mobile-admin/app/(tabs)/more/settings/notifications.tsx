import { useCallback } from "react";
import {
  View,
  ScrollView,
  Switch,
  Alert,
  Linking,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { usePushPreference } from "@/lib/hooks/use-push-preference";
import { BackHeader, Hairline, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";

export default function NotificationSettingsScreen() {
  const dockPad = useDockClearance();
  const { enabled, permission, loading, busy, setPushEnabled } = usePushPreference();

  // Push only truly arrives when BOTH the in-app preference is on and the OS
  // has granted permission — so the switch reflects the effective state.
  const effectiveOn = enabled && permission === "granted";
  const blocked = permission === "denied";

  const onToggle = useCallback(
    async (next: boolean) => {
      const result = await setPushEnabled(next);
      if (!result.ok && result.reason === "permission") {
        Alert.alert(
          "Notifications are turned off",
          "Allow notifications for Mark8ly Admin in your device settings to receive alerts.",
          [
            { text: "Not now", style: "cancel" },
            { text: "Open Settings", onPress: () => Linking.openSettings() },
          ],
        );
      }
    },
    [setPushEnabled],
  );

  return (
    <Screen>
      <BackHeader eyebrow="SETTINGS" title="Notifications" />
      {loading ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : (
        <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
          <View style={styles.switchRow}>
            <View style={styles.switchText}>
              <Text preset="body" color="text">
                Push notifications
              </Text>
              <Text preset="caption" color="textTertiary">
                New orders, low stock and cancellations on this device.
              </Text>
            </View>
            <Switch
              value={effectiveOn}
              onValueChange={onToggle}
              disabled={busy}
              trackColor={{ true: theme.colors.accent }}
              accessibilityLabel="Toggle push notifications"
            />
          </View>

          {blocked ? (
            <>
              <Hairline />
              <Text preset="caption" color="warning" style={styles.note}>
                Notifications are blocked in your device settings. Turn them on for
                Mark8ly Admin to receive alerts.
              </Text>
              <Text
                preset="caption"
                color="accent"
                style={styles.link}
                onPress={() => Linking.openSettings()}
                accessibilityRole="link"
              >
                Open device settings
              </Text>
            </>
          ) : null}

          <Text preset="caption" color="textTertiary" style={styles.footnote}>
            You can also review past notifications from More › Notifications.
          </Text>
        </ScrollView>
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: theme.spacing.lg, gap: theme.spacing.md },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  switchRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: theme.spacing.md,
  },
  switchText: { flex: 1, gap: 2 },
  note: { marginTop: theme.spacing.xs },
  link: { marginTop: theme.spacing.xs },
  footnote: { marginTop: theme.spacing.md },
});
