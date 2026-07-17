import { useCallback, useState } from "react";
import { ScrollView, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useIssueGiftCard } from "@/lib/admin-api/gift-card-actions";
import { BackHeader, FieldInput, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

export default function NewGiftCardScreen() {
  const router = useRouter();
  const storeCurrency = useTenantStore((s) => s.activeStore?.currency_code) || "AUD";
  const issue = useIssueGiftCard();

  const [amount, setAmount] = useState("");
  const [currency, setCurrency] = useState(storeCurrency);
  const [recipientName, setRecipientName] = useState("");
  const [recipientEmail, setRecipientEmail] = useState("");
  const [senderName, setSenderName] = useState("");
  const [message, setMessage] = useState("");

  const amountNum = Number(amount.trim());
  const canSubmit =
    Number.isFinite(amountNum) && amountNum > 0 && currency.trim().length === 3 && !issue.isPending;

  const onSubmit = useCallback(() => {
    if (!canSubmit) return;
    issue.mutate(
      {
        initial_balance: amountNum,
        currency_code: currency.trim().toUpperCase(),
        ...(recipientName.trim() ? { recipient_name: recipientName.trim() } : {}),
        ...(recipientEmail.trim() ? { recipient_email: recipientEmail.trim() } : {}),
        ...(senderName.trim() ? { sender_name: senderName.trim() } : {}),
        ...(message.trim() ? { message: message.trim() } : {}),
      },
      {
        onSuccess: () => router.back(),
        onError: (err) =>
          Alert.alert(
            "Couldn't issue gift card",
            err instanceof ApiError ? err.message : "Please try again.",
          ),
      },
    );
  }, [canSubmit, issue, amountNum, currency, recipientName, recipientEmail, senderName, message, router]);

  return (
    <Screen>
      <BackHeader eyebrow="MARKETING" title="Issue gift card" />
      <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
        <FieldInput
          label="Amount"
          value={amount}
          onChangeText={setAmount}
          placeholder="50"
          keyboardType="decimal-pad"
        />
        <FieldInput
          label="Currency"
          value={currency}
          onChangeText={(t) => setCurrency(t.toUpperCase())}
          placeholder="AUD"
          autoCapitalize="characters"
          maxLength={3}
        />
        <FieldInput
          label="Recipient name (optional)"
          value={recipientName}
          onChangeText={setRecipientName}
          placeholder="Who it's for"
        />
        <FieldInput
          label="Recipient email (optional)"
          value={recipientEmail}
          onChangeText={setRecipientEmail}
          placeholder="recipient@example.com"
          keyboardType="email-address"
          autoCapitalize="none"
        />
        <FieldInput
          label="From (optional)"
          value={senderName}
          onChangeText={setSenderName}
          placeholder="Sender name"
        />
        <FieldInput
          label="Message (optional)"
          value={message}
          onChangeText={setMessage}
          placeholder="A note for the recipient"
          multiline
        />
        <Text preset="caption" color="textTertiary">
          The card is issued immediately with no expiry. Set an expiry date from the web dashboard.
        </Text>
        <Pressable
          style={[styles.btn, !canSubmit && styles.disabled]}
          onPress={onSubmit}
          disabled={!canSubmit}
          accessibilityRole="button"
          accessibilityLabel="Issue gift card"
        >
          {issue.isPending ? (
            <ActivityIndicator size="small" color={theme.colors.inverse} />
          ) : (
            <Text preset="bodyEmphasis" color="inverse">
              Issue gift card
            </Text>
          )}
        </Pressable>
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: theme.spacing.lg, gap: theme.spacing.md, paddingBottom: theme.spacing.huge },
  btn: {
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
    marginTop: theme.spacing.sm,
  },
  disabled: { opacity: 0.4 },
});
