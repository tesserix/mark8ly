ALTER TABLE stores
ADD COLUMN storefront_theme JSONB NOT NULL DEFAULT '{}'::jsonb;
