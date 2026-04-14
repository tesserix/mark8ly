"use client";

// apps/storefront/app/checkout/page.tsx
//
// Checkout page. Reads items from CartProvider, collects shipping
// address, fetches shipping rates + payment methods, and submits
// the order via the extended checkout endpoint. On success redirects
// to /orders/[id].

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import Image from "next/image";
import Link from "next/link";
import { useCart } from "@/components/CartProvider";
import { StorefrontNav } from "@/components/StorefrontNav";
import { CouponInput } from "@/components/checkout/CouponInput";
import { GiftCardInput } from "@/components/checkout/GiftCardInput";
import { LoyaltyRedemption } from "@/components/checkout/LoyaltyRedemption";
import {
  fetchPaymentMethods,
  fetchShippingRates,
  submitCheckout,
  type PaymentMethod,
  type ShippingRate,
  type CheckoutBody,
  type CheckoutItemBody,
  type CheckoutAddressBody,
} from "@/lib/api/checkout-api";

// ---------------------------------------------------------------------------
// Store slug — client-side resolution via hostname + fallback
// ---------------------------------------------------------------------------

function resolveSlugClient(): string {
  if (typeof window === "undefined") return process.env.NEXT_PUBLIC_DEFAULT_STORE_SLUG ?? "default";
  const host = window.location.hostname;
  if (host === "localhost" || host === "127.0.0.1") {
    return process.env.NEXT_PUBLIC_DEFAULT_STORE_SLUG ?? "default";
  }
  const parts = host.split(".");
  if (parts.length >= 2) {
    const sub = parts[0] ?? "";
    if (sub && sub !== "www" && sub !== "api" && !sub.endsWith("-admin")) {
      return sub;
    }
  }
  return process.env.NEXT_PUBLIC_DEFAULT_STORE_SLUG ?? "default";
}

// ---------------------------------------------------------------------------
// Address form state
// ---------------------------------------------------------------------------

interface AddressFields {
  name: string;
  line1: string;
  line2: string;
  city: string;
  region: string;
  postal_code: string;
  country_code: string;
}

const EMPTY_ADDRESS: AddressFields = {
  name: "",
  line1: "",
  line2: "",
  city: "",
  region: "",
  postal_code: "",
  country_code: "",
};

function isAddressFilled(a: AddressFields): boolean {
  return a.name.trim() !== "" && a.line1.trim() !== "" && a.city.trim() !== "" && a.country_code.trim().length === 2;
}

function toCheckoutAddress(a: AddressFields): CheckoutAddressBody {
  return {
    name: a.name.trim(),
    line1: a.line1.trim(),
    line2: a.line2.trim() || undefined,
    city: a.city.trim(),
    region: a.region.trim() || undefined,
    postal_code: a.postal_code.trim() || undefined,
    country_code: a.country_code.trim().toUpperCase(),
  };
}

// ---------------------------------------------------------------------------
// Price formatting
// ---------------------------------------------------------------------------

function formatPrice(amount: number, currencyCode: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currencyCode,
    }).format(amount);
  } catch {
    return `${currencyCode} ${amount.toFixed(2)}`;
  }
}

// ---------------------------------------------------------------------------
// Checkout page
// ---------------------------------------------------------------------------

export default function CheckoutPage() {
  const router = useRouter();
  const { items, subtotal, count, clear } = useCart();
  const storeSlug = useMemo(() => resolveSlugClient(), []);
  const currencyCode = items[0]?.currencyCode ?? "USD";

  // Form state
  const [address, setAddress] = useState<AddressFields>(EMPTY_ADDRESS);
  const [email, setEmail] = useState("");
  const [customerName, setCustomerName] = useState("");

  // Async state
  const [paymentMethods, setPaymentMethods] = useState<PaymentMethod[]>([]);
  const [shippingRates, setShippingRates] = useState<ShippingRate[]>([]);
  const [selectedShipping, setSelectedShipping] = useState<string>("");
  const [selectedProvider, setSelectedProvider] = useState<string>("");
  const [loadingRates, setLoadingRates] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Coupon state
  const [couponCode, setCouponCode] = useState<string | null>(null);
  const [couponDiscount, setCouponDiscount] = useState(0);
  const [couponFreeShipping, setCouponFreeShipping] = useState(false);

  // Gift card state
  const [giftCardCode, setGiftCardCode] = useState<string | null>(null);
  const [giftCardAmount, setGiftCardAmount] = useState(0);

  // Loyalty points state — populated via /api/checkout/loyalty/init once
  // an email is entered. `loyaltyBalance > 0` gates the redemption toggle.
  const [redeemPoints, setRedeemPoints] = useState<number | null>(null);
  const [loyaltyBalance, setLoyaltyBalance] = useState(0);
  const [loyaltyPointsValue, setLoyaltyPointsValue] = useState("0.00");
  const [loyaltyPointsCurrency, setLoyaltyPointsCurrency] = useState("points");
  const [loyaltyMinRedeem, setLoyaltyMinRedeem] = useState(100);

  // Computed totals
  const selectedRate = shippingRates.find((r) => r.service === selectedShipping);
  const shippingTotal = selectedRate ? Number.parseFloat(selectedRate.price) : 0;
  // Tax is computed server-side at checkout; show 0 until order is placed.
  const loyaltyDiscount = redeemPoints ? redeemPoints * parseFloat(loyaltyPointsValue) : 0;
  const totalBeforeGC = subtotal + (couponFreeShipping ? 0 : shippingTotal) - couponDiscount - loyaltyDiscount;
  const giftCardDeduction = Math.min(giftCardAmount, Math.max(0, totalBeforeGC));
  const total = totalBeforeGC - giftCardDeduction;

  const canSubmit =
    items.length > 0 &&
    email.trim() !== "" &&
    isAddressFilled(address) &&
    selectedShipping !== "" &&
    selectedProvider !== "" &&
    !submitting;

  // Fetch payment methods on mount
  useEffect(() => {
    let cancelled = false;
    fetchPaymentMethods(storeSlug).then((methods) => {
      if (!cancelled) {
        setPaymentMethods(methods);
        if (methods.length === 1) setSelectedProvider(methods[0]!.provider);
      }
    });
    return () => { cancelled = true; };
  }, [storeSlug]);

  // Fetch shipping rates when address is filled
  const fetchRates = useCallback(async () => {
    if (!isAddressFilled(address) || items.length === 0) return;
    setLoadingRates(true);
    setShippingRates([]);
    setSelectedShipping("");
    const rates = await fetchShippingRates(storeSlug, {
      items: items.map((i) => ({
        product_id: i.productId,
        variant_id: i.variantId,
        quantity: i.qty,
        weight_grams: 500, // Default weight — real weight comes from product data in a follow-up
      })),
      ship_to: {
        line1: address.line1.trim(),
        city: address.city.trim(),
        region: address.region.trim() || undefined,
        postal_code: address.postal_code.trim() || undefined,
        country_code: address.country_code.trim().toUpperCase(),
      },
    });
    setShippingRates(rates);
    if (rates.length === 1) setSelectedShipping(rates[0]!.service);
    setLoadingRates(false);
  }, [address, items, storeSlug]);

  useEffect(() => {
    if (isAddressFilled(address)) {
      const timer = setTimeout(fetchRates, 400);
      return () => clearTimeout(timer);
    }
  }, [address, fetchRates]);

  // Fetch loyalty program + balance when email is entered. Debounced so we
  // don't hammer the API while the user is still typing. Clears redemption
  // state when the email becomes invalid or the customer isn't enrolled.
  useEffect(() => {
    const trimmed = email.trim();
    if (!trimmed || !trimmed.includes("@")) {
      setLoyaltyBalance(0);
      setRedeemPoints(null);
      return;
    }
    let cancelled = false;
    const timer = setTimeout(async () => {
      try {
        const qs = new URLSearchParams({
          store: storeSlug,
          email: trimmed,
        }).toString();
        const res = await fetch(`/api/checkout/loyalty/init?${qs}`, {
          cache: "no-store",
        });
        if (!res.ok || cancelled) return;
        const body = (await res.json()) as {
          program: {
            is_active: boolean;
            points_value: string;
            points_currency: string;
            min_redeem_points: number;
          } | null;
          customer: { points_balance: number } | null;
        };
        if (cancelled) return;
        if (!body.program || !body.program.is_active || !body.customer) {
          setLoyaltyBalance(0);
          setRedeemPoints(null);
          return;
        }
        setLoyaltyPointsValue(body.program.points_value);
        setLoyaltyPointsCurrency(body.program.points_currency);
        setLoyaltyMinRedeem(body.program.min_redeem_points);
        setLoyaltyBalance(body.customer.points_balance);
      } catch {
        if (!cancelled) {
          setLoyaltyBalance(0);
          setRedeemPoints(null);
        }
      }
    }, 500);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [email, storeSlug]);

  // Submit checkout
  const handleSubmit = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    setError(null);

    const checkoutItems: CheckoutItemBody[] = items.map((i) => ({
      product_id: i.productId,
      variant_id: i.variantId,
      title_snapshot: i.title,
      sku_snapshot: i.variantId,
      unit_price: i.priceAmount,
      quantity: i.qty,
      line_total: (Number.parseFloat(i.priceAmount) * i.qty).toFixed(2),
      currency_code: i.currencyCode,
      image_url: i.imageUrl,
    }));

    const body: CheckoutBody = {
      idempotency_key: crypto.randomUUID(),
      customer_email: email.trim(),
      customer_name: customerName.trim() || undefined,
      items: checkoutItems,
      shipping_address: toCheckoutAddress(address),
      shipping_service: selectedShipping,
      payment_provider: selectedProvider,
      subtotal: subtotal.toFixed(2),
      coupon_code: couponCode ?? undefined,
      gift_card_code: giftCardCode ?? undefined,
      redeem_points: redeemPoints ?? undefined,
    };

    const result = await submitCheckout(storeSlug, body);
    if (!result) {
      setError("Something went wrong placing your order. Please try again.");
      setSubmitting(false);
      return;
    }

    // Stash everything the order page needs to open the payment widget
    // before clearing the cart. Keyed by order id so a customer juggling
    // two checkouts in different tabs doesn't cross streams.
    const grandTotal = (
      subtotal +
      Number.parseFloat(result.shipping_total ?? "0") +
      Number.parseFloat(result.tax_total ?? "0") -
      Number.parseFloat(result.discount_total ?? "0") -
      Number.parseFloat(result.gift_card_applied ?? "0")
    ).toFixed(2);
    const pm = paymentMethods.find((p) => p.provider === result.provider);
    const pending = {
      orderId: result.order_id,
      provider: result.provider,
      paymentToken: result.payment_token,
      publicKey: pm?.public_key ?? "",
      amount: grandTotal,
      currencyCode: items[0]?.currencyCode ?? "INR",
      customerName: customerName.trim() || undefined,
      customerEmail: email.trim() || undefined,
    };
    if (typeof window !== "undefined") {
      try {
        sessionStorage.setItem(
          `mark8ly.pendingPayment.${result.order_id}`,
          JSON.stringify(pending),
        );
      } catch {
        // sessionStorage may be unavailable in incognito; fall through.
      }
    }

    clear();
    router.push(`/orders/${result.order_id}`);
  };

  if (items.length === 0 && !submitting) {
    return (
      <main id="main" className="min-h-screen bg-[color:var(--paper-200)]">
        <div className="mx-auto max-w-3xl px-6 py-8 sm:px-8">
          <StorefrontNav storeName="" />
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl text-[color:var(--ink-900)]">
            Checkout
          </h1>
          <div className="mt-12 text-center">
            <p className="text-lg text-[color:var(--ink-900)] opacity-60">
              Your cart is empty.
            </p>
            <Link
              href="/products"
              className="mt-4 inline-block text-sm font-semibold text-[color:var(--moss-700)] transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              Continue shopping
            </Link>
          </div>
        </div>
      </main>
    );
  }

  return (
    <main id="main" className="min-h-screen bg-[color:var(--paper-200)]">
      <div className="mx-auto max-w-3xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName="" />
        <Link
          href="/cart"
          className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--ink-900)] opacity-60 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          ← Back to cart
        </Link>
        <h1 className="mt-4 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl text-[color:var(--ink-900)]">
          Checkout
        </h1>

        {/* Step indicator */}
        <CheckoutSteps
          contactDone={email.trim() !== ""}
          addressDone={isAddressFilled(address)}
          shippingDone={selectedShipping !== ""}
          paymentDone={selectedProvider !== ""}
        />

        {/* Order summary */}
        <section aria-labelledby="order-summary-heading" className="mt-8">
          <h2
            id="order-summary-heading"
            className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)] opacity-60"
          >
            Order summary
          </h2>
          <ul className="mt-4 divide-y divide-[color:var(--ink-900)]/10">
            {items.map((item) => {
              const lineTotal = Number.parseFloat(item.priceAmount) * item.qty;
              return (
                <li
                  key={`${item.productId}-${item.variantId}`}
                  className="flex gap-4 py-4"
                >
                  {item.imageUrl ? (
                    <div className="relative h-16 w-16 shrink-0 overflow-hidden rounded-md bg-[color:var(--paper-200)]">
                      <Image
                        src={item.imageUrl}
                        alt={item.title}
                        fill
                        sizes="64px"
                        className="object-cover"
                      />
                    </div>
                  ) : (
                    <div className="h-16 w-16 shrink-0 rounded-md bg-[color:var(--ink-900)]/5" />
                  )}
                  <div className="flex flex-1 items-start justify-between">
                    <div>
                      <p className="text-sm font-medium text-[color:var(--ink-900)]">
                        {item.title}
                      </p>
                      <p className="mt-0.5 text-xs text-[color:var(--ink-900)] opacity-50">
                        Qty {item.qty}
                      </p>
                    </div>
                    <p
                      className="text-sm text-[color:var(--ink-900)]"
                      style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
                    >
                      {formatPrice(lineTotal, item.currencyCode)}
                    </p>
                  </div>
                </li>
              );
            })}
          </ul>
        </section>

        {/* Coupon input — below order summary */}
        <div className="mt-6">
          <CouponInput
            storeSlug={storeSlug}
            customerEmail={email}
            subtotal={subtotal}
            currencyCode={currencyCode}
            onApplied={(result) => {
              setCouponCode(result.code);
              setCouponDiscount(Number.parseFloat(result.discount_amount));
              setCouponFreeShipping(result.free_shipping);
            }}
            onRemoved={() => {
              setCouponCode(null);
              setCouponDiscount(0);
              setCouponFreeShipping(false);
            }}
          />
        </div>

        {/* Gift card input — below coupon */}
        <div className="mt-4">
          <GiftCardInput
            storeSlug={storeSlug}
            currencyCode={currencyCode}
            onApplied={(code, balance) => {
              setGiftCardCode(code);
              setGiftCardAmount(Number.parseFloat(balance));
            }}
            onRemoved={() => {
              setGiftCardCode(null);
              setGiftCardAmount(0);
            }}
          />
        </div>

        {/* Contact */}
        <section aria-labelledby="contact-heading" className="mt-10">
          <h2
            id="contact-heading"
            className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)] opacity-60"
          >
            Contact
          </h2>
          <div className="mt-4 grid gap-4 sm:grid-cols-2">
            <div className="sm:col-span-2">
              <label htmlFor="email" className="block text-sm text-[color:var(--ink-900)]">
                Email address
              </label>
              <input
                id="email"
                type="email"
                required
                autoComplete="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="mt-1 w-full rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                placeholder="you@example.com"
              />
            </div>
            <div className="sm:col-span-2">
              <label htmlFor="customer-name" className="block text-sm text-[color:var(--ink-900)]">
                Full name
              </label>
              <input
                id="customer-name"
                type="text"
                autoComplete="name"
                value={customerName}
                onChange={(e) => setCustomerName(e.target.value)}
                className="mt-1 w-full rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                placeholder="Jane Doe"
              />
            </div>
          </div>
        </section>

        {/* Shipping address */}
        <section aria-labelledby="shipping-heading" className="mt-10">
          <h2
            id="shipping-heading"
            className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)] opacity-60"
          >
            Shipping address
          </h2>
          <AddressForm address={address} onChange={setAddress} />
        </section>

        {/* Shipping method */}
        <section aria-labelledby="shipping-method-heading" className="mt-10">
          <h2
            id="shipping-method-heading"
            className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)] opacity-60"
          >
            Shipping method
          </h2>
          <div aria-live="polite" aria-atomic="true">
          {!isAddressFilled(address) ? (
            <p className="mt-4 text-sm text-[color:var(--ink-900)] opacity-40">
              Enter your shipping address to see available methods.
            </p>
          ) : loadingRates ? (
            <p className="mt-4 text-sm text-[color:var(--ink-900)] opacity-40">
              Loading shipping rates...
            </p>
          ) : shippingRates.length === 0 ? (
            <p className="mt-4 text-sm text-[color:var(--ink-900)] opacity-40">
              We couldn&apos;t find shipping options for this address. Please double-check your details or try a different address.
            </p>
          ) : (
            <fieldset className="mt-4">
              <legend className="sr-only">Select a shipping method</legend>
              <div className="space-y-3">
                {shippingRates.map((rate) => (
                  <label
                    key={rate.service}
                    className={`flex cursor-pointer items-center justify-between rounded-md border px-4 py-3 transition-all duration-150 ${
                      selectedShipping === rate.service
                        ? "border-[color:var(--moss-700)] bg-[color:var(--moss-700)]/5"
                        : "border-[color:var(--ink-900)]/15 hover:border-[color:var(--ink-900)]/30"
                    }`}
                  >
                    <div className="flex items-center gap-3">
                      <input
                        type="radio"
                        name="shipping-method"
                        value={rate.service}
                        checked={selectedShipping === rate.service}
                        onChange={() => setSelectedShipping(rate.service)}
                        className="accent-[color:var(--moss-700)]"
                      />
                      <div>
                        <p className="text-sm font-medium text-[color:var(--ink-900)]">
                          {humanizeService(rate.service)}
                        </p>
                        <p className="text-xs text-[color:var(--ink-900)] opacity-50">
                          Est. {rate.estimated_days} business day{rate.estimated_days !== 1 ? "s" : ""}
                        </p>
                      </div>
                    </div>
                    <span
                      className="text-sm text-[color:var(--ink-900)]"
                      style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
                    >
                      {formatPrice(Number.parseFloat(rate.price), rate.currency_code)}
                    </span>
                  </label>
                ))}
              </div>
            </fieldset>
          )}
          </div>
        </section>

        {/* Payment */}
        <section aria-labelledby="payment-heading" className="mt-10">
          <h2
            id="payment-heading"
            className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)] opacity-60"
          >
            Payment
          </h2>
          {paymentMethods.length === 0 ? (
            <p className="mt-4 text-sm text-[color:var(--ink-900)] opacity-40">
              Payment methods are being loaded. If this persists, please refresh the page.
            </p>
          ) : (
            <fieldset className="mt-4">
              <legend className="sr-only">Select a payment method</legend>
              <div className="space-y-3">
                {paymentMethods.map((pm) => (
                  <label
                    key={pm.provider}
                    className={`flex cursor-pointer items-center gap-3 rounded-md border px-4 py-3 transition-all duration-150 ${
                      selectedProvider === pm.provider
                        ? "border-[color:var(--moss-700)] bg-[color:var(--moss-700)]/5"
                        : "border-[color:var(--ink-900)]/15 hover:border-[color:var(--ink-900)]/30"
                    }`}
                  >
                    <input
                      type="radio"
                      name="payment-provider"
                      value={pm.provider}
                      checked={selectedProvider === pm.provider}
                      onChange={() => setSelectedProvider(pm.provider)}
                      className="accent-[color:var(--moss-700)]"
                    />
                    <span className="text-sm font-medium text-[color:var(--ink-900)]">
                      {providerLabel(pm.provider)}
                    </span>
                  </label>
                ))}
              </div>
            </fieldset>
          )}
          <p className="mt-3 text-xs text-[color:var(--ink-900)] opacity-40">
            Your payment is processed securely. We never store your card details.
          </p>
        </section>

        {/* Order totals */}
        <section aria-labelledby="totals-heading" className="mt-10 border-t border-[color:var(--ink-900)]/10 pt-6">
          <h2 id="totals-heading" className="sr-only">
            Order totals
          </h2>
          <dl aria-live="polite" className="space-y-2 text-sm text-[color:var(--ink-900)]">
            <div className="flex justify-between">
              <dt className="opacity-60">Subtotal ({count} {count === 1 ? "item" : "items"})</dt>
              <dd style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                {formatPrice(subtotal, currencyCode)}
              </dd>
            </div>
            {loyaltyBalance > 0 && (
              <LoyaltyRedemption
                pointsBalance={loyaltyBalance}
                pointsValue={loyaltyPointsValue}
                pointsCurrency={loyaltyPointsCurrency}
                currencyCode={currencyCode}
                minRedeemPoints={loyaltyMinRedeem}
                onToggle={setRedeemPoints}
              />
            )}
            {loyaltyDiscount > 0 && (
              <div className="flex justify-between">
                <dt className="opacity-60">Loyalty discount</dt>
                <dd className="text-[color:var(--moss-700)]" style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                  -{formatPrice(loyaltyDiscount, currencyCode)}
                </dd>
              </div>
            )}
            <div className="flex justify-between">
              <dt className="opacity-60">Shipping</dt>
              <dd style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                {selectedShipping
                  ? formatPrice(shippingTotal, currencyCode)
                  : "--"}
              </dd>
            </div>
            {couponDiscount > 0 && (
              <div className="flex justify-between">
                <dt className="opacity-60">Discount</dt>
                <dd className="text-[color:var(--moss-700)]" style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                  -{formatPrice(couponDiscount, currencyCode)}
                </dd>
              </div>
            )}
            {couponFreeShipping && (
              <div className="flex justify-between">
                <dt className="opacity-60">Shipping</dt>
                <dd className="text-[color:var(--moss-700)]" style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                  Free
                </dd>
              </div>
            )}
            {giftCardDeduction > 0 && (
              <div className="flex justify-between">
                <dt className="opacity-60">Gift card</dt>
                <dd className="text-[color:var(--moss-700)]" style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                  -{formatPrice(giftCardDeduction, currencyCode)}
                </dd>
              </div>
            )}
            <div className="flex justify-between">
              <dt className="opacity-60">Tax</dt>
              <dd className="opacity-40" style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                Calculated at checkout
              </dd>
            </div>
            <div className="flex justify-between border-t border-[color:var(--ink-900)]/10 pt-2 font-medium">
              <dt>Estimated total</dt>
              <dd
                className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg"
                style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
              >
                {formatPrice(total, currencyCode)}
              </dd>
            </div>
          </dl>
        </section>

        {/* Error */}
        {error && (
          <div role="alert" className="mt-6 rounded-md border border-[color:var(--danger,#8B2500)]/20 bg-[color:var(--danger,#8B2500)]/5 px-4 py-3 text-sm text-[color:var(--danger,#8B2500)]">
            {error}
          </div>
        )}

        {/* Place order */}
        <div className="mt-8">
          <button
            type="button"
            disabled={!canSubmit}
            onClick={handleSubmit}
            className="w-full rounded-md bg-[color:var(--ink-900)] px-6 py-3 text-sm font-medium text-[color:var(--paper-200)] transition-all duration-150 hover:opacity-90 active:scale-[0.99] disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            {submitting ? "Placing order..." : "Place order"}
          </button>
        </div>
      </div>
    </main>
  );
}

// ---------------------------------------------------------------------------
// Address form sub-component
// ---------------------------------------------------------------------------

interface AddressFormProps {
  address: AddressFields;
  onChange: (address: AddressFields) => void;
}

function AddressForm({ address, onChange }: AddressFormProps) {
  const update = (field: keyof AddressFields, value: string) =>
    onChange({ ...address, [field]: value });

  const inputClass =
    "mt-1 w-full rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]";

  return (
    <div className="mt-4 grid gap-4 sm:grid-cols-2">
      <div className="sm:col-span-2">
        <label htmlFor="ship-name" className="block text-sm text-[color:var(--ink-900)]">
          Full name
        </label>
        <input
          id="ship-name"
          type="text"
          required
          autoComplete="shipping name"
          value={address.name}
          onChange={(e) => update("name", e.target.value)}
          className={inputClass}
          placeholder="Jane Doe"
        />
      </div>
      <div className="sm:col-span-2">
        <label htmlFor="ship-line1" className="block text-sm text-[color:var(--ink-900)]">
          Address line 1
        </label>
        <input
          id="ship-line1"
          type="text"
          required
          autoComplete="shipping address-line1"
          value={address.line1}
          onChange={(e) => update("line1", e.target.value)}
          className={inputClass}
          placeholder="123 Main St"
        />
      </div>
      <div className="sm:col-span-2">
        <label htmlFor="ship-line2" className="block text-sm text-[color:var(--ink-900)]">
          Address line 2
        </label>
        <input
          id="ship-line2"
          type="text"
          autoComplete="shipping address-line2"
          value={address.line2}
          onChange={(e) => update("line2", e.target.value)}
          className={inputClass}
          placeholder="Apt, suite, etc. (optional)"
        />
      </div>
      <div>
        <label htmlFor="ship-city" className="block text-sm text-[color:var(--ink-900)]">
          City
        </label>
        <input
          id="ship-city"
          type="text"
          required
          autoComplete="shipping address-level2"
          value={address.city}
          onChange={(e) => update("city", e.target.value)}
          className={inputClass}
        />
      </div>
      <div>
        <label htmlFor="ship-region" className="block text-sm text-[color:var(--ink-900)]">
          State / Region
        </label>
        <input
          id="ship-region"
          type="text"
          autoComplete="shipping address-level1"
          value={address.region}
          onChange={(e) => update("region", e.target.value)}
          className={inputClass}
        />
      </div>
      <div>
        <label htmlFor="ship-postal" className="block text-sm text-[color:var(--ink-900)]">
          Postal code
        </label>
        <input
          id="ship-postal"
          type="text"
          autoComplete="shipping postal-code"
          value={address.postal_code}
          onChange={(e) => update("postal_code", e.target.value)}
          className={inputClass}
        />
      </div>
      <div>
        <label htmlFor="ship-country" className="block text-sm text-[color:var(--ink-900)]">
          Country
        </label>
        <select
          id="ship-country"
          required
          autoComplete="shipping country"
          value={address.country_code}
          onChange={(e) => update("country_code", e.target.value)}
          className={inputClass}
        >
          <option value="">Select country</option>
          {SUPPORTED_COUNTRIES.map((c) => (
            <option key={c.code} value={c.code}>{c.name}</option>
          ))}
        </select>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Supported countries (matches migration 000008 seed)
// ---------------------------------------------------------------------------

const SUPPORTED_COUNTRIES = [
  { code: "AU", name: "Australia" },
  { code: "CA", name: "Canada" },
  { code: "DE", name: "Germany" },
  { code: "ES", name: "Spain" },
  { code: "FR", name: "France" },
  { code: "GB", name: "United Kingdom" },
  { code: "ID", name: "Indonesia" },
  { code: "IN", name: "India" },
  { code: "IT", name: "Italy" },
  { code: "MY", name: "Malaysia" },
  { code: "NL", name: "Netherlands" },
  { code: "PH", name: "Philippines" },
  { code: "SG", name: "Singapore" },
  { code: "TH", name: "Thailand" },
  { code: "US", name: "United States" },
];

// ---------------------------------------------------------------------------
// Provider label helper
// ---------------------------------------------------------------------------

function providerLabel(provider: string): string {
  switch (provider) {
    case "stripe": return "Credit / Debit card";
    case "razorpay": return "Card, UPI, or Netbanking";
    case "paypal": return "PayPal";
    default: return provider.charAt(0).toUpperCase() + provider.slice(1);
  }
}

// ---------------------------------------------------------------------------
// Human-friendly shipping service name
// ---------------------------------------------------------------------------

function humanizeService(service: string): string {
  const map: Record<string, string> = {
    ground: "Standard Ground",
    express: "Express",
    priority: "Priority",
    economy: "Economy",
    overnight: "Overnight",
    two_day: "2-Day Shipping",
    standard: "Standard Shipping",
  };
  const key = service.toLowerCase().replace(/[\s-]+/g, "_");
  return map[key] ?? service.charAt(0).toUpperCase() + service.slice(1).replace(/_/g, " ");
}

// ---------------------------------------------------------------------------
// Step indicator
// ---------------------------------------------------------------------------

interface CheckoutStepsProps {
  contactDone: boolean;
  addressDone: boolean;
  shippingDone: boolean;
  paymentDone: boolean;
}

function CheckoutSteps({ contactDone, addressDone, shippingDone, paymentDone }: CheckoutStepsProps) {
  const steps = [
    { label: "Contact", done: contactDone },
    { label: "Address", done: addressDone },
    { label: "Shipping", done: shippingDone },
    { label: "Payment", done: paymentDone },
  ];
  return (
    <nav aria-label="Checkout progress" className="mt-6 flex items-center gap-1">
      {steps.map((step, i) => (
        <div key={step.label} className="flex items-center gap-1">
          {i > 0 && (
            <div className={`h-px w-6 ${step.done || steps[i - 1]!.done ? "bg-[color:var(--moss-700)]" : "bg-[color:var(--ink-900)]/15"}`} />
          )}
          <span
            className={`text-xs tracking-wide ${
              step.done
                ? "font-semibold text-[color:var(--moss-700)]"
                : "text-[color:var(--ink-900)] opacity-40"
            }`}
          >
            {step.label}
          </span>
        </div>
      ))}
    </nav>
  );
}
