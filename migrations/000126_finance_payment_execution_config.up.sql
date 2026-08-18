ALTER TABLE treasury_payment_batches
    ADD COLUMN IF NOT EXISTS payment_connection_id BIGINT
        REFERENCES connector_connections(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS source_bank_account_id BIGINT
        REFERENCES bank_accounts(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_treasury_payment_batches_connection
    ON treasury_payment_batches(company_id, payment_connection_id)
    WHERE payment_connection_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_treasury_payment_batches_source_bank
    ON treasury_payment_batches(company_id, source_bank_account_id)
    WHERE source_bank_account_id IS NOT NULL;
