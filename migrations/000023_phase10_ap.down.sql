DROP TABLE IF EXISTS ap_payment_allocations;
DROP TABLE IF EXISTS ap_invoice_lines;

ALTER TABLE ap_payments DROP COLUMN IF EXISTS created_by;
ALTER TABLE ap_payments DROP COLUMN IF EXISTS created_at;
ALTER TABLE ap_payments DROP COLUMN IF EXISTS updated_at;

ALTER TABLE ap_invoices DROP COLUMN IF EXISTS subtotal;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS tax_amount;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS posted_at;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS posted_by;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS voided_at;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS voided_by;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS void_reason;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS created_by;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS updated_at;

DELETE FROM permissions WHERE name LIKE 'finance.ap.%';
