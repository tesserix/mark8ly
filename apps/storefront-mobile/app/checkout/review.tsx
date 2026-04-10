import { useCallback, useMemo } from "react";
import {
  View,
  Text,
  Image,
  ScrollView,
  Pressable,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import { useCartStore } from "@/stores/cart-store";
import { useCheckoutStore } from "@/stores/checkout-store";
import { useSubmitCheckout } from "@/lib/hooks/use-checkout";
import { CouponInput } from "@/components/CouponInput";
import { GiftCardInput } from "@/components/GiftCardInput";
import { LoyaltyRedemption } from "@/components/LoyaltyRedemption";
import type { CheckoutSubmitBody } from "@repo/mobile-shared/api/storefront-types";

const PROVIDER_LABELS: Record<string, string> = {
  stripe: "Credit / Debit Card",
  razorpay: "Razorpay",
  paypal: "PayPal",
  digital_wallet: "Digital Wallet",
};

export default function CheckoutReviewScreen() {
  const router = useRouter();
  const cartItems = useCartStore((s) => s.items);
  const subtotalStr = useCartStore((s) => s.subtotal);
  const checkout = useCheckoutStore();
  const submitMutation = useSubmitCheckout();

  const subtotal = parseFloat(subtotalStr());

  const discountAmount = useMemo(() => {
    let discount = 0;
    if (checkout.coupon) {
      if (checkout.coupon.type === "fixed") {
        discount += checkout.coupon.amount;
      } else {
        discount += subtotal * (checkout.coupon.amount / 100);
      }
    }
    if (checkout.giftCard) {
      discount += checkout.giftCard.amount;
    }
    return discount;
  }, [checkout.coupon, checkout.giftCard, subtotal]);

  const estimatedTax = subtotal * 0.08; // Estimated 8%
  const shippingEstimate = 0; // Will be populated from actual rate
  const total = Math.max(0, subtotal - discountAmount + shippingEstimate + estimatedTax);

  const handleEdit = useCallback(() => {
    checkout.setStep(1);
    router.navigate("/checkout/details");
  }, [checkout, router]);

  const handlePlaceOrder = useCallback(async () => {
    const address = checkout.address;
    if (!address) return;

    const idempotencyKey = crypto.randomUUID();

    const body: CheckoutSubmitBody = {
      email: checkout.email,
      customer_name: checkout.customerName,
      line_items: cartItems.map((item) => ({
        product_id: item.productId,
        variant_id: item.variantId,
        quantity: item.qty,
      })),
      shipping_address: {
        name: checkout.customerName,
        line1: address.line1,
        line2: address.line2 ?? "",
        city: address.city,
        region: address.state,
        postal_code: address.postalCode,
        country: address.country,
      },
      shipping_rate_id: checkout.shippingRateId ?? "",
      payment_provider: checkout.paymentProvider ?? "stripe",
      idempotency_key: idempotencyKey,
      save_address: checkout.saveAddress,
    };

    if (checkout.coupon) {
      body.coupon_code = checkout.coupon.code;
    }
    if (checkout.giftCard) {
      body.gift_card_code = checkout.giftCard.code;
    }
    if (checkout.loyaltyPoints > 0) {
      body.loyalty_points = checkout.loyaltyPoints;
    }

    submitMutation.mutate(body);
  }, [checkout, cartItems, submitMutation]);

  return (
    <View style={styles.container}>
      <ScrollView
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {/* Line items */}
        <View style={styles.section}>
          <View style={styles.sectionHeader}>
            <Text style={styles.sectionTitle}>Order summary</Text>
            <Pressable onPress={handleEdit} accessibilityLabel="Edit order details">
              <Text style={styles.editLink}>Edit</Text>
            </Pressable>
          </View>

          <View style={styles.itemsList}>
            {cartItems.map((item) => (
              <View key={item.variantId} style={styles.lineItem}>
                <View style={styles.lineImageContainer}>
                  {item.imageUrl ? (
                    <Image
                      source={{ uri: item.imageUrl }}
                      style={styles.lineImage}
                      accessibilityLabel={item.title}
                    />
                  ) : (
                    <View style={styles.lineImagePlaceholder} />
                  )}
                </View>
                <View style={styles.lineDetails}>
                  <Text style={styles.lineTitle} numberOfLines={1}>
                    {item.title}
                  </Text>
                  <Text style={styles.lineQty}>Qty: {item.qty}</Text>
                </View>
                <Text style={styles.linePrice}>
                  {item.currencyCode} {(item.priceAmount * item.qty).toFixed(2)}
                </Text>
              </View>
            ))}
          </View>
        </View>

        <View style={styles.divider} />

        {/* Contact & address */}
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Details</Text>
          <View style={styles.detailBlock}>
            <Text style={styles.detailLabel}>Contact</Text>
            <Text style={styles.detailValue}>{checkout.customerName}</Text>
            <Text style={styles.detailValue}>{checkout.email}</Text>
          </View>

          {checkout.address && (
            <View style={styles.detailBlock}>
              <Text style={styles.detailLabel}>Shipping address</Text>
              <Text style={styles.detailValue}>
                {checkout.address.line1}
                {checkout.address.line2 ? `, ${checkout.address.line2}` : ""}
              </Text>
              <Text style={styles.detailValue}>
                {checkout.address.city}, {checkout.address.state}{" "}
                {checkout.address.postalCode}
              </Text>
              <Text style={styles.detailValue}>
                {checkout.address.country}
              </Text>
            </View>
          )}

          <View style={styles.detailBlock}>
            <Text style={styles.detailLabel}>Payment</Text>
            <Text style={styles.detailValue}>
              {PROVIDER_LABELS[checkout.paymentProvider ?? ""] ??
                checkout.paymentProvider ??
                "Not selected"}
            </Text>
          </View>
        </View>

        <View style={styles.divider} />

        {/* Discounts */}
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Discounts</Text>
          <View style={styles.discountGroup}>
            <CouponInput />
          </View>
          <View style={styles.discountGroup}>
            <GiftCardInput />
          </View>
          <View style={styles.discountGroup}>
            <LoyaltyRedemption />
          </View>
        </View>

        <View style={styles.divider} />

        {/* Totals */}
        <View style={styles.section}>
          <View style={styles.totalRow}>
            <Text style={styles.totalLabel}>Subtotal</Text>
            <Text style={styles.totalValue}>${subtotal.toFixed(2)}</Text>
          </View>
          <View style={styles.totalRow}>
            <Text style={styles.totalLabel}>Shipping</Text>
            <Text style={styles.totalValue}>
              {shippingEstimate === 0 ? "Calculated" : `$${shippingEstimate.toFixed(2)}`}
            </Text>
          </View>
          {discountAmount > 0 && (
            <View style={styles.totalRow}>
              <Text style={styles.totalLabel}>Discount</Text>
              <Text style={styles.discountValue}>
                -${discountAmount.toFixed(2)}
              </Text>
            </View>
          )}
          <View style={styles.totalRow}>
            <Text style={styles.totalLabel}>Tax (est.)</Text>
            <Text style={styles.totalValue}>${estimatedTax.toFixed(2)}</Text>
          </View>
          <View style={styles.totalDivider} />
          <View style={styles.totalRow}>
            <Text style={styles.grandTotalLabel}>Total</Text>
            <Text style={styles.grandTotalValue}>${total.toFixed(2)}</Text>
          </View>
        </View>
      </ScrollView>

      {/* Place order CTA */}
      <View style={styles.stickyBar}>
        {submitMutation.isError && (
          <Text style={styles.errorText}>
            {submitMutation.error instanceof Error
              ? submitMutation.error.message
              : "Something went wrong. Please try again."}
          </Text>
        )}
        <Pressable
          style={[
            styles.placeOrderButton,
            submitMutation.isPending && styles.placeOrderButtonLoading,
          ]}
          onPress={handlePlaceOrder}
          disabled={submitMutation.isPending}
          accessibilityRole="button"
          accessibilityLabel="Place order"
        >
          {submitMutation.isPending ? (
            <ActivityIndicator size="small" color="#FFFFFF" />
          ) : (
            <Text style={styles.placeOrderText}>Place order</Text>
          )}
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
  scrollContent: {
    paddingBottom: 120,
  },
  section: {
    padding: 16,
    gap: 12,
  },
  sectionHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: "#0E0E0C",
    fontFamily: "SourceSerif4",
  },
  editLink: {
    fontSize: 14,
    color: "#2D4A2B",
    fontWeight: "600",
  },
  divider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: "#E5E4DF",
  },
  itemsList: {
    gap: 10,
  },
  lineItem: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
  },
  lineImageContainer: {
    width: 44,
    height: 44,
    borderRadius: 6,
    overflow: "hidden",
    backgroundColor: "#E5E4DF",
  },
  lineImage: {
    width: 44,
    height: 44,
  },
  lineImagePlaceholder: {
    width: 44,
    height: 44,
    backgroundColor: "#E5E4DF",
  },
  lineDetails: {
    flex: 1,
    gap: 2,
  },
  lineTitle: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  lineQty: {
    fontSize: 12,
    color: "#666666",
  },
  linePrice: {
    fontSize: 14,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  detailBlock: {
    gap: 2,
  },
  detailLabel: {
    fontSize: 12,
    fontWeight: "600",
    color: "#999999",
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  detailValue: {
    fontSize: 14,
    color: "#0E0E0C",
    lineHeight: 20,
  },
  discountGroup: {
    gap: 8,
  },
  totalRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  totalLabel: {
    fontSize: 14,
    color: "#666666",
  },
  totalValue: {
    fontSize: 14,
    color: "#0E0E0C",
  },
  discountValue: {
    fontSize: 14,
    color: "#2D4A2B",
    fontWeight: "600",
  },
  totalDivider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: "#E5E4DF",
    marginVertical: 4,
  },
  grandTotalLabel: {
    fontSize: 16,
    fontWeight: "700",
    color: "#0E0E0C",
  },
  grandTotalValue: {
    fontSize: 18,
    fontWeight: "700",
    color: "#0E0E0C",
    fontFamily: "SourceSerif4",
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
    gap: 8,
  },
  errorText: {
    fontSize: 13,
    color: "#8B2500",
    textAlign: "center",
  },
  placeOrderButton: {
    backgroundColor: "#0E0E0C",
    paddingVertical: 16,
    borderRadius: 6,
    alignItems: "center",
  },
  placeOrderButtonLoading: {
    opacity: 0.7,
  },
  placeOrderText: {
    color: "#FFFFFF",
    fontSize: 16,
    fontWeight: "600",
  },
});
