ALTER TABLE bank_feed_events
    ADD COLUMN IF NOT EXISTS connection_id BIGINT REFERENCES bank_connections(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS payload_hash TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_bank_feed_events_connection_status
    ON bank_feed_events(connection_id, status);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bank_feed_events_connection_payload
    ON bank_feed_events(connection_id, payload_hash)
    WHERE payload_hash <> '';
