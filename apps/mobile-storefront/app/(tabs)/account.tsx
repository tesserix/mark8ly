import { Alert, Linking, StyleSheet, TouchableOpacity, View } from "react-native";
import {
  ChevronRight,
  ExternalLink,
  Heart,
  Package,
  UserRound,
} from "lucide-react-native";
import { useRouter } from "expo-router";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { Button, Card, EmptyState, Hairline, PageHeader, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { getMerchant } from "@/lib/merchant";

export default function AccountScreen() {
  const { user, signOut } = useAuth();
  const router = useRouter();
  const merchant = getMerchant();

  const handleSignOut = () =>
    Alert.alert("Sign out", "Are you sure you want to sign out?", [
      { text: "Cancel", style: "cancel" },
      { text: "Sign out", style: "destructive", onPress: () => signOut() },
    ]);

  if (!user) {
    return (
      <Screen>
        <PageHeader eyebrow="ACCOUNT" title="Sign in" />
        <View style={styles.center}>
          <EmptyState
            icon={<UserRound size={28} color={theme.colors.textTertiary} strokeWidth={1.5} />}
            title="Sign in to your account"
            message="Track orders, save favorites, and check out faster."
            action={
              <View style={{ gap: theme.spacing.sm, alignSelf: "stretch" }}>
                <Button label="Sign in" onPress={() => router.push("/sign-in")} fullWidth />
                <Button
                  label="Create an account"
                  variant="secondary"
                  onPress={() => router.push("/sign-up")}
                  fullWidth
                />
              </View>
            }
          />
        </View>
      </Screen>
    );
  }

  return (
    <Screen>
      <PageHeader
        eyebrow="ACCOUNT"
        title={user.displayName || user.email || "You"}
        subtitle={user.email ?? undefined}
      />

      <Card padding={0} style={styles.card}>
        <Row
          icon={<Package size={18} color={theme.colors.text} strokeWidth={1.75} />}
          label="Orders"
          onPress={() => router.push("/orders")}
        />
        <Hairline inset={theme.spacing.huge + theme.spacing.xs} />
        <Row
          icon={<Heart size={18} color={theme.colors.text} strokeWidth={1.75} />}
          label="Wishlist"
          onPress={() => router.push("/wishlist")}
        />
        <Hairline inset={theme.spacing.huge + theme.spacing.xs} />
        <Row
          icon={<UserRound size={18} color={theme.colors.text} strokeWidth={1.75} />}
          label="Profile & addresses"
          onPress={() => router.push("/profile")}
        />
        <Hairline inset={theme.spacing.huge + theme.spacing.xs} />
        <Row
          icon={<ExternalLink size={18} color={theme.colors.text} strokeWidth={1.75} />}
          label="Visit website"
          onPress={() =>
            Linking.openURL(`https://${merchant.defaultStoreSlug}.mark8ly.com`)
          }
        />
      </Card>

      <View style={styles.actions}>
        <Button label="Sign out" variant="secondary" onPress={handleSignOut} fullWidth />
      </View>
    </Screen>
  );
}

function Row({
  icon,
  label,
  onPress,
}: {
  icon: React.ReactNode;
  label: string;
  onPress: () => void;
}) {
  return (
    <TouchableOpacity
      onPress={onPress}
      activeOpacity={0.6}
      style={styles.row}
      accessibilityRole="button"
      accessibilityLabel={label}
    >
      <View style={styles.iconWrap}>{icon}</View>
      <Text preset="bodyEmphasis" color="text" style={{ flex: 1 }}>
        {label}
      </Text>
      <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  center: { flex: 1, justifyContent: "center" },
  card: { marginHorizontal: theme.spacing.lg, marginTop: theme.spacing.xs },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    minHeight: 56,
    gap: theme.spacing.md,
  },
  iconWrap: { width: 22, alignItems: "center" },
  actions: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.xl,
  },
});
