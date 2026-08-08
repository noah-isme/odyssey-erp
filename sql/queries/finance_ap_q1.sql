-- name: GetPOLineProgress :many
SELECT * FROM po_line_progress
WHERE po_id = $1;

-- name: UpdateAPInvoiceDuplicateStatus :exec
UPDATE ap_invoices
SET duplicate_status = $2, updated_at = NOW()
WHERE id = $1;

-- name: CheckDuplicateInvoice :one
SELECT id, status, total, issued_at 
FROM ap_invoices
WHERE supplier_id = $1 AND supplier_document_number = $2 AND status != 'VOID'
LIMIT 1;
