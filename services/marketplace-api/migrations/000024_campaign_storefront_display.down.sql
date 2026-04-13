ALTER TABLE campaigns
  DROP COLUMN IF EXISTS show_on_storefront,
  DROP COLUMN IF EXISTS storefront_label,
  DROP COLUMN IF EXISTS storefront_priority;
