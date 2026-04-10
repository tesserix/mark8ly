import { useCallback } from "react";
import { View, Text, Pressable, StyleSheet, Switch, ActivityIndicator } from "react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useLoyaltyBalance } from "@/lib/hooks/use-checkout";
import { useCheckoutStore } from "@/stores/checkout-store";

export function LoyaltyRedemption() {
  const { user } = useAuth();
  const isAuthenticated = !!user;
  const { data: loyalty, isLoading } = useLoyaltyBalance();
  const loyaltyPoints = useCheckoutStore((s) => s.loyaltyPoints);
  const setLoyaltyPoints = useCheckoutStore((s) => s.setLoyaltyPoints);

  const handleToggle = useCallback(
    (enabled: boolean) => {
      if (!loyalty) return;
      setLoyaltyPoints(enabled ? loyalty.points_balance : 0);
    },
    [loyalty, setLoyaltyPoints],
  );

  if (!isAuthenticated) return null;

  if (isLoading) {
    return (
      <View style={styles.container}>
        <ActivityIndicator size="small" color="#0E0E0C" />
      </View>
    );
  }

  if (!loyalty || loyalty.points_balance <= 0) return null;

  const currencyValue = loyalty.points_balance * loyalty.currency_per_point;
  const isActive = loyaltyPoints > 0;

  return (
    <View style={styles.container}>
      <View style={styles.row}>
        <View style={styles.info}>
          <Text style={styles.label}>Redeem loyalty points</Text>
          <Text style={styles.balance}>
            {loyalty.points_balance.toLocaleString()} points available (${currencyValue.toFixed(2)})
          </Text>
        </View>
        <Switch
          value={isActive}
          onValueChange={handleToggle}
          trackColor={{ false: "#E5E4DF", true: "#2D4A2B" }}
          thumbColor="#FFFFFF"
          accessibilityLabel="Redeem loyalty points"
        />
      </View>
      {isActive && (
        <View style={styles.appliedBanner}>
          <Text style={styles.appliedText}>
            Redeeming {loyalty.points_balance.toLocaleString()} points (-${currencyValue.toFixed(2)})
          </Text>
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    gap: 8,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  info: {
    flex: 1,
    gap: 2,
    marginRight: 12,
  },
  label: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  balance: {
    fontSize: 12,
    color: "#666666",
  },
  appliedBanner: {
    backgroundColor: "#EDF4ED",
    paddingHorizontal: 10,
    paddingVertical: 6,
    borderRadius: 6,
  },
  appliedText: {
    fontSize: 13,
    color: "#2D4A2B",
    fontWeight: "500",
  },
});
