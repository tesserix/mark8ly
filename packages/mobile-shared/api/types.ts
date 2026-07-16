export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  next_cursor: string | null;
  has_more: boolean;
}

// Re-exported from the schema module so types can never drift from validation
// again. The hand-written versions invented `name`/`total_sold`/`stock` and a
// 5-field setup checklist; the wire has `title`/`units_sold`/`quantity` and 8.
// Imported (not just re-exported) because CustomerDetail below references
// RecentOrder locally — a bare `export type {} from` does not bind the name.
import type {
  DashboardStats,
  RecentOrder,
  TopProduct,
  LowStockItem,
  SetupChecklist,
  DashboardResponse,
} from "./schemas/dashboard";

export type {
  DashboardStats,
  RecentOrder,
  TopProduct,
  LowStockItem,
  SetupChecklist,
  DashboardResponse,
};

// Re-exported from the schema module so the type can never drift from the
// validation again. Imported (not just re-exported) because OrderDetail
// below extends it locally — a bare `export type {} from` does not bind
// the name.
import type { Order } from "./schemas/orders";

export type { Order };

/**
 * ⚠️ LARGELY FICTIONAL — deliberately left un-migrated. Verified against
 * AdminOrderResponse (orders_dto.go:152-188) on 2026-07-16: `line_items`
 * (the wire sends `items`), `shipping_address` (the wire sends `addresses[]`),
 * `timeline`, `tracking_number`, `payment_method` and
 * `payment_transaction_id` DO NOT EXIST on the wire. The detail screen has
 * always been broken and is unreachable in practice (the store has 0 orders).
 * Rewriting it is its own sub-project — see
 * docs/superpowers/specs/2026-07-16-mobile-admin-lists-bcd-design.md.
 * Nothing passes a schema for this type, so it fails the same way it always has.
 */
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

// Re-exported from the schema module so the type can never drift from the
// validation again. The hand-written `Customer` required first_name/last_name
// and `CustomerDetail` invented average_order_value/recent_orders/review_count
// that the backend has never had.
export type { Customer, CustomerDetail, CustomerAddress } from "./schemas/customers";
