ALTER TABLE gift_cards DROP CONSTRAINT IF EXISTS gift_cards_refunded_amount_non_negative;

ALTER TABLE gift_cards DROP COLUMN IF EXISTS refunded_amount;
