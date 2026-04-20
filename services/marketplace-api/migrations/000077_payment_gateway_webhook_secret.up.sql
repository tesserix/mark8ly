-- 000077_payment_gateway_webhook_secret.up.sql
-- Prod-readiness spec (2026-04-10) §2.6: separate the API secret from the
-- webhook-signature secret on payment_gateway_configs. Razorpay in
-- particular uses the same string for both today; a compromised API
-- secret lets an attacker forge signed webhooks. This migration adds a
-- dedicated column; the application prefers it when non-empty and falls
-- back to secret_key_encrypted for merchants who have not yet split
-- their credentials (legacy rollout safety).

ALTER TABLE payment_gateway_configs
    ADD COLUMN webhook_secret_encrypted TEXT;

-- No backfill: merchants are prompted to supply a webhook secret in the
-- admin settings UI. Until they do, legacy behaviour (use secret_key as
-- the webhook secret) applies.

COMMENT ON COLUMN payment_gateway_configs.webhook_secret_encrypted IS
    'Envelope-encrypted webhook signing secret (separate from api/secret key). When NULL, verification falls back to secret_key_encrypted for legacy merchants.';
