import { View, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { PressableRow, Text, StatusBadge } from "@/components/ui";
import { theme } from "@/lib/theme";
import { campaignStatusTone, titleizeStatus } from "@/lib/campaign-display";
import type { Campaign } from "@repo/mobile-shared/api/types";

interface CampaignRowProps {
  campaign: Campaign;
  onPress: (campaign: Campaign) => void;
  /**
   * Opens the long-press action menu on the Campaigns screen. Optional so the
   * row stays usable on any list that has no per-row menu — the same contract
   * `ProductRow`, `CustomerRow` and `CouponRow` carry.
   */
  onLongPress?: (campaign: Campaign) => void;
}

export function CampaignRow({ campaign, onPress, onLongPress }: CampaignRowProps) {
  const sub =
    campaign.status === "sent"
      ? `${campaign.delivered} delivered · ${campaign.opened} opened`
      : campaign.subject || "No subject yet";
  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(campaign)}
      onLongPress={onLongPress ? () => onLongPress(campaign) : undefined}
      style={styles.row}
      testID={`campaign-row-${campaign.id}`}
      accessibilityLabel={`Campaign ${campaign.name}, ${campaign.status}`}
      // Announced only when there is actually a menu behind the gesture —
      // the row is also mounted with the handler suppressed while its own
      // delete is in flight, and promising actions that aren't there is worse
      // than promising none.
      accessibilityHint={onLongPress ? "Long press for more actions" : undefined}
    >
      <View style={styles.info}>
        <View style={styles.topRow}>
          <Text preset="bodyEmphasis" color="text" numberOfLines={1} style={styles.name}>
            {campaign.name}
          </Text>
          <StatusBadge label={titleizeStatus(campaign.status)} tone={campaignStatusTone(campaign.status)} />
        </View>
        <Text preset="caption" color="textSecondary" numberOfLines={1}>
          {sub}
        </Text>
      </View>
      <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  row: {
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
  },
  info: { flex: 1, gap: 4 },
  topRow: { flexDirection: "row", alignItems: "center", gap: theme.spacing.sm },
  name: { flexShrink: 1 },
});
