import {
  View,
  Text,
  StyleSheet,
  Pressable,
  ActivityIndicator,
  ScrollView,
} from "react-native";
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
  const { user, signOut, loading: authLoading } = useAuth();
  const router = useRouter();

  if (authLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#0E0E0C" />
      </View>
    );
  }

  if (!user) {
    return (
      <View style={styles.centered}>
        <Text style={styles.guestTitle}>Sign in to your account</Text>
        <Text style={styles.guestSubtitle}>
          View orders, manage addresses, and more.
        </Text>
        <Pressable
          style={styles.signInButton}
          onPress={() => router.push("/(auth)/login")}
          accessibilityRole="button"
          accessibilityLabel="Sign in"
        >
          <Text style={styles.signInButtonText}>Sign in</Text>
        </Pressable>
        <Pressable
          onPress={() => router.push("/(auth)/register")}
          accessibilityRole="link"
          accessibilityLabel="Create account"
        >
          <Text style={styles.createAccountLink}>Create account</Text>
        </Pressable>
      </View>
    );
  }

  return <AuthenticatedDashboard onSignOut={signOut} />;
}

function AuthenticatedDashboard({ onSignOut }: { onSignOut: () => void }) {
  const router = useRouter();
  const { data: profile, isLoading } = useCustomerProfile();

  const handleSignOut = async () => {
    await onSignOut();
    router.replace("/(tabs)");
  };

  const displayName = profile
    ? `${profile.first_name} ${profile.last_name}`.trim()
    : "Account";

  return (
    <ScrollView
      style={styles.container}
      contentContainerStyle={styles.content}
      showsVerticalScrollIndicator={false}
    >
      <View style={styles.profileHeader}>
        <View style={styles.avatar}>
          <Text style={styles.avatarText}>
            {isLoading ? "..." : (profile?.first_name?.[0] ?? "A").toUpperCase()}
          </Text>
        </View>
        <View style={styles.profileInfo}>
          <Text style={styles.profileName}>{displayName}</Text>
          {profile?.email && (
            <Text style={styles.profileEmail}>{profile.email}</Text>
          )}
        </View>
      </View>

      <View style={styles.divider} />

      <View style={styles.grid}>
        {QUICK_LINKS.map((link) => (
          <Pressable
            key={link.route}
            style={styles.gridItem}
            onPress={() => router.push(link.route as never)}
            accessibilityRole="button"
            accessibilityLabel={link.label}
          >
            <link.icon size={24} color="#0E0E0C" />
            <Text style={styles.gridLabel}>{link.label}</Text>
          </Pressable>
        ))}
        <Pressable
          style={styles.gridItem}
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
    backgroundColor: "#F7F6F2",
  },
  content: {
    padding: 20,
    paddingBottom: 40,
  },
  centered: {
    flex: 1,
    backgroundColor: "#F7F6F2",
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    gap: 12,
  },
  guestTitle: {
    fontSize: 20,
    fontWeight: "700",
    color: "#0E0E0C",
    fontFamily: "SourceSerif4",
  },
  guestSubtitle: {
    fontSize: 15,
    color: "#666666",
    textAlign: "center",
    lineHeight: 22,
    marginBottom: 8,
  },
  signInButton: {
    backgroundColor: "#0E0E0C",
    height: 48,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 48,
    marginTop: 4,
  },
  signInButtonText: {
    color: "#FFFFFF",
    fontSize: 16,
    fontWeight: "600",
  },
  createAccountLink: {
    fontSize: 14,
    fontWeight: "600",
    color: "#2D4A2B",
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
    backgroundColor: "#0E0E0C",
    alignItems: "center",
    justifyContent: "center",
  },
  avatarText: {
    color: "#F7F6F2",
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
    color: "#0E0E0C",
  },
  profileEmail: {
    fontSize: 14,
    color: "#666666",
  },
  divider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: "#E5E4DF",
    marginVertical: 20,
  },
  grid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 12,
  },
  gridItem: {
    width: "47%",
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    padding: 20,
    gap: 10,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
  },
  gridLabel: {
    fontSize: 15,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  logoutLabel: {
    color: "#8B2020",
  },
});
