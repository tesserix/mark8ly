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
  /**
   * Optional package dimensions in centimetres. The carrier (Australia
   * Post via ShipEngine, etc.) requires these — if any are missing the
   * server falls back to a default 30 × 20 × 10 cm envelope.
   */
  length_cm?: number;
  width_cm?: number;
  height_cm?: number;
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
  // Tax classification copied from the product at the time the item was
  // added to cart. The backend interprets these per the store's country
  // tax strategy (india_gst / flat_rate / taxjar).
  tax_code?: string;
  tax_rate_override?: string; // percentage as decimal string, e.g. "18.00"
  tax_category?: "standard" | "reduced" | "zero_rated" | "exempt";
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
  /**
   * Embedded-SDK token (Razorpay order_id, etc.) — set when the buyer
   * completes payment on a JS widget rendered on the order page.
   */
  payment_token: string;
  /**
   * Hosted-checkout redirect URL (Stripe Checkout). When set, the
   * storefront should redirect the browser straight to this URL after
   * placing the order — the buyer pays on the provider's domain and is
   * sent back via success_url. payment_token is empty in this path.
   */
  payment_redirect_url?: string;
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

export interface OrderShipment {
  carrier: string;
  service?: string;
  tracking_number?: string;
  status: string;
  estimated_delivery?: string;
  // delivered_at / shipped_at are stamped server-side when the merchant
  // transitions the shipment status. Surfaced so the receipt PDF can
  // print the real delivery moment instead of falling back to
  // order.updated_at as a proxy.
  delivered_at?: string;
  shipped_at?: string;
}

export interface OrderTaxLine {
  description: string;
  rate: string;
  amount: string;
  jurisdiction?: string;
}

export interface OrderTimelineEntry {
  kind: string;
  description: string;
  status?: string;
  carrier?: string;
  tracking_number?: string;
  occurred_at: string;
}

export interface Order {
  id: string;
  order_number: string;
  status: string;
  payment_status: string;
  // Buyer identity captured at checkout — surfaced so the customer-
  // rendered invoice/receipt PDF can stamp the bill-to contact line
  // and the email subject can address the customer by name.
  customer_email?: string;
  customer_name?: string;
  subtotal: string;
  shipping_total: string;
  tax_total: string;
  // discount_total: aggregate of coupon, loyalty, and manual discounts
  // applied at checkout. "0.00" when none. Used by the totals block on
  // the customer-rendered invoice/receipt PDF.
  discount_total?: string;
  grand_total: string;
  // refunded_amount: total refunded against this order. "0.00" when no
  // refunds have been issued. Surfaces partial + full refund history on
  // the customer's account order page.
  refunded_amount?: string;
  // tax_lines: per-jurisdiction breakdown (CGST/SGST/IGST for India,
  // VAT for flat-rate countries, state+county+city for TaxJar). Empty
  // for orders pre-dating the persistence wiring.
  tax_lines?: OrderTaxLine[];
  currency_code: string;
  items: OrderItem[];
  shipping_address: OrderAddress;
  // billing_address is omitted by the API when the shopper reused the
  // shipping address at checkout. PDF builders fall back to shipping_
  // address when this is undefined.
  billing_address?: OrderAddress;
  shipment?: OrderShipment;
  timeline?: OrderTimelineEntry[];
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

export interface ShippingOption {
  carrier: string;
  mode?: string;
  services: string[];
  supported_countries: string[];
}

/**
 * Fetches the merchant's configured carriers + the countries each one
 * ships to. Used as a helpful fallback on the checkout page when
 * /shipping-rates comes back empty for the entered address.
 */
export async function fetchShippingOptions(
  storeSlug: string,
): Promise<ShippingOption[]> {
  const url = IS_BROWSER
    ? proxyUrl("shipping-options", storeSlug)
    : `${storeUrl(storeSlug)}/shipping-options`;
  try {
    const res = await fetch(url, { headers: commonHeaders() });
    if (!res.ok) return [];
    const json = (await res.json()) as { data: ShippingOption[] };
    return json.data ?? [];
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

export interface TaxPreviewBody {
  items: CheckoutItemBody[];
  shipping_address: CheckoutAddressBody;
  subtotal: string;
  shipping_total?: string;
}

export interface TaxPreviewResult {
  tax_total: string;
  currency_code: string;
}

/**
 * Fetches an authoritative tax estimate from the marketplace-api so the
 * checkout page can show "Tax: A$24.99" before submit instead of asking
 * the buyer to trust an "Estimated total" without a breakdown. Same
 * calculator runs at submit time, so the number here is what they pay.
 */
export async function fetchTaxPreview(
  storeSlug: string,
  body: TaxPreviewBody,
): Promise<TaxPreviewResult | null> {
  const url = IS_BROWSER
    ? proxyUrl("tax-preview", storeSlug)
    : `${storeUrl(storeSlug)}/tax-preview`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: commonHeaders(),
      body: JSON.stringify(body),
    });
    if (!res.ok) return null;
    return (await res.json()) as TaxPreviewResult;
  } catch {
    return null;
  }
}

/**
 * Submits the checkout. Returns the checkout result with payment token
 * and computed totals. Returns null on failure.
 */
/**
 * A checkout that failed because someone else took the stock (#230/#232).
 *
 * Distinguished from every other failure on purpose. The backend answers
 * 409 `out_of_stock` naming the variant, and a shopper can act on that —
 * remove the line and retry. Collapsing it into the generic "something went
 * wrong" path, which is what this function used to do for every non-2xx,
 * tells a buyer nothing and reads as a site fault rather than a sold-out
 * item.
 */
export interface CheckoutOutOfStock {
  outOfStock: true;
  variantId?: string;
}

export type CheckoutOutcome = CheckoutResult | CheckoutOutOfStock | null;

export function isOutOfStock(o: CheckoutOutcome): o is CheckoutOutOfStock {
  return o !== null && (o as CheckoutOutOfStock).outOfStock === true;
}

export async function submitCheckout(
  storeSlug: string,
  body: CheckoutBody,
): Promise<CheckoutOutcome> {
  const url = IS_BROWSER
    ? proxyUrl("submit", storeSlug)
    : `${storeUrl(storeSlug)}/checkout`;
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: commonHeaders(),
      body: JSON.stringify(body),
    });
    if (res.status === 409) {
      // Read the variant so the page can name the item. A body we cannot
      // parse still yields outOfStock: the status is the fact that
      // matters, and losing it would drop the shopper back into the
      // generic error they cannot act on.
      try {
        const parsed = (await res.json()) as { error?: string; variant_id?: string };
        if (parsed?.error === "out_of_stock") {
          return { outOfStock: true, variantId: parsed.variant_id };
        }
      } catch {
        return { outOfStock: true };
      }
      return { outOfStock: true };
    }
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

// ---------------------------------------------------------------------------
// Stock holds (#232)
// ---------------------------------------------------------------------------

export interface CartHoldItemResult {
  variant_id: string;
  /** "held" or "insufficient". */
  status: string;
  /** What the shopper can actually have right now. */
  available: number;
}

export interface CartHoldsResult {
  cart_token: string;
  expires_at: string;
  items: CartHoldItemResult[];
}

/**
 * Place or refresh stock holds for the cart.
 *
 * Browser-only: the cart identity is an httpOnly cookie owned by the
 * `/api/checkout/cart-holds` route handler, so there is nothing useful this
 * can do server-side.
 *
 * Returns null on any failure, and callers treat that as "no reservation" —
 * NOT as an error to show. A failed hold must never block adding to a cart:
 * checkout enforces availability regardless (#230), so the worst case is the
 * shopper learns at checkout instead of at add-time. Blocking the add would
 * turn a backend blip into a store that cannot sell.
 */
export async function placeCartHolds(
  storeSlug: string,
  items: ReadonlyArray<{ variantId: string; qty: number }>,
): Promise<CartHoldsResult | null> {
  if (!IS_BROWSER || items.length === 0) return null;
  try {
    const res = await fetch(proxyUrl("cart-holds", storeSlug), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        items: items.map((i) => ({ variant_id: i.variantId, quantity: i.qty })),
      }),
    });
    if (!res.ok) return null;
    const body = (await res.json()) as { data?: CartHoldsResult };
    return body?.data ?? null;
  } catch {
    return null;
  }
}

/** Release the cart's holds. Best-effort: an unreleased hold expires anyway. */
export async function releaseCartHolds(storeSlug: string): Promise<void> {
  if (!IS_BROWSER) return;
  try {
    await fetch(proxyUrl("cart-holds", storeSlug), { method: "DELETE" });
  } catch {
    // Nothing to do: holds expire by the clock, so a failed release costs
    // at most one TTL of a reservation nobody is using.
  }
}
