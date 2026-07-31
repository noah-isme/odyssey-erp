DROP INDEX IF EXISTS uq_pos_payments_ticket_idempotency;
ALTER TABLE pos_payments DROP COLUMN IF EXISTS idempotency_key;
