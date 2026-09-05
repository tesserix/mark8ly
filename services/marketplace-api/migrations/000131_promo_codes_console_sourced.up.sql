-- 000131_promo_codes_console_sourced.up.sql
-- #726 step 1 — let promo_codes hold what the tesserix-home console publishes.
--
-- mark8ly's promo_codes (000060) and the console's table (tesserix-home 0046)
-- were designed independently. The fatal mismatch: the console allows a
-- TRIAL-EXTENSION-ONLY code — trial_extension_days set, discount null, no
-- Stripe coupon minted — and this table has discount_type NOT NULL,
-- discount_value NOT NULL, stripe_coupon_id NOT NULL, and no trial-extension
-- column at all. #620 ("redeem a promo code at onboarding to extend the
-- trial") is entirely about those codes.
--
-- Loosening costs nothing today: promo.Repository.Create has zero production
-- callers, no migration seeds a row, and no endpoint creates one. The table is
-- empty, so no existing data has invariants to weaken. What IS lost is the
-- NOT NULLs' implicit guarantees, so this migration replaces them with
-- explicit CHECKs rather than simply dropping them.

-- 1. The trial extension itself. Constraint mirrors the console's own.
ALTER TABLE promo_codes ADD COLUMN trial_extension_days INTEGER;

ALTER TABLE promo_codes
    ADD CONSTRAINT promo_codes_trial_extension_days_positive
    CHECK (trial_extension_days IS NULL OR trial_extension_days > 0);

COMMENT ON COLUMN promo_codes.trial_extension_days IS
    'Days added to the store trial when this code is redeemed (#620). NULL = the code extends no trial.';

-- 2. A console code may carry no discount and no Stripe coupon.
--
-- The column-level CHECK (discount_value > 0) from 000060 is deliberately left
-- in place: per SQL semantics a CHECK is satisfied when it evaluates to TRUE
-- *or* UNKNOWN, so NULL > 0 yields NULL and passes. It therefore keeps meaning
-- "a discount that exists must be positive" without blocking a null discount.
-- Verified against a live database by
-- TestPromoCodes_TrialExtensionOnlyRowInserts in internal/promo.
ALTER TABLE promo_codes ALTER COLUMN discount_type    DROP NOT NULL;
ALTER TABLE promo_codes ALTER COLUMN discount_value   DROP NOT NULL;
ALTER TABLE promo_codes ALTER COLUMN stripe_coupon_id DROP NOT NULL;

-- 3. Replace the >= 12 length floor.
--
-- 12 is right for codes WE generate — promo.MinCodeLength keeps enforcing it
-- there, because a self-minted code's length is its entropy budget (§7.3).
-- It cannot hold for codes the console defines: the console has no length
-- constraint and its own worked example is 'LAUNCH50', 8 characters. Human
-- campaign codes are routinely 6-10.
--
-- The new floor is 4, which is not an entropy claim — a console code's safety
-- comes from its redemption limits, not from being unguessable. It exists to
-- reject the cases that are a bug under any policy: the empty string, a stray
-- single character, a truncated ingest. 4 is the shortest length any plausible
-- real campaign code reaches ('XMAS', 'B2B1'), so it rejects garbage without
-- rejecting anything the console can legitimately publish.
ALTER TABLE promo_codes DROP CONSTRAINT promo_codes_code_length;
ALTER TABLE promo_codes
    ADD CONSTRAINT promo_codes_code_length CHECK (char_length(code) >= 4);

-- 4. Replace what the NOT NULLs guaranteed.
--
-- A row must deliver SOMETHING. Without this, three nullable columns permit a
-- row that neither discounts nor extends — a code that is accepted at
-- redemption and does nothing.
ALTER TABLE promo_codes
    ADD CONSTRAINT promo_codes_has_benefit
    CHECK (trial_extension_days IS NOT NULL OR discount_type IS NOT NULL);

-- Discount type and value are set together or not at all. Nullable
-- individually, they would otherwise permit 'percentage' with no value (which
-- validator.go would have to invent a meaning for) or a bare value with no
-- type. Both sides are IS NULL tests, so the expression is never UNKNOWN and
-- the constraint cannot pass by accident.
ALTER TABLE promo_codes
    ADD CONSTRAINT promo_codes_discount_pair
    CHECK ((discount_type IS NULL) = (discount_value IS NULL));

-- 5. stripe_coupon_id is now nullable, and rows without a coupon are not worth
-- indexing — the index exists to find a row BY its coupon id.
DROP INDEX promo_codes_stripe_idx;
CREATE INDEX promo_codes_stripe_idx ON promo_codes (stripe_coupon_id)
    WHERE stripe_coupon_id IS NOT NULL;

COMMENT ON TABLE promo_codes IS
    '§7.1 — subscription promo codes. Definitions originate in the tesserix-home console (#726); a code may carry a discount (Stripe Coupon is then the backend of record), a trial extension (#620), or both.';
