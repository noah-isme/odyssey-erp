-- name: CreateAPInvoice :one
INSERT INTO ap_invoices (
    number, supplier_id, grn_id, po_id, currency, 
    subtotal, tax_amount, total, status, 
    due_at, created_by, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, 
    $6, $7, $8, $9, 
    $10, $11, NOW(), NOW()
) RETURNING id;

-- name: UpdateAPStatus :exec
UPDATE ap_invoices 
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: PostAPInvoice :exec
UPDATE ap_invoices 
SET status = 'POSTED', posted_at = NOW(), posted_by = $2, updated_at = NOW()
WHERE id = $1 AND status = 'DRAFT';

-- name: VoidAPInvoice :exec
UPDATE ap_invoices 
SET status = 'VOID', voided_at = NOW(), voided_by = $2, void_reason = $3, updated_at = NOW()
WHERE id = $1 AND status IN ('DRAFT', 'POSTED');

-- name: GetAPInvoice :one
SELECT 
    i.id, i.number, i.supplier_id, s.name AS supplier_name, i.grn_id, i.po_id, i.currency, 
    subtotal, tax_amount, total, status, due_at, 
    posted_at, posted_by, voided_at, voided_by, void_reason,
    created_by, created_at, updated_at 
FROM ap_invoices i
JOIN suppliers s ON s.id = i.supplier_id
WHERE i.id = $1;

-- name: GetAPInvoiceByNumber :one
SELECT 
    i.id, i.number, i.supplier_id, s.name AS supplier_name, i.grn_id, i.po_id, i.currency, 
    subtotal, tax_amount, total, status, due_at, 
    posted_at, posted_by, voided_at, voided_by, void_reason,
    created_by, created_at, updated_at 
FROM ap_invoices i
JOIN suppliers s ON s.id = i.supplier_id
WHERE i.number = $1;

-- name: CreateAPInvoiceLine :one
INSERT INTO ap_invoice_lines (
    ap_invoice_id, grn_line_id, product_id, description,
    quantity, unit_price, discount_pct, tax_pct,
    subtotal, tax_amount, total, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW()
) RETURNING id;

-- name: ListAPInvoiceLines :many
SELECT 
    id, ap_invoice_id, grn_line_id, product_id,
    description, quantity, unit_price, discount_pct, tax_pct,
    subtotal, tax_amount, total, created_at
FROM ap_invoice_lines
WHERE ap_invoice_id = $1
ORDER BY id;

-- name: ListAPInvoices :many
SELECT 
    i.id, i.number, i.supplier_id, s.name AS supplier_name, i.grn_id, i.po_id, i.currency, 
    subtotal, tax_amount, total, status, due_at, 
    posted_at, posted_by, voided_at, voided_by, void_reason,
    created_by, created_at, updated_at 
FROM ap_invoices i
JOIN suppliers s ON s.id = i.supplier_id
ORDER BY i.created_at DESC;

-- name: ListAPInvoicesByStatus :many
SELECT 
    i.id, i.number, i.supplier_id, s.name AS supplier_name, i.grn_id, i.po_id, i.currency, 
    subtotal, tax_amount, total, status, due_at, 
    posted_at, posted_by, voided_at, voided_by, void_reason,
    created_by, created_at, updated_at 
FROM ap_invoices i
JOIN suppliers s ON s.id = i.supplier_id
WHERE i.status = $1
ORDER BY i.created_at DESC;

-- name: ListAPInvoicesBySupplier :many
SELECT 
    i.id, i.number, i.supplier_id, s.name AS supplier_name, i.grn_id, i.po_id, i.currency, 
    subtotal, tax_amount, total, status, due_at, 
    posted_at, posted_by, voided_at, voided_by, void_reason,
    created_by, created_at, updated_at 
FROM ap_invoices i
JOIN suppliers s ON s.id = i.supplier_id
WHERE i.supplier_id = $1
ORDER BY i.created_at DESC;

-- name: CreateAPPayment :one
INSERT INTO ap_payments (
    number, ap_invoice_id, supplier_id, amount, paid_at, method, note, 
    created_by, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW()
) RETURNING id;

-- name: CreateAPPaymentAllocation :one
INSERT INTO ap_payment_allocations (
    ap_payment_id, ap_invoice_id, amount, created_at
) VALUES ($1, $2, $3, NOW())
RETURNING id;

-- name: ListAPPayments :many
SELECT 
    p.id, p.number, p.ap_invoice_id, p.supplier_id, COALESCE(s.name, '') AS supplier_name, p.amount, p.paid_at, p.method, p.note, 
    p.created_by, p.created_at, p.updated_at 
FROM ap_payments p
LEFT JOIN suppliers s ON s.id = p.supplier_id
ORDER BY p.paid_at DESC;

-- name: GetAPPayment :one
SELECT 
    p.id, p.number, p.ap_invoice_id, p.supplier_id, COALESCE(s.name, '') AS supplier_name, p.amount, p.paid_at, p.method, p.note, 
    p.created_by, p.created_at, p.updated_at
FROM ap_payments p
LEFT JOIN suppliers s ON s.id = p.supplier_id
WHERE p.id = $1;

-- name: ListAPInvoicePayments :many
SELECT 
    p.id, p.number, p.amount, p.paid_at, p.method, p.note,
    pa.amount AS allocated_amount
FROM ap_payments p
JOIN ap_payment_allocations pa ON pa.ap_payment_id = p.id
WHERE pa.ap_invoice_id = $1
ORDER BY p.paid_at;

-- name: ListAPPaymentAllocations :many
SELECT
    pa.id, pa.ap_payment_id, pa.ap_invoice_id, pa.amount,
    i.number AS invoice_number, i.po_id, i.total, i.status, i.due_at
FROM ap_payment_allocations pa
JOIN ap_invoices i ON i.id = pa.ap_invoice_id
WHERE pa.ap_payment_id = $1
ORDER BY i.number;

-- name: IsAPPaymentPosted :one
SELECT EXISTS (
    SELECT 1
    FROM journal_entries
    WHERE source_module = $1 AND source_id = $2 AND status = 'POSTED'
);

-- name: GetAPInvoiceBalance :one
SELECT 
    i.total,
    COALESCE(SUM(pa.amount), 0)::NUMERIC AS paid_amount,
    (i.total - COALESCE(SUM(pa.amount), 0))::NUMERIC AS balance
FROM ap_invoices i
LEFT JOIN ap_payment_allocations pa ON pa.ap_invoice_id = i.id
WHERE i.id = $1
GROUP BY i.id;

-- name: GenerateAPInvoiceNumber :one
SELECT 'INV-' || TO_CHAR(NOW(), 'YYYYMMDD') || '-' || LPAD(NEXTVAL('ap_invoices_id_seq')::TEXT, 4, '0');

-- name: GenerateAPPaymentNumber :one
SELECT 'PAY-' || TO_CHAR(NOW(), 'YYYYMMDD') || '-' || LPAD(NEXTVAL('ap_payments_id_seq')::TEXT, 4, '0');
