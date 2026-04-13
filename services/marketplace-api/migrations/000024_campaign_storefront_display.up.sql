ALTER TABLE campaigns
  ADD COLUMN IF NOT EXISTS show_on_storefront boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS storefront_label   varchar(120),
  ADD COLUMN IF NOT EXISTS storefront_priority int NOT NULL DEFAULT 0;
