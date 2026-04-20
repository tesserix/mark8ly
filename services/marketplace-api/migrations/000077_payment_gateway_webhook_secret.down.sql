-- 000077_payment_gateway_webhook_secret.down.sql
-- Drop the dedicated webhook-secret column. Webhook verification reverts
-- to using secret_key_encrypted for every provider.

ALTER TABLE payment_gateway_configs
    DROP COLUMN IF EXISTS webhook_secret_encrypted;
