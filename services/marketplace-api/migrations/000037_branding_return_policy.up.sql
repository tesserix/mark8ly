-- Adds a return_policy column to store_branding so merchants can record
-- their returns/refunds policy alongside other brand settings. Used by the
-- onboarding checklist (has_return_policy) and by storefront policy pages.
ALTER TABLE store_branding ADD COLUMN IF NOT EXISTS return_policy TEXT;
