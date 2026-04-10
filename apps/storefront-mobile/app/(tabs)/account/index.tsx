import { useMemo } from "react";
import {
  View,
  Text,
  StyleSheet,
  Pressable,
  ActivityIndicator,
  ScrollView,
} from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useRouter } from "expo-router";
import { useCustomerProfile } from "@/lib/hooks/use-customer";
import {
  ShoppingBag,
  MapPin,
  Heart,
  Star,
  MessageSquare,
  LogOut,
} from "lucide-react-native";

interface QuickLinkItem {
  label: string;
  icon: typeof ShoppingBag;
  route: string;
}

const QUICK_LINKS: QuickLinkItem[] = [
  { label: "Orders", icon: ShoppingBag, route: "/(tabs)/account/orders" },
  { label: "Addresses", icon: MapPin, route: "/(tabs)/account/addresses" },
  { label: "Wishlist", icon: Heart, route: "/(tabs)/account/wishlist" },
  { label: "Loyalty", icon: Star, route: "/(tabs)/account/loyalty" },
  { label: "Reviews", icon: MessageSquare, route: "/(tabs)/account/reviews" },
];

export default function AccountDashboardScreen() {
  const theme = useTheme();
  const { user, signOut, loading: authLoading } = useAuth();
  const router = useRouter();

  const themed = useMemo(
    () => ({
      centered: { backgroundColor: theme.background },
      guestTitle: { color: theme.text },
      guestSubtitle: { color: theme.textSecondary },
      signInButton: { backgroundColor: theme.primary },
      signInButtonText: { color: theme.elevated },
      createAccountLink: { color: theme.accent },
    }),
    [theme],
  );

  if (authLoading) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <ActivityIndicator size="large" color={theme.primary} />
      </View>
    );
  }

  if (!user) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <Text style={[styles.guestTitle, themed.guestTitle]}>Sign in to your account</Text>
        <Text style={[styles.guestSubtitle, themed.guestSubtitle]}>
          View orders, manage addresses, and more.
        </Text>
        <Pressable
          style={[styles.signInButton, themed.signInButton]}
          onPress={() => router.push("/(auth)/login")}
          accessibilityRole="button"
          accessibilityLabel="Sign in"
        >
          <Text style={[styles.signInButtonText, themed.signInButtonText]}>Sign in</Text>
        </Pressable>
        <Pressable
          onPress={() => router.push("/(auth)/register")}
          accessibilityRole="link"
          accessibilityLabel="Create account"
        >
          <Text style={[styles.createAccountLink, themed.createAccountLink]}>Create account</Text>
        </Pressable>
      </View>
    );
  }

  return <AuthenticatedDashboard onSignOut={signOut} />;
}

function AuthenticatedDashboard({ onSignOut }: { onSignOut: () => void }) {
  const theme = useTheme();
  const router = useRouter();
  const { data: profile, isLoading } = useCustomerProfile();

  const themed = useMemo(
    () => ({
      container: { backgroundColor: theme.background },
      avatar: { backgroundColor: theme.primary },
      avatarText: { color: theme.background },
      profileName: { color: theme.text },
      profileEmail: { color: theme.textSecondary },
      divider: { backgroundColor: theme.border },
      gridItem: { backgroundColor: theme.elevated, borderColor: theme.border },
      gridLabel: { color: theme.text },
    }),
    [theme],
  );

  const handleSignOut = async () => {
    await onSignOut();
    router.replace("/(tabs)");
  };

  const displayName = profile
    ? `${profile.first_name} ${profile.last_name}`.trim()
    : "Account";

  return (
    <ScrollView
      style={[styles.container, themed.container]}
      contentContainerStyle={styles.content}
      showsVerticalScrollIndicator={false}
    >
      <View style={styles.profileHeader}>
        <View style={[styles.avatar, themed.avatar]}>
          <Text style={[styles.avatarText, themed.avatarText]}>
            {isLoading ? "..." : (profile?.first_name?.[0] ?? "A").toUpperCase()}
          </Text>
        </View>
        <View style={styles.profileInfo}>
          <Text style={[styles.profileName, themed.profileName]}>{displayName}</Text>
          {profile?.email && (
            <Text style={[styles.profileEmail, themed.profileEmail]}>{profile.email}</Text>
          )}
        </View>
      </View>

      <View style={[styles.divider, themed.divider]} />

      <View style={styles.grid}>
        {QUICK_LINKS.map((link) => (
          <Pressable
            key={link.route}
            style={[styles.gridItem, themed.gridItem]}
            onPress={() => router.push(link.route as never)}
            accessibilityRole="button"
            accessibilityLabel={link.label}
          >
            <link.icon size={24} color={theme.text} />
            <Text style={[styles.gridLabel, themed.gridLabel]}>{link.label}</Text>
          </Pressable>
        ))}
        <Pressable
          style={[styles.gridItem, themed.gridItem]}
          onPress={handleSignOut}
          accessibilityRole="button"
          accessibilityLabel="Sign out"
        >
          <LogOut size={24} color="#8B2020" />
          <Text style={[styles.gridLabel, styles.logoutLabel]}>Sign out</Text>
        </Pressable>
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
  },
  content: {
    padding: 20,
    paddingBottom: 40,
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    gap: 12,
  },
  guestTitle: {
    fontSize: 20,
    fontWeight: "700",
    fontFamily: "SourceSerif4",
  },
  guestSubtitle: {
    fontSize: 15,
    textAlign: "center",
    lineHeight: 22,
    marginBottom: 8,
  },
  signInButton: {
    height: 48,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 48,
    marginTop: 4,
  },
  signInButtonText: {
    fontSize: 16,
    fontWeight: "600",
  },
  createAccountLink: {
    fontSize: 14,
    fontWeight: "600",
    marginTop: 8,
  },
  profileHeader: {
    flexDirection: "row",
    alignItems: "center",
    gap: 14,
  },
  avatar: {
    width: 56,
    height: 56,
    borderRadius: 28,
    alignItems: "center",
    justifyContent: "center",
  },
  avatarText: {
    fontSize: 22,
    fontWeight: "700",
  },
  profileInfo: {
    flex: 1,
    gap: 2,
  },
  profileName: {
    fontSize: 18,
    fontWeight: "700",
  },
  profileEmail: {
    fontSize: 14,
  },
  divider: {
    height: StyleSheet.hairlineWidth,
    marginVertical: 20,
  },
  grid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 12,
  },
  gridItem: {
    width: "47%",
    borderRadius: 6,
    padding: 20,
    gap: 10,
    borderWidth: StyleSheet.hairlineWidth,
  },
  gridLabel: {
    fontSize: 15,
    fontWeight: "600",
  },
  logoutLabel: {
    color: "#8B2020",
  },
});
