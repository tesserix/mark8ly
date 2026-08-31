-- Restores 000001's function verbatim and drops the DELETE arm. Safe while
-- every variant has one stock row, which is the only state a rollback to
-- this point can be in: nothing writes a second location until PR 5.
DROP TRIGGER IF EXISTS variant_stock_sync_delete ON variant_stock;

CREATE OR REPLACE FUNCTION sync_variant_inventory() RETURNS trigger AS $$
BEGIN
    UPDATE product_variants
    SET inventory_quantity = NEW.quantity
    WHERE id = NEW.variant_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
