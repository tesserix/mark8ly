import { useEffect, useCallback, useRef, useMemo } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  StyleSheet,
} from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { haptics } from "@repo/mobile-shared/haptics/feedback";
import { useCartStore } from "@/stores/cart-store";
import { useCheckoutStore } from "@/stores/checkout-store";
import { GuestAccountPrompt } from "@/components/GuestAccountPrompt";

export default function OrderConfirmationScreen() {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { user } = useAuth();
  const isAuthenticated = !!user;
  const checkoutEmail = useCheckoutStore((s) => s.email);
  const checkoutName = useCheckoutStore((s) => s.customerName);
  const clearCart = useCartStore((s) => s.clear);
  const resetCheckout = useCheckoutStore((s) => s.reset);

  // Only run once on mount
  const didCleanup = useRef(false);
  useEffect(() => {
    if (!didCleanup.current) {
      didCleanup.current = true;
      haptics.orderPlaced();
      clearCart();
      // Delay reset so email/name stay available for guest prompt
      const timer = setTimeout(() => resetCheckout(), 500);
      return () => clearTimeout(timer);
    }
  }, [clearCart, resetCheckout]);

  const handleContinueShopping = useCallback(() => {
    router.navigate("/(tabs)");
  }, [router]);

  return (
    <View style={styles.container}>
      <ScrollView
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {/* Success header */}
        <View style={styles.successSection}>
          <View style={styles.checkCircle}>
            <Text style={styles.checkMark}>✓</Text>
          </View>
          <Text style={styles.thankYou}>Thank you!</Text>
          <Text style={styles.subtitle}>Your order has been placed.</Text>
        </View>

        {/* Order number */}
        <View style={styles.orderCard}>
          <Text style={styles.orderLabel}>Order number</Text>
          <Text style={styles.orderNumber}>{id}</Text>
          <Text style={styles.orderNote}>
            A confirmation email will be sent to your inbox.
          </Text>
        </View>

        {/* Guest account prompt */}
        {!isAuthenticated && checkoutEmail.length > 0 && (
          <View style={styles.promptSection}>
            <GuestAccountPrompt
              email={checkoutEmail}
              name={checkoutName}
              onDismiss={() => {}}
            />
          </View>
        )}

        {/* Continue shopping */}
        <View style={styles.ctaSection}>
          <Pressable
            style={styles.continueButton}
            onPress={handleContinueShopping}
            accessibilityRole="button"
            accessibilityLabel="Continue shopping"
          >
            <Text style={styles.continueButtonText}>Continue shopping</Text>
          </Pressable>
        </View>
      </ScrollView>
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: theme.background,
  },
  scrollContent: {
    paddingBottom: 40,
  },
  successSection: {
    alignItems: "center",
    paddingTop: 40,
    paddingBottom: 24,
    gap: 12,
  },
  checkCircle: {
    width: 64,
    height: 64,
    borderRadius: 32,
    backgroundColor: theme.accent,
    alignItems: "center",
    justifyContent: "center",
    marginBottom: 8,
  },
  checkMark: {
    color: theme.elevated,
    fontSize: 28,
    fontWeight: "700",
  },
  thankYou: {
    fontSize: 28,
    fontWeight: "700",
    color: theme.text,
    fontFamily: "SourceSerif4",
  },
  subtitle: {
    fontSize: 16,
    color: theme.textSecondary,
  },
  orderCard: {
    marginHorizontal: 16,
    backgroundColor: theme.elevated,
    borderRadius: 6,
    padding: 20,
    alignItems: "center",
    gap: 8,
    borderWidth: 1,
    borderColor: theme.border,
  },
  orderLabel: {
    fontSize: 12,
    fontWeight: "600",
    color: theme.textSecondary,
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  orderNumber: {
    fontSize: 22,
    fontWeight: "700",
    color: theme.text,
    fontFamily: "SourceSerif4",
  },
  orderNote: {
    fontSize: 13,
    color: theme.textSecondary,
    textAlign: "center",
    lineHeight: 18,
  },
  promptSection: {
    padding: 16,
  },
  ctaSection: {
    padding: 16,
  },
  continueButton: {
    backgroundColor: theme.primary,
    paddingVertical: 16,
    borderRadius: 6,
    alignItems: "center",
  },
  continueButtonText: {
    color: theme.elevated,
    fontSize: 16,
    fontWeight: "600",
  },
});
}
