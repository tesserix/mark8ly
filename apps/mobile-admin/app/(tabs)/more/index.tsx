import { View, ScrollView, StyleSheet, Linking } from "react-native";
import { useRouter, type Href } from "expo-router";
import {
  Bell,
  BellRing,
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
import { GroupedList, GroupedRow, PageHeader, Screen, Text, type GroupedListSection } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";

const APP_VERSION = "1.0.0";

// Live legal pages served from mark8ly.com — surfaced in-app so the privacy
// policy stays reachable post-login (App Store 5.1.1(i) / Play requirement).
const PRIVACY_URL = "https://mark8ly.com/privacy";
const TERMS_URL = "https://mark8ly.com/terms";

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
      { key: "tickets", icon: Ticket, label: "Tickets", a11y: "Tickets — customer support requests", href: "/(tabs)/more/settings/tickets" },
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

  // The construction is more/index.tsx's own — GroupedList/GroupedRow are
  // this screen's hand-built eyebrow+Card+Hairline section loop and `Row`,
  // promoted into a shared primitive. This screen must render visually
  // unchanged: it is the primitive's SOURCE, not a consumer being migrated
  // to a new look.
  const sections: GroupedListSection[] = [
    ...SECTIONS.map((section) => ({
      key: section.title,
      label: section.title,
      rows: section.items.map((item) => {
        const Icon = item.icon;
        const showBadge = item.showUnread && unreadCount > 0;
        return (
          <GroupedRow
            key={item.key}
            icon={<Icon size={18} color={theme.colors.text} strokeWidth={1.75} />}
            label={item.label}
            accessibilityLabel={
              showBadge ? `${item.a11y}, ${unreadCount} unread` : item.a11y
            }
            onPress={() => router.push(item.href)}
            badge={showBadge ? (unreadCount > 99 ? "99+" : String(unreadCount)) : undefined}
          />
        );
      }),
    })),
    {
      key: "legal",
      label: "Legal",
      rows: [
        <GroupedRow
          key="privacy"
          icon={<FileText size={18} color={theme.colors.text} strokeWidth={1.75} />}
          label="Privacy Policy"
          accessibilityLabel="Privacy Policy — opens in your browser"
          accessibilityRole="link"
          onPress={() => Linking.openURL(PRIVACY_URL)}
        />,
        <GroupedRow
          key="terms"
          icon={<Scale size={18} color={theme.colors.text} strokeWidth={1.75} />}
          label="Terms of Service"
          accessibilityLabel="Terms of Service — opens in your browser"
          accessibilityRole="link"
          onPress={() => Linking.openURL(TERMS_URL)}
        />,
      ],
    },
  ];

  return (
    <Screen>
      <PageHeader eyebrow="MORE" title="Settings" />
      <ScrollView contentContainerStyle={[styles.body, { paddingBottom: dockPad }]}>
        <GroupedList sections={sections} />

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
    // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH so
    // rows sit flush with PageHeader above. Not theme.spacing.lg — that
    // token is shared with non-gutter spacing throughout the app.
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xs,
    // Spaces GroupedList from the footer below it. GroupedList owns the gap
    // BETWEEN its own sections internally (also theme.spacing.lg), so the
    // rhythm here is identical to the pre-extraction hand-built section loop.
    gap: theme.spacing.lg,
  },
  footer: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: theme.spacing.xs,
    paddingVertical: theme.spacing.huge,
  },
});
