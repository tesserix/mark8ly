import { View, TouchableOpacity, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { Text, StatusBadge } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { giftCardStatusTone, titleize } from "@/lib/gift-card-display";
import type { GiftCard } from "@repo/mobile-shared/api/types";

interface GiftCardRowProps {
  card: GiftCard;
  onPress: (card: GiftCard) => void;
}

export function GiftCardRow({ card, onPress }: GiftCardRowProps) {
  return (
    <TouchableOpacity
      style={styles.container}
      onPress={() => onPress(card)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`Gift card ${card.code_display}, ${card.status}`}
    >
      <View style={styles.info}>
        <View style={styles.topRow}>
          <Text preset="bodyEmphasis" color="text" style={styles.code} numberOfLines={1}>
            {card.code_display}
          </Text>
          <StatusBadge label={titleize(card.status)} tone={giftCardStatusTone(card.status)} />
        </View>
        <Text preset="caption" color="textSecondary" numberOfLines={1}>
          {formatMoney(card.current_balance, card.currency_code)} of{" "}
          {formatMoney(card.initial_balance, card.currency_code)}
        </Text>
      </View>
      <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  container: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
    gap: theme.spacing.md,
  },
  info: { flex: 1, gap: 4 },
  topRow: { flexDirection: "row", alignItems: "center", gap: theme.spacing.sm },
  code: { flexShrink: 1, fontVariant: ["tabular-nums"] },
});
