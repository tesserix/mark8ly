import { z } from "zod";
import { money } from "../schema-helpers";

/**
 * Wire truth for coupons — AdminCouponResponse (coupons_dto.go:49-70).
 * `value/min_purchase/max_discount` are shopspring/decimal → QUOTED strings →
 * `money`. The nullable fields are Go pointers WITHOUT omitempty, so they
 * marshal as JSON `null` (present-as-null → `.nullable()`), NOT absent.
 */
export const couponSchema = z.object({
  id: z.string(),
  code: z.string(),
  title: z.string(),
  description: z.string().nullable(),
  // "percentage" | "fixed_amount" | "free_shipping"
  type: z.string(),
  value: money,
  currency_code: z.string().nullable(),
  min_purchase: money.nullable(),
  max_discount: money.nullable(),
  usage_limit: z.number().nullable(),
  per_customer: z.number(),
  target_type: z.string(),
  target_ids: z.array(z.string()),
  stackable: z.boolean(),
  starts_at: z.string(),
  ends_at: z.string().nullable(),
  // "active" | "scheduled" | "expired" | "disabled"
  status: z.string(),
  usage_count: z.number(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Coupon = z.infer<typeof couponSchema>;

/** A single coupon_usage row (coupons_dto.go:73-80). */
export const couponUsageSchema = z.object({
  id: z.string(),
  order_id: z.string(),
  customer_email: z.string(),
  discount_amount: money,
  currency_code: z.string(),
  created_at: z.string(),
});
export type CouponUsage = z.infer<typeof couponUsageSchema>;

/** LIST envelope: `{data, total, page}` (coupons.go:73). */
export const couponListSchema = z.object({
  data: z.array(couponSchema),
  total: z.number(),
  page: z.number(),
});
export type CouponListResponse = z.infer<typeof couponListSchema>;

/**
 * GET envelope: `{data, usage?, usage_total?}` (coupons.go:168/177). The usage
 * arrays are dropped on the non-fatal usage-lookup-failed path, so both are
 * optional.
 */
export const couponDetailEnvelopeSchema = z.object({
  data: couponSchema,
  usage: z.array(couponUsageSchema).optional(),
  usage_total: z.number().optional(),
});
export type CouponDetailEnvelope = z.infer<typeof couponDetailEnvelopeSchema>;

/** Discount types the create form offers. */
export const COUPON_TYPES = ["percentage", "fixed_amount", "free_shipping"] as const;
export type CouponType = (typeof COUPON_TYPES)[number];
