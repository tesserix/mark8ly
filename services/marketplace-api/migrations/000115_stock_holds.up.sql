-- #231 — server-side stock holds.
--
-- Availability is COMPUTED, never stored:
--
--   available = variant_stock.quantity
--             - SUM(qty) FILTER (WHERE state = 'held' AND expires_at > now())
--
-- so a hold expires by the clock rather than by a job running. If the sweeper
-- stops, availability stays correct and only dead rows accumulate. Storing a
-- reserved counter would make the sweeper load-bearing for correctness, and a
-- missed run would silently strand stock.
CREATE TABLE stock_holds (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id  uuid NOT NULL REFERENCES product_variants(id) ON DELETE CASCADE,
    location_id uuid NOT NULL,
    cart_token  uuid NOT NULL,
    qty         integer NOT NULL CHECK (qty > 0),
    expires_at  timestamptz NOT NULL,
    state       text NOT NULL DEFAULT 'held'
                CHECK (state IN ('held','committed','released')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    -- One hold per cart per variant per location, so a repeat Hold refreshes
    -- rather than stacking a second reservation against the same cart.
    UNIQUE (cart_token, variant_id, location_id)
);

-- Partial on state='held': the availability query only ever sums live holds,
-- and committed/released rows are dead weight in that index.
CREATE INDEX stock_holds_live_idx ON stock_holds (variant_id, location_id)
    WHERE state = 'held';

-- Partial on state='held': the sweeper only ever scans live rows.
CREATE INDEX stock_holds_expiry_idx ON stock_holds (expires_at)
    WHERE state = 'held';

-- A decrement must never take stock below zero. The application serialises
-- contenders with SELECT ... FOR UPDATE, but that is a discipline; this is the
-- guarantee. Verified before adding: 0 negative rows of 380 in production.
ALTER TABLE variant_stock
    ADD CONSTRAINT variant_stock_quantity_non_negative CHECK (quantity >= 0);
