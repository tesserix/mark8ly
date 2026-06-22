import { View, TouchableOpacity, Linking, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import {
  Bell,
  ChevronRight,
  ExternalLink,
  LifeBuoy,
  Settings,
  UserRound,
} from "lucide-react-native";
import { useNotifications } from "../../../lib/hooks/use-notifications";
import { useEnvironment } from "@repo/mobile-shared/config/env";
import {
  Card,
  Hairline,
  PageHeader,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";

const APP_VERSION = "1.0.0";

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

export default function MoreScreen() {
  const router = useRouter();
  const env = useEnvironment();
  const { data: notifications } = useNotifications();
  const unreadCount = notifications?.items.filter((n) => !n.read).length ?? 0;

  const browserUrl = env.adminWebUrl;

  return (
    <Screen>
      <PageHeader eyebrow="MORE" title="Settings" />
      <Card padding={0} style={styles.card}>
        <Row
          icon={<Bell size={18} color={theme.colors.text} strokeWidth={1.75} />}
          label="Notifications"
          accessibilityLabel={`Notifications${unreadCount > 0 ? `, ${unreadCount} unread` : ""}`}
          onPress={() => router.push("/(tabs)/more/notifications")}
          trailing={
            unreadCount > 0 ? (
              <View style={styles.badge}>
                <Text preset="caption" color="inverse" style={styles.badgeLabel}>
                  {unreadCount > 99 ? "99+" : String(unreadCount)}
                </Text>
              </View>
            ) : null
          }
        />
        <Hairline inset={theme.spacing.huge + theme.spacing.xs} />
        <Row
          icon={<UserRound size={18} color={theme.colors.text} strokeWidth={1.75} />}
          label="Account"
          accessibilityLabel="Account settings"
          onPress={() => router.push("/(tabs)/more/account")}
        />
        <Hairline inset={theme.spacing.huge + theme.spacing.xs} />
        <Row
          icon={<LifeBuoy size={18} color={theme.colors.text} strokeWidth={1.75} />}
          label="Tesserix Support"
          accessibilityLabel="Chat with Tesserix platform support"
          onPress={() => router.push("/(tabs)/more/support")}
        />
        <Hairline inset={theme.spacing.huge + theme.spacing.xs} />
        <Row
          icon={<ExternalLink size={18} color={theme.colors.text} strokeWidth={1.75} />}
          label="Open in Browser"
          accessibilityLabel="Open admin dashboard in browser"
          onPress={() => Linking.openURL(browserUrl)}
        />
      </Card>

      <View style={styles.footer}>
        <Settings size={14} color={theme.colors.textTertiary} strokeWidth={1.75} />
        <Text preset="caption" color="textTertiary">
          Mark8ly Admin · v{APP_VERSION}
        </Text>
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  card: {
    marginHorizontal: theme.spacing.lg,
    marginTop: theme.spacing.xs,
  },
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
    marginTop: "auto",
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: theme.spacing.xs,
    paddingVertical: theme.spacing.huge,
  },
});
