DROP INDEX IF EXISTS gift_cards_checkout_session_id_idx;
DROP INDEX IF EXISTS gift_cards_payment_intent_id_idx;

ALTER TABLE gift_cards
  DROP COLUMN IF EXISTS purchased_by_email,
  DROP COLUMN IF EXISTS purchased_by_name,
  DROP COLUMN IF EXISTS purchased_via_storefront,
  DROP COLUMN IF EXISTS checkout_session_id,
  DROP COLUMN IF EXISTS payment_intent_id,
  DROP COLUMN IF EXISTS payment_provider,
  DROP COLUMN IF EXISTS payment_status;
