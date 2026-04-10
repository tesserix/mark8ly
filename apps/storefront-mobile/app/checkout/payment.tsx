import { useState, useCallback } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  StyleSheet,
  Platform,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import { haptics } from "@repo/mobile-shared/haptics/feedback";
import { useCheckoutStore } from "@/stores/checkout-store";
import { usePaymentMethods } from "@/lib/hooks/use-checkout";
import type { PaymentMethod } from "@repo/mobile-shared/api/storefront-types";

const PROVIDER_LABELS: Record<string, string> = {
  stripe: "Credit / Debit Card",
  razorpay: "Razorpay",
  paypal: "PayPal",
};

const PROVIDER_ICONS: Record<string, string> = {
  stripe: "💳",
  razorpay: "🏦",
  paypal: "🅿️",
};

function getDigitalWalletLabel(): string {
  return Platform.OS === "ios" ? "Apple Pay" : "Google Pay";
}

function getDigitalWalletIcon(): string {
  return Platform.OS === "ios" ? "🍎" : "📱";
}

export default function CheckoutPaymentScreen() {
  const router = useRouter();
  const checkoutStore = useCheckoutStore();

  const {
    data: methods,
    isLoading,
    error,
    refetch,
  } = usePaymentMethods();

  const [selectedProvider, setSelectedProvider] = useState<string | null>(
    checkoutStore.paymentProvider,
  );

  const handleSelect = useCallback((provider: string) => {
    setSelectedProvider(provider);
  }, []);

  const handleContinue = useCallback(async () => {
    if (!selectedProvider) return;

    await haptics.checkoutStep();
    checkoutStore.setPaymentProvider(selectedProvider as "stripe" | "razorpay");
    checkoutStore.setStep(4);
    router.push("/checkout/review");
  }, [selectedProvider, checkoutStore, router]);

  // Loading
  if (isLoading) {
    return (
      <View style={styles.container}>
        <ScrollView contentContainerStyle={styles.scrollContent}>
          <View style={styles.section}>
            <Text style={styles.sectionTitle}>Payment method</Text>
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

  // Error
  if (error) {
    return (
      <View style={styles.centeredContainer}>
        <Text style={styles.errorTitle}>Unable to load payment methods</Text>
        <Text style={styles.errorSubtitle}>
          Please check your connection and try again.
        </Text>
        <Pressable style={styles.retryButton} onPress={() => refetch()}>
          <Text style={styles.retryText}>Retry</Text>
        </Pressable>
      </View>
    );
  }

  // Build display list from enabled methods
  const enabledMethods = (methods ?? []).filter((m) => m.enabled);

  // Check if digital wallet is supported by any provider
  const hasDigitalWallet = enabledMethods.some(
    (m) =>
      (Platform.OS === "ios" && m.supports_apple_pay) ||
      (Platform.OS === "android" && m.supports_google_pay),
  );

  return (
    <View style={styles.container}>
      <ScrollView
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Payment method</Text>

          <View style={styles.methodsList}>
            {enabledMethods.map((method: PaymentMethod) => {
              const isSelected = selectedProvider === method.provider;
              const label =
                PROVIDER_LABELS[method.provider] ?? method.provider;
              const icon = PROVIDER_ICONS[method.provider] ?? "💰";
              const isPaypal = method.provider === "paypal";

              return (
                <Pressable
                  key={method.provider}
                  style={[
                    styles.methodCard,
                    isSelected && styles.methodCardSelected,
                  ]}
                  onPress={() => handleSelect(method.provider)}
                  accessibilityRole="radio"
                  accessibilityState={{ selected: isSelected }}
                  accessibilityLabel={label}
                >
                  <View style={styles.radioOuter}>
                    {isSelected && <View style={styles.radioInner} />}
                  </View>

                  <Text style={styles.methodIcon}>{icon}</Text>

                  <View style={styles.methodDetails}>
                    <Text style={styles.methodLabel}>{label}</Text>
                    {isPaypal && (
                      <Text style={styles.methodNote}>Opens in browser</Text>
                    )}
                  </View>

                  {/* Stub card input for Stripe */}
                  {isSelected && method.provider === "stripe" && (
                    <View style={styles.cardStub}>
                      <Text style={styles.cardStubText}>
                        Card details collected at payment
                      </Text>
                    </View>
                  )}
                </Pressable>
              );
            })}

            {/* Digital wallet option */}
            {hasDigitalWallet && (
              <Pressable
                style={[
                  styles.methodCard,
                  selectedProvider === "digital_wallet" &&
                    styles.methodCardSelected,
                ]}
                onPress={() => handleSelect("digital_wallet")}
                accessibilityRole="radio"
                accessibilityState={{
                  selected: selectedProvider === "digital_wallet",
                }}
                accessibilityLabel={getDigitalWalletLabel()}
              >
                <View style={styles.radioOuter}>
                  {selectedProvider === "digital_wallet" && (
                    <View style={styles.radioInner} />
                  )}
                </View>
                <Text style={styles.methodIcon}>
                  {getDigitalWalletIcon()}
                </Text>
                <View style={styles.methodDetails}>
                  <Text style={styles.methodLabel}>
                    {getDigitalWalletLabel()}
                  </Text>
                  <Text style={styles.methodNote}>Express checkout</Text>
                </View>
              </Pressable>
            )}
          </View>
        </View>
      </ScrollView>

      <View style={styles.stickyBar}>
        <Pressable
          style={[
            styles.continueButton,
            !selectedProvider && styles.continueButtonDisabled,
          ]}
          onPress={handleContinue}
          disabled={!selectedProvider}
          accessibilityRole="button"
          accessibilityLabel="Continue to review"
        >
          <Text style={styles.continueButtonText}>Continue</Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#F7F6F2",
  },
  centeredContainer: {
    flex: 1,
    backgroundColor: "#F7F6F2",
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
    color: "#0E0E0C",
    fontFamily: "SourceSerif4",
  },
  methodsList: {
    gap: 8,
  },
  methodCard: {
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: "#FFFFFF",
    padding: 14,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: "#E5E4DF",
    gap: 12,
    flexWrap: "wrap",
  },
  methodCardSelected: {
    borderColor: "#0E0E0C",
  },
  radioOuter: {
    width: 20,
    height: 20,
    borderRadius: 10,
    borderWidth: 2,
    borderColor: "#E5E4DF",
    alignItems: "center",
    justifyContent: "center",
  },
  radioInner: {
    width: 10,
    height: 10,
    borderRadius: 5,
    backgroundColor: "#0E0E0C",
  },
  methodIcon: {
    fontSize: 20,
  },
  methodDetails: {
    flex: 1,
    gap: 2,
  },
  methodLabel: {
    fontSize: 15,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  methodNote: {
    fontSize: 12,
    color: "#666666",
  },
  cardStub: {
    width: "100%",
    marginTop: 8,
    marginLeft: 44,
    backgroundColor: "#F7F6F2",
    padding: 12,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: "#E5E4DF",
    borderStyle: "dashed",
  },
  cardStubText: {
    fontSize: 13,
    color: "#999999",
    textAlign: "center",
  },
  skeletonCard: {
    backgroundColor: "#FFFFFF",
    padding: 16,
    borderRadius: 6,
    gap: 8,
  },
  skeletonLine: {
    height: 14,
    width: "60%",
    backgroundColor: "#E5E4DF",
    borderRadius: 4,
  },
  skeletonLineShort: {
    height: 12,
    width: "30%",
    backgroundColor: "#E5E4DF",
    borderRadius: 4,
  },
  errorTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: "#0E0E0C",
    textAlign: "center",
  },
  errorSubtitle: {
    fontSize: 14,
    color: "#666666",
    textAlign: "center",
    lineHeight: 20,
  },
  retryButton: {
    marginTop: 8,
    paddingHorizontal: 24,
    paddingVertical: 12,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: "#0E0E0C",
  },
  retryText: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  stickyBar: {
    position: "absolute",
    bottom: 0,
    left: 0,
    right: 0,
    backgroundColor: "#FFFFFF",
    paddingHorizontal: 16,
    paddingTop: 12,
    paddingBottom: 32,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: "#E5E4DF",
  },
  continueButton: {
    backgroundColor: "#0E0E0C",
    paddingVertical: 16,
    borderRadius: 6,
    alignItems: "center",
  },
  continueButtonDisabled: {
    backgroundColor: "#E5E4DF",
  },
  continueButtonText: {
    color: "#FFFFFF",
    fontSize: 16,
    fontWeight: "600",
  },
});
