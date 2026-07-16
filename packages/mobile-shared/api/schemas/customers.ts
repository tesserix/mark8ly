import { z } from "zod";
import { money, paginated } from "../schema-helpers";

/**
 * Wire truth for the admin customer endpoints, verified against prod
 * 2026-07-16 (`GET /api/v1/mobile/admin/stores/{id}/customers`).
 *
 * Every optional field below is a Go `omitempty` string
 * (customers_dto.go:24-36) — it is ABSENT from the JSON, not null. Using
 * .nullable() here would reject the only real customer that exists.
 *
 * There is deliberately no `recent_orders`, `average_order_value` or
 * `review_count`: the backend has never had them. The app derives the
 * average from order_count/total_spent and does not show recent orders.
 */
export const customerSchema = z.object({
  id: z.string(),
  email: z.string(),
  first_name: z.string().optional(),
  last_name: z.string().optional(),
  phone: z.string().optional(),
  avatar_url: z.string().optional(),
  tags: z.array(z.string()),
  status: z.string(),
  block_reason: z.string().optional(),
  notes: z.string().optional(),
  marketing_opt_in: z.boolean(),
  order_count: z.number(),
  // float64 on the wire today, but money tolerates the quoted-decimal form
  // the rest of the API uses, so this survives a backend switch to decimal.
  total_spent: money,
  last_order_at: z.string().optional(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Customer = z.infer<typeof customerSchema>;

/** customers_dto.go:47-59. Only line1/city/country_code/name are guaranteed. */
export const customerAddressSchema = z.object({
  id: z.string(),
  label: z.string().optional(),
  is_default: z.boolean(),
  name: z.string(),
  line1: z.string(),
  line2: z.string().optional(),
  city: z.string(),
  region: z.string().optional(),
  postal_code: z.string().optional(),
  country_code: z.string(),
  phone: z.string().optional(),
});
export type CustomerAddress = z.infer<typeof customerAddressSchema>;

/**
 * The detail endpoint returns the customer FLAT plus `addresses` — it does
 * NOT use the `{data}` envelope (customers.go:108 `c.JSON(200, resp)`).
 */
export const customerDetailSchema = customerSchema.extend({
  addresses: z.array(customerAddressSchema),
});
export type CustomerDetail = z.infer<typeof customerDetailSchema>;

export const customerListSchema = paginated(customerSchema);
export type CustomerListResponse = z.infer<typeof customerListSchema>;
