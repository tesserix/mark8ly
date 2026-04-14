-- 000031_branding_homepage_content.up.sql
--
-- Homepage content JSONB. Shape documented at
-- docs/superpowers/specs/2026-04-15-storefront-homepage-content-design.md
-- Default is an empty object so existing stores don't need a backfill.

ALTER TABLE store_branding
  ADD COLUMN IF NOT EXISTS homepage_content JSONB NOT NULL DEFAULT '{}'::jsonb;
