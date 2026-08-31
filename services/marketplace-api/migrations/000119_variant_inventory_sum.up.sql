-- #177: product_variants.inventory_quantity becomes the SUM across a
-- variant's locations.
--
-- 000001 created this trigger with `SET inventory_quantity = NEW.quantity`
-- and a comment saying slice 2 would change it to a SUM. That assignment is
-- the total only while a variant has one stock row; with two warehouses the
-- number browse, PDP and cart all read would become whichever location was
-- written most recently.
--
-- The DELETE arm is new. Without it, removing a warehouse's stock row leaves
-- inventory_quantity counting units that no longer exist — an oversell with
-- no error anywhere.
--
-- NOTE for anyone editing this: in a DELETE trigger PL/pgSQL leaves NEW
-- unassigned, and referencing NEW.variant_id there raises "record new is not
-- assigned yet". Branch on TG_OP; do not reach for COALESCE(NEW, OLD).

CREATE OR REPLACE FUNCTION sync_variant_inventory() RETURNS trigger AS $$
DECLARE
    v_variant uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_variant := OLD.variant_id;
    ELSE
        v_variant := NEW.variant_id;
    END IF;

    UPDATE product_variants
       SET inventory_quantity = COALESCE(
               (SELECT SUM(quantity) FROM variant_stock WHERE variant_id = v_variant), 0)
     WHERE id = v_variant;

    -- AFTER trigger: the return value is ignored.
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER variant_stock_sync_delete
    AFTER DELETE ON variant_stock
    FOR EACH ROW EXECUTE FUNCTION sync_variant_inventory();
