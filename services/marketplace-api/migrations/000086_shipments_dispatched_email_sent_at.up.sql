-- shipments.dispatched_email_sent_at — dedup column for the
-- "your order has shipped" customer email.
--
-- Two different paths can transition a shipment to in_transit:
--
--   1. Admin manually marks shipment in_transit via
--      PATCH /shipments/:id/status (handlers/admin/shipments.go).
--   2. Carrier webhook (Delhivery) reports the package was picked up,
--      and the unified advanceShipmentFromTracking path advances the
--      record (handlers/admin/shipments.go advanceShipmentFromTracking,
--      shared by the public webhook handler).
--
-- Without a dedup column, both paths firing in quick succession would
-- email the customer twice. We gate the dispatch on a single atomic
-- UPDATE … WHERE dispatched_email_sent_at IS NULL. The first transition
-- to win the row sends the email; the second sees 0 rows affected and
-- silently skips.
--
-- Nullable on purpose: legacy shipments shipped before this column
-- existed will have NULL, which means the next status transition (or
-- manual re-emit) is allowed to fire. That's the correct behavior —
-- those customers may not have received the email yet.

ALTER TABLE shipments
    ADD COLUMN IF NOT EXISTS dispatched_email_sent_at TIMESTAMPTZ;
