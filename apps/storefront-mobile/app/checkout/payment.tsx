import { useState, useCallback, useMemo } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  StyleSheet,
  Platform,
  ActivityIndicator,
} from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
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
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
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
        <Pressable style={styles.retryButton} onPress={() => refetch()} accessibilityRole="button">
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
  methodsList: {
    gap: 8,
  },
  methodCard: {
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: theme.elevated,
    padding: 14,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: theme.border,
    gap: 12,
    flexWrap: "wrap",
  },
  methodCardSelected: {
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
    color: theme.text,
  },
  methodNote: {
    fontSize: 12,
    color: theme.textSecondary,
  },
  cardStub: {
    width: "100%",
    marginTop: 8,
    marginLeft: 44,
    backgroundColor: theme.background,
    padding: 12,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: theme.border,
    borderStyle: "dashed",
  },
  cardStubText: {
    fontSize: 13,
    color: theme.textSecondary,
    textAlign: "center",
  },
  skeletonCard: {
    backgroundColor: theme.elevated,
    padding: 16,
    borderRadius: 6,
    gap: 8,
  },
  skeletonLine: {
    height: 14,
    width: "60%",
    backgroundColor: theme.border,
    borderRadius: 4,
  },
  skeletonLineShort: {
    height: 12,
    width: "30%",
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
