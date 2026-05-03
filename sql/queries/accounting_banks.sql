-- name: CreateBankStatement :one
INSERT INTO bank_statements (
    bank_account_id, statement_date, starting_balance, ending_balance, status, imported_at
) VALUES (
    $1, $2, $3, $4, $5, NOW()
) RETURNING *;

-- name: GetBankStatement :one
SELECT * FROM bank_statements WHERE id = $1;

-- name: UpdateBankStatementStatus :exec
UPDATE bank_statements SET status = $2, updated_at = NOW() WHERE id = $1;

-- name: ListBankStatements :many
SELECT * FROM bank_statements 
WHERE bank_account_id = $1 
ORDER BY statement_date DESC 
LIMIT $2 OFFSET $3;

-- name: CreateBankStatementLine :one
INSERT INTO bank_statement_lines (
    statement_id, trx_date, description, amount, reference_number, status, matched_doc_type, matched_doc_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetBankStatementLine :one
SELECT * FROM bank_statement_lines WHERE id = $1;

-- name: ListBankStatementLines :many
SELECT * FROM bank_statement_lines 
WHERE statement_id = $1 
ORDER BY trx_date ASC, id ASC;

-- name: UpdateBankStatementLineStatus :exec
UPDATE bank_statement_lines 
SET status = $2, matched_doc_type = $3, matched_doc_id = $4, updated_at = NOW() 
WHERE id = $1;

-- name: DeleteBankStatement :exec
DELETE FROM bank_statements WHERE id = $1;

-- name: FindUnpaidARInvoicesForMatching :many
SELECT id, number, total, (total - COALESCE((SELECT SUM(amount) FROM ar_payments WHERE invoice_id = ar_invoices.id), 0)) as amount_due
FROM ar_invoices 
WHERE status = 'POSTED'
AND (total - COALESCE((SELECT SUM(amount) FROM ar_payments WHERE invoice_id = ar_invoices.id), 0)) = $1;
