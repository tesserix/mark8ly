import { useState, useCallback, useMemo } from "react";
import {
  View,
  Text,
  TextInput,
  Pressable,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useCheckGiftCardBalance } from "@/lib/hooks/use-checkout";
import { useCheckoutStore, type GiftCardCredit } from "@/stores/checkout-store";

export function GiftCardInput() {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  const [code, setCode] = useState("");
  const giftCard = useCheckoutStore((s) => s.giftCard);
  const setGiftCard = useCheckoutStore((s) => s.setGiftCard);
  const clearGiftCard = useCheckoutStore((s) => s.clearGiftCard);
  const checkBalance = useCheckGiftCardBalance();

  const handleCheck = useCallback(async () => {
    if (!code.trim()) return;

    try {
      const result = await checkBalance.mutateAsync(code.trim());
      const balance = parseFloat(result.data.balance);
      if (balance > 0) {
        const credit: GiftCardCredit = {
          code: code.trim().toUpperCase(),
          amount: balance,
        };
        setGiftCard(credit);
        setCode("");
      }
    } catch {
      // Error handled by mutation state
    }
  }, [code, checkBalance, setGiftCard]);

  const handleRemove = useCallback(() => {
    clearGiftCard();
  }, [clearGiftCard]);

  if (giftCard) {
    return (
      <View style={styles.appliedRow}>
        <View style={styles.appliedBadge}>
          <Text style={styles.appliedCode}>{giftCard.code}</Text>
          <Text style={styles.appliedAmount}>
            ${giftCard.amount.toFixed(2)} balance
          </Text>
        </View>
        <Pressable onPress={handleRemove} accessibilityLabel="Remove gift card">
          <Text style={styles.removeText}>Remove</Text>
        </Pressable>
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <View style={styles.inputRow}>
        <TextInput
          style={styles.input}
          value={code}
          onChangeText={setCode}
          placeholder="Gift card code"
          placeholderTextColor={theme.textSecondary}
          autoCapitalize="characters"
          accessibilityLabel="Gift card code"
        />
        <Pressable
          style={[
            styles.checkButton,
            checkBalance.isPending && styles.checkButtonLoading,
          ]}
          onPress={handleCheck}
          disabled={checkBalance.isPending || !code.trim()}
          accessibilityRole="button"
          accessibilityLabel="Check gift card balance"
        >
          {checkBalance.isPending ? (
            <ActivityIndicator size="small" color={theme.primary} />
          ) : (
            <Text style={styles.checkText}>Check balance</Text>
          )}
        </Pressable>
      </View>
      {checkBalance.isError && (
        <Text style={styles.errorText}>Invalid gift card code</Text>
      )}
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  container: {
    gap: 6,
  },
  inputRow: {
    flexDirection: "row",
    gap: 8,
  },
  input: {
    flex: 1,
    borderWidth: 1,
    borderColor: theme.border,
    borderRadius: 6,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 14,
    color: theme.text,
    backgroundColor: theme.elevated,
  },
  checkButton: {
    borderWidth: 1,
    borderColor: theme.primary,
    paddingHorizontal: 12,
    paddingVertical: 10,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
  },
  checkButtonLoading: {
    opacity: 0.7,
  },
  checkText: {
    fontSize: 13,
    fontWeight: "600",
    color: theme.text,
  },
  errorText: {
    fontSize: 12,
    color: "#8B2500",
  },
  appliedRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  appliedBadge: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    backgroundColor: "#EDF4ED",
    paddingHorizontal: 10,
    paddingVertical: 6,
    borderRadius: 6,
  },
  appliedCode: {
    fontSize: 13,
    fontWeight: "700",
    color: theme.accent,
  },
  appliedAmount: {
    fontSize: 13,
    color: theme.accent,
  },
  removeText: {
    fontSize: 13,
    color: "#8B2500",
    fontWeight: "600",
  },
});
}
