-- 000006_store_watermarks_orders.up.sql
-- Add a separate watermark column for storefront-visible order changes,
-- so polling clients can distinguish "products changed" from "orders
-- changed" without flooding both signals on every product edit.
--
-- The outbox publisher (internal/outbox/publisher.go) is updated in the
-- same Orders M2 commit to bump orders_updated_at when it sees an event
-- whose aggregate is one of {order, return, abandoned_cart}, and to keep
-- bumping products_updated_at for everything else (products, categories,
-- media).
--
-- Spec reference: §14.1 (watermark separation), §14.6 (publisher
-- semantics). M2 plan Option A — replaces the never-built external
-- delivery drainer with a watermark-bumping publisher branch.

BEGIN;

ALTER TABLE store_watermarks
    ADD COLUMN IF NOT EXISTS orders_updated_at timestamptz NOT NULL DEFAULT now();

COMMIT;
