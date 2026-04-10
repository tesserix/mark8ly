/**
 * Storefront client for checkout-related endpoints on marketplace-api.
 * Follows the same patterns as marketplace-api.ts — public-only,
 * STOREFRONT_KEY header, null on 404.
 */

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

function commonHeaders(): HeadersInit {
  const headers: Record<string, string> = {
    Accept: "application/json",
    "Content-Type": "application/json",
  };
  if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;
  return headers;
}

function storeUrl(storeSlug: string): string {
  return `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}`;
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface PaymentMethod {
  provider: string;
  methods: string[];
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
}

export interface CheckoutResult {
  order_id: string;
  order_number: string;
  payment_token: string;
  provider: string;
  tax_total: string;
  shipping_total: string;
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
  const url = `${storeUrl(storeSlug)}/payment-methods`;
  try {
    const res = await fetch(url, {
      headers: commonHeaders(),
      next: { revalidate: 120 },
    });
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
  const url = `${storeUrl(storeSlug)}/shipping-rates`;
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
  const url = `${storeUrl(storeSlug)}/checkout`;
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
