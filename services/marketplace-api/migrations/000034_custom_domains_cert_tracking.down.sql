ALTER TABLE custom_domains
  DROP COLUMN IF EXISTS cert_status,
  DROP COLUMN IF EXISTS cert_secret_name,
  DROP COLUMN IF EXISTS cert_error;
