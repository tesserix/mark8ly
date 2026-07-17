import { z } from "zod";
import { money, loyaltyPaged } from "../schema-helpers";

/** A loyalty tier (loyalty_dto.go:55-59). `multiplier` is a decimal → `money`. */
export const loyaltyTierSchema = z.object({
  name: z.string(),
  min_points: z.number(),
  multiplier: money,
});
export type LoyaltyTier = z.infer<typeof loyaltyTierSchema>;

/**
 * Loyalty program config — LoyaltyProgramResponse (loyalty_dto.go:39-53).
 * `points_per_unit`/`points_value`/tier `multiplier` are decimals → `money`.
 * `point_expiry_days` is a pointer WITHOUT omitempty → JSON `null` →
 * `.nullable()`. GetProgram returns `{data: null}` when unconfigured, so the
 * api wraps this in `envelopedNullable`.
 */
export const loyaltyProgramSchema = z.object({
  id: z.string(),
  is_active: z.boolean(),
  points_per_unit: money,
  points_currency: z.string(),
  signup_bonus: z.number(),
  referral_bonus: z.number(),
  referee_bonus: z.number(),
  point_expiry_days: z.number().nullable(),
  min_redeem_points: z.number(),
  points_value: money,
  tiers: z.array(loyaltyTierSchema),
  created_at: z.string(),
  updated_at: z.string(),
});
export type LoyaltyProgram = z.infer<typeof loyaltyProgramSchema>;

/** A loyalty member (loyalty_dto.go:61-70). `customer_name` omitempty → optional. */
export const loyaltyMemberSchema = z.object({
  id: z.string(),
  customer_email: z.string(),
  customer_name: z.string().optional(),
  points_balance: z.number(),
  lifetime_points: z.number(),
  tier: z.string(),
  referral_code: z.string(),
  enrolled_at: z.string(),
});
export type LoyaltyMember = z.infer<typeof loyaltyMemberSchema>;

/** A points-ledger row (loyalty_dto.go:72-81). All nullable fields omitempty → optional. */
export const loyaltyTxnSchema = z.object({
  id: z.string(),
  // "earn" | "redeem" | "adjust" | "expire" | "signup" | "referral"
  type: z.string(),
  points: z.number(),
  balance_after: z.number(),
  description: z.string().optional(),
  adjusted_by: z.string().optional(),
  order_id: z.string().optional(),
  created_at: z.string(),
});
export type LoyaltyTxn = z.infer<typeof loyaltyTxnSchema>;

/** A referral (loyalty_dto.go:83-92). `completed_at` omitempty → optional. */
export const referralSchema = z.object({
  id: z.string(),
  referrer_id: z.string(),
  referee_id: z.string(),
  status: z.string(),
  referrer_bonus: z.number(),
  referee_bonus: z.number(),
  completed_at: z.string().optional(),
  created_at: z.string(),
});
export type Referral = z.infer<typeof referralSchema>;

/** GET member envelope: `{data: member, transactions: {data, meta:{total,page,limit}}}`. */
export const loyaltyMemberDetailSchema = z.object({
  data: loyaltyMemberSchema,
  transactions: loyaltyPaged(loyaltyTxnSchema),
});
export type LoyaltyMemberDetail = z.infer<typeof loyaltyMemberDetailSchema>;

/** LIST envelopes: loyalty's own `{data, meta:{total,page,limit}}` shape. */
export const loyaltyMemberListSchema = loyaltyPaged(loyaltyMemberSchema);
export type LoyaltyMemberListResponse = z.infer<typeof loyaltyMemberListSchema>;
export const referralListSchema = loyaltyPaged(referralSchema);
export type ReferralListResponse = z.infer<typeof referralListSchema>;
