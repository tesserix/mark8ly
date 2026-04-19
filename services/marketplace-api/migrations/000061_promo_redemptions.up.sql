-- 000061_promo_redemptions.up.sql
-- §7.3 — promo redemption log: one row per (promo_code, store) use.

CREATE TABLE promo_redemptions (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    promo_code_id    UUID        NOT NULL REFERENCES promo_codes(id),
    store_id         UUID        NOT NULL,
    subscription_id  UUID        NOT NULL,
    email            TEXT        NOT NULL,
    redeemed_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- §7.3 abuse prevention: one redemption per (promo_code, email).
CREATE UNIQUE INDEX promo_redemptions_code_email_uidx ON promo_redemptions (promo_code_id, email);
CREATE INDEX promo_redemptions_store_idx ON promo_redemptions (store_id);
CREATE INDEX promo_redemptions_code_idx  ON promo_redemptions (promo_code_id);

COMMENT ON TABLE promo_redemptions IS '§7.3 — one row per promo code redemption; enforces per-email uniqueness.';
