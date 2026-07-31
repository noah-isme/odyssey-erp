ALTER TABLE pos_payments ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

UPDATE pos_payments
SET idempotency_key = 'legacy-pos-payment-' || id::text
WHERE idempotency_key IS NULL;

ALTER TABLE pos_payments ALTER COLUMN idempotency_key SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_pos_payments_ticket_idempotency
    ON pos_payments(ticket_id, idempotency_key);
