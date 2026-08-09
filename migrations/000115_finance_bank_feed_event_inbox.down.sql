DROP INDEX IF EXISTS idx_bank_feed_events_connection_payload;
DROP INDEX IF EXISTS idx_bank_feed_events_connection_status;

ALTER TABLE bank_feed_events
    DROP COLUMN IF EXISTS payload_hash,
    DROP COLUMN IF EXISTS connection_id;
