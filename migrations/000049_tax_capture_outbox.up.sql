-- Durable boundary between AR/AP posting transactions and tax capture.
CREATE TABLE tax_capture_outbox (
    id BIGSERIAL PRIMARY KEY,
    source_type TEXT NOT NULL CHECK (source_type IN ('AR_INVOICE','AR_CREDIT_NOTE','AP_INVOICE','AP_DEBIT_NOTE','AP_PAYMENT')),
    source_id BIGINT NOT NULL,
    actor_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source_type,source_id)
);
CREATE INDEX idx_tax_capture_outbox_pending ON tax_capture_outbox(available_at,id) WHERE completed_at IS NULL;

ALTER TABLE tax_export_schemas
    ADD COLUMN xml_declaration TEXT NOT NULL DEFAULT '<?xml version="1.0" encoding="UTF-8"?>',
    ADD COLUMN include_sign_element BOOLEAN NOT NULL DEFAULT FALSE;

CREATE OR REPLACE FUNCTION enqueue_posted_tax_source() RETURNS TRIGGER AS $$
DECLARE source_type_value TEXT;
BEGIN
    source_type_value := CASE TG_TABLE_NAME
        WHEN 'ar_invoices' THEN 'AR_INVOICE'
        WHEN 'ar_credit_notes' THEN 'AR_CREDIT_NOTE'
        WHEN 'ap_invoices' THEN 'AP_INVOICE'
        WHEN 'ap_debit_notes' THEN 'AP_DEBIT_NOTE'
    END;
    INSERT INTO tax_capture_outbox(source_type,source_id,actor_id)
    VALUES(source_type_value,NEW.id,COALESCE(NEW.posted_by,NEW.created_by))
    ON CONFLICT(source_type,source_id) DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ar_invoice_tax_capture AFTER UPDATE OF status ON ar_invoices
FOR EACH ROW WHEN (NEW.status IN ('POSTED','PAID') AND OLD.status='DRAFT') EXECUTE FUNCTION enqueue_posted_tax_source();
CREATE TRIGGER trg_ar_credit_note_tax_capture AFTER UPDATE OF status ON ar_credit_notes
FOR EACH ROW WHEN (NEW.status='POSTED' AND OLD.status='DRAFT') EXECUTE FUNCTION enqueue_posted_tax_source();
CREATE TRIGGER trg_ap_invoice_tax_capture AFTER UPDATE OF status ON ap_invoices
FOR EACH ROW WHEN (NEW.status IN ('POSTED','PAID') AND OLD.status='DRAFT') EXECUTE FUNCTION enqueue_posted_tax_source();
CREATE TRIGGER trg_ap_debit_note_tax_capture AFTER UPDATE OF status ON ap_debit_notes
FOR EACH ROW WHEN (NEW.status='POSTED' AND OLD.status='DRAFT') EXECUTE FUNCTION enqueue_posted_tax_source();

CREATE OR REPLACE FUNCTION enqueue_ap_payment_tax_capture() RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO tax_capture_outbox(source_type,source_id,actor_id)
    SELECT 'AP_PAYMENT',p.id,p.created_by
    FROM ap_payments p
    JOIN ap_invoices i ON i.id=NEW.ap_invoice_id
    JOIN tax_withholding_types w ON w.id=i.withholding_type_id AND w.recognition_event='PAYMENT'
    WHERE p.id=NEW.ap_payment_id
    ON CONFLICT(source_type,source_id) DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trg_ap_payment_tax_capture AFTER INSERT ON ap_payment_allocations
FOR EACH ROW EXECUTE FUNCTION enqueue_ap_payment_tax_capture();
