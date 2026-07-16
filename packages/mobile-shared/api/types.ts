// Re-exported from the schema module so types can never drift from validation
// again. The hand-written versions invented `name`/`total_sold`/`stock` and a
// 5-field setup checklist; the wire has `title`/`units_sold`/`quantity` and 8.
// Imported (not just re-exported) for consistency with the Order re-export
// pattern below. Nothing in this file references these names locally any
// more — Task 2 deleted the hand-written CustomerDetail, the last local
// user of RecentOrder — but the import+export pair is harmless and kept so
// this block doesn't need special-casing relative to the rest of the file.
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

// Re-exported from the schema module so the types can never drift from the
// validation again. The hand-written versions were entirely fictional:
// `name`, `price`, `compare_at_price`, `sku`, `stock`, `thumbnail_url`,
// `barcode`, `category_id`, `category_name` and `Variant.name`/`Variant.stock`
// — not one of those keys exists on the wire. The real shape is `title` plus
// `variants[]` (price is a QUOTED decimal string) and `media[]`.
export type {
  Product,
  ProductDetail,
  ProductVariant,
  ProductMedia,
} from "./schemas/products";

// Re-exported from the schema module so the type can never drift from
// validation again. The hand-written version invented `body`, `read` and
// `deep_link` — the wire sends `message` (optional), `is_read`, and
// resource_type/resource_id; there is no deep_link anywhere in the backend.
export type { Notification } from "./schemas/notifications";

// Re-exported from the schema module so the type can never drift from the
// validation again. Was a hand-written 3-field interface; the wire has 6.
export type { Store } from "./schemas/stores";

// Re-exported from the schema module so the type can never drift from the
// validation again. The hand-written `Customer` required first_name/last_name
// and `CustomerDetail` invented average_order_value/recent_orders/review_count
// that the backend has never had.
export type { Customer, CustomerDetail, CustomerAddress } from "./schemas/customers";
