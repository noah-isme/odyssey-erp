ALTER TABLE treasury_payment_batches
ADD COLUMN exported_file_hash VARCHAR(255),
ADD COLUMN exported_at TIMESTAMPTZ,
ADD COLUMN exported_by BIGINT;
