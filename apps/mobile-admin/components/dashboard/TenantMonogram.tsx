import { View, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useNotifications } from "@/lib/hooks/use-notifications";
import { IconButton, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

interface TenantMonogramProps {
  /** The active store's name — its first letter becomes the monogram. */
  storeName?: string;
}

/** Disc diameter. Sits inside IconButton's real 44pt touch box. */
const DISC = 40;
const BADGE_MIN = 16;

function initial(name: string | undefined): string {
  const trimmed = name?.trim() ?? "";
  return trimmed ? trimmed.charAt(0).toUpperCase() : "?";
}

/**
 * The Dashboard header's right slot: the store's serif monogram on an ink
 * disc, carrying the unread-notification badge. Replaces the standalone
 * `NotificationBell` on this screen only — the bell is still the right
 * affordance on screens that don't have a store identity to show.
 *
 * Three deliberate decisions:
 *
 * 1. ONE touch target, not two. The badge is layered ON the disc, so giving
 *    it its own `hitSlop` region would necessarily overlap the disc it sits
 *    on — the exact overlap the app-wide 44pt rule exists to prevent. The
 *    disc is therefore the only pressable (a real 44pt `minWidth`/
 *    `minHeight` box via `IconButton`), the badge is `accessible={false}`,
 *    and the unread count is folded into the button's own
 *    `accessibilityLabel`.
 * 2. The badge is NOT moss. This screen spends its one accent on the
 *    revenue chart and the Approve swipe; `NotificationBell`'s moss badge
 *    would be a third. `danger` (oxblood) is a functional token — "there is
 *    something unread" is a real state, not decoration — and paper on
 *    oxblood clears ~8.8:1.
 * 3. Pressing it opens the notifications inbox, which is what the badge
 *    counts. The monogram is visual identity, not a second destination.
 */
export function TenantMonogram({ storeName }: TenantMonogramProps) {
  const router = useRouter();
  const { data } = useNotifications();
  const unread = data?.notifications.filter((n) => !n.is_read).length ?? 0;

  return (
    <IconButton
      onPress={() => router.push("/notifications")}
      accessibilityLabel={
        unread > 0 ? `Notifications, ${unread} unread` : "Notifications"
      }
      testID="tenant-monogram"
    >
      <View style={styles.disc} accessible={false}>
        <Text preset="h3" color="inverse" style={styles.initial}>
          {initial(storeName)}
        </Text>
        {unread > 0 ? (
          <View style={styles.badge} accessible={false} testID="tenant-monogram-badge">
            <Text preset="caption" color="inverse" style={styles.badgeText}>
              {unread > 9 ? "9+" : String(unread)}
            </Text>
          </View>
        ) : null}
      </View>
    </IconButton>
  );
}

const styles = StyleSheet.create({
  disc: {
    width: DISC,
    height: DISC,
    borderRadius: DISC / 2,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
  // h3's 26pt line height would push the glyph off-centre in a 40pt disc;
  // pinning the line height to the disc keeps it optically centred.
  initial: { lineHeight: DISC, includeFontPadding: false },
  badge: {
    position: "absolute",
    top: -2,
    right: -2,
    minWidth: BADGE_MIN,
    height: BADGE_MIN,
    paddingHorizontal: 4,
    borderRadius: theme.radii.pill,
    backgroundColor: theme.colors.danger,
    alignItems: "center",
    justifyContent: "center",
    // Paper ring so the badge separates from the ink disc beneath it.
    borderWidth: 1.5,
    borderColor: theme.colors.background,
  },
  badgeText: {
    fontSize: 9,
    lineHeight: 12,
    fontWeight: "700",
  },
});
