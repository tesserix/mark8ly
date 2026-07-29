-- Cumulative value refunded against a gift card's own PURCHASE.
--
-- Stripe fires `charge.refunded` for partial refunds as well as full ones,
-- and its `amount_refunded` is a running total, not the value of the single
-- refund that triggered the event. Storing the total we have already applied
-- to the card is what makes the two hard cases correct:
--
--   * sequential partials  — a second $20 refund arrives as amount_refunded
--                            = 30.00, so only the 20.00 delta is clawed back
--   * webhook redelivery   — the same total arrives twice and the second
--                            delivery has a zero delta, so it is a no-op
--
-- Without this column the only honest options are to claw back the whole
-- cumulative total on every delivery (double-charging the customer) or to
-- void the card outright (destroying value the merchant did not refund).
--
-- Scale 3, not 2 like the balance columns. This is a bookkeeping figure, not
-- a balance: it is compared against the provider's reported total to decide
-- whether a delivery is new, so it must round-trip that total EXACTLY. At
-- scale 2 a three-decimal currency (KWD, BHD, ...) would store 10.505 as
-- 10.50 and the next redelivery of the same event would see a positive
-- 0.005 delta and claw back again.
ALTER TABLE gift_cards
  ADD COLUMN IF NOT EXISTS refunded_amount numeric(12,3) NOT NULL DEFAULT 0;

-- NOT VALID skips the full-table validation scan and the ACCESS EXCLUSIVE
-- lock that comes with it; the constraint is still enforced on every insert
-- and update from this moment on. Existing rows trivially satisfy it — the
-- column is new and defaulted to 0 — so VALIDATE only has to confirm that,
-- and it takes a SHARE UPDATE EXCLUSIVE lock that does not block reads or
-- writes.
ALTER TABLE gift_cards DROP CONSTRAINT IF EXISTS gift_cards_refunded_amount_non_negative;

ALTER TABLE gift_cards
  ADD CONSTRAINT gift_cards_refunded_amount_non_negative
  CHECK (refunded_amount >= 0) NOT VALID;

ALTER TABLE gift_cards VALIDATE CONSTRAINT gift_cards_refunded_amount_non_negative;

-- Any card already sitting in the terminal `refunded` state had its purchase
-- refunded, so seed its cumulative total from the face value. Leaving these
-- at 0 would let a redelivered webhook see a positive delta.
--
-- This is a floor for the idempotency guard, not an audit figure: the code
-- that wrote those rows voided a card on ANY refund event, so the real
-- amount the provider refunded may have been less. Do not reconcile this
-- backfilled value against the provider dashboard. `refunded` is terminal
-- and short-circuits before the delta is even computed, so the exact number
-- does not change behaviour for these rows.
UPDATE gift_cards
   SET refunded_amount = initial_balance
 WHERE status = 'refunded'
   AND refunded_amount = 0;
