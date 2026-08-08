ALTER TABLE bank_transactions 
    DROP COLUMN IF EXISTS import_run_id,
    DROP COLUMN IF EXISTS external_reference,
    DROP COLUMN IF EXISTS fingerprint,
    DROP COLUMN IF EXISTS skip_reason;

DROP TABLE IF EXISTS statement_import_runs CASCADE;
