ALTER TABLE notifications
    ADD COLUMN dedupe_key TEXT NULL;

CREATE UNIQUE INDEX idx_notifications_delivery_dedupe
    ON notifications (recipient_id, type, dedupe_key)
    WHERE dedupe_key IS NOT NULL;
