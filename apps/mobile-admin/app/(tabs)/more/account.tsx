import { useCallback, useMemo, useState } from "react";
import { Platform, View, ScrollView, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { StoreSelector } from "../../../components/StoreSelector";
import { useDeleteAccount } from "@/lib/admin-api/account-actions";
import {
  BackHeader,
  FieldInput,
  GroupedList,
  GroupedRow,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";

const DELETE_CONFIRMATION_WORD = "DELETE";

export default function AccountScreen() {
  const { user, signOut } = useAuth();
  const activeStore = useTenantStore((s) => s.activeStore);
  const dockPad = useDockClearance();
  const [storeSelectorVisible, setStoreSelectorVisible] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");
  const deleteMutation = useDeleteAccount();
  // NativeWind's JSX interop doesn't resolve a function `style` prop the way
  // it resolves a plain array — press state is tracked explicitly instead.
  const [logoutPressed, setLogoutPressed] = useState(false);
  const [deletePressed, setDeletePressed] = useState(false);

  const handleLogout = useCallback(() => {
    Alert.alert("Sign Out", "Are you sure you want to sign out?", [
      { text: "Cancel", style: "cancel" },
      { text: "Sign Out", style: "destructive", onPress: () => signOut() },
    ]);
  }, [signOut]);

  const canDeleteAccount = useMemo(
    () => deleteConfirmText.trim() === DELETE_CONFIRMATION_WORD && !deleteMutation.isPending,
    [deleteConfirmText, deleteMutation.isPending],
  );

  const handleDeleteAccount = useCallback(() => {
    Alert.alert(
      "Delete account?",
      "This permanently deletes your account. If you own this store, your store and all its data — products, orders, and customers — will be removed. This cannot be undone.",
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete account",
          style: "destructive",
          onPress: () => deleteMutation.mutate(),
        },
      ],
    );
  }, [deleteMutation]);

  return (
    <Screen>
      <BackHeader eyebrow="ACCOUNT" title="Account" />

      {/* This screen had NO ScrollView at all pre-Task-9 — its fixed set of
          rows always fit. A GroupedList plus the danger zone's copy + field
          + button no longer reliably does at raised text sizes, so it now
          scrolls, with clearance for the floating dock at the bottom. */}
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
        <GroupedList
          sections={[
            {
              key: "profile",
              label: "Profile",
              rows: [
                <GroupedRow key="name" label="Name" value={user?.displayName ?? "Not set"} />,
                <GroupedRow key="email" label="Email" value={user?.email ?? "—"} />,
              ],
            },
            {
              key: "store",
              label: "Store",
              rows: [
                <GroupedRow
                  key="store"
                  label={activeStore?.name ?? "No store selected"}
                  hint={activeStore?.slug}
                  onPress={() => setStoreSelectorVisible(true)}
                  accessibilityLabel={`Current store: ${activeStore?.name ?? "None"}. Tap to switch.`}
                />,
              ],
            },
          ]}
        />

        <View style={styles.actions}>
          <Pressable
            onPress={handleLogout}
            onPressIn={() => setLogoutPressed(true)}
            onPressOut={() => setLogoutPressed(false)}
            accessibilityRole="button"
            accessibilityLabel="Sign out"
            android_ripple={theme.press.rippleDanger}
            style={[
              styles.logoutBtn,
              logoutPressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
            ]}
          >
            <Text preset="bodyEmphasis" color="danger">
              Sign Out
            </Text>
          </Pressable>
        </View>

        <View style={styles.dangerZone}>
          <Text preset="bodyEmphasis" color="danger">
            Delete account
          </Text>
          <Text preset="caption" color="textTertiary" style={styles.dangerCopy}>
            Deleting your account is permanent. If you own this store, your store and all its
            data will be removed. This can&rsquo;t be undone.
          </Text>
          <FieldInput
            label="Type DELETE to confirm"
            value={deleteConfirmText}
            onChangeText={setDeleteConfirmText}
            autoCapitalize="characters"
            autoCorrect={false}
            editable={!deleteMutation.isPending}
            accessibilityLabel="Type DELETE to confirm account deletion"
            style={styles.dangerInput}
          />
          <Pressable
            onPress={handleDeleteAccount}
            onPressIn={() => setDeletePressed(true)}
            onPressOut={() => setDeletePressed(false)}
            disabled={!canDeleteAccount}
            accessibilityRole="button"
            accessibilityLabel="Delete account"
            android_ripple={theme.press.rippleDanger}
            style={[
              styles.deleteBtn,
              !canDeleteAccount ? styles.deleteBtnDisabled : null,
              deletePressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
            ]}
          >
            {deleteMutation.isPending ? (
              <ActivityIndicator size="small" color={theme.colors.danger} />
            ) : (
              <Text preset="bodyEmphasis" color="danger">
                Delete account
              </Text>
            )}
          </Pressable>
          {deleteMutation.error ? (
            <Text
              preset="caption"
              color="danger"
              accessibilityRole="alert"
              accessibilityLiveRegion="polite"
              style={styles.dangerError}
            >
              {deleteMutation.error.message}
            </Text>
          ) : null}
        </View>
      </ScrollView>

      <StoreSelector
        visible={storeSelectorVisible}
        onClose={() => setStoreSelectorVisible(false)}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH so
  // GroupedList's eyebrows/cards and the actions/danger-zone blocks below
  // share one left edge. Not theme.spacing.lg — that token is shared with
  // non-gutter spacing throughout the app and must not move.
  scroll: {
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xs,
    gap: theme.spacing.xl,
  },
  actions: {
    paddingTop: theme.spacing.md,
  },
  logoutBtn: {
    backgroundColor: "transparent",
    borderRadius: theme.radii.md,
    borderWidth: 1,
    borderColor: theme.colors.danger,
    height: 48,
    alignItems: "center",
    justifyContent: "center",
  },
  dangerZone: {
    // No paddingHorizontal of its own — the ScrollView's `scroll` gutter
    // above already applies theme.spacing.xl to every direct child.
    paddingTop: theme.spacing.md,
    gap: theme.spacing.sm,
  },
  dangerCopy: { lineHeight: 18 },
  dangerInput: { marginTop: theme.spacing.xs },
  deleteBtn: {
    backgroundColor: "transparent",
    borderRadius: theme.radii.md,
    borderWidth: 1,
    borderColor: theme.colors.danger,
    height: 48,
    alignItems: "center",
    justifyContent: "center",
    marginTop: theme.spacing.xs,
  },
  deleteBtnDisabled: { opacity: 0.4 },
  dangerError: { marginTop: theme.spacing.xs },
});
