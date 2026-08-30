ALTER TABLE variant_stock DROP CONSTRAINT IF EXISTS variant_stock_quantity_non_negative;
DROP TABLE IF EXISTS stock_holds;
