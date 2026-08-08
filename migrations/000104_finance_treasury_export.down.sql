ALTER TABLE treasury_payment_batches
DROP COLUMN IF EXISTS exported_file_hash,
DROP COLUMN IF EXISTS exported_at,
DROP COLUMN IF EXISTS exported_by;
