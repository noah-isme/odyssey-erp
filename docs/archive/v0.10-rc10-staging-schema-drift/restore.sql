-- HISTORICAL (2026-10-09): restores the out-of-band objects that were added to
-- the staging database for the invalidated rc.10 certification run (FIND-010).
-- Kept only as a rollback for the removal in drop.sql. Do not apply otherwise.
-- Verified: on a PG17 database migrated to 000124 this reproduces staging's
-- pre-removal schema exactly (pg_dump --schema-only diff of 0 lines).
\set ON_ERROR_STOP 1
BEGIN;
-- function: mark_document_signature_challenge_used
CREATE OR REPLACE FUNCTION public.mark_document_signature_challenge_used()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
    UPDATE document_signature_challenges SET used = TRUE WHERE challenge_id = NEW.challenge_id;
    RETURN NEW;
END;
$function$
;

-- function: set_ap_invoice_line_po_line
CREATE OR REPLACE FUNCTION public.set_ap_invoice_line_po_line()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
    IF NEW.po_line_id IS NULL AND NEW.grn_line_id IS NOT NULL THEN
        NEW.po_line_id := (
            SELECT pl.id 
            FROM po_lines pl 
            JOIN grns g ON g.po_id = pl.po_id
            JOIN grn_lines gl ON gl.grn_id = g.id AND gl.product_id = pl.product_id
            WHERE gl.id = NEW.grn_line_id 
            LIMIT 1
        );
    END IF;
    RETURN NEW;
END;
$function$
;

-- function: set_ar_invoice_subtotal_default
CREATE OR REPLACE FUNCTION public.set_ar_invoice_subtotal_default()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
		BEGIN
			IF (NEW.subtotal IS NULL OR NEW.subtotal = 0) AND (NEW.tax_amount IS NULL OR NEW.tax_amount = 0) AND NEW.total > 0 THEN
				NEW.subtotal := NEW.total;
			END IF;
			RETURN NEW;
		END;
		$function$
;

-- function: set_document_version_classification
CREATE OR REPLACE FUNCTION public.set_document_version_classification()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
    IF NEW.classification_id IS NULL OR NEW.classification_id = 0 OR NOT EXISTS (SELECT 1 FROM document_classifications WHERE id = NEW.classification_id) THEN
        SELECT classification_id INTO NEW.classification_id FROM documents WHERE id = NEW.document_id;
        IF NEW.classification_id IS NULL OR NEW.classification_id = 0 THEN
            SELECT id INTO NEW.classification_id FROM document_classifications ORDER BY id LIMIT 1;
        END IF;
    END IF;
    IF NEW.blob_id IS NULL OR NEW.blob_id = 0 OR NOT EXISTS (SELECT 1 FROM storage_blobs WHERE id = NEW.blob_id) THEN
        NEW.blob_id := (SELECT id FROM storage_blobs WHERE company_id = NEW.company_id ORDER BY id DESC LIMIT 1);
    END IF;
    RETURN NEW;
END;
$function$
;

-- function: trg_ap_invoice_post_journal
CREATE OR REPLACE FUNCTION public.trg_ap_invoice_post_journal()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
DECLARE
    v_period_id bigint;
    v_je_id bigint;
    v_debit_acc bigint;
    v_credit_acc bigint;
BEGIN
    IF NEW.status = 'POSTED' AND (OLD IS NULL OR OLD.status IS NULL OR OLD.status != 'POSTED') THEN
        IF NOT EXISTS (
            SELECT 1 FROM journal_entries 
            WHERE source_module = 'PROCUREMENT.AP_INVOICE' AND memo = 'AP Invoice ' || NEW.number
        ) THEN
            SELECT id INTO v_period_id FROM periods 
            WHERE start_date <= COALESCE(NEW.posted_at::date, CURRENT_DATE) 
              AND end_date >= COALESCE(NEW.posted_at::date, CURRENT_DATE) 
            LIMIT 1;

            SELECT account_id INTO v_debit_acc FROM account_mappings 
            WHERE module = 'AP' AND key = CASE WHEN NEW.grn_id IS NOT NULL THEN 'ap.invoice.inventory' ELSE 'ap.invoice.expense' END 
            LIMIT 1;
            IF v_debit_acc IS NULL THEN v_debit_acc := 9; END IF;

            SELECT account_id INTO v_credit_acc FROM account_mappings 
            WHERE module = 'AP' AND key = 'ap.invoice.ap' 
            LIMIT 1;
            IF v_credit_acc IS NULL THEN v_credit_acc := 15; END IF;

            INSERT INTO journal_entries (
                period_id, date, source_module, memo, posted_by, posted_at, status, created_at, updated_at
            ) VALUES (
                v_period_id, COALESCE(NEW.posted_at::date, CURRENT_DATE), 'PROCUREMENT.AP_INVOICE',
                'AP Invoice ' || NEW.number, COALESCE(NEW.posted_by, 1), COALESCE(NEW.posted_at, NOW()),
                'POSTED', NOW(), NOW()
            ) RETURNING id INTO v_je_id;

            INSERT INTO journal_lines (je_id, account_id, debit, credit, created_at, updated_at)
            VALUES 
            (v_je_id, v_debit_acc, NEW.total, 0, NOW(), NOW()),
            (v_je_id, v_credit_acc, 0, NEW.total, NOW(), NOW());

            INSERT INTO audit_logs (actor_id, action, entity, entity_id, meta, occurred_at)
            VALUES (COALESCE(NEW.posted_by, 1), 'post', 'ap_invoice', NEW.id::text, '{}'::jsonb, NOW());
        END IF;
    END IF;
    RETURN NEW;
END;
$function$
;

-- view: document_blobs
CREATE VIEW document_blobs AS  SELECT id,
    company_id,
    storage_key,
    storage_driver,
    bucket,
    size_bytes,
    checksum_sha256,
    declared_content_type,
    detected_content_type,
    encryption_metadata,
    malware_scan_status,
    malware_scan_details,
    metadata,
    reference_count,
    created_at,
    created_by
   FROM storage_blobs;
;
ALTER TABLE public.document_signature_challenges ALTER COLUMN challenge_id SET DEFAULT gen_random_uuid(), ALTER COLUMN meaning SET DEFAULT 'Approved as effective version'::character varying, ALTER COLUMN policy_version SET DEFAULT 1;
-- trigger: trg_ap_invoice_post_journal
CREATE TRIGGER trg_ap_invoice_post_journal AFTER INSERT OR UPDATE OF status ON public.ap_invoices FOR EACH ROW EXECUTE FUNCTION trg_ap_invoice_post_journal();

-- trigger: trg_mark_document_signature_challenge_used
CREATE TRIGGER trg_mark_document_signature_challenge_used AFTER INSERT ON public.document_signatures FOR EACH ROW EXECUTE FUNCTION mark_document_signature_challenge_used();

-- trigger: trg_set_ap_invoice_line_po_line
CREATE TRIGGER trg_set_ap_invoice_line_po_line BEFORE INSERT ON public.ap_invoice_lines FOR EACH ROW EXECUTE FUNCTION set_ap_invoice_line_po_line();

-- trigger: trg_set_ar_invoice_subtotal
CREATE TRIGGER trg_set_ar_invoice_subtotal BEFORE INSERT OR UPDATE ON public.ar_invoices FOR EACH ROW EXECUTE FUNCTION set_ar_invoice_subtotal_default();

-- trigger: trg_set_document_version_classification
CREATE TRIGGER trg_set_document_version_classification BEFORE INSERT ON public.document_versions FOR EACH ROW EXECUTE FUNCTION set_document_version_classification();
COMMIT;
