DROP INDEX IF EXISTS notif_recipient_unread_idx;
ALTER TABLE notifications DROP COLUMN IF EXISTS recipient_user_id;
