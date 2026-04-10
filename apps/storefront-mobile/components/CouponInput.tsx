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
import { useValidateCoupon } from "@/lib/hooks/use-checkout";
import { useCheckoutStore, type CouponDiscount } from "@/stores/checkout-store";

export function CouponInput() {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  const [code, setCode] = useState("");
  const coupon = useCheckoutStore((s) => s.coupon);
  const setCoupon = useCheckoutStore((s) => s.setCoupon);
  const clearCoupon = useCheckoutStore((s) => s.clearCoupon);
  const validate = useValidateCoupon();

  const handleApply = useCallback(async () => {
    if (!code.trim()) return;

    try {
      const result = await validate.mutateAsync(code.trim());
      const disc = result.data;
      if (disc.valid) {
        const applied: CouponDiscount = {
          code: code.trim().toUpperCase(),
          amount: parseFloat(disc.discount_amount),
          type: disc.discount_type,
        };
        setCoupon(applied);
        setCode("");
      }
    } catch {
      // Error handled by mutation state
    }
  }, [code, validate, setCoupon]);

  const handleRemove = useCallback(() => {
    clearCoupon();
  }, [clearCoupon]);

  if (coupon) {
    return (
      <View style={styles.appliedRow}>
        <View style={styles.appliedBadge}>
          <Text style={styles.appliedCode}>{coupon.code}</Text>
          <Text style={styles.appliedDiscount}>
            {coupon.type === "percent"
              ? `${coupon.amount}% off`
              : `-$${coupon.amount.toFixed(2)}`}
          </Text>
        </View>
        <Pressable onPress={handleRemove} accessibilityLabel="Remove coupon">
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
          placeholder="Coupon code"
          placeholderTextColor={theme.textSecondary}
          autoCapitalize="characters"
          accessibilityLabel="Coupon code"
        />
        <Pressable
          style={[styles.applyButton, validate.isPending && styles.applyButtonLoading]}
          onPress={handleApply}
          disabled={validate.isPending || !code.trim()}
          accessibilityRole="button"
          accessibilityLabel="Apply coupon"
        >
          {validate.isPending ? (
            <ActivityIndicator size="small" color={theme.elevated} />
          ) : (
            <Text style={styles.applyText}>Apply</Text>
          )}
        </Pressable>
      </View>
      {validate.isError && (
        <Text style={styles.errorText}>Invalid coupon code</Text>
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
  applyButton: {
    backgroundColor: theme.primary,
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
  },
  applyButtonLoading: {
    opacity: 0.7,
  },
  applyText: {
    color: theme.elevated,
    fontSize: 14,
    fontWeight: "600",
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
  appliedDiscount: {
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
