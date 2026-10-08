export interface StorefrontProductImage {
  id: string;
  url: string;
  alt: string;
  position: number;
}

export interface StorefrontProduct {
  id: string;
  handle: string;
  title: string;
  description: string;
  price_amount: string;
  compare_at_price: string;
  currency_code: string;
  status: string;
  stock_status: string;
  stock_quantity: number;
  images: StorefrontProductImage[];
  category_name: string;
  average_rating: number;
  review_count: number;
}

export interface StorefrontVariant {
  id: string;
  sku: string;
  price_amount: string;
  compare_at_price: string;
  stock_quantity: number;
  stock_status: string;
  option_values: Record<string, string>;
}

export interface StorefrontProductOption {
  name: string;
  values: string[];
}

/** One value of a choice field. */
export interface StorefrontPersonalisationOption {
  id: string;
  label: string;
  price_delta: string;
}

/**
 * One thing the buyer fills in before a product can be added to the cart
 * (#962, #965). Mirrors marketplace-api's StorefrontPersonalisationField
 * exactly; the server sends only the keys that apply to a kind, so
 * presence is the signal — `max_length` on a text field, `min_px` on an
 * image.
 */
export interface StorefrontPersonalisationField {
  id: string;
  key: string;
  label: string;
  kind: "image" | "text" | "textarea" | "select" | "checkbox";
  required: boolean;
  position: number;
  help_text?: string;
  max_length?: number;
  max_images?: number;
  /** Below this short-edge size the buyer is warned their image may print soft. */
  min_px?: number;
  /** The merchant's mockup and the rectangle, in PERCENTAGES, the artwork occupies in it (#966). Both or neither. */
  mockup_url?: string;
  print_area?: { x: number; y: number; w: number; h: number };
  /** Advisory only: checkout re-reads every delta from the catalog (#967). */
  price_delta?: string;
  options?: StorefrontPersonalisationOption[];
}

export interface StorefrontProductDetail extends StorefrontProduct {
  variants: StorefrontVariant[];
  options: StorefrontProductOption[];
  /** Detail-only; absent on a product that asks the buyer for nothing. */
  personalisation?: StorefrontPersonalisationField[];
}

export interface StorefrontCategory {
  id: string;
  name: string;
  slug: string;
  image_url: string;
  product_count: number;
}

/** One answer, in the shape the checkout endpoint validates and prices (#967). */
export interface CheckoutLinePersonalisation {
  field_id: string;
  upload_id?: string;
  text?: string;
  option_id?: string;
  checked?: boolean;
}

export interface CheckoutLineItem {
  product_id: string;
  variant_id: string;
  quantity: number;
  /** Omitted, not empty, for a line with nothing on it. */
  personalisation?: CheckoutLinePersonalisation[];
}

/** The signed PUT marketplace-api issues for one buyer image (#963). */
export interface PersonalisationUploadURL {
  upload_id: string;
  url: string;
  expires_at: string;
}

export interface ShippingRate {
  id: string;
  carrier: string;
  service: string;
  estimated_days: string;
  price_amount: string;
  currency_code: string;
}

export interface StorefrontAddress {
  id: string;
  name: string;
  line1: string;
  line2: string;
  city: string;
  region: string;
  postal_code: string;
  country: string;
  is_default: boolean;
}

export interface CheckoutSubmitBody {
  email: string;
  customer_name: string;
  line_items: CheckoutLineItem[];
  shipping_address: Omit<StorefrontAddress, "id" | "is_default">;
  shipping_rate_id: string;
  payment_provider: string;
  coupon_code?: string;
  gift_card_code?: string;
  loyalty_points?: number;
  idempotency_key: string;
  save_address?: boolean;
  /**
   * The cart identity the buyer's uploads were created under (#969). The
   * web storefront carries this in a cookie; a native app has no cookie
   * jar it can rely on, so it travels in the body. Without it checkout
   * cannot claim the artwork, and the 72h sweeper destroys it.
   */
  cart_token?: string;
}

export interface CheckoutResult {
  order_id: string;
  order_number: string;
  payment_token: string;
  payment_provider: string;
  totals: {
    subtotal: string;
    shipping: string;
    discount: string;
    tax: string;
    total: string;
    currency_code: string;
  };
}

export interface PaymentMethod {
  provider: string;
  enabled: boolean;
  supports_apple_pay: boolean;
  supports_google_pay: boolean;
}

export interface CustomerProfile {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  phone: string;
  created_at: string;
}

export interface WishlistItem {
  product_id: string;
  handle: string;
  title: string;
  price_amount: string;
  currency_code: string;
  image_url: string;
  stock_status: string;
  added_at: string;
}

export interface LoyaltyTier {
  name: string;
  min_points: number;
  multiplier: number;
}

export interface LoyaltyProgram {
  name: string;
  enabled: boolean;
  points_per_currency: number;
  currency_per_point: number;
  tiers: LoyaltyTier[];
  signup_bonus: number;
  referral_bonus: number;
}

export interface LoyaltyMe {
  points_balance: number;
  lifetime_points: number;
  current_tier: string;
  next_tier: string;
  points_to_next_tier: number;
  referral_code: string;
}

export interface StorefrontOrderSummary {
  id: string;
  order_number: string;
  status: string;
  total_amount: string;
  currency_code: string;
  item_count: number;
  created_at: string;
}

export interface StorefrontOrderLineItem {
  product_id: string;
  variant_id: string;
  title: string;
  variant_title: string;
  sku: string;
  quantity: number;
  unit_price: string;
  line_total: string;
  image_url: string;
}

export interface StorefrontOrderEvent {
  type: string;
  description: string;
  created_at: string;
}

export interface StorefrontOrderDetail extends StorefrontOrderSummary {
  customer_email: string;
  customer_name: string;
  line_items: StorefrontOrderLineItem[];
  shipping_address: StorefrontAddress;
  shipping_method: string;
  shipping_cost: string;
  tracking_number: string;
  payment_method: string;
  payment_amount: string;
  subtotal: string;
  discount_amount: string;
  tax_amount: string;
  timeline: StorefrontOrderEvent[];
}

export interface ReviewItem {
  id: string;
  product_id: string;
  product_handle: string;
  product_title: string;
  product_image_url: string;
  rating: number;
  title: string;
  body: string;
  created_at: string;
}

export interface StoreBranding {
  store_name: string;
  logo_url: string;
  primary_color: string;
  accent_color: string;
  background_color: string;
  font_family: string;
  banner_url: string;
  banner_title: string;
}
