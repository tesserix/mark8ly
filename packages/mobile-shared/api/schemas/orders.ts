import { z } from "zod";
import { money, paginated } from "../schema-helpers";

/**
 * Wire truth for the admin order LIST endpoint — AdminOrderResponse
 * (orders_dto.go:152-188). The Bondi store has zero orders, so unlike
 * products/customers this could not be verified against a live payload;
 * it is derived from the Go DTO, which is the only truth available.
 *
 * Every money field is a shopspring/decimal.Decimal, which marshals QUOTED
 * ("84.00") unless MarshalJSONWithoutQuotes is set — a repo-wide grep
 * confirms it never is. Hence `money`, not z.number().
 *
 * Deliberately NOT modelled here: the order DETAIL shape. The detail screen
 * still uses the hand-written OrderDetail type and passes no schema — see
 * the spec (2026-07-16-mobile-admin-lists-bcd-design.md, "C: orders").
 * Six of the twelve fields it reads do not exist on the wire; fixing that is
 * its own sub-project against a seeded order.
 */
export const orderSchema = z.object({
  id: z.string(),
  tenant_id: z.string(),
  store_id: z.string(),
  order_number: z.string(),
  idempotency_key: z.string(),
  customer_email: z.string(),
  // *string + omitempty -> ABSENT, not null.
  customer_name: z.string().optional(),
  status: z.string(),
  payment_status: z.string(),
  fulfillment_status: z.string(),
  subtotal: money,
  shipping_total: money,
  tax_total: money,
  discount_total: money,
  grand_total: money,
  refunded_amount: money,
  currency_code: z.string(),
  shipping_service: z.string().optional(),
  shipping_carrier: z.string().optional(),
  // Present on the wire; the list UI does not read them yet. Kept loose on
  // purpose: modelling them properly belongs with the detail sub-project.
  items: z.array(z.unknown()),
  addresses: z.array(z.unknown()),
  placed_at: z.string(),
  cancelled_at: z.string().optional(),
  fulfilled_at: z.string().optional(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Order = z.infer<typeof orderSchema>;

export const orderListSchema = paginated(orderSchema);
export type OrderListResponse = z.infer<typeof orderListSchema>;

/**
 * Order DETAIL shapes — from AdminOrderItemResponse / AdminAddressResponse /
 * AdminOrderTaxLineResponse (orders_dto.go:124-196). Same list envelope caveat:
 * `omitempty` Go pointers are ABSENT (`.optional()`), money is a quoted decimal.
 *
 * The list `orderSchema` keeps `items`/`addresses` as `z.array(z.unknown())`
 * (the list UI never reads them); the DETAIL tightens them into real shapes and
 * adds `tax_lines` (Get-only, omitempty). Fields the wire does NOT send —
 * `line_items`, `shipping_address`, `timeline`, `tracking_number`,
 * `payment_method`, `payment_transaction_id` — are deliberately absent.
 */
export const orderItemSchema = z.object({
  id: z.string(),
  product_id: z.string().optional(),
  variant_id: z.string().optional(),
  title_snapshot: z.string(),
  sku_snapshot: z.string(),
  option_summary: z.string().optional(),
  unit_price: money,
  quantity: z.number(),
  line_total: money,
  currency_code: z.string(),
});
export type OrderItem = z.infer<typeof orderItemSchema>;

/** kind is "shipping" | "billing"; region/postal/line2/phone are omitempty. */
export const orderAddressSchema = z.object({
  kind: z.string(),
  name: z.string(),
  line1: z.string(),
  line2: z.string().optional(),
  city: z.string(),
  region: z.string().optional(),
  postal_code: z.string().optional(),
  country_code: z.string(),
  phone: z.string().optional(),
});
export type OrderAddress = z.infer<typeof orderAddressSchema>;

export const orderTaxLineSchema = z.object({
  description: z.string(),
  rate: money,
  amount: money,
  jurisdiction: z.string().optional(),
});
export type OrderTaxLine = z.infer<typeof orderTaxLineSchema>;

export const orderDetailSchema = orderSchema.extend({
  items: z.array(orderItemSchema),
  addresses: z.array(orderAddressSchema),
  tax_lines: z.array(orderTaxLineSchema).optional(),
});
export type OrderDetail = z.infer<typeof orderDetailSchema>;
