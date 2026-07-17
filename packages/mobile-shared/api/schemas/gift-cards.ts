import { z } from "zod";
import { money, paginated } from "../schema-helpers";

/**
 * Wire truth for gift cards — AdminGiftCardResponse (gift_cards_dto.go:24-48).
 * `initial_balance`/`current_balance` are shopspring/decimal → `money`. Every
 * nullable field here IS omitempty → ABSENT when nil → `.optional()`.
 * `created_at` is a non-pointer time.Time → always present (RFC3339 string).
 */
export const giftCardSchema = z.object({
  id: z.string(),
  code: z.string(),
  code_display: z.string(),
  initial_balance: money,
  current_balance: money,
  currency_code: z.string(),
  // "active" | "redeemed" | "expired" | "disabled"
  status: z.string(),
  sender_name: z.string().optional(),
  sender_email: z.string().optional(),
  recipient_name: z.string().optional(),
  recipient_email: z.string().optional(),
  message: z.string().optional(),
  purchased_at: z.string().optional(),
  expires_at: z.string().optional(),
  created_at: z.string(),
  purchased_via_storefront: z.boolean(),
  payment_status: z.string().optional(),
  payment_provider: z.string().optional(),
  purchased_by_name: z.string().optional(),
  purchased_by_email: z.string().optional(),
});
export type GiftCard = z.infer<typeof giftCardSchema>;

/** A gift-card ledger row (gift_cards_dto.go:51-59). */
export const giftCardTxnSchema = z.object({
  id: z.string(),
  // "issue" | "redeem" | "refund" | "adjust"
  type: z.string(),
  amount: money,
  balance_after: money,
  order_id: z.string().optional(),
  note: z.string().optional(),
  created_at: z.string(),
});
export type GiftCardTxn = z.infer<typeof giftCardTxnSchema>;

/** GET detail = card + its transaction ledger (gift_cards_dto.go:62-65). */
export const giftCardDetailSchema = giftCardSchema.extend({
  transactions: z.array(giftCardTxnSchema),
});
export type GiftCardDetail = z.infer<typeof giftCardDetailSchema>;

/** LIST envelope: standard `{data, meta}` (gift_cards.go:73). */
export const giftCardListSchema = paginated(giftCardSchema);
export type GiftCardListResponse = z.infer<typeof giftCardListSchema>;
