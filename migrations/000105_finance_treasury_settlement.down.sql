ALTER TABLE treasury_payment_batches
DROP COLUMN IF EXISTS settled_at,
DROP COLUMN IF EXISTS settled_by;
