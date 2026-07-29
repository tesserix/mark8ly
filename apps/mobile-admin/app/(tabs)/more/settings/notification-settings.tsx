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
import { BackHeader, GroupedList, GroupedRow, Screen, Text } from "@/components/ui";
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
              this governs whether THIS device buzzes at all. Unlabelled
              section — a single row, no eyebrow above it, same as it had no
              heading pre-migration. */}
          <GroupedList
            sections={[
              {
                key: "push",
                rows: [
                  <GroupedRow
                    key="push-toggle"
                    label="Push notifications"
                    hint="Alerts on this device when something needs you."
                    accessibilityLabel="Push notifications"
                    trailing={
                      <Switch
                        value={effectiveOn}
                        onValueChange={onToggle}
                        disabled={busy}
                        trackColor={{ true: theme.colors.accent }}
                        accessibilityLabel="Toggle push notifications"
                      />
                    }
                  />,
                ],
              },
            ]}
          />

          {blocked ? (
            <View style={styles.blockedNote}>
              <Text preset="caption" color="warning">
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
            </View>
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

  // Loading/error are transient, non-list states — rendered as a plain
  // eyebrow + status block rather than through GroupedList, matching how
  // this section looked pre-migration (it never had a Card to begin with).
  if (isLoading || !local) {
    return (
      <View style={styles.section}>
        <Text preset="eyebrow" color="textTertiary" style={styles.sectionLabel}>
          Alert types
        </Text>
        <View style={styles.rowLoading}>
          <ActivityIndicator size="small" color={theme.colors.textTertiary} />
        </View>
      </View>
    );
  }

  if (isError) {
    return (
      <View style={styles.section}>
        <Text preset="eyebrow" color="textTertiary" style={styles.sectionLabel}>
          Alert types
        </Text>
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
      </View>
    );
  }

  return (
    <GroupedList
      sections={[
        {
          key: "alert-types",
          label: "Alert types",
          rows: PREFERENCE_TYPES.map((key) => (
            <GroupedRow
              key={key}
              label={TYPE_COPY[key].label}
              hint={TYPE_COPY[key].hint}
              accessibilityLabel={`${TYPE_COPY[key].label}, ${TYPE_COPY[key].hint}`}
              trailing={
                <Switch
                  value={local[key]}
                  onValueChange={(next) => onToggleType(key, next)}
                  disabled={update.isPending}
                  trackColor={{ true: theme.colors.accent }}
                  accessibilityLabel={`Toggle ${TYPE_COPY[key].label} notifications`}
                />
              }
            />
          )),
          // The screen's original intro copy, describing the five toggles
          // below it — GroupedList's construction always renders `footer`
          // AFTER the Card, so it now reads as a footnote under the list
          // rather than a lead-in above it. See the task-9 report for why
          // this binds to the Alert types section rather than the push
          // section above.
          footer:
            "Choose what your store notifies you about. Applies to everyone on this store, in the inbox and push.",
        },
      ]}
    />
  );
}

const styles = StyleSheet.create({
  // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH so the
  // push toggle, the Alert types eyebrow/card and the blocked-permission
  // note all share one left edge. Not theme.spacing.lg.
  scroll: {
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xs,
    gap: theme.spacing.lg,
  },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  blockedNote: { gap: theme.spacing.xs },
  link: { marginTop: theme.spacing.xs },
  section: { gap: theme.spacing.xs },
  sectionLabel: { paddingHorizontal: theme.spacing.xs },
  rowLoading: {
    paddingVertical: theme.spacing.lg,
    alignItems: "center",
    gap: theme.spacing.xs,
  },
});
