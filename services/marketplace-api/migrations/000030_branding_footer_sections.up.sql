-- 000030_branding_footer_sections.up.sql
--
-- Phase D of the Storefront Pages CMS & Footer. Adds a JSONB column
-- to store merchant-authored footer link sections. Default is an
-- empty array so existing stores with no footer sections don't need
-- a backfill. Shape (validated in service layer):
--   [
--     {"label": "Company", "items": [
--       {"label": "About", "kind": "page", "page_slug": "about"},
--       {"label": "Docs",  "kind": "url",  "url": "https://..."}
--     ]}
--   ]

ALTER TABLE store_branding ADD COLUMN IF NOT EXISTS footer_sections JSONB NOT NULL DEFAULT '[]'::jsonb;
