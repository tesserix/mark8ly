import { ScrollView, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { Palette, ScrollText, LifeBuoy, Users, Bell } from "lucide-react-native";
import { BackHeader, Screen } from "@/components/ui";
import { MarketingRow } from "@/components/marketing/MarketingRow";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";

const ICON_SIZE = 20;
const ICON_STROKE = 1.75;

export default function StoreSettingsHubScreen() {
  const router = useRouter();
  const dockPad = useDockClearance();

  return (
    <Screen>
      <BackHeader eyebrow="MORE" title="Store settings" />
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
        <MarketingRow
          icon={<Palette size={ICON_SIZE} color={theme.colors.text} strokeWidth={ICON_STROKE} />}
          label="Branding"
          description="Storefront tagline, announcement, socials"
          onPress={() => router.push("/(tabs)/more/settings/branding")}
        />
        <MarketingRow
          icon={<Bell size={ICON_SIZE} color={theme.colors.text} strokeWidth={ICON_STROKE} />}
          label="Notifications"
          description="Push alerts for orders, stock and more"
          onPress={() => router.push("/(tabs)/more/settings/notifications")}
        />
        <MarketingRow
          icon={<Users size={ICON_SIZE} color={theme.colors.text} strokeWidth={ICON_STROKE} />}
          label="Team"
          description="Members, roles and invitations"
          onPress={() => router.push("/(tabs)/more/settings/team")}
        />
        <MarketingRow
          icon={<LifeBuoy size={ICON_SIZE} color={theme.colors.text} strokeWidth={ICON_STROKE} />}
          label="Support tickets"
          description="Customer support requests"
          onPress={() => router.push("/(tabs)/more/settings/tickets")}
        />
        <MarketingRow
          icon={<ScrollText size={ICON_SIZE} color={theme.colors.text} strokeWidth={ICON_STROKE} />}
          label="Audit log"
          description="Recent activity in your store"
          onPress={() => router.push("/(tabs)/more/settings/audit-logs")}
        />
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingTop: theme.spacing.sm },
});
