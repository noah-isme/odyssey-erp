DROP INDEX IF EXISTS idx_notifications_delivery_dedupe;

ALTER TABLE notifications
    DROP COLUMN IF EXISTS dedupe_key;
