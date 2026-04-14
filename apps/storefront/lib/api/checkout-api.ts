/**
 * Storefront client for checkout-related endpoints on marketplace-api.
 *
 * Dual-mode: these helpers may be called from either server components
 * (e.g. /orders/[id]/page.tsx) or client components (e.g. /checkout).
 * On the server we hit marketplace-api directly with the
 * `X-Storefront-Key` secret. In the browser we cannot attach that
 * secret (and `MARKETPLACE_API_URL` is not a `NEXT_PUBLIC_*` var, so
 * the fallback would ship `http://localhost:8088` into the bundle) —
 * so client calls route through the same-origin `/api/checkout/*`
 * route handlers which re-issue the upstream request server-side.
 */

const IS_BROWSER = typeof window !== "undefined";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

function commonHeaders(): HeadersInit {
  const headers: Record<string, string> = {
    Accept: "application/json",
    "Content-Type": "application/json",
  };
  // Only the server has the storefront key. In the browser we rely on
  // the proxy route to attach it.
  if (!IS_BROWSER && STOREFRONT_KEY) {
    headers["X-Storefront-Key"] = STOREFRONT_KEY;
  }
  return headers;
}

function storeUrl(storeSlug: string): string {
  return `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}`;
}

// Same-origin proxy URL builder for browser-side calls.
function proxyUrl(path: string, storeSlug: string): string {
  const qs = new URLSearchParams({ store: storeSlug }).toString();
  return `/api/checkout/${path}?${qs}`;
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface PaymentMethod {
  provider: string;
  methods: string[];
  public_key?: string;
  mode?: "test" | "live" | string;
}

export interface ShippingRateItem {
  product_id: string;
  variant_id: string;
  quantity: number;
  weight_grams: number;
}

export interface ShippingRateAddress {
  line1: string;
  city: string;
  region?: string;
  postal_code?: string;
  country_code: string;
}

export interface ShippingRatesBody {
  items: ShippingRateItem[];
  ship_to: ShippingRateAddress;
}

export interface ShippingRate {
  service: string;
  carrier: string;
  price: string;
  currency_code: string;
  estimated_days: number;
}

export interface CheckoutItemBody {
  product_id?: string;
  variant_id?: string;
  title_snapshot: string;
  sku_snapshot: string;
  option_summary?: string;
  unit_price: string;
  quantity: number;
  line_total: string;
  currency_code: string;
  image_url?: string;
}

export interface CheckoutAddressBody {
  name: string;
  line1: string;
  line2?: string;
  city: string;
  region?: string;
  postal_code?: string;
  country_code: string;
  phone?: string;
}

export interface CheckoutBody {
  idempotency_key: string;
  cart_session_id?: string;
  customer_email: string;
  customer_name?: string;
  items: CheckoutItemBody[];
  shipping_address: CheckoutAddressBody;
  billing_address?: CheckoutAddressBody;
  shipping_service: string;
  payment_provider: string;
  subtotal: string;
  discount_total?: string;
  coupon_code?: string;
  gift_card_code?: string;
  redeem_points?: number;
}

export interface CheckoutResult {
  order_id: string;
  order_number: string;
  payment_token: string;
  provider: string;
  tax_total: string;
  shipping_total: string;
  discount_total?: string;
  coupon_code?: string;
  gift_card_applied?: string;
  points_redeemed?: number;
  points_earned?: number;
  total: string;
}

export interface OrderItem {
  title_snapshot: string;
  sku_snapshot: string;
  option_summary?: string;
  unit_price: string;
  quantity: number;
  line_total: string;
  currency_code: string;
  image_url?: string;
}

export interface OrderAddress {
  name: string;
  line1: string;
  line2?: string;
  city: string;
  region?: string;
  postal_code?: string;
  country_code: string;
  phone?: string;
}

export interface Order {
  id: string;
  order_number: string;
  status: string;
  payment_status: string;
  subtotal: string;
  shipping_total: string;
  tax_total: string;
  grand_total: string;
  currency_code: string;
  items: OrderItem[];
  shipping_address: OrderAddress;
  placed_at: string;
}

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

/**
 * Lists available payment methods for a store.
 */
export async function fetchPaymentMethods(
  storeSlug: string,
): Promise<PaymentMethod[]> {
  const url = IS_BROWSER
    ? proxyUrl("payment-methods", storeSlug)
    : `${storeUrl(storeSlug)}/payment-methods`;
  try {
    const res = await fetch(
      url,
      IS_BROWSER
        ? { headers: commonHeaders(), cache: "no-store" }
        : { headers: commonHeaders(), next: { revalidate: 120 } },
    );
    if (!res.ok) return [];
    const body = (await res.json()) as { data: PaymentMethod[] };
    return body.data ?? [];
  } catch {
    return [];
  }
}

/**
 * Fetches shipping rate quotes for a cart + destination address.
 */
export async function fetchShippingRates(
  storeSlug: string,
  body: ShippingRatesBody,
): Promise<ShippingRate[]> {
  const url = IS_BROWSER
    ? proxyUrl("shipping-rates", storeSlug)
    : `${storeUrl(storeSlug)}/shipping-rates`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: commonHeaders(),
      body: JSON.stringify(body),
    });
    if (!res.ok) return [];
    const json = (await res.json()) as { data: ShippingRate[] };
    return json.data ?? [];
  } catch {
    return [];
  }
}

/**
 * Submits the checkout. Returns the checkout result with payment token
 * and computed totals. Returns null on failure.
 */
export async function submitCheckout(
  storeSlug: string,
  body: CheckoutBody,
): Promise<CheckoutResult | null> {
  const url = IS_BROWSER
    ? proxyUrl("submit", storeSlug)
    : `${storeUrl(storeSlug)}/checkout`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: commonHeaders(),
      body: JSON.stringify(body),
    });
    if (!res.ok) return null;
    return (await res.json()) as CheckoutResult;
  } catch {
    return null;
  }
}

/**
 * Fetches an order by ID. Returns null on 404 (unknown order).
 */
export async function fetchOrder(
  storeSlug: string,
  orderId: string,
): Promise<Order | null> {
  const url = `${storeUrl(storeSlug)}/orders/${encodeURIComponent(orderId)}`;
  try {
    const res = await fetch(url, {
      headers: commonHeaders(),
      cache: "no-store",
    });
    if (res.status === 404) return null;
    if (!res.ok) return null;
    const body = (await res.json()) as { data: Order } | Order;
    // Handle both envelope { data: Order } and direct Order responses.
    if ("data" in body && body.data) return body.data;
    return body as Order;
  } catch {
    return null;
  }
}

// ─────────────────────────────────────────────────────────────────────────
// Marketing M2: Gift Card balance check
// ─────────────────────────────────────────────────────────────────────────

export interface GiftCardBalanceResult {
  code: string;
  current_balance: string;
  currency_code: string;
  status: string;
  expires_at?: string;
}

export async function checkGiftCardBalance(
  storeSlug: string,
  code: string,
): Promise<GiftCardBalanceResult | null> {
  const url = IS_BROWSER
    ? proxyUrl("gift-cards/check-balance", storeSlug)
    : `${storeUrl(storeSlug)}/gift-cards/check-balance`;
  const res = await fetch(url, {
    method: "POST",
    headers: commonHeaders(),
    body: JSON.stringify({ code }),
  });

  if (res.status === 404 || res.status === 410) {
    return null;
  }
  if (res.status === 429) {
    throw new Error("Too many requests. Please wait a moment and try again.");
  }
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new Error(body?.message ?? "Failed to check balance");
  }

  const json = await res.json();
  return json.data as GiftCardBalanceResult;
}
