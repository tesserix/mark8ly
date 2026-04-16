ALTER TABLE store_branding
  DROP COLUMN IF EXISTS seo_title_template,
  DROP COLUMN IF EXISTS seo_default_description,
  DROP COLUMN IF EXISTS seo_og_image_url,
  DROP COLUMN IF EXISTS seo_twitter_handle,
  DROP COLUMN IF EXISTS seo_google_verification,
  DROP COLUMN IF EXISTS seo_bing_verification,
  DROP COLUMN IF EXISTS seo_json_ld,
  DROP COLUMN IF EXISTS seo_ai_policy,
  DROP COLUMN IF EXISTS seo_llms_txt;
