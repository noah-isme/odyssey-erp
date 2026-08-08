DROP VIEW IF EXISTS po_line_progress;

DROP INDEX IF EXISTS idx_ap_invoices_supplier_doc;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS duplicate_status;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS attachment_hash;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS supplier_document_number;
ALTER TABLE ap_invoice_lines DROP COLUMN IF EXISTS po_line_id;

ALTER TABLE pos DROP COLUMN IF EXISTS is_service;
