ALTER TABLE custom_domains DROP COLUMN IF EXISTS dns_method;
ALTER TABLE custom_domains DROP COLUMN IF EXISTS cname_target;
ALTER TABLE custom_domains ALTER COLUMN cf_api_token_encrypted SET NOT NULL;
ALTER TABLE custom_domains ALTER COLUMN cf_api_token_encrypted SET DEFAULT '';
