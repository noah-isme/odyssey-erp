-- name: UpdateTreasuryPaymentBatchSettlement :one
UPDATE treasury_payment_batches
SET status = 'SETTLED',
    settled_by = $2,
    settled_at = NOW(),
    updated_at = NOW()
WHERE id = $1
RETURNING *;
