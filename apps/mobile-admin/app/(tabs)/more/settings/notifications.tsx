import { useCallback, useEffect, useState } from "react";
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
import {
  useNotificationPreferences,
  useUpdateNotificationPreferences,
  PREFERENCE_TYPES,
  type PreferenceType,
} from "@/lib/hooks/use-notification-preferences";
import { BackHeader, Hairline, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";

// Copy for the store-wide per-type toggles. Store-wide (not per-device): each
// governs whether that notification type is generated at all — bell and push.
const TYPE_COPY: Record<PreferenceType, { label: string; hint: string }> = {
  new_order: { label: "New orders", hint: "When a customer places an order" },
  low_stock: { label: "Low stock", hint: "When a product runs low on inventory" },
  return_requested: { label: "Return requests", hint: "When a customer requests a return" },
  payment_received: { label: "Payments", hint: "When a payment is received" },
  review_submitted: { label: "New reviews", hint: "When a customer leaves a review" },
};

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
          {/* Device-level push master. Separate from the per-type prefs below:
              this governs whether THIS device buzzes at all. */}
          <View style={styles.switchRow}>
            <View style={styles.switchText}>
              <Text preset="body" color="text">
                Push notifications
              </Text>
              <Text preset="caption" color="textTertiary">
                Alerts on this device when something needs you.
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

          <AlertTypesSection />
        </ScrollView>
      )}
    </Screen>
  );
}

// Store-wide per-type toggles. Turning one off stops that notification being
// generated for the whole store, in both the bell and push.
function AlertTypesSection() {
  const { data, isLoading, isError, refetch } = useNotificationPreferences();
  const update = useUpdateNotificationPreferences();

  // Optimistic local mirror so a toggle flips instantly; reverts on error.
  const [local, setLocal] = useState<Record<PreferenceType, boolean> | null>(null);
  useEffect(() => {
    if (data) setLocal(data);
  }, [data]);

  const onToggleType = useCallback(
    (key: PreferenceType, next: boolean) => {
      if (!local) return;
      const previous = local;
      const optimistic = { ...local, [key]: next };
      setLocal(optimistic);
      // The backend overwrites the whole JSONB, so send every key, not just
      // the one that changed.
      update.mutate(optimistic, {
        onError: () => setLocal(previous),
      });
    },
    [local, update],
  );

  return (
    <View style={styles.section}>
      <Text preset="eyebrow" color="textTertiary" style={styles.sectionLabel}>
        Alert types
      </Text>
      <Text preset="caption" color="textTertiary" style={styles.sectionIntro}>
        Choose what your store notifies you about. Applies to everyone on this
        store, in the inbox and push.
      </Text>

      {isLoading || !local ? (
        <View style={styles.rowLoading}>
          <ActivityIndicator size="small" color={theme.colors.textTertiary} />
        </View>
      ) : isError ? (
        <View style={styles.rowLoading}>
          <Text preset="caption" color="textTertiary">
            Couldn&apos;t load alert types.
          </Text>
          <Text
            preset="caption"
            color="accent"
            onPress={() => refetch()}
            accessibilityRole="button"
            style={styles.link}
          >
            Retry
          </Text>
        </View>
      ) : (
        PREFERENCE_TYPES.map((key, i) => (
          <View key={key}>
            {i > 0 ? <Hairline /> : null}
            <View style={styles.switchRow}>
              <View style={styles.switchText}>
                <Text preset="body" color="text">
                  {TYPE_COPY[key].label}
                </Text>
                <Text preset="caption" color="textTertiary">
                  {TYPE_COPY[key].hint}
                </Text>
              </View>
              <Switch
                value={local[key]}
                onValueChange={(next) => onToggleType(key, next)}
                disabled={update.isPending}
                trackColor={{ true: theme.colors.accent }}
                accessibilityLabel={`Toggle ${TYPE_COPY[key].label} notifications`}
              />
            </View>
          </View>
        ))
      )}
    </View>
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
    paddingVertical: theme.spacing.sm,
  },
  switchText: { flex: 1, gap: 2 },
  note: { marginTop: theme.spacing.xs },
  link: { marginTop: theme.spacing.xs },
  section: { marginTop: theme.spacing.lg, gap: theme.spacing.xs },
  sectionLabel: { marginBottom: 2 },
  sectionIntro: { marginBottom: theme.spacing.xs },
  rowLoading: {
    paddingVertical: theme.spacing.lg,
    alignItems: "center",
    gap: theme.spacing.xs,
  },
});
