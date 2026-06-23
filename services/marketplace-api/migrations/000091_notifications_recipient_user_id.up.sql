-- Customer-targeted notifications. When recipient_user_id is set, the
-- notification belongs to a storefront customer (GIP string user id) and
-- powers the customer notification bell; when NULL it is the existing
-- store/staff notification. Nullable so existing rows are unaffected.
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS recipient_user_id VARCHAR(255);

-- Partial index for the customer bell queries (list + unread count), scoped
-- to a store + recipient. Only indexes customer rows to keep it small.
CREATE INDEX IF NOT EXISTS notif_recipient_unread_idx
    ON notifications (store_id, recipient_user_id, is_read, created_at DESC)
    WHERE recipient_user_id IS NOT NULL;
