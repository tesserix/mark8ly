import { useCallback, useState } from "react";
import { View, TouchableOpacity, Alert, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { StoreSelector } from "../../../components/StoreSelector";
import {
  BackHeader,
  Card,
  Eyebrow,
  Hairline,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";

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

  const handleLogout = useCallback(() => {
    Alert.alert("Sign Out", "Are you sure you want to sign out?", [
      { text: "Cancel", style: "cancel" },
      { text: "Sign Out", style: "destructive", onPress: () => signOut() },
    ]);
  }, [signOut]);

  return (
    <Screen>
      <BackHeader eyebrow="ACCOUNT" title="Account" />

      <Eyebrow label="Profile" />
      <Card padding="md" style={styles.card}>
        <InfoRow label="Name" value={user?.displayName ?? "Not set"} />
        <Hairline style={styles.divider} />
        <InfoRow label="Email" value={user?.email} />
      </Card>

      <Eyebrow label="Store" />
      <Card padding={0} style={styles.card}>
        <TouchableOpacity
          style={styles.storeRow}
          onPress={() => setStoreSelectorVisible(true)}
          activeOpacity={0.6}
          accessibilityRole="button"
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
        </TouchableOpacity>
      </Card>

      <View style={styles.actions}>
        <TouchableOpacity
          style={styles.logoutBtn}
          onPress={handleLogout}
          activeOpacity={0.7}
          accessibilityRole="button"
          accessibilityLabel="Sign out"
        >
          <Text preset="bodyEmphasis" color="danger">
            Sign Out
          </Text>
        </TouchableOpacity>
      </View>

      <StoreSelector
        visible={storeSelectorVisible}
        onClose={() => setStoreSelectorVisible(false)}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  card: { marginHorizontal: theme.spacing.lg },
  infoRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: theme.spacing.sm,
  },
  divider: { marginVertical: 2 },
  storeRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.md,
    minHeight: 56,
  },
  storeInfo: { flex: 1, gap: 2 },
  actions: {
    paddingHorizontal: theme.spacing.lg,
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
});
