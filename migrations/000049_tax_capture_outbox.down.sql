DROP TRIGGER IF EXISTS trg_ap_payment_tax_capture ON ap_payment_allocations;
DROP FUNCTION IF EXISTS enqueue_ap_payment_tax_capture();
DROP TRIGGER IF EXISTS trg_ap_debit_note_tax_capture ON ap_debit_notes;
DROP TRIGGER IF EXISTS trg_ap_invoice_tax_capture ON ap_invoices;
DROP TRIGGER IF EXISTS trg_ar_credit_note_tax_capture ON ar_credit_notes;
DROP TRIGGER IF EXISTS trg_ar_invoice_tax_capture ON ar_invoices;
DROP FUNCTION IF EXISTS enqueue_posted_tax_source();
DROP TABLE IF EXISTS tax_capture_outbox;
ALTER TABLE tax_export_schemas DROP COLUMN IF EXISTS include_sign_element,
    DROP COLUMN IF EXISTS xml_declaration;
