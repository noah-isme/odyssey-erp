-- name: UpdateTreasuryPaymentBatchExport :one
UPDATE treasury_payment_batches
SET status = 'EXPORTED',
    exported_file_hash = $2,
    exported_by = $3,
    exported_at = NOW(),
    updated_at = NOW()
WHERE id = $1
RETURNING *;
