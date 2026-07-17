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

// Re-exported from the schema module so the types can never drift from the
// validation again. The old OrderDetail/LineItem/Address/TimelineEvent were
// hand-written fictions (`line_items`/`shipping_address`/`timeline`/
// `tracking_number`/`payment_method`/`payment_transaction_id` never existed on
// the wire — the DTO sends `items[]`/`addresses[]`). Now schema-derived: the
// detail get() validates `orderDetailSchema`.
export type {
  Order,
  OrderDetail,
  OrderItem,
  OrderAddress,
  OrderTaxLine,
} from "./schemas/orders";

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

// Re-exported from the schema module so the types can never drift from the
// validation. Reviews had no schema at all before — the app punted with
// z.unknown(); this is the real wire shape from reviews_dto.go.
export type {
  Review,
  ReviewMedia,
  ReviewReply,
  ReviewStatus,
  ReviewListResponse,
} from "./schemas/reviews";
