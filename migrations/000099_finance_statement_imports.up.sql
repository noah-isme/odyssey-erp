CREATE TABLE statement_import_runs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    bank_account_id BIGINT NOT NULL REFERENCES bank_accounts(id) ON DELETE CASCADE,
    filename TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    imported_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE bank_transactions
    ADD COLUMN import_run_id BIGINT REFERENCES statement_import_runs(id) ON DELETE SET NULL,
    ADD COLUMN external_reference TEXT,
    ADD COLUMN fingerprint TEXT,
    ADD COLUMN skip_reason TEXT;

CREATE UNIQUE INDEX idx_bank_transactions_fingerprint ON bank_transactions(bank_account_id, fingerprint) WHERE fingerprint IS NOT NULL;
CREATE UNIQUE INDEX idx_bank_transactions_external_ref ON bank_transactions(bank_account_id, external_reference) WHERE external_reference IS NOT NULL;
