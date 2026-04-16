-- Track SSL certificate provisioning for manual DNS-method custom domains.
-- cert_status: pending | issuing | ready | failed
-- cert_secret_name: istio-ingress namespace secret holding the cert + key
-- cert_error: latest error message from cert-manager (nullable)
ALTER TABLE custom_domains
  ADD COLUMN IF NOT EXISTS cert_status      VARCHAR(20)  NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS cert_secret_name VARCHAR(253),
  ADD COLUMN IF NOT EXISTS cert_error       TEXT;
