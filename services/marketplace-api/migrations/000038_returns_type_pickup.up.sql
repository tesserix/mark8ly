-- Distinguish a refund-only return from a replacement request, and give
-- staff a free-text field to capture the logistics they promise the
-- customer after approving the request (courier, pickup window,
-- replacement shipment details, etc.).
ALTER TABLE returns
    ADD COLUMN IF NOT EXISTS type           varchar(20)  NOT NULL DEFAULT 'return',
    ADD COLUMN IF NOT EXISTS pickup_details text,
    ADD COLUMN IF NOT EXISTS approved_at    timestamptz,
    ADD COLUMN IF NOT EXISTS rejected_at    timestamptz,
    ADD COLUMN IF NOT EXISTS reject_reason  varchar(200);

ALTER TABLE returns
    DROP CONSTRAINT IF EXISTS returns_type_valid;

ALTER TABLE returns
    ADD CONSTRAINT returns_type_valid CHECK (type IN ('return', 'replace'));

-- Index the inbox queries the admin RMA page will drive — it needs the
-- pending-first list per store, plus per-order lookup from the order
-- detail page.
CREATE INDEX IF NOT EXISTS returns_store_status_requested_idx
    ON returns (store_id, status, requested_at DESC);
