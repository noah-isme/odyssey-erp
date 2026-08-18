DROP INDEX IF EXISTS idx_treasury_payment_batches_source_bank;
DROP INDEX IF EXISTS idx_treasury_payment_batches_connection;

ALTER TABLE treasury_payment_batches
    DROP COLUMN IF EXISTS source_bank_account_id,
    DROP COLUMN IF EXISTS payment_connection_id;
