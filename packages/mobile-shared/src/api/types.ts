export interface ApiClientConfig {
  baseUrl: string;
  storeSlug: string;
  getToken?: () => Promise<string | null>;
}

export interface ApiResponse<T> {
  data: T;
  meta?: {
    total?: number;
    page?: number;
    limit?: number;
  };
}

export interface ApiErrorBody {
  error: string;
  message: string;
  statusCode?: number;
}

export class ApiError extends Error {
  readonly statusCode: number;
  readonly body: ApiErrorBody;

  constructor(statusCode: number, body: ApiErrorBody) {
    super(body.message);
    this.name = "ApiError";
    this.statusCode = statusCode;
    this.body = body;
  }
}

// Product types
export interface ProductImage {
  id: string;
  url: string;
  alt: string | null;
  position: number;
}

export interface ProductVariant {
  id: string;
  title: string;
  sku: string | null;
  price: number;
  compareAtPrice: number | null;
  inventory: number;
  options: Record<string, string>;
}

export interface ProductOption {
  name: string;
  values: string[];
}

export interface ProductListItem {
  id: string;
  handle: string;
  title: string;
  description: string | null;
  price: number;
  compareAtPrice: number | null;
  images: ProductImage[];
  categoryId: string | null;
  tags: string[];
  rating: number | null;
  reviewCount: number;
}

export interface ProductDetail extends ProductListItem {
  variants: ProductVariant[];
  options: ProductOption[];
  vendor: string | null;
  metaTitle: string | null;
  metaDescription: string | null;
}

// Category types
export interface Category {
  id: string;
  slug: string;
  name: string;
  description: string | null;
  imageUrl: string | null;
  parentId: string | null;
  children?: Category[];
}

// Order types
export interface Address {
  firstName: string;
  lastName: string;
  line1: string;
  line2: string | null;
  city: string;
  state: string;
  postalCode: string;
  country: string;
  phone: string | null;
}

export interface OrderLineItem {
  id: string;
  productId: string;
  variantId: string;
  title: string;
  variantTitle: string;
  quantity: number;
  price: number;
  imageUrl: string | null;
}

export interface OrderEvent {
  id: string;
  type: string;
  message: string;
  createdAt: string;
}

export interface OrderSummary {
  id: string;
  orderNumber: string;
  status: string;
  paymentStatus: string;
  total: number;
  currency: string;
  itemCount: number;
  createdAt: string;
}

export interface OrderDetail extends OrderSummary {
  lineItems: OrderLineItem[];
  shippingAddress: Address | null;
  billingAddress: Address | null;
  subtotal: number;
  shippingCost: number;
  taxAmount: number;
  discountAmount: number;
  events: OrderEvent[];
}

// Checkout types
export interface CheckoutLineItem {
  variantId: string;
  quantity: number;
}

export interface ShippingRate {
  id: string;
  name: string;
  price: number;
  estimatedDays: number | null;
}

export interface CheckoutSubmitBody {
  lineItems: CheckoutLineItem[];
  shippingAddress: Address;
  billingAddress?: Address;
  shippingRateId: string;
  paymentMethodId?: string;
  couponCode?: string;
}

export interface CheckoutResult {
  orderId: string;
  orderNumber: string;
  total: number;
  currency: string;
  paymentIntentClientSecret?: string;
}

// Payment types
export interface PaymentMethod {
  id: string;
  brand: string;
  last4: string;
  expMonth: number;
  expYear: number;
  isDefault: boolean;
}

// Customer types
export interface CustomerProfile {
  uid: string;
  email: string;
  displayName: string | null;
  phone: string | null;
  avatarUrl: string | null;
  defaultAddress: Address | null;
}

// Wishlist types
export interface WishlistItem {
  id: string;
  productId: string;
  product: ProductListItem;
  addedAt: string;
}

// Loyalty types
export interface LoyaltyTier {
  id: string;
  name: string;
  minPoints: number;
  multiplier: number;
  perks: string[];
}

export interface LoyaltyProgram {
  id: string;
  name: string;
  pointsName: string;
  tiers: LoyaltyTier[];
  earnRules: Array<{
    action: string;
    points: number;
    description: string;
  }>;
}

export interface LoyaltyMe {
  points: number;
  tier: LoyaltyTier | null;
  lifetimePoints: number;
  nextTier: LoyaltyTier | null;
  pointsToNextTier: number | null;
}

// Review types
export interface ReviewItem {
  id: string;
  productId: string;
  product: Pick<ProductListItem, "id" | "title" | "handle" | "images">;
  rating: number;
  title: string | null;
  body: string | null;
  createdAt: string;
  updatedAt: string;
}

// Store branding
export interface StoreBranding {
  name: string;
  logoUrl: string | null;
  faviconUrl: string | null;
  primaryColor: string;
  accentColor: string | null;
  currency: string;
  locale: string;
}
