-- Add manual DNS method support to custom_domains.
-- dns_method: 'manual' (CNAME-based) or 'cloudflare' (automated via CF API).
-- cname_target: the target merchants should point their CNAME at (e.g. india-store.mark8ly.com).
-- cf_api_token_encrypted becomes nullable (not needed for manual mode).

ALTER TABLE custom_domains ADD COLUMN IF NOT EXISTS dns_method VARCHAR(20) NOT NULL DEFAULT 'manual';
ALTER TABLE custom_domains ADD COLUMN IF NOT EXISTS cname_target VARCHAR(253);
ALTER TABLE custom_domains ALTER COLUMN cf_api_token_encrypted DROP NOT NULL;
ALTER TABLE custom_domains ALTER COLUMN cf_api_token_encrypted SET DEFAULT '';
