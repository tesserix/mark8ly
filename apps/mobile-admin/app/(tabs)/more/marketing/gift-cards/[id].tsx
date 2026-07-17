import { View, ScrollView, ActivityIndicator, StyleSheet } from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useGiftCard } from "@/lib/hooks/use-gift-cards";
import { BackHeader, Eyebrow, Hairline, Screen, StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { giftCardStatusTone, titleize } from "@/lib/gift-card-display";
import type { GiftCardTxn } from "@repo/mobile-shared/api/types";

function formatDate(iso?: string): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleDateString("en-AU", { year: "numeric", month: "short", day: "numeric" });
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.row}>
      <Text preset="body" color="textSecondary">
        {label}
      </Text>
      <Text preset="body" color="text">
        {value}
      </Text>
    </View>
  );
}

function TxnRow({ txn, currency }: { txn: GiftCardTxn; currency: string }) {
  return (
    <View style={styles.txn}>
      <View style={styles.txnInfo}>
        <Text preset="bodyEmphasis" color="text">
          {titleize(txn.type)}
        </Text>
        <Text preset="caption" color="textTertiary">
          {formatDate(txn.created_at)}
          {txn.note ? ` · ${txn.note}` : ""}
        </Text>
      </View>
      <View style={styles.txnAmounts}>
        <Text preset="bodyEmphasis" color="text">
          {formatMoney(txn.amount, currency)}
        </Text>
        <Text preset="caption" color="textTertiary">
          Bal {formatMoney(txn.balance_after, currency)}
        </Text>
      </View>
    </View>
  );
}

export default function GiftCardDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: card, isLoading, error } = useGiftCard(id);

  if (error) {
    return (
      <Screen>
        <BackHeader eyebrow="GIFT CARD" />
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load gift card
          </Text>
        </View>
      </Screen>
    );
  }

  if (isLoading || !card) {
    return (
      <Screen>
        <BackHeader eyebrow="GIFT CARD" title="Loading…" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  const currency = card.currency_code;
  const expiresAt = formatDate(card.expires_at);
  const recipient = card.recipient_name || card.recipient_email;
  const sender = card.sender_name || card.sender_email;

  return (
    <Screen>
      <BackHeader eyebrow="GIFT CARD" title={card.code_display} />
      <ScrollView contentContainerStyle={styles.scroll}>
        <View style={styles.heading}>
          <Text preset="h1" color="text">
            {formatMoney(card.current_balance, currency)}
          </Text>
          <StatusBadge label={titleize(card.status)} tone={giftCardStatusTone(card.status)} />
          <Text preset="body" color="textSecondary">
            of {formatMoney(card.initial_balance, currency)} · {card.code_display}
          </Text>
        </View>

        <Eyebrow label="Details" style={styles.section} />
        <View style={styles.card}>
          {recipient ? <Row label="Recipient" value={recipient} /> : null}
          {sender ? <Row label="From" value={sender} /> : null}
          {card.message ? <Row label="Message" value={card.message} /> : null}
          <Row label="Expires" value={expiresAt ?? "No expiry"} />
          <Row label="Source" value={card.purchased_via_storefront ? "Storefront purchase" : "Admin issued"} />
        </View>

        <Eyebrow label="Transactions" style={styles.section} />
        <View style={styles.card}>
          {card.transactions.length === 0 ? (
            <Text preset="body" color="textTertiary">
              No transactions yet.
            </Text>
          ) : (
            card.transactions.map((txn, i) => (
              <View key={txn.id}>
                {i > 0 ? <Hairline /> : null}
                <TxnRow txn={txn} currency={currency} />
              </View>
            ))
          )}
        </View>
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingBottom: theme.spacing.huge },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  heading: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.lg, gap: theme.spacing.sm },
  section: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.xl },
  card: { paddingHorizontal: theme.spacing.lg, marginTop: theme.spacing.sm },
  row: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: theme.spacing.xs,
    gap: theme.spacing.md,
  },
  txn: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: theme.spacing.md,
    gap: theme.spacing.md,
  },
  txnInfo: { flex: 1, gap: 2 },
  txnAmounts: { alignItems: "flex-end", gap: 2 },
});
