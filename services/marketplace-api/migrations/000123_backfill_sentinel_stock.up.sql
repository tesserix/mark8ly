-- #177 PR 6 (contract, step 1 of 2) — move every unit off the SENTINEL
-- location and onto a real warehouse.
--
-- variant_stock and stock_holds both carry
-- location_id = '00000000-0000-0000-0000-000000000001', the DefaultLocationID
-- constant. Allocation reasons about real warehouse ids, so while stock sits
-- on the sentinel the allocator is structurally correct and economically
-- hollow: it can only tolerate those rows, never fill from them properly.
--
-- This migration does NOT remove the sentinel tolerance in code, and does not
-- delete DefaultLocationID. Both stay until this backfill is confirmed run in
-- production — a contract step that lands with its own tolerance still in
-- place is safe in either order; the reverse is not.
--
-- CORRECTION TO THE SPEC. It says "production has zero warehouses and zero
-- carrier configs", so the backfill may simply CREATE a warehouse named
-- 'Main Warehouse' per store. That has been false since 2026-09-01:
-- the-bondi-store has a warehouse named exactly 'Main Warehouse' with a real
-- address, and warehouses is keyed (store_id, name) — partially, since 000122.
-- A naive INSERT violates that index; an ON CONFLICT DO NOTHING silently
-- leaves the stock on the sentinel while the code believes it migrated.
-- So: find-or-create, and never overwrite an address a merchant has entered.

-- ---------------------------------------------------------------------
-- 1. Every store holding sentinel stock needs somewhere to put it.
--
-- Only for stores with NO live warehouse at all. A store that already has
-- one — under any name — uses it; creating a second 'Main Warehouse'
-- alongside the merchant's own is exactly the duplicate this issue exists
-- to stop.
--
-- The address is left blank on purpose: that store has never told us where
-- it ships from, and the warehouses page is what fills it in. Writing a
-- plausible-looking placeholder would be a lie that quotes rates.
-- ---------------------------------------------------------------------
INSERT INTO warehouses (
    id, tenant_id, store_id, name, line1, line2, city, region,
    postal_code, country_code, phone, is_default, priority
)
SELECT gen_random_uuid(), s.tenant_id, s.id, 'Main Warehouse',
       '', '', '', '', '', COALESCE(NULLIF(s.country_code, ''), 'IN'), '',
       true, 0
  FROM stores s
 WHERE EXISTS (
           SELECT 1
             FROM variant_stock vs
             JOIN product_variants pv ON pv.id = vs.variant_id
            WHERE pv.store_id = s.id
              AND vs.location_id = '00000000-0000-0000-0000-000000000001'
       )
   AND NOT EXISTS (
           SELECT 1 FROM warehouses w
            WHERE w.store_id = s.id AND w.archived_at IS NULL
       );

-- ---------------------------------------------------------------------
-- 2. Which warehouse each store's sentinel stock lands on.
--
-- Repeated as a CTE in each statement below rather than materialised once
-- into a TEMP TABLE. A temp table created ON COMMIT DROP only survives if
-- the whole migration runs inside one transaction — true under
-- golang-migrate, false under `psql -f` in autocommit, which is how a
-- migration gets applied by hand during an incident. It failed exactly
-- that way in testing. A repeated CTE has no such dependency.
--
-- Precedence matches warehouse.DefaultForStore: the flagged default, then
-- fill order, then oldest. Deterministic because the answer decides where
-- a merchant's entire inventory is recorded. Archived warehouses are
-- excluded — the allocator refuses to fill from them (#528), so
-- backfilling onto one would park every unit somewhere unsellable.
-- ---------------------------------------------------------------------

-- ---------------------------------------------------------------------
-- 3. variant_stock: sentinel rows become real-warehouse rows.
--
-- SUMS on conflict rather than overwriting. A variant can legitimately
-- have BOTH a sentinel row and a real row for the same warehouse: PR 5e
-- writes real rows and clears the sentinel, but the multi-variant product
-- save still writes the sentinel, so a variant edited both ways carries
-- the two. checkout_availability adds them together today (that is the
-- tolerance), so summing here preserves the total the merchant is already
-- selling. Overwriting would silently delete stock.
-- ---------------------------------------------------------------------
WITH target AS (
    SELECT DISTINCT ON (w.store_id) w.store_id, w.id AS warehouse_id
      FROM warehouses w
     WHERE w.archived_at IS NULL
     ORDER BY w.store_id, w.is_default DESC, w.priority ASC, w.created_at ASC
)
INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
SELECT vs.variant_id, t.warehouse_id, vs.quantity, now()
  FROM variant_stock vs
  JOIN product_variants pv ON pv.id = vs.variant_id
  JOIN target t            ON t.store_id = pv.store_id
 WHERE vs.location_id = '00000000-0000-0000-0000-000000000001'
    ON CONFLICT (variant_id, location_id)
    DO UPDATE SET quantity   = variant_stock.quantity + EXCLUDED.quantity,
                  updated_at = now();

DELETE FROM variant_stock vs
 USING product_variants pv
 WHERE vs.variant_id = pv.id
   AND vs.location_id = '00000000-0000-0000-0000-000000000001'
   AND EXISTS (
           SELECT 1 FROM warehouses w
            WHERE w.store_id = pv.store_id AND w.archived_at IS NULL
       );

-- ---------------------------------------------------------------------
-- 4. stock_holds: live reservations move with the stock.
--
-- Deleting them instead would release a shopper's reservation mid-checkout
-- and let someone else take the units they are paying for. Summing on
-- conflict for the same reason as above.
-- ---------------------------------------------------------------------
WITH target AS (
    SELECT DISTINCT ON (w.store_id) w.store_id, w.id AS warehouse_id
      FROM warehouses w
     WHERE w.archived_at IS NULL
     ORDER BY w.store_id, w.is_default DESC, w.priority ASC, w.created_at ASC
)
INSERT INTO stock_holds (variant_id, location_id, cart_token, qty, expires_at, state)
SELECT sh.variant_id, t.warehouse_id, sh.cart_token, sh.qty, sh.expires_at, sh.state
  FROM stock_holds sh
  JOIN product_variants pv ON pv.id = sh.variant_id
  JOIN target t            ON t.store_id = pv.store_id
 WHERE sh.location_id = '00000000-0000-0000-0000-000000000001'
    ON CONFLICT (cart_token, variant_id, location_id)
    DO UPDATE SET qty        = stock_holds.qty + EXCLUDED.qty,
                  expires_at = GREATEST(stock_holds.expires_at, EXCLUDED.expires_at);

DELETE FROM stock_holds sh
 USING product_variants pv
 WHERE sh.variant_id = pv.id
   AND sh.location_id = '00000000-0000-0000-0000-000000000001'
   AND EXISTS (
           SELECT 1 FROM warehouses w
            WHERE w.store_id = pv.store_id AND w.archived_at IS NULL
       );
