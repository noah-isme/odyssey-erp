DROP INDEX IF EXISTS idx_ap_payments_supplier;
DROP INDEX IF EXISTS idx_ap_invoices_po;

ALTER TABLE ap_payments
    ALTER COLUMN ap_invoice_id SET NOT NULL;

ALTER TABLE ap_payments
    DROP COLUMN IF EXISTS supplier_id;

ALTER TABLE ap_invoices
    DROP COLUMN IF EXISTS po_id;
