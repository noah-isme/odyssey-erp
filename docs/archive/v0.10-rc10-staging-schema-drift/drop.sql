-- HISTORICAL (2026-10-09): removal of the FIND-010 out-of-band objects from the
-- staging database. Run with: psql -X -f drop.sql. Rollback: restore.sql.
-- Remove out-of-band objects from the staging database so its schema matches
-- migrations 000001..000124 again. Backup of definitions: staging-patches-backup.sql
\set ON_ERROR_STOP 1
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

DROP TRIGGER trg_mark_document_signature_challenge_used ON document_signatures;
DROP TRIGGER trg_set_document_version_classification ON document_versions;
DROP TRIGGER trg_set_ar_invoice_subtotal ON ar_invoices;
DROP TRIGGER trg_ap_invoice_post_journal ON ap_invoices;
DROP TRIGGER trg_set_ap_invoice_line_po_line ON ap_invoice_lines;

DROP FUNCTION mark_document_signature_challenge_used();
DROP FUNCTION set_document_version_classification();
DROP FUNCTION set_ar_invoice_subtotal_default();
DROP FUNCTION trg_ap_invoice_post_journal();
DROP FUNCTION set_ap_invoice_line_po_line();

DROP VIEW document_blobs;

ALTER TABLE document_signature_challenges
  ALTER COLUMN challenge_id DROP DEFAULT,
  ALTER COLUMN meaning DROP DEFAULT,
  ALTER COLUMN policy_version DROP DEFAULT;

-- Self-check: abort unless every object is gone.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_trigger WHERE NOT tgisinternal AND tgname IN (
       'trg_mark_document_signature_challenge_used','trg_set_document_version_classification',
       'trg_set_ar_invoice_subtotal','trg_ap_invoice_post_journal','trg_set_ap_invoice_line_po_line'))
     OR EXISTS (SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.proname IN (
       'mark_document_signature_challenge_used','set_document_version_classification',
       'set_ar_invoice_subtotal_default','trg_ap_invoice_post_journal','set_ap_invoice_line_po_line'))
     OR EXISTS (SELECT 1 FROM information_schema.views WHERE table_name='document_blobs')
     OR EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='document_signature_challenges'
       AND column_name IN ('challenge_id','meaning','policy_version') AND column_default IS NOT NULL)
  THEN RAISE EXCEPTION 'staging patch removal incomplete';
  END IF;
END $$;

COMMIT;
