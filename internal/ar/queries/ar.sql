-- name: CreateARInvoice :one
INSERT INTO ar_invoices (
    number, 
    customer_id, 
    so_id, 
    currency, 
    total, 
    status, 
    due_at, 
    created_at, 
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING id;

-- name: UpdateARStatus :exec
UPDATE ar_invoices 
SET status = $1 
WHERE id = $2;

-- name: CreateARPayment :one
INSERT INTO ar_payments (
    number, 
    ar_invoice_id, 
    amount, 
    paid_at, 
    method, 
    note, 
    created_at, 
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING id;

-- name: ListARInvoices :many
SELECT 
    id, 
    number, 
    customer_id, 
    so_id, 
    currency, 
    total, 
    status, 
    due_at, 
    created_at, 
    updated_at 
FROM ar_invoices 
ORDER BY id;

-- name: ListARPayments :many
SELECT 
    id, 
    number, 
    ar_invoice_id, 
    amount, 
    paid_at, 
    method, 
    note, 
    created_at, 
    updated_at 
FROM ar_payments 
ORDER BY id;

-- name: ListAROutstanding :many
SELECT 
    id, 
    number, 
    customer_id, 
    so_id, 
    currency, 
    total, 
    status, 
    due_at, 
    created_at, 
    updated_at 
FROM ar_invoices 
WHERE status IN ('POSTED','PAID') 
ORDER BY due_at;
