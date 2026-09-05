-- 000131_promo_codes_console_sourced.down.sql
--
-- Reverting re-imposes NOT NULL on discount_type, discount_value and
-- stripe_coupon_id, restores the >= 12 code-length floor, and drops
-- trial_extension_days. Any row that only the loosened schema could hold has
-- no representation afterwards.
--
-- This migration therefore REFUSES to run when such rows exist rather than
-- deleting or defaulting them. Deleting them would make the rollback succeed
-- by destroying exactly the data #620 exists to store, and a promo_redemptions
-- row can already point at one. Reverting is only safe while the table holds
-- nothing the old shape could not express.

DO $$
DECLARE
    unrepresentable bigint;
BEGIN
    SELECT count(*) INTO unrepresentable
    FROM promo_codes
    WHERE discount_type        IS NULL
       OR discount_value       IS NULL
       OR stripe_coupon_id     IS NULL
       OR trial_extension_days IS NOT NULL
       OR char_length(code) < 12;

    IF unrepresentable > 0 THEN
        RAISE EXCEPTION
            'migration 000131 cannot be reverted: % promo_codes row(s) carry a null discount, a null stripe_coupon_id, a trial extension, or a code shorter than 12 characters — none of which the pre-000131 schema can hold. Reverting would have to delete them. Remove or rewrite these rows deliberately, or restore from a pre-migration snapshot.',
            unrepresentable;
    END IF;
END $$;

-- Reverse step 5: back to a full index on a NOT NULL column.
DROP INDEX promo_codes_stripe_idx;
CREATE INDEX promo_codes_stripe_idx ON promo_codes (stripe_coupon_id);

-- Reverse step 4.
ALTER TABLE promo_codes DROP CONSTRAINT promo_codes_discount_pair;
ALTER TABLE promo_codes DROP CONSTRAINT promo_codes_has_benefit;

-- Reverse step 3.
ALTER TABLE promo_codes DROP CONSTRAINT promo_codes_code_length;
ALTER TABLE promo_codes
    ADD CONSTRAINT promo_codes_code_length CHECK (char_length(code) >= 12);

-- Reverse step 2. Safe: the guard above proved no null rows remain.
ALTER TABLE promo_codes ALTER COLUMN stripe_coupon_id SET NOT NULL;
ALTER TABLE promo_codes ALTER COLUMN discount_value   SET NOT NULL;
ALTER TABLE promo_codes ALTER COLUMN discount_type    SET NOT NULL;

-- Reverse step 1.
ALTER TABLE promo_codes DROP CONSTRAINT promo_codes_trial_extension_days_positive;
ALTER TABLE promo_codes DROP COLUMN trial_extension_days;

COMMENT ON TABLE promo_codes IS
    '§7.1 — subscription promo codes; Stripe Coupon is backend of record.';
