import { useState, useCallback } from "react";
import {
  View,
  Text,
  TouchableOpacity,
  Alert,
  StyleSheet,
} from "react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { StoreSelector } from "../../../components/StoreSelector";
import { theme } from "@/lib/theme";

export default function AccountScreen() {
  const { user, signOut } = useAuth();
  const activeStore = useTenantStore((s) => s.activeStore);
  const [storeSelectorVisible, setStoreSelectorVisible] = useState(false);

  const handleLogout = useCallback(() => {
    Alert.alert("Sign Out", "Are you sure you want to sign out?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Sign Out",
        style: "destructive",
        onPress: () => signOut(),
      },
    ]);
  }, [signOut]);

  return (
    <View style={styles.screen}>
      {/* Profile */}
      <View style={styles.card}>
        <Text style={styles.sectionHeader}>Profile</Text>
        <View style={styles.infoRow}>
          <Text style={styles.infoLabel}>Name</Text>
          <Text style={styles.infoValue}>
            {user?.displayName ?? "Not set"}
          </Text>
        </View>
        <View style={styles.infoRow}>
          <Text style={styles.infoLabel}>Email</Text>
          <Text style={styles.infoValue}>
            {user?.email ?? "Unknown"}
          </Text>
        </View>
      </View>

      {/* Store */}
      <TouchableOpacity
        style={styles.card}
        onPress={() => setStoreSelectorVisible(true)}
        activeOpacity={0.7}
        accessibilityRole="button"
        accessibilityLabel={`Current store: ${activeStore?.name ?? "None"}. Tap to switch.`}
      >
        <Text style={styles.sectionHeader}>Store</Text>
        <View style={styles.storeRow}>
          <View style={styles.storeInfo}>
            <Text style={styles.storeName}>
              {activeStore?.name ?? "No store selected"}
            </Text>
            {activeStore?.slug && (
              <Text style={styles.storeSlug}>{activeStore.slug}</Text>
            )}
          </View>
          <Text style={styles.chevron}>&#x203A;</Text>
        </View>
      </TouchableOpacity>

      {/* Sign out */}
      <TouchableOpacity
        style={styles.logoutBtn}
        onPress={handleLogout}
        activeOpacity={0.7}
        accessibilityRole="button"
        accessibilityLabel="Sign out"
      >
        <Text style={styles.logoutText}>Sign Out</Text>
      </TouchableOpacity>

      <StoreSelector
        visible={storeSelectorVisible}
        onClose={() => setStoreSelectorVisible(false)}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: theme.colors.background,
    padding: theme.spacing.lg,
    gap: theme.spacing.md,
  },
  card: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    padding: 14,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  sectionHeader: {
    fontSize: 12,
    fontWeight: "600",
    color: theme.colors.text,
    opacity: 0.5,
    textTransform: "uppercase",
    letterSpacing: 0.5,
    marginBottom: 10,
  },
  infoRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    paddingVertical: theme.spacing.sm,
  },
  infoLabel: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.5,
    flex: 1,
  },
  infoValue: {
    fontSize: 13,
    color: theme.colors.text,
    fontWeight: "500",
    flex: 2,
    textAlign: "right",
  },
  storeRow: {
    flexDirection: "row",
    alignItems: "center",
  },
  storeInfo: {
    flex: 1,
  },
  storeName: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.text,
    marginBottom: 2,
  },
  storeSlug: {
    fontSize: 12,
    color: theme.colors.text,
    opacity: 0.4,
  },
  chevron: {
    fontSize: 20,
    color: theme.colors.text,
    opacity: 0.3,
    marginLeft: theme.spacing.sm,
  },
  logoutBtn: {
    backgroundColor: "transparent",
    borderRadius: theme.radius,
    paddingVertical: 14,
    alignItems: "center",
    borderWidth: 1,
    borderColor: theme.colors.danger,
    marginTop: theme.spacing.sm,
  },
  logoutText: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.danger,
  },
});
