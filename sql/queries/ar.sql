-- name: CreateARInvoice :one
INSERT INTO ar_invoices (
    number, 
    customer_id, 
    so_id, 
    delivery_order_id,
    currency, 
    subtotal,
    tax_amount,
    total, 
    status, 
    due_at,
    created_by,
    created_at, 
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
) RETURNING id;

-- name: UpdateARStatus :exec
UPDATE ar_invoices 
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: PostARInvoice :exec
UPDATE ar_invoices 
SET status = 'POSTED', posted_at = NOW(), posted_by = $2, updated_at = NOW()
WHERE id = $1 AND status = 'DRAFT';

-- name: VoidARInvoice :exec
UPDATE ar_invoices 
SET status = 'VOID', voided_at = NOW(), voided_by = $2, void_reason = $3, updated_at = NOW()
WHERE id = $1 AND status IN ('DRAFT', 'POSTED');

-- name: GetARInvoice :one
SELECT 
    id, number, customer_id, so_id, delivery_order_id, currency, 
    subtotal, tax_amount, total, status, due_at, 
    posted_at, posted_by, voided_at, voided_by, void_reason,
    created_by, created_at, updated_at 
FROM ar_invoices 
WHERE id = $1;

-- name: GetARInvoiceByNumber :one
SELECT 
    id, number, customer_id, so_id, delivery_order_id, currency, 
    subtotal, tax_amount, total, status, due_at, 
    posted_at, posted_by, voided_at, voided_by, void_reason,
    created_by, created_at, updated_at 
FROM ar_invoices 
WHERE number = $1;

-- name: CreateARPayment :one
INSERT INTO ar_payments (
    number, 
    ar_invoice_id, 
    amount, 
    paid_at, 
    method, 
    note,
    created_by,
    created_at, 
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING id;

-- name: CreateARInvoiceLine :one
INSERT INTO ar_invoice_lines (
    ar_invoice_id,
    delivery_order_line_id,
    product_id,
    description,
    quantity,
    unit_price,
    discount_pct,
    tax_pct,
    subtotal,
    tax_amount,
    total,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW()
) RETURNING id;

-- name: ListARInvoiceLines :many
SELECT 
    id, ar_invoice_id, delivery_order_line_id, product_id,
    description, quantity, unit_price, discount_pct, tax_pct,
    subtotal, tax_amount, total, created_at
FROM ar_invoice_lines
WHERE ar_invoice_id = $1
ORDER BY id;

-- name: CreatePaymentAllocation :one
INSERT INTO ar_payment_allocations (
    ar_payment_id, ar_invoice_id, amount, created_at
) VALUES ($1, $2, $3, NOW())
RETURNING id;

-- name: ListPaymentAllocations :many
SELECT id, ar_payment_id, ar_invoice_id, amount, created_at
FROM ar_payment_allocations
WHERE ar_payment_id = $1
ORDER BY id;

-- name: ListInvoicePayments :many
SELECT 
    p.id, p.number, p.amount, p.paid_at, p.method, p.note,
    pa.amount AS allocated_amount
FROM ar_payments p
JOIN ar_payment_allocations pa ON pa.ar_payment_id = p.id
WHERE pa.ar_invoice_id = $1
ORDER BY p.paid_at;

-- name: GetInvoiceBalance :one
SELECT 
    i.total,
    COALESCE(SUM(pa.amount), 0)::NUMERIC AS paid_amount,
    (i.total - COALESCE(SUM(pa.amount), 0))::NUMERIC AS balance
FROM ar_invoices i
LEFT JOIN ar_payment_allocations pa ON pa.ar_invoice_id = i.id
WHERE i.id = $1
GROUP BY i.id;

-- name: ListARInvoices :many
SELECT 
    id, number, customer_id, so_id, delivery_order_id, currency, 
    subtotal, tax_amount, total, status, due_at, 
    posted_at, posted_by, voided_at, voided_by, void_reason,
    created_by, created_at, updated_at 
FROM ar_invoices 
ORDER BY created_at DESC;

-- name: ListARInvoicesByStatus :many
SELECT 
    id, number, customer_id, so_id, delivery_order_id, currency, 
    subtotal, tax_amount, total, status, due_at, 
    posted_at, posted_by, voided_at, voided_by, void_reason,
    created_by, created_at, updated_at 
FROM ar_invoices 
WHERE status = $1
ORDER BY created_at DESC;

-- name: ListARInvoicesByCustomer :many
SELECT 
    id, number, customer_id, so_id, delivery_order_id, currency, 
    subtotal, tax_amount, total, status, due_at, 
    posted_at, posted_by, voided_at, voided_by, void_reason,
    created_by, created_at, updated_at 
FROM ar_invoices 
WHERE customer_id = $1
ORDER BY created_at DESC;

-- name: ListARPayments :many
SELECT 
    id, number, ar_invoice_id, amount, paid_at, method, note, 
    created_by, created_at, updated_at 
FROM ar_payments 
ORDER BY paid_at DESC;

-- name: ListAROutstanding :many
SELECT 
    id, number, customer_id, so_id, delivery_order_id, currency, 
    subtotal, tax_amount, total, status, due_at, 
    posted_at, posted_by, voided_at, voided_by, void_reason,
    created_by, created_at, updated_at 
FROM ar_invoices 
WHERE status = 'POSTED'
ORDER BY due_at;

-- name: GenerateARInvoiceNumber :one
SELECT generate_ar_invoice_number();

-- name: GenerateARPaymentNumber :one
SELECT generate_ar_payment_number();

-- name: CountARInvoicesByDelivery :one
SELECT COUNT(*) FROM ar_invoices WHERE delivery_order_id = $1;
