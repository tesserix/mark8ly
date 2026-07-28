import { useCallback, useMemo, useState } from "react";
import { Platform, View, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { StoreSelector } from "../../../components/StoreSelector";
import { useDeleteAccount } from "@/lib/admin-api/account-actions";
import {
  BackHeader,
  Card,
  Eyebrow,
  FieldInput,
  Hairline,
  PressableRow,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";

const DELETE_CONFIRMATION_WORD = "DELETE";

interface InfoRowProps {
  label: string;
  value?: string | null;
}

function InfoRow({ label, value }: InfoRowProps) {
  return (
    <View style={styles.infoRow}>
      <Text preset="caption" color="textTertiary">
        {label}
      </Text>
      <Text preset="body" color="text">
        {value ?? "—"}
      </Text>
    </View>
  );
}

export default function AccountScreen() {
  const { user, signOut } = useAuth();
  const activeStore = useTenantStore((s) => s.activeStore);
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

      <Eyebrow label="Profile" style={styles.eyebrow} />
      <Card padding="md" style={styles.card}>
        <InfoRow label="Name" value={user?.displayName ?? "Not set"} />
        <Hairline style={styles.divider} />
        <InfoRow label="Email" value={user?.email} />
      </Card>

      <Eyebrow label="Store" style={styles.eyebrow} />
      <Card padding={0} style={styles.card}>
        <PressableRow
          style={styles.storeRow}
          onPress={() => setStoreSelectorVisible(true)}
          accessibilityLabel={`Current store: ${activeStore?.name ?? "None"}. Tap to switch.`}
        >
          <View style={styles.storeInfo}>
            <Text preset="bodyEmphasis" color="text">
              {activeStore?.name ?? "No store selected"}
            </Text>
            {activeStore?.slug ? (
              <Text preset="caption" color="textTertiary">
                {activeStore.slug}
              </Text>
            ) : null}
          </View>
          <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
        </PressableRow>
      </Card>

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

      <StoreSelector
        visible={storeSelectorVisible}
        onClose={() => setStoreSelectorVisible(false)}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH. Not
  // theme.spacing.lg — that token is shared with non-gutter spacing
  // throughout the app and must not move.
  card: { marginHorizontal: theme.spacing.xl },
  // Eyebrow's own default gutter is theme.spacing.lg (16) — it's a shared
  // primitive used across screens with different gutters, so it can't carry
  // this screen's decision itself. Override explicitly to match `card`
  // above, so the "Profile"/"Store" labels sit flush with their cards.
  eyebrow: { paddingHorizontal: theme.spacing.xl },
  infoRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: theme.spacing.sm,
  },
  divider: { marginVertical: 2 },
  // Pre-migration this row had no backgroundColor of its own (transparent),
  // letting the parent Card's elevated (white) surface show through.
  // PressableRow's base sets backgroundColor: theme.colors.background
  // (paper), which would otherwise paint a visible seam against the Card —
  // match that surface explicitly instead of relying on transparency (same
  // fix as StorePicker).
  storeRow: { backgroundColor: theme.colors.elevated },
  storeInfo: { flex: 1, gap: 2 },
  actions: {
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xl,
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
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xxl,
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
