-- Rollback Phase 9.3: AR Invoice Enhancement

-- Remove trigger
DROP TRIGGER IF EXISTS trg_update_invoice_status ON ar_payment_allocations;
DROP FUNCTION IF EXISTS update_invoice_status_on_payment();

-- Remove helper functions
DROP FUNCTION IF EXISTS generate_ar_payment_number();
DROP FUNCTION IF EXISTS generate_ar_invoice_number();

-- Remove view
DROP VIEW IF EXISTS v_ar_invoice_balance;

-- Remove permissions
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE name IN (
        'finance.ar.create', 'finance.ar.post', 'finance.ar.void', 'finance.ar.payment'
    )
);
DELETE FROM permissions WHERE name IN (
    'finance.ar.create', 'finance.ar.post', 'finance.ar.void', 'finance.ar.payment'
);

-- Remove payment allocations table
DROP TABLE IF EXISTS ar_payment_allocations;

-- Remove added columns from ar_payments
ALTER TABLE ar_payments DROP COLUMN IF EXISTS created_by;

-- Remove added columns from ar_invoices
ALTER TABLE ar_invoices DROP COLUMN IF EXISTS created_by;
ALTER TABLE ar_invoices DROP COLUMN IF EXISTS void_reason;
ALTER TABLE ar_invoices DROP COLUMN IF EXISTS voided_by;
ALTER TABLE ar_invoices DROP COLUMN IF EXISTS voided_at;
ALTER TABLE ar_invoices DROP COLUMN IF EXISTS posted_by;
ALTER TABLE ar_invoices DROP COLUMN IF EXISTS posted_at;
ALTER TABLE ar_invoices DROP COLUMN IF EXISTS tax_amount;
ALTER TABLE ar_invoices DROP COLUMN IF EXISTS subtotal;
ALTER TABLE ar_invoices DROP COLUMN IF EXISTS delivery_order_id;

-- Remove invoice lines table
DROP TABLE IF EXISTS ar_invoice_lines;
