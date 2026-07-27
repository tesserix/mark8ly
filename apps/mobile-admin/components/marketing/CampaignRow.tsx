import { View, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { PressableRow, Text, StatusBadge } from "@/components/ui";
import { theme } from "@/lib/theme";
import { campaignStatusTone, titleizeStatus } from "@/lib/campaign-display";
import type { Campaign } from "@repo/mobile-shared/api/types";

interface CampaignRowProps {
  campaign: Campaign;
  onPress: (campaign: Campaign) => void;
}

export function CampaignRow({ campaign, onPress }: CampaignRowProps) {
  const sub =
    campaign.status === "sent"
      ? `${campaign.delivered} delivered · ${campaign.opened} opened`
      : campaign.subject || "No subject yet";
  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(campaign)}
      style={styles.row}
      testID={`campaign-row-${campaign.id}`}
      accessibilityLabel={`Campaign ${campaign.name}, ${campaign.status}`}
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
