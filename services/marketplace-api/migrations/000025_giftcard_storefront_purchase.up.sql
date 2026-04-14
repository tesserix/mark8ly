ALTER TABLE gift_cards
  ADD COLUMN IF NOT EXISTS payment_status            varchar(20),
  ADD COLUMN IF NOT EXISTS payment_provider          varchar(20),
  ADD COLUMN IF NOT EXISTS payment_intent_id         varchar(255),
  ADD COLUMN IF NOT EXISTS checkout_session_id       varchar(255),
  ADD COLUMN IF NOT EXISTS purchased_via_storefront  boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS purchased_by_name         varchar(200),
  ADD COLUMN IF NOT EXISTS purchased_by_email        varchar(300);

-- `pending` is a new lifecycle state: the card exists but the purchase
-- payment hasn't cleared. Redeem is blocked while pending.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.check_constraints
    WHERE constraint_name = 'gift_cards_status_valid'
  ) THEN
    -- No existing constraint — no-op, status column just has a default
    NULL;
  END IF;
END$$;

-- Look up card by checkout session id in webhook path.
CREATE INDEX IF NOT EXISTS gift_cards_checkout_session_id_idx
  ON gift_cards (checkout_session_id) WHERE checkout_session_id IS NOT NULL;

-- Look up card by payment intent id in webhook path.
CREATE INDEX IF NOT EXISTS gift_cards_payment_intent_id_idx
  ON gift_cards (payment_intent_id) WHERE payment_intent_id IS NOT NULL;
