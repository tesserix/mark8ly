-- The store's Australian Business Number, printed on its receipts so they
-- can serve as tax invoices (#1108). Empty when not set. Owned here, not
-- synced from platform-api.
ALTER TABLE stores ADD COLUMN abn varchar(11) NOT NULL DEFAULT '';
