import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  ActivityIndicator,
  Alert,
  KeyboardAvoidingView,
  Platform,
  ScrollView,
  StyleSheet,
  TextInput,
  TouchableOpacity,
  View,
} from "react-native";
import { Stack, useRouter } from "expo-router";
import {
  Check,
  ChevronLeft,
  CreditCard,
  MapPin,
  Plus,
  Truck,
} from "lucide-react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import type {
  CheckoutLineItem,
  PaymentMethod,
  ShippingRate,
  StorefrontAddress,
} from "@repo/mobile-shared/api/storefront-types";
import { useCartStore } from "@/lib/cart-store";
import { useAddresses } from "@/lib/hooks/use-account";
import {
  usePaymentMethods,
  useShippingRates,
  useSubmitCheckout,
  useValidateCoupon,
} from "@/lib/hooks/use-checkout";
import {
  Button,
  Card,
  EmptyState,
  Hairline,
  PageHeader,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/format";

// New idempotency key per checkout attempt — stops a retried tap from
// charging the customer twice.
function newIdempotencyKey() {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

export default function CheckoutScreen() {
  const router = useRouter();
  const { user } = useAuth();
  const lines = useCartStore((s) => s.lines);
  const subtotalAmount = useCartStore((s) => s.subtotalAmount());
  const clearCart = useCartStore((s) => s.clear);

  const checkoutLines = useMemo<CheckoutLineItem[]>(
    () =>
      lines.map((l) => ({
        product_id: l.productId,
        variant_id: l.variantId,
        quantity: l.quantity,
      })),
    [lines],
  );
  const currency = lines[0]?.currencyCode ?? "USD";

  const addresses = useAddresses();
  const paymentMethods = usePaymentMethods();
  const shippingRates = useShippingRates();
  const validateCoupon = useValidateCoupon();
  const submit = useSubmitCheckout();

  const [selectedAddressId, setSelectedAddressId] = useState<string | null>(null);
  const [rates, setRates] = useState<ShippingRate[]>([]);
  const [selectedRateId, setSelectedRateId] = useState<string | null>(null);
  const [selectedProvider, setSelectedProvider] = useState<string | null>(null);
  const [couponCode, setCouponCode] = useState("");
  const [appliedCoupon, setAppliedCoupon] = useState<{ code: string; discount: number } | null>(
    null,
  );
  const [idempotencyKey] = useState(newIdempotencyKey());

  // Auto-select default address.
  useEffect(() => {
    if (selectedAddressId || !addresses.data?.items?.length) return;
    const def = addresses.data.items.find((a) => a.is_default) ?? addresses.data.items[0];
    if (def) setSelectedAddressId(def.id);
  }, [addresses.data, selectedAddressId]);

  // Auto-select first enabled payment method.
  useEffect(() => {
    if (selectedProvider || !paymentMethods.data?.items?.length) return;
    const enabled = paymentMethods.data.items.find((p) => p.enabled);
    if (enabled) setSelectedProvider(enabled.provider);
  }, [paymentMethods.data, selectedProvider]);

  const selectedAddress = addresses.data?.items?.find((a) => a.id === selectedAddressId) ?? null;

  // Fetch shipping rates whenever address or cart changes.
  useEffect(() => {
    if (!selectedAddress || checkoutLines.length === 0) return;
    setRates([]);
    setSelectedRateId(null);
    shippingRates.mutate(
      {
        shipping_address: stripAddress(selectedAddress),
        line_items: checkoutLines,
      },
      {
        onSuccess: (resp) => {
          setRates(resp.items);
          if (resp.items.length === 1) setSelectedRateId(resp.items[0]!.id);
        },
      },
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedAddress?.id, checkoutLines.length]);

  const selectedRate = rates.find((r) => r.id === selectedRateId) ?? null;

  const totals = useMemo(() => {
    const shipping = selectedRate ? Number(selectedRate.price_amount) : 0;
    const discount = appliedCoupon?.discount ?? 0;
    const total = subtotalAmount + shipping - discount;
    return { subtotal: subtotalAmount, shipping, discount, total: Math.max(total, 0) };
  }, [subtotalAmount, selectedRate, appliedCoupon]);

  const canPlaceOrder =
    !!user &&
    !!selectedAddress &&
    !!selectedRate &&
    !!selectedProvider &&
    checkoutLines.length > 0;

  const handleApplyCoupon = () => {
    if (!couponCode.trim()) return;
    validateCoupon.mutate(couponCode.trim(), {
      onSuccess: (r) => {
        if (r.valid) {
          setAppliedCoupon({ code: couponCode.trim(), discount: Number(r.discount_amount) });
        } else {
          Alert.alert("Invalid coupon", r.message ?? "That coupon isn't valid right now.");
        }
      },
      onError: (err: unknown) =>
        Alert.alert("Couldn't validate", err instanceof Error ? err.message : "Try again."),
    });
  };

  const handlePlaceOrder = () => {
    if (!canPlaceOrder || !selectedAddress || !selectedRate || !selectedProvider) return;
    submit.mutate(
      {
        email: user?.email ?? "",
        customer_name: selectedAddress.name,
        line_items: checkoutLines,
        shipping_address: stripAddress(selectedAddress),
        shipping_rate_id: selectedRate.id,
        payment_provider: selectedProvider,
        ...(appliedCoupon ? { coupon_code: appliedCoupon.code } : {}),
        idempotency_key: idempotencyKey,
      },
      {
        onSuccess: (result) => {
          clearCart();
          router.replace(`/order/${result.order_id}`);
        },
        onError: (err: unknown) =>
          Alert.alert(
            "Couldn't place order",
            err instanceof Error ? err.message : "Try again in a moment.",
          ),
      },
    );
  };

  if (!user) {
    return (
      <Screen>
        <Stack.Screen options={{ headerShown: false }} />
        <BackBar onBack={() => router.back()} />
        <View style={styles.center}>
          <EmptyState
            title="Sign in to check out"
            message="Tracking and order history live in your account."
            action={
              <Button label="Sign in" onPress={() => router.push("/sign-in")} fullWidth />
            }
          />
        </View>
      </Screen>
    );
  }

  if (lines.length === 0) {
    return (
      <Screen>
        <Stack.Screen options={{ headerShown: false }} />
        <BackBar onBack={() => router.back()} />
        <View style={styles.center}>
          <EmptyState
            title="Your bag is empty"
            message="Add something to your cart before checking out."
            action={<Button label="Browse shop" onPress={() => router.replace("/shop")} />}
          />
        </View>
      </Screen>
    );
  }

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: false }} />
      <KeyboardAvoidingView
        style={{ flex: 1 }}
        behavior={Platform.OS === "ios" ? "padding" : undefined}
      >
        <BackBar onBack={() => router.back()} />
        <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
          <PageHeader eyebrow="CHECKOUT" title="Review & pay" />

          {/* Shipping address */}
          <Section icon={<MapPin size={16} color={theme.colors.text} strokeWidth={1.75} />} title="Shipping address">
            {addresses.isLoading ? (
              <ActivityIndicator size="small" color={theme.colors.text} />
            ) : addresses.data?.items?.length ? (
              addresses.data.items.map((address, i) => (
                <View key={address.id}>
                  {i > 0 ? <Hairline inset={theme.spacing.lg} /> : null}
                  <SelectableRow
                    selected={selectedAddressId === address.id}
                    onPress={() => setSelectedAddressId(address.id)}
                    primary={address.name}
                    secondary={`${address.line1}${address.line2 ? `, ${address.line2}` : ""}`}
                    tertiary={`${address.city}, ${address.region} ${address.postal_code}, ${address.country}`}
                  />
                </View>
              ))
            ) : (
              <EmptyInline message="No saved addresses." />
            )}
            <TouchableOpacity
              onPress={() => router.push("/addresses")}
              style={styles.linkRow}
              accessibilityRole="link"
              accessibilityLabel="Manage addresses"
            >
              <Plus size={14} color={theme.colors.text} strokeWidth={1.75} />
              <Text preset="bodyEmphasis" color="text">
                Add or edit addresses
              </Text>
            </TouchableOpacity>
          </Section>

          {/* Shipping rate */}
          <Section icon={<Truck size={16} color={theme.colors.text} strokeWidth={1.75} />} title="Shipping method">
            {!selectedAddress ? (
              <EmptyInline message="Pick an address to see shipping options." />
            ) : shippingRates.isPending ? (
              <ActivityIndicator size="small" color={theme.colors.text} />
            ) : rates.length === 0 ? (
              <EmptyInline message="No shipping options for this address." />
            ) : (
              rates.map((rate, i) => (
                <View key={rate.id}>
                  {i > 0 ? <Hairline inset={theme.spacing.lg} /> : null}
                  <SelectableRow
                    selected={selectedRateId === rate.id}
                    onPress={() => setSelectedRateId(rate.id)}
                    primary={`${rate.carrier} · ${rate.service}`}
                    secondary={`${rate.estimated_days} days`}
                    trailing={formatMoney(rate.price_amount, rate.currency_code || currency)}
                  />
                </View>
              ))
            )}
          </Section>

          {/* Payment method */}
          <Section icon={<CreditCard size={16} color={theme.colors.text} strokeWidth={1.75} />} title="Payment method">
            {paymentMethods.isLoading ? (
              <ActivityIndicator size="small" color={theme.colors.text} />
            ) : paymentMethods.data?.items?.length ? (
              paymentMethods.data.items
                .filter((m) => m.enabled)
                .map((method, i) => (
                  <View key={method.provider}>
                    {i > 0 ? <Hairline inset={theme.spacing.lg} /> : null}
                    <SelectableRow
                      selected={selectedProvider === method.provider}
                      onPress={() => setSelectedProvider(method.provider)}
                      primary={prettyProvider(method)}
                    />
                  </View>
                ))
            ) : (
              <EmptyInline message="No payment methods configured for this store." />
            )}
          </Section>

          {/* Coupon */}
          <Section title="Promo code">
            <View style={styles.couponRow}>
              <TextInput
                value={couponCode}
                onChangeText={setCouponCode}
                placeholder="Enter code"
                placeholderTextColor={theme.colors.textTertiary}
                style={styles.couponInput}
                autoCapitalize="characters"
                autoCorrect={false}
              />
              <Button
                label={appliedCoupon ? "Applied" : "Apply"}
                onPress={handleApplyCoupon}
                loading={validateCoupon.isPending}
                disabled={!couponCode.trim() || !!appliedCoupon}
                variant="secondary"
              />
            </View>
            {appliedCoupon ? (
              <Text preset="caption" color="accent" style={{ marginTop: theme.spacing.xs }}>
                {appliedCoupon.code} applied · −{formatMoney(appliedCoupon.discount, currency)}
              </Text>
            ) : null}
          </Section>

          {/* Totals */}
          <Card padding="md" style={styles.card}>
            <TotalsRow label="Subtotal" value={formatMoney(totals.subtotal, currency)} />
            <TotalsRow label="Shipping" value={formatMoney(totals.shipping, currency)} />
            {totals.discount > 0 ? (
              <TotalsRow label="Discount" value={`-${formatMoney(totals.discount, currency)}`} />
            ) : null}
            <Hairline style={{ marginVertical: theme.spacing.sm }} />
            <TotalsRow label="Total" value={formatMoney(totals.total, currency)} emphasis />
          </Card>
        </ScrollView>

        <View style={styles.footer}>
          <Hairline />
          <View style={styles.footerInner}>
            <Button
              label={`Place order · ${formatMoney(totals.total, currency)}`}
              onPress={handlePlaceOrder}
              loading={submit.isPending}
              disabled={!canPlaceOrder}
              fullWidth
            />
          </View>
        </View>
      </KeyboardAvoidingView>
    </Screen>
  );
}

function stripAddress(a: StorefrontAddress) {
  const { id: _id, is_default: _d, ...rest } = a;
  return rest;
}

function prettyProvider(m: PaymentMethod): string {
  switch (m.provider.toLowerCase()) {
    case "stripe":
      return "Card (Stripe)";
    case "razorpay":
      return "Razorpay";
    case "apple_pay":
      return "Apple Pay";
    case "google_pay":
      return "Google Pay";
    default:
      return m.provider.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
  }
}

function BackBar({ onBack }: { onBack: () => void }) {
  return (
    <View style={styles.headerBar}>
      <TouchableOpacity onPress={onBack} hitSlop={12} accessibilityLabel="Back" style={styles.backBtn}>
        <ChevronLeft size={22} color={theme.colors.text} strokeWidth={1.75} />
      </TouchableOpacity>
    </View>
  );
}

function Section({
  icon,
  title,
  children,
}: {
  icon?: ReactNode;
  title: string;
  children: ReactNode;
}) {
  return (
    <View style={styles.section}>
      <View style={styles.sectionHeader}>
        {icon}
        <Text preset="eyebrow" color="textTertiary">
          {title.toUpperCase()}
        </Text>
      </View>
      <Card padding={0}>{children}</Card>
    </View>
  );
}

function SelectableRow({
  selected,
  onPress,
  primary,
  secondary,
  tertiary,
  trailing,
}: {
  selected: boolean;
  onPress: () => void;
  primary: string;
  secondary?: string;
  tertiary?: string;
  trailing?: string;
}) {
  return (
    <TouchableOpacity
      onPress={onPress}
      activeOpacity={0.6}
      style={styles.row}
      accessibilityRole="button"
      accessibilityState={{ selected }}
      accessibilityLabel={primary}
    >
      <View style={[styles.radio, selected && styles.radioActive]}>
        {selected ? <Check size={12} color={theme.colors.inverse} strokeWidth={2.5} /> : null}
      </View>
      <View style={{ flex: 1, gap: 2 }}>
        <Text preset="bodyEmphasis" color="text">
          {primary}
        </Text>
        {secondary ? (
          <Text preset="caption" color="textSecondary" numberOfLines={1}>
            {secondary}
          </Text>
        ) : null}
        {tertiary ? (
          <Text preset="caption" color="textTertiary" numberOfLines={1}>
            {tertiary}
          </Text>
        ) : null}
      </View>
      {trailing ? (
        <Text preset="bodyEmphasis" color="text">
          {trailing}
        </Text>
      ) : null}
    </TouchableOpacity>
  );
}

function EmptyInline({ message }: { message: string }) {
  return (
    <View style={{ padding: theme.spacing.md }}>
      <Text preset="caption" color="textTertiary">
        {message}
      </Text>
    </View>
  );
}

function TotalsRow({
  label,
  value,
  emphasis,
}: {
  label: string;
  value: string;
  emphasis?: boolean;
}) {
  return (
    <View style={styles.totalsRow}>
      <Text
        preset={emphasis ? "bodyEmphasis" : "body"}
        color={emphasis ? "text" : "textSecondary"}
      >
        {label}
      </Text>
      <Text preset={emphasis ? "price" : "body"} color="text">
        {value}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  headerBar: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.sm },
  backBtn: {
    width: 36,
    height: 36,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.radii.pill,
  },
  scroll: {
    paddingBottom: theme.spacing.huge * 2,
    gap: theme.spacing.md,
  },
  card: { marginHorizontal: theme.spacing.lg },
  section: {
    marginHorizontal: theme.spacing.lg,
    gap: theme.spacing.sm,
  },
  sectionHeader: { flexDirection: "row", alignItems: "center", gap: theme.spacing.sm },
  row: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.md,
    padding: theme.spacing.md,
  },
  radio: {
    width: 22,
    height: 22,
    borderRadius: 11,
    borderWidth: 1.5,
    borderColor: theme.colors.border,
    alignItems: "center",
    justifyContent: "center",
  },
  radioActive: {
    backgroundColor: theme.colors.primary,
    borderColor: theme.colors.primary,
  },
  linkRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.xs,
    padding: theme.spacing.md,
  },
  couponRow: {
    flexDirection: "row",
    gap: theme.spacing.sm,
    padding: theme.spacing.md,
  },
  couponInput: {
    flex: 1,
    height: 48,
    paddingHorizontal: theme.spacing.md,
    fontFamily: theme.fonts.sans,
    fontSize: 16,
    color: theme.colors.text,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    backgroundColor: theme.colors.elevated,
  },
  totalsRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: 4,
  },
  footer: { backgroundColor: theme.colors.background },
  footerInner: { padding: theme.spacing.lg },
  center: { flex: 1, justifyContent: "center", paddingHorizontal: theme.spacing.lg },
});
