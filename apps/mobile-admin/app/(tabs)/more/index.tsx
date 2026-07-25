import { View, ScrollView, TouchableOpacity, StyleSheet, Linking } from "react-native";
import { useRouter, type Href } from "expo-router";
import {
  Bell,
  BellRing,
  ChevronRight,
  FileText,
  LifeBuoy,
  Megaphone,
  Palette,
  Scale,
  ScrollText,
  Settings,
  ShieldCheck,
  Ticket,
  UserRound,
  Users,
  type LucideIcon,
} from "lucide-react-native";
import { useNotifications } from "../../../lib/hooks/use-notifications";
import { Card, Hairline, PageHeader, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";

const APP_VERSION = "1.0.0";

// Live legal pages served from mark8ly.com — surfaced in-app so the privacy
// policy stays reachable post-login (App Store 5.1.1(i) / Play requirement).
const PRIVACY_URL = "https://mark8ly.com/privacy";
const TERMS_URL = "https://mark8ly.com/terms";

interface RowProps {
  icon: React.ReactNode;
  label: string;
  trailing?: React.ReactNode;
  onPress: () => void;
  accessibilityLabel: string;
}

function Row({ icon, label, trailing, onPress, accessibilityLabel }: RowProps) {
  return (
    <TouchableOpacity
      style={styles.row}
      onPress={onPress}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
    >
      <View style={styles.rowIcon}>{icon}</View>
      <Text preset="bodyEmphasis" color="text" style={styles.rowLabel}>
        {label}
      </Text>
      {trailing ? <View style={styles.rowTrailing}>{trailing}</View> : null}
      <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
    </TouchableOpacity>
  );
}

interface NavItem {
  key: string;
  icon: LucideIcon;
  label: string;
  a11y: string;
  href: Href;
  /** Only the inbox row uses this — renders the unread pill. */
  showUnread?: boolean;
}

// Grouped sections replace the old flat list. Store settings is flattened up
// one level (no intermediate hub); the two notification entries are named
// apart — "Notifications" is the inbox, "Notification settings" is the
// per-type push controls.
const SECTIONS: ReadonlyArray<{ title: string; items: readonly NavItem[] }> = [
  {
    title: "Store",
    items: [
      { key: "branding", icon: Palette, label: "Branding", a11y: "Branding — storefront tagline, announcement, socials", href: "/(tabs)/more/settings/branding" },
      { key: "team", icon: Users, label: "Team", a11y: "Team — members, roles and invitations", href: "/(tabs)/more/settings/team" },
      { key: "tickets", icon: Ticket, label: "Support tickets", a11y: "Support tickets — customer support requests", href: "/(tabs)/more/settings/tickets" },
      { key: "audit", icon: ScrollText, label: "Audit log", a11y: "Audit log — recent activity in your store", href: "/(tabs)/more/settings/audit-logs" },
      { key: "notif-settings", icon: BellRing, label: "Notification settings", a11y: "Notification settings — which alerts your store sends", href: "/(tabs)/more/settings/notification-settings" },
    ],
  },
  {
    title: "General",
    items: [
      { key: "marketing", icon: Megaphone, label: "Marketing", a11y: "Marketing — coupons, campaigns, loyalty and more", href: "/(tabs)/more/marketing" },
      { key: "notifications", icon: Bell, label: "Notifications", a11y: "Notifications inbox", href: "/notifications", showUnread: true },
      { key: "account", icon: UserRound, label: "Account", a11y: "Account settings", href: "/(tabs)/more/account" },
      { key: "security", icon: ShieldCheck, label: "Security", a11y: "Security and sign-in methods", href: "/(tabs)/more/security" },
    ],
  },
  {
    title: "Platform",
    items: [
      { key: "support", icon: LifeBuoy, label: "Tesserix Support", a11y: "Chat with Tesserix platform support", href: "/(tabs)/more/support" },
    ],
  },
];

export default function MoreScreen() {
  const router = useRouter();
  const dockPad = useDockClearance();
  const { data: notifications } = useNotifications();
  const unreadCount = notifications?.notifications.filter((n) => !n.is_read).length ?? 0;

  return (
    <Screen>
      <PageHeader eyebrow="MORE" title="Settings" />
      <ScrollView contentContainerStyle={[styles.body, { paddingBottom: dockPad }]}>
        {SECTIONS.map((section) => (
          <View key={section.title} style={styles.section}>
            <Text preset="eyebrow" color="textTertiary" style={styles.sectionLabel}>
              {section.title}
            </Text>
            <Card padding={0}>
              {section.items.map((item, i) => {
                const Icon = item.icon;
                const showBadge = item.showUnread && unreadCount > 0;
                return (
                  <View key={item.key}>
                    {i > 0 ? (
                      <Hairline inset={theme.spacing.huge + theme.spacing.xs} />
                    ) : null}
                    <Row
                      icon={<Icon size={18} color={theme.colors.text} strokeWidth={1.75} />}
                      label={item.label}
                      accessibilityLabel={
                        showBadge ? `${item.a11y}, ${unreadCount} unread` : item.a11y
                      }
                      onPress={() => router.push(item.href)}
                      trailing={
                        showBadge ? (
                          <View style={styles.badge}>
                            <Text preset="caption" color="inverse" style={styles.badgeLabel}>
                              {unreadCount > 99 ? "99+" : String(unreadCount)}
                            </Text>
                          </View>
                        ) : null
                      }
                    />
                  </View>
                );
              })}
            </Card>
          </View>
        ))}

        <View style={styles.section}>
          <Text preset="eyebrow" color="textTertiary" style={styles.sectionLabel}>
            Legal
          </Text>
          <Card padding={0}>
            <Row
              icon={<FileText size={18} color={theme.colors.text} strokeWidth={1.75} />}
              label="Privacy Policy"
              accessibilityLabel="Privacy Policy — opens in your browser"
              onPress={() => Linking.openURL(PRIVACY_URL)}
            />
            <Hairline inset={theme.spacing.huge + theme.spacing.xs} />
            <Row
              icon={<Scale size={18} color={theme.colors.text} strokeWidth={1.75} />}
              label="Terms of Service"
              accessibilityLabel="Terms of Service — opens in your browser"
              onPress={() => Linking.openURL(TERMS_URL)}
            />
          </Card>
        </View>

        <View style={styles.footer}>
          <Settings size={14} color={theme.colors.textTertiary} strokeWidth={1.75} />
          <Text preset="caption" color="textTertiary">
            Mark8ly Admin · v{APP_VERSION}
          </Text>
        </View>
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  body: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.xs,
    gap: theme.spacing.lg,
  },
  section: { gap: theme.spacing.sm },
  sectionLabel: { paddingHorizontal: theme.spacing.xs },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    minHeight: 56,
    gap: theme.spacing.md,
  },
  rowIcon: { width: 22, alignItems: "center" },
  rowLabel: { flex: 1 },
  rowTrailing: { marginRight: theme.spacing.xs },
  badge: {
    backgroundColor: theme.colors.danger,
    borderRadius: 10,
    minWidth: 22,
    height: 20,
    paddingHorizontal: theme.spacing.xs,
    alignItems: "center",
    justifyContent: "center",
  },
  badgeLabel: { fontSize: 10, fontWeight: "700" },
  footer: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: theme.spacing.xs,
    paddingVertical: theme.spacing.huge,
  },
});
