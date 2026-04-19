-- 000060_promo_codes.up.sql
-- §7.1-§7.4 — Promo codes (billing subscription discounts). Backend of record is Stripe Coupon.

CREATE TYPE promo_discount_type AS ENUM ('percentage', 'amount');

CREATE TABLE promo_codes (
    id                               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    code                             VARCHAR(64)  NOT NULL,
    stripe_coupon_id                 VARCHAR(100) NOT NULL,
    discount_type                    promo_discount_type NOT NULL,
    -- percentage: basis points (5000 = 50.00%). amount: minor units in billing_currency.
    discount_value                   INTEGER      NOT NULL CHECK (discount_value > 0),
    -- months the discount applies; NULL = forever.
    max_duration_months              INTEGER,
    valid_from                       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    valid_until                      TIMESTAMPTZ,
    -- NULL = unlimited global redemptions.
    max_redemptions                  INTEGER,
    -- max_per_email = max redemptions by the same email address (§7.3 abuse prevention).
    max_per_email                    INTEGER      NOT NULL DEFAULT 1,
    -- min_effective_price_per_currency: JSONB map {"usd": minor_int, "inr": minor_int, ...}
    -- contains absolute floor overrides per currency (§7.4). NULL = no floor check.
    min_effective_price_per_currency JSONB,
    -- plans this code can be applied to; NULL = all plans.
    allowed_plans                    TEXT[],
    -- annual_only: if true, only applies to annual billing period.
    annual_only                      BOOLEAN      NOT NULL DEFAULT FALSE,
    created_by                       TEXT         NOT NULL,
    created_at                       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at                       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT promo_codes_code_length CHECK (char_length(code) >= 12)
);

CREATE UNIQUE INDEX promo_codes_code_uidx ON promo_codes (code);
CREATE INDEX promo_codes_valid_until_idx  ON promo_codes (valid_until) WHERE valid_until IS NOT NULL;
CREATE INDEX promo_codes_stripe_idx       ON promo_codes (stripe_coupon_id);

COMMENT ON TABLE promo_codes IS '§7.1 — subscription promo codes; Stripe Coupon is backend of record.';
