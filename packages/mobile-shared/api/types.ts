export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  next_cursor: string | null;
  has_more: boolean;
}

export interface DashboardStats {
  revenue_today: number;
  revenue_week: number;
  revenue_month: number;
  revenue_change_pct: number;
  revenue_trend: number[];
  orders_today: number;
  orders_pending: number;
  orders_fulfilled: number;
  orders_cancelled: number;
  customers_total: number;
  customers_new_this_week: number;
  pending_reviews: number;
}

export interface RecentOrder {
  id: string;
  order_number: string;
  customer_email: string;
  grand_total: number;
  status: string;
  created_at: string;
}

export interface TopProduct {
  id: string;
  name: string;
  total_sold: number;
  revenue: number;
}

export interface LowStockItem {
  id: string;
  name: string;
  stock: number;
  thumbnail_url: string | null;
}

export interface SetupChecklist {
  has_products: boolean;
  has_payment: boolean;
  has_shipping: boolean;
  has_domain: boolean;
  has_branding: boolean;
}

export interface DashboardResponse {
  stats: DashboardStats;
  recent_orders: RecentOrder[];
  top_products: TopProduct[];
  low_stock: LowStockItem[];
  setup_checklist: SetupChecklist;
}

export interface Order {
  id: string;
  order_number: string;
  status: string;
  customer_email: string;
  customer_name: string;
  grand_total: number;
  item_count: number;
  created_at: string;
  updated_at: string;
}

export interface OrderDetail extends Order {
  line_items: LineItem[];
  shipping_address: Address | null;
  shipping_method: string | null;
  tracking_number: string | null;
  payment_method: string | null;
  payment_transaction_id: string | null;
  timeline: TimelineEvent[];
}

export interface LineItem {
  id: string;
  product_id: string;
  product_name: string;
  variant_name: string | null;
  quantity: number;
  unit_price: number;
  thumbnail_url: string | null;
}

export interface Address {
  line1: string;
  line2: string | null;
  city: string;
  state: string;
  postal_code: string;
  country: string;
}

export interface TimelineEvent {
  type: string;
  message: string;
  created_at: string;
}

export interface Product {
  id: string;
  name: string;
  status: string;
  price: number;
  compare_at_price: number | null;
  sku: string | null;
  stock: number;
  thumbnail_url: string | null;
  created_at: string;
}

export interface ProductDetail extends Product {
  description: string | null;
  barcode: string | null;
  category_id: string | null;
  category_name: string | null;
  tags: string[];
  media: MediaItem[];
  variants: Variant[];
}

export interface MediaItem {
  id: string;
  url: string;
  position: number;
}

export interface Variant {
  id: string;
  name: string;
  sku: string | null;
  price: number;
  stock: number;
}

export interface Customer {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  phone: string | null;
  order_count: number;
  total_spent: number;
  status: string;
  created_at: string;
}

export interface CustomerDetail extends Customer {
  avatar_url: string | null;
  average_order_value: number;
  recent_orders: RecentOrder[];
  review_count: number;
}

export interface Notification {
  id: string;
  type: string;
  title: string;
  body: string;
  read: boolean;
  deep_link: string | null;
  created_at: string;
}

// Re-exported from the schema module so the type can never drift from the
// validation again. Was a hand-written 3-field interface; the wire has 6.
export type { Store } from "./schemas/stores";
