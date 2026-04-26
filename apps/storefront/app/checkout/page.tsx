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
import { toast } from "@/lib/toast";
import { CouponInput } from "@/components/checkout/CouponInput";
import { GiftCardInput } from "@/components/checkout/GiftCardInput";
import { LoyaltyRedemption } from "@/components/checkout/LoyaltyRedemption";
import {
  fetchPaymentMethods,
  fetchShippingOptions,
  fetchShippingRates,
  submitCheckout,
  type PaymentMethod,
  type ShippingOption,
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
  phone: string;
}

interface SavedAddress {
  id: string;
  label?: string;
  name?: string;
  line1: string;
  line2?: string;
  city?: string;
  region?: string;
  postal_code?: string;
  country_code: string;
  phone?: string;
  is_default?: boolean;
}

const EMPTY_ADDRESS: AddressFields = {
  name: "",
  line1: "",
  line2: "",
  city: "",
  region: "",
  postal_code: "",
  country_code: "",
  phone: "",
};

// Indian mobile numbers are 10 digits starting with 6/7/8/9. Delhivery
// rejects shipment-create for IN destinations with an empty phone, so
// we enforce the format client-side as well as server-side.
const IN_PHONE_RE = /^[6-9]\d{9}$/;

function isAddressFilled(a: AddressFields): boolean {
  const basicsOk =
    a.name.trim() !== "" &&
    a.line1.trim() !== "" &&
    a.city.trim() !== "" &&
    a.country_code.trim().length === 2;
  if (!basicsOk) return false;
  if (a.country_code.trim().toUpperCase() === "IN") {
    return IN_PHONE_RE.test(a.phone.trim());
  }
  return true;
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
    phone: a.phone.trim() || undefined,
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
  const [shippingOptions, setShippingOptions] = useState<ShippingOption[]>([]);
  const [selectedShipping, setSelectedShipping] = useState<string>("");
  const [selectedProvider, setSelectedProvider] = useState<string>("");
  const [loadingRates, setLoadingRates] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // "Save this address to my profile" prompt. Shown only when the
  // current form address doesn't match any of the customer's saved
  // addresses (exact-compare on line1 + postal_code + country).
  const [savedAddresses, setSavedAddresses] = useState<SavedAddress[]>([]);
  const [saveAddress, setSaveAddress] = useState(false);
  const [saveAddressLabel, setSaveAddressLabel] = useState<"Home" | "Office" | "Other">("Home");

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
    // Also pull the merchant's configured carrier list so we can render
    // a helpful fallback when the address is outside any shipping zone.
    fetchShippingOptions(storeSlug).then((opts) => {
      if (!cancelled) setShippingOptions(opts);
    });
    // And the customer's saved address book — if the typed address is
    // new, we prompt the customer to save it after submit.
    fetch("/api/account/addresses", { cache: "no-store" })
      .then((r) => (r.ok ? r.json() : null))
      .then((body: { data: SavedAddress[] } | null) => {
        if (!cancelled && body?.data) setSavedAddresses(body.data);
      })
      .catch(() => { /* not signed in — ignore */ });
    // Pre-populate Contact (email + full name) from the customer's
    // profile so returning buyers don't retype it. Only sets the field
    // when it's still empty so a guest checkout flow that types an
    // email first isn't overwritten.
    fetch("/api/account/profile", { cache: "no-store" })
      .then((r) => (r.ok ? r.json() : null))
      .then(
        (body: {
          data?: {
            email?: string;
            first_name?: string;
            last_name?: string;
          };
        } | null) => {
          if (cancelled || !body?.data) return;
          setEmail((prev) => prev || (body.data?.email ?? ""));
          const full = [body.data.first_name, body.data.last_name]
            .filter((s): s is string => Boolean(s && s.trim()))
            .join(" ");
          if (full) setCustomerName((prev) => prev || full);
        },
      )
      .catch(() => { /* not signed in — ignore */ });
    return () => { cancelled = true; };
  }, [storeSlug]);

  // Unique destinations the merchant ships to, derived from the loaded
  // shipping_options. Used both to populate the country dropdown and
  // to lock it when the merchant only ships to one country (the typical
  // case in v1 — `australia-store` ships to AU only).
  const shipsToCountries = useMemo(() => {
    const set = new Set<string>();
    for (const o of shippingOptions) {
      for (const c of o.supported_countries) set.add(c);
    }
    return Array.from(set);
  }, [shippingOptions]);

  // Pre-populate the buyer's country from the store's single shipping
  // destination — saves a tap and avoids the "Sorry, we don't ship to
  // <wrong country>" dead-end on an empty form.
  useEffect(() => {
    if (
      shipsToCountries.length === 1 &&
      shipsToCountries[0] &&
      !address.country_code
    ) {
      setAddress((prev) => ({ ...prev, country_code: shipsToCountries[0]! }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shipsToCountries]);

  // Derive whether the current address is already in the book.
  const addressKey = (line1: string, postal: string, country: string): string =>
    `${line1.trim().toLowerCase()}|${postal.trim()}|${country.trim().toUpperCase()}`;
  const currentKey = addressKey(address.line1, address.postal_code, address.country_code);
  const addressAlreadySaved = savedAddresses.some(
    (a) => addressKey(a.line1, a.postal_code ?? "", a.country_code) === currentKey,
  );
  const canOfferSave =
    !addressAlreadySaved &&
    isAddressFilled(address) &&
    savedAddresses.length < 5;

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
        // Real weight + dims from the variant the buyer selected.
        // Server falls back to default envelope when these are 0/missing.
        weight_grams: i.weightGrams && i.weightGrams > 0 ? i.weightGrams : 500,
        length_cm: i.lengthCm,
        width_cm: i.widthCm,
        height_cm: i.heightCm,
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
      tax_code: i.taxCode,
      tax_rate_override: i.taxRateOverride,
      tax_category: i.taxCategory,
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
      toast({
        title: "Could not place order",
        description: "Please review your details and try again.",
        tone: "error",
      });
      setSubmitting(false);
      return;
    }
    toast({
      title: `Order ${result.order_number} placed`,
      description: "Complete payment to confirm.",
      tone: "success",
    });

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

    // If the customer asked to save this shipping address, do that
    // best-effort — failures shouldn't block the order from completing.
    if (saveAddress) {
      try {
        await fetch("/api/account/addresses", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            label: saveAddressLabel,
            name: address.name,
            line1: address.line1,
            line2: address.line2 || undefined,
            city: address.city,
            region: address.region || undefined,
            postal_code: address.postal_code || undefined,
            country_code: address.country_code,
          }),
        });
      } catch {
        // ignore — user can still save the address from /account/addresses
      }
    }

    clear();
    router.push(`/orders/${result.order_id}`);
  };

  if (items.length === 0 && !submitting) {
    return (
      <main id="main" className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
        {/* Nav sits in a full-width wrapper so it renders at its own
            max-width (matches home, products, cart, etc). The narrower
            max-w-3xl is only for the checkout content below. */}
        <div className="mx-auto max-w-6xl px-6 pt-8 sm:px-8">
          <StorefrontNav storeName="" />
        </div>
        <div className="mx-auto max-w-3xl px-6 pb-8 sm:px-8">
          <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-3xl text-[color:var(--storefront-text,var(--ink-900))]">
            Checkout
          </h1>
          <div className="mt-12 text-center">
            <p className="text-lg text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
              Your cart is empty.
            </p>
            <Link
              href="/products"
              className="mt-4 inline-block text-sm font-semibold text-[color:var(--storefront-accent,var(--moss-700))] transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
            >
              Continue shopping
            </Link>
          </div>
        </div>
      </main>
    );
  }

  return (
    <main id="main" className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <div className="mx-auto max-w-6xl px-6 pt-8 sm:px-8">
        <StorefrontNav storeName="" />
      </div>
      <div className="mx-auto max-w-3xl px-6 pb-8 sm:px-8">
        <Link
          href="/cart"
          className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        >
          ← Back to cart
        </Link>
        <h1 className="mt-4 font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-3xl text-[color:var(--storefront-text,var(--ink-900))]">
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
            className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60"
          >
            Order summary
          </h2>
          <ul className="mt-4 divide-y divide-[color:var(--storefront-text,var(--ink-900))]/10">
            {items.map((item) => {
              const lineTotal = Number.parseFloat(item.priceAmount) * item.qty;
              return (
                <li
                  key={`${item.productId}-${item.variantId}`}
                  className="flex gap-4 py-4"
                >
                  {item.imageUrl ? (
                    <div className="relative h-16 w-16 shrink-0 overflow-hidden rounded-md bg-[color:var(--storefront-background,var(--paper-200))]">
                      <Image
                        src={item.imageUrl}
                        alt={item.title}
                        fill
                        sizes="64px"
                        className="object-cover"
                      />
                    </div>
                  ) : (
                    <div className="h-16 w-16 shrink-0 rounded-md bg-[color:var(--storefront-accent,var(--ink-900))]/5" />
                  )}
                  <div className="flex flex-1 items-start justify-between">
                    <div>
                      <p className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
                        {item.title}
                      </p>
                      <p className="mt-0.5 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
                        Qty {item.qty}
                      </p>
                    </div>
                    <p
                      className="text-sm text-[color:var(--storefront-text,var(--ink-900))]"
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
            className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60"
          >
            Contact
          </h2>
          <div className="mt-4 grid gap-4 sm:grid-cols-2">
            <div className="sm:col-span-2">
              <label htmlFor="email" className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]">
                Email address
              </label>
              <input
                id="email"
                type="email"
                required
                autoComplete="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="mt-1 w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-surface)] px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
                placeholder="you@example.com"
              />
            </div>
            <div className="sm:col-span-2">
              <label htmlFor="customer-name" className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]">
                Full name
              </label>
              <input
                id="customer-name"
                type="text"
                autoComplete="name"
                value={customerName}
                onChange={(e) => setCustomerName(e.target.value)}
                className="mt-1 w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-surface)] px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
                placeholder="Jane Doe"
              />
            </div>
          </div>
        </section>

        {/* Shipping address */}
        <section aria-labelledby="shipping-heading" className="mt-10">
          <h2
            id="shipping-heading"
            className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60"
          >
            Shipping address
          </h2>
          {savedAddresses.length > 0 &&
            savedAddresses.some((a) =>
              shipsToCountries.length === 0 ||
              shipsToCountries.includes(a.country_code.toUpperCase()),
            ) && (
            <SavedAddressPicker
              addresses={savedAddresses}
              shipsToCountries={shipsToCountries}
              currentLine1={address.line1}
              currentPostal={address.postal_code}
              onSelect={(a) =>
                setAddress({
                  name: a.name ?? "",
                  line1: a.line1 ?? "",
                  line2: a.line2 ?? "",
                  city: a.city ?? "",
                  region: a.region ?? "",
                  postal_code: a.postal_code ?? "",
                  country_code: a.country_code.toUpperCase(),
                  phone: a.phone ?? "",
                })
              }
              onUseNew={() => setAddress(EMPTY_ADDRESS)}
            />
          )}
          <AddressForm
            address={address}
            onChange={setAddress}
            shipsToCountries={shipsToCountries}
          />

          {canOfferSave && (
            <div className="mt-4 rounded-md border border-[color:var(--storefront-accent,var(--moss-700))]/20 bg-[color:var(--storefront-accent,var(--moss-700))]/5 px-4 py-3">
              <label className="flex flex-wrap items-center gap-3 text-sm text-[color:var(--storefront-text,var(--ink-900))]">
                <input
                  type="checkbox"
                  checked={saveAddress}
                  onChange={(e) => setSaveAddress(e.target.checked)}
                  className="h-4 w-4 accent-[color:var(--storefront-accent,var(--moss-700))]"
                />
                <span>Save this address to my profile as</span>
                <select
                  value={saveAddressLabel}
                  onChange={(e) =>
                    setSaveAddressLabel(e.target.value as "Home" | "Office" | "Other")
                  }
                  disabled={!saveAddress}
                  className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-surface)] px-2 py-1 text-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))] disabled:opacity-40"
                >
                  <option value="Home">Home</option>
                  <option value="Office">Office</option>
                  <option value="Other">Other</option>
                </select>
              </label>
            </div>
          )}
          {addressAlreadySaved && (
            <p className="mt-3 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
              This address is already in your address book.
            </p>
          )}
        </section>

        {/* Shipping method */}
        <section aria-labelledby="shipping-method-heading" className="mt-10">
          <h2
            id="shipping-method-heading"
            className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60"
          >
            Shipping method
          </h2>
          <div aria-live="polite" aria-atomic="true">
          {!isAddressFilled(address) ? (
            <div className="mt-4 space-y-2">
              <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
                Enter your shipping address above to see live rates.
              </p>
              {shippingOptions.length > 0 && (
                <div className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-background,var(--paper-200))] px-4 py-3 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-80">
                  <p className="mb-1 font-semibold uppercase tracking-wider opacity-70">
                    This store ships via
                  </p>
                  <ul className="space-y-1">
                    {shippingOptions.map((o) => (
                      <li key={o.carrier} className="flex flex-wrap items-baseline gap-1.5">
                        <span className="font-medium">{prettyCarrier(o.carrier)}</span>
                        <span className="opacity-60">
                          ({o.services.map((s) => s.charAt(0).toUpperCase() + s.slice(1)).join(" / ")})
                        </span>
                        <span className="opacity-60">
                          — ships to {o.supported_countries.join(", ")}
                        </span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          ) : loadingRates ? (
            <p className="mt-4 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-40">
              Loading shipping rates...
            </p>
          ) : shippingRates.length === 0 ? (
            <NoShippingFallback
              options={shippingOptions}
              country={address.country_code}
            />
          ) : (
            <fieldset className="mt-4">
              <legend className="sr-only">Select a shipping method</legend>
              <div className="space-y-3">
                {shippingRates.map((rate) => (
                  <label
                    key={rate.service}
                    className={`flex cursor-pointer items-center justify-between rounded-md border px-4 py-3 transition-colors duration-150 ${
                      selectedShipping === rate.service
                        ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]/5"
                        : "border-[color:var(--storefront-text,var(--ink-900))]/15 hover:border-[color:var(--storefront-text,var(--ink-900))]/30"
                    }`}
                  >
                    <div className="flex items-center gap-3">
                      <input
                        type="radio"
                        name="shipping-method"
                        value={rate.service}
                        checked={selectedShipping === rate.service}
                        onChange={() => setSelectedShipping(rate.service)}
                        className="accent-[color:var(--storefront-accent,var(--moss-700))]"
                      />
                      <div>
                        <p className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
                          {humanizeService(rate.service)}
                        </p>
                        <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
                          Est. {rate.estimated_days} business day{rate.estimated_days !== 1 ? "s" : ""}
                        </p>
                      </div>
                    </div>
                    <span
                      className="text-sm text-[color:var(--storefront-text,var(--ink-900))]"
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
            className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60"
          >
            Payment
          </h2>
          {paymentMethods.length === 0 ? (
            <p className="mt-4 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-40">
              Payment methods are being loaded. If this persists, please refresh the page.
            </p>
          ) : (
            <fieldset className="mt-4">
              <legend className="sr-only">Select a payment method</legend>
              <div className="space-y-3">
                {paymentMethods.map((pm) => (
                  <label
                    key={pm.provider}
                    className={`flex cursor-pointer items-center gap-3 rounded-md border px-4 py-3 transition-colors duration-150 ${
                      selectedProvider === pm.provider
                        ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]/5"
                        : "border-[color:var(--storefront-text,var(--ink-900))]/15 hover:border-[color:var(--storefront-text,var(--ink-900))]/30"
                    }`}
                  >
                    <input
                      type="radio"
                      name="payment-provider"
                      value={pm.provider}
                      checked={selectedProvider === pm.provider}
                      onChange={() => setSelectedProvider(pm.provider)}
                      className="accent-[color:var(--storefront-accent,var(--moss-700))]"
                    />
                    <span className="flex flex-col">
                      <span className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
                        {providerBrand(pm.provider)}
                      </span>
                      <span className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
                        {providerLabel(pm.provider)}
                      </span>
                    </span>
                  </label>
                ))}
              </div>
            </fieldset>
          )}
          <p className="mt-3 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-40">
            Your payment is processed securely. We never store your card details.
          </p>
        </section>

        {/* Order totals */}
        <section aria-labelledby="totals-heading" className="mt-10 border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-6">
          <h2 id="totals-heading" className="sr-only">
            Order totals
          </h2>
          <dl aria-live="polite" className="space-y-2 text-sm text-[color:var(--storefront-text,var(--ink-900))]">
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
                <dd className="text-[color:var(--storefront-accent,var(--moss-700))]" style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
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
                <dd className="text-[color:var(--storefront-accent,var(--moss-700))]" style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                  -{formatPrice(couponDiscount, currencyCode)}
                </dd>
              </div>
            )}
            {couponFreeShipping && (
              <div className="flex justify-between">
                <dt className="opacity-60">Shipping</dt>
                <dd className="text-[color:var(--storefront-accent,var(--moss-700))]" style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
                  Free
                </dd>
              </div>
            )}
            {giftCardDeduction > 0 && (
              <div className="flex justify-between">
                <dt className="opacity-60">Gift card</dt>
                <dd className="text-[color:var(--storefront-accent,var(--moss-700))]" style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}>
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
            <div className="flex justify-between border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-2 font-medium">
              <dt>Estimated total</dt>
              <dd
                className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-lg"
                style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
              >
                {formatPrice(total, currencyCode)}
              </dd>
            </div>
          </dl>
        </section>

        {/* Error */}
        {error && (
          <div role="alert" className="mt-6 rounded-md border border-[color:var(--storefront-danger)]/20 bg-[color:var(--storefront-danger)]/5 px-4 py-3 text-sm text-[color:var(--storefront-danger)]">
            {error}
          </div>
        )}

        {/* Place order */}
        <div className="mt-8">
          <button
            type="button"
            disabled={!canSubmit}
            onClick={handleSubmit}
            className="w-full rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm font-medium text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity duration-150 hover:opacity-90 active:opacity-80 disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
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
  /**
   * ISO alpha-2 country codes the merchant accepts shipments to,
   * derived from the store's shipping_options. When the list has a
   * single entry the country select is locked to it (no point letting
   * the buyer pick a country the store can't ship to).
   */
  shipsToCountries?: string[];
}

function AddressForm({ address, onChange, shipsToCountries }: AddressFormProps) {
  const update = (field: keyof AddressFields, value: string) =>
    onChange({ ...address, [field]: value });

  const inputClass =
    "mt-1 w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-surface)] px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]";

  return (
    <div className="mt-4 grid gap-4 sm:grid-cols-2">
      <div className="sm:col-span-2">
        <label htmlFor="ship-name" className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]">
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
        <label htmlFor="ship-line1" className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]">
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
        <label htmlFor="ship-line2" className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]">
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
        <label htmlFor="ship-city" className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]">
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
        <label htmlFor="ship-region" className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]">
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
        <label htmlFor="ship-postal" className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]">
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
        <label htmlFor="ship-phone" className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]">
          Phone number
          {address.country_code.trim().toUpperCase() === "IN" && (
            <span className="ml-1 opacity-60">(required)</span>
          )}
        </label>
        <input
          id="ship-phone"
          type="tel"
          autoComplete="shipping tel"
          required={address.country_code.trim().toUpperCase() === "IN"}
          pattern={
            address.country_code.trim().toUpperCase() === "IN"
              ? "[6-9][0-9]{9}"
              : undefined
          }
          value={address.phone}
          onChange={(e) => update("phone", e.target.value)}
          className={inputClass}
          placeholder={
            address.country_code.trim().toUpperCase() === "IN"
              ? "10-digit mobile e.g. 9876543210"
              : ""
          }
        />
      </div>
      <div>
        <label htmlFor="ship-country" className="block text-sm text-[color:var(--storefront-text,var(--ink-900))]">
          Country
        </label>
        {shipsToCountries && shipsToCountries.length === 1 ? (
          <div
            id="ship-country"
            aria-readonly
            className={`${inputClass} flex cursor-not-allowed items-center gap-2 bg-[color:var(--storefront-text,var(--ink-900))]/[0.04]`}
          >
            <span>
              {SUPPORTED_COUNTRIES.find((c) => c.code === address.country_code)?.name ??
                address.country_code}
            </span>
            <span className="ml-auto text-[10px] uppercase tracking-[0.12em] opacity-50">
              Only destination served
            </span>
          </div>
        ) : (
          <select
            id="ship-country"
            required
            autoComplete="shipping country"
            value={address.country_code}
            onChange={(e) => update("country_code", e.target.value)}
            className={inputClass}
          >
            <option value="">Select country</option>
            {(shipsToCountries && shipsToCountries.length > 0
              ? SUPPORTED_COUNTRIES.filter((c) => shipsToCountries.includes(c.code))
              : SUPPORTED_COUNTRIES
            ).map((c) => (
              <option key={c.code} value={c.code}>
                {c.name}
              </option>
            ))}
          </select>
        )}
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

// SavedAddressPicker — radio list of the customer's saved addresses
// rendered above the AddressForm. Picking one fills the entire form so
// returning customers don't have to re-type their home / office address.
// Filtered to addresses in the store's ship-to country (an address in
// US is useless to a buyer checking out at an AU-only store).
function SavedAddressPicker({
  addresses,
  shipsToCountries,
  currentLine1,
  currentPostal,
  onSelect,
  onUseNew,
}: {
  addresses: SavedAddress[];
  shipsToCountries: string[];
  currentLine1: string;
  currentPostal: string;
  onSelect: (a: SavedAddress) => void;
  onUseNew: () => void;
}) {
  const usable =
    shipsToCountries.length === 0
      ? addresses
      : addresses.filter((a) =>
          shipsToCountries.includes(a.country_code.toUpperCase()),
        );

  // "selected" mirrors the form state — exact-compare on line1+postal so
  // the radio updates when the AddressForm is edited by hand below.
  const selectedId =
    usable.find(
      (a) =>
        a.line1.trim().toLowerCase() === currentLine1.trim().toLowerCase() &&
        (a.postal_code ?? "").trim() === currentPostal.trim() &&
        currentLine1.trim() !== "",
    )?.id ?? null;
  const usingNew = selectedId === null && currentLine1.trim() !== "";

  if (usable.length === 0) return null;

  return (
    <div className="mt-4 space-y-2">
      <p className="text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        Use a saved address
      </p>
      <ul className="space-y-2">
        {usable.map((a) => {
          const active = a.id === selectedId;
          return (
            <li key={a.id}>
              <button
                type="button"
                onClick={() => onSelect(a)}
                className={`flex w-full items-start gap-3 rounded-md border px-4 py-3 text-left text-sm transition-colors ${
                  active
                    ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]/[0.06]"
                    : "border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-surface)] hover:border-[color:var(--storefront-text,var(--ink-900))]/30"
                }`}
              >
                <span
                  aria-hidden
                  className={`mt-0.5 inline-block h-4 w-4 flex-shrink-0 rounded-full border ${
                    active
                      ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]"
                      : "border-[color:var(--storefront-text,var(--ink-900))]/30"
                  }`}
                />
                <span className="flex-1">
                  <span className="flex flex-wrap items-baseline gap-2">
                    <span className="font-medium text-[color:var(--storefront-text,var(--ink-900))]">
                      {a.label || "Home"}
                    </span>
                    {a.is_default && (
                      <span className="rounded-full bg-[color:var(--storefront-accent,var(--moss-700))]/10 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-[color:var(--storefront-accent,var(--moss-700))]">
                        Default
                      </span>
                    )}
                  </span>
                  <span className="mt-1 block text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
                    {a.name && <>{a.name} · </>}
                    {a.line1}
                    {a.line2 ? `, ${a.line2}` : ""}
                    {a.city ? `, ${a.city}` : ""}
                    {a.region ? `, ${a.region}` : ""}
                    {a.postal_code ? ` ${a.postal_code}` : ""}
                    {`, ${a.country_code}`}
                  </span>
                </span>
              </button>
            </li>
          );
        })}
        <li>
          <button
            type="button"
            onClick={onUseNew}
            className={`flex w-full items-start gap-3 rounded-md border px-4 py-3 text-left text-sm transition-colors ${
              usingNew
                ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]/[0.06]"
                : "border-dashed border-[color:var(--storefront-text,var(--ink-900))]/20 bg-transparent hover:border-[color:var(--storefront-text,var(--ink-900))]/40"
            }`}
          >
            <span
              aria-hidden
              className={`mt-0.5 inline-block h-4 w-4 flex-shrink-0 rounded-full border ${
                usingNew
                  ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]"
                  : "border-[color:var(--storefront-text,var(--ink-900))]/30"
              }`}
            />
            <span className="font-medium text-[color:var(--storefront-text,var(--ink-900))]">
              Use a new address
            </span>
          </button>
        </li>
      </ul>
    </div>
  );
}

// prettyCarrier maps a lowercase carrier slug (shipengine, delhivery, …)
// to the display name the carrier uses in its own marketing. Falls
// back to a capitalized version for unknown providers.
function prettyCarrier(slug: string): string {
  const map: Record<string, string> = {
    shipengine: "ShipEngine",
    delhivery: "Delhivery",
    ninjavan: "Ninja Van",
    fedex: "FedEx",
    dhl: "DHL",
    ups: "UPS",
    usps: "USPS",
    auspost: "Australia Post",
  };
  if (map[slug]) return map[slug];
  return slug.charAt(0).toUpperCase() + slug.slice(1);
}

// NoShippingFallback — renders when /shipping-rates comes back empty.
// If the merchant has any carriers configured, surface them (and the
// countries they cover) so the customer understands why their address
// didn't get a rate instead of seeing a generic dead-end message.
function NoShippingFallback({
  options,
  country,
}: {
  options: ShippingOption[];
  country: string;
}) {
  const pretty = (code: string): string =>
    SUPPORTED_COUNTRIES.find((c: { code: string; name: string }) => c.code === code)?.name ?? code;
  const serviceLabel = (s: string): string =>
    s.charAt(0).toUpperCase() + s.slice(1);

  if (options.length === 0) {
    return (
      <p className="mt-4 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        We couldn&apos;t find shipping options for this address. Please double-check your details or try a different address.
      </p>
    );
  }

  const shipsHere = options.some((o) =>
    country ? o.supported_countries.includes(country) : false,
  );
  const allCountries = Array.from(
    new Set(options.flatMap((o) => o.supported_countries)),
  );

  return (
    <div className="mt-4 space-y-3 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-background,var(--paper-200))] px-4 py-3 text-sm">
      <p className="text-[color:var(--storefront-text,var(--ink-900))]">
        {shipsHere
          ? "We couldn\u2019t get a live rate for this address right now. Please try again in a moment or double-check your postal code."
          : `Sorry \u2014 we don\u2019t currently ship to ${pretty(country) || "this country"}.`}
      </p>
      <div className="space-y-1.5 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-80">
        <p className="font-semibold uppercase tracking-wider opacity-70">
          This store ships via
        </p>
        <ul className="space-y-1">
          {options.map((o) => (
            <li key={o.carrier} className="flex flex-wrap items-baseline gap-1.5">
              <span className="font-medium">{prettyCarrier(o.carrier)}</span>
              <span className="opacity-60">
                ({o.services.map(serviceLabel).join(" / ")})
              </span>
              <span className="opacity-60">
                — {o.supported_countries.map(pretty).join(", ")}
              </span>
            </li>
          ))}
        </ul>
        {!shipsHere && allCountries.length > 0 && (
          <p className="mt-1 opacity-60">
            Try one of: {allCountries.map(pretty).join(", ")}.
          </p>
        )}
      </div>
    </div>
  );
}

function providerBrand(provider: string): string {
  switch (provider) {
    case "stripe": return "Stripe";
    case "razorpay": return "Razorpay";
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
            <div className={`h-px w-6 ${step.done || steps[i - 1]!.done ? "bg-[color:var(--storefront-accent,var(--moss-700))]" : "bg-[color:var(--storefront-accent,var(--ink-900))]/15"}`} />
          )}
          <span
            className={`text-xs tracking-wide ${
              step.done
                ? "font-semibold text-[color:var(--storefront-accent,var(--moss-700))]"
                : "text-[color:var(--storefront-text,var(--ink-900))] opacity-40"
            }`}
          >
            {step.label}
          </span>
        </div>
      ))}
    </nav>
  );
}
