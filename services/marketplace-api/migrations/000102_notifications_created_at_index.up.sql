-- The platform console's cross-tenant notification log (#332) orders by
-- created_at DESC across every store. Both existing indexes are
-- store-scoped — notif_store_unread_idx (store_id, is_read, created_at DESC)
-- and notif_store_recent_idx (store_id, created_at DESC) — so neither can
-- serve a query with no store_id predicate.
--
-- Same reason migration 000101 added idx_audit_logs_created_at for #276.
CREATE INDEX notif_created_at_idx ON notifications (created_at DESC);
