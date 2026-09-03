-- name: CreateTreasuryPaymentBatch :one
INSERT INTO treasury_payment_batches (
    company_id, reference_code, currency, proposed_by
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: GetTreasuryPaymentBatch :one
SELECT * FROM treasury_payment_batches WHERE id = $1;

-- name: UpdateTreasuryPaymentBatchStatus :one
UPDATE treasury_payment_batches
SET status = $2,
    approved_by = $3,
    approved_at = $4,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateTreasuryPaymentBatchRevision :one
UPDATE treasury_payment_batches
SET revision_number = revision_number + 1,
    status = 'DRAFT',
    total_amount = COALESCE((
        SELECT SUM(amount)
        FROM treasury_payment_batch_items
        WHERE batch_id = $1 AND status = 'ACTIVE'
    ), $2),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateTreasuryPaymentBatchTotal :one
UPDATE treasury_payment_batches
SET total_amount = COALESCE((
        SELECT SUM(amount)
        FROM treasury_payment_batch_items
        WHERE batch_id = $1 AND status = 'ACTIVE'
    ), $2),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: CreateTreasuryPaymentBatchItem :one
INSERT INTO treasury_payment_batch_items (
    batch_id, supplier_id, bank_account_id, amount, ap_invoice_id
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: ListTreasuryPaymentBatchItems :many
SELECT * FROM treasury_payment_batch_items WHERE batch_id = $1 AND status = 'ACTIVE';

-- name: RemoveTreasuryPaymentBatchItem :exec
UPDATE treasury_payment_batch_items
SET status = 'REMOVED'
WHERE id = $1;
