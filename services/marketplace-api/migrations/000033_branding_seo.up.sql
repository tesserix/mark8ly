-- SEO + AI SEO fields on store_branding. All nullable — existing stores
-- get reasonable defaults emitted at the storefront layer.
ALTER TABLE store_branding
  ADD COLUMN IF NOT EXISTS seo_title_template       VARCHAR(200),
  ADD COLUMN IF NOT EXISTS seo_default_description  TEXT,
  ADD COLUMN IF NOT EXISTS seo_og_image_url         TEXT,
  ADD COLUMN IF NOT EXISTS seo_twitter_handle       VARCHAR(100),
  ADD COLUMN IF NOT EXISTS seo_google_verification  VARCHAR(200),
  ADD COLUMN IF NOT EXISTS seo_bing_verification    VARCHAR(200),
  ADD COLUMN IF NOT EXISTS seo_json_ld              TEXT,
  ADD COLUMN IF NOT EXISTS seo_ai_policy            VARCHAR(30) NOT NULL DEFAULT 'allow',
  ADD COLUMN IF NOT EXISTS seo_llms_txt             TEXT;
