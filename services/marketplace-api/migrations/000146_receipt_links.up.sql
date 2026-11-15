-- In-store receipt links (#1106). Only the token's SHA-256 is stored; an
-- order can have several links, and old ones keep working.
CREATE TABLE receipt_links (
    token_hash char(64) PRIMARY KEY,
    order_id   uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    store_id   uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX receipt_links_order_created_idx ON receipt_links (order_id, created_at DESC);
