import { useState, useCallback, useMemo } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useRouter } from "expo-router";
import { haptics } from "@repo/mobile-shared/haptics/feedback";
import { useCheckoutStore } from "@/stores/checkout-store";
import { useCartStore } from "@/stores/cart-store";
import { useShippingRates } from "@/lib/hooks/use-checkout";
import type { ShippingRate } from "@repo/mobile-shared/api/storefront-types";

export default function CheckoutShippingScreen() {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  const router = useRouter();
  const checkoutStore = useCheckoutStore();
  const cartItems = useCartStore((s) => s.items);

  // Build address for the API from checkout store
  const apiAddress = useMemo(() => {
    const addr = checkoutStore.address;
    if (!addr) return null;
    return {
      name: checkoutStore.customerName,
      line1: addr.line1,
      line2: addr.line2 ?? "",
      city: addr.city,
      region: addr.state,
      postal_code: addr.postalCode,
      country: addr.country,
    };
  }, [checkoutStore.address, checkoutStore.customerName]);

  const { data: rates, isLoading, error, refetch } = useShippingRates(
    apiAddress,
    cartItems,
  );

  const [selectedRateId, setSelectedRateId] = useState<string | null>(
    checkoutStore.shippingRateId,
  );

  const handleSelect = useCallback((rateId: string) => {
    setSelectedRateId(rateId);
  }, []);

  const handleContinue = useCallback(async () => {
    if (!selectedRateId) return;

    await haptics.checkoutStep();
    checkoutStore.setShippingRate(selectedRateId);
    checkoutStore.setStep(3);
    router.push("/checkout/payment");
  }, [selectedRateId, checkoutStore, router]);

  // Loading state
  if (isLoading) {
    return (
      <View style={styles.container}>
        <ScrollView contentContainerStyle={styles.scrollContent}>
          <View style={styles.section}>
            <Text style={styles.sectionTitle}>Shipping method</Text>
            {Array.from({ length: 3 }).map((_, i) => (
              <View key={`skeleton-${i}`} style={styles.skeletonCard}>
                <View style={styles.skeletonLine} />
                <View style={styles.skeletonLineShort} />
              </View>
            ))}
          </View>
        </ScrollView>
      </View>
    );
  }

  // Error state
  if (error) {
    return (
      <View style={styles.centeredContainer}>
        <Text style={styles.errorTitle}>Unable to load shipping options</Text>
        <Text style={styles.errorSubtitle}>
          Please check your connection and try again.
        </Text>
        <Pressable style={styles.retryButton} onPress={() => refetch()} accessibilityRole="button">
          <Text style={styles.retryText}>Retry</Text>
        </Pressable>
      </View>
    );
  }

  // No rates
  if (!rates || rates.length === 0) {
    return (
      <View style={styles.centeredContainer}>
        <Text style={styles.errorTitle}>No shipping options available</Text>
        <Text style={styles.errorSubtitle}>
          We couldn't find shipping options for this address. Please go back and
          check your address.
        </Text>
        <Pressable style={styles.retryButton} onPress={() => router.back()} accessibilityRole="button">
          <Text style={styles.retryText}>Go back</Text>
        </Pressable>
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <ScrollView
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Shipping method</Text>

          <View style={styles.ratesList}>
            {rates.map((rate: ShippingRate) => {
              const isSelected = selectedRateId === rate.id;
              return (
                <Pressable
                  key={rate.id}
                  style={[styles.rateCard, isSelected && styles.rateCardSelected]}
                  onPress={() => handleSelect(rate.id)}
                  accessibilityRole="radio"
                  accessibilityState={{ selected: isSelected }}
                  accessibilityLabel={`${rate.carrier} ${rate.service}, ${rate.estimated_days}, ${rate.currency_code} ${rate.price_amount}`}
                >
                  <View style={styles.radioOuter}>
                    {isSelected && <View style={styles.radioInner} />}
                  </View>

                  <View style={styles.rateDetails}>
                    <Text style={styles.rateCarrier}>{rate.carrier}</Text>
                    <Text style={styles.rateService}>{rate.service}</Text>
                    <Text style={styles.rateDays}>
                      {rate.estimated_days}
                    </Text>
                  </View>

                  <Text style={styles.ratePrice}>
                    {rate.currency_code} {rate.price_amount}
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </View>
      </ScrollView>

      <View style={styles.stickyBar}>
        <Pressable
          style={[
            styles.continueButton,
            !selectedRateId && styles.continueButtonDisabled,
          ]}
          onPress={handleContinue}
          disabled={!selectedRateId}
          accessibilityRole="button"
          accessibilityLabel="Continue to payment"
        >
          <Text style={styles.continueButtonText}>Continue</Text>
        </Pressable>
      </View>
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: theme.background,
  },
  centeredContainer: {
    flex: 1,
    backgroundColor: theme.background,
    alignItems: "center",
    justifyContent: "center",
    padding: 32,
    gap: 12,
  },
  scrollContent: {
    paddingBottom: 100,
  },
  section: {
    padding: 16,
    gap: 16,
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: theme.text,
    fontFamily: "SourceSerif4",
  },
  ratesList: {
    gap: 8,
  },
  rateCard: {
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: theme.elevated,
    padding: 14,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: theme.border,
    gap: 12,
  },
  rateCardSelected: {
    borderColor: theme.primary,
  },
  radioOuter: {
    width: 20,
    height: 20,
    borderRadius: 10,
    borderWidth: 2,
    borderColor: theme.border,
    alignItems: "center",
    justifyContent: "center",
  },
  radioInner: {
    width: 10,
    height: 10,
    borderRadius: 5,
    backgroundColor: theme.primary,
  },
  rateDetails: {
    flex: 1,
    gap: 2,
  },
  rateCarrier: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.text,
  },
  rateService: {
    fontSize: 13,
    color: theme.textSecondary,
  },
  rateDays: {
    fontSize: 12,
    color: theme.accent,
    fontWeight: "500",
  },
  ratePrice: {
    fontSize: 15,
    fontWeight: "700",
    color: theme.text,
  },
  skeletonCard: {
    backgroundColor: theme.elevated,
    padding: 16,
    borderRadius: 6,
    gap: 8,
  },
  skeletonLine: {
    height: 14,
    width: "70%",
    backgroundColor: theme.border,
    borderRadius: 4,
  },
  skeletonLineShort: {
    height: 12,
    width: "40%",
    backgroundColor: theme.border,
    borderRadius: 4,
  },
  errorTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: theme.text,
    textAlign: "center",
  },
  errorSubtitle: {
    fontSize: 14,
    color: theme.textSecondary,
    textAlign: "center",
    lineHeight: 20,
  },
  retryButton: {
    marginTop: 8,
    paddingHorizontal: 24,
    paddingVertical: 12,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: theme.primary,
  },
  retryText: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.text,
  },
  stickyBar: {
    position: "absolute",
    bottom: 0,
    left: 0,
    right: 0,
    backgroundColor: theme.elevated,
    paddingHorizontal: 16,
    paddingTop: 12,
    paddingBottom: 32,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: theme.border,
  },
  continueButton: {
    backgroundColor: theme.primary,
    paddingVertical: 16,
    borderRadius: 6,
    alignItems: "center",
  },
  continueButtonDisabled: {
    backgroundColor: theme.border,
  },
  continueButtonText: {
    color: theme.elevated,
    fontSize: 16,
    fontWeight: "600",
  },
});
}
