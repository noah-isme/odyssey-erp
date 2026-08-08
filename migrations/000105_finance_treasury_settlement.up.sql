ALTER TABLE treasury_payment_batches
ADD COLUMN settled_at TIMESTAMPTZ,
ADD COLUMN settled_by BIGINT;
