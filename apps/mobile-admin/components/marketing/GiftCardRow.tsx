import { View, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { PressableRow, Text, StatusBadge } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { giftCardStatusTone, titleize } from "@/lib/gift-card-display";
import type { GiftCard } from "@repo/mobile-shared/api/types";

interface GiftCardRowProps {
  card: GiftCard;
  onPress: (card: GiftCard) => void;
  /**
   * Opens the row's action menu. OPTIONAL, and the screen passes `undefined`
   * while that card's own request is in flight — `SwipeRow.enabled` does not
   * reach this handler, so the busy guard has to be applied here separately
   * or the menu stays a live second route onto a row already being changed.
   */
  onLongPress?: (card: GiftCard) => void;
}

export function GiftCardRow({ card, onPress, onLongPress }: GiftCardRowProps) {
  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(card)}
      onLongPress={onLongPress ? () => onLongPress(card) : undefined}
      style={styles.row}
      testID={`gift-card-row-${card.id}`}
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
  code: { flexShrink: 1, fontVariant: ["tabular-nums"] },
});
