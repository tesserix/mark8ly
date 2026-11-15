-- Refunds of in-store sales made outside mark8ly (#1110): on the shop's own
-- terminal or in cash. One row per refund (request_id makes a retry a
-- no-op), one per line put back in stock, so restocks stay within what
-- was sold.
CREATE TABLE in_store_refunds (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    request_id  varchar(64) NOT NULL,
    amount      numeric(12,2) NOT NULL CHECK (amount > 0),
    method      varchar(20) NOT NULL,
    reason      text,
    refunded_by varchar(64),
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (order_id, request_id)
);

CREATE TABLE in_store_refund_items (
    refund_id     uuid NOT NULL REFERENCES in_store_refunds(id) ON DELETE CASCADE,
    order_item_id uuid NOT NULL,
    quantity      integer NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (refund_id, order_item_id)
);
CREATE INDEX in_store_refund_items_item_idx ON in_store_refund_items (order_item_id);
