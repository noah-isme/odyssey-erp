DROP VIEW IF EXISTS v_tax_document_status;
DROP TRIGGER IF EXISTS trg_tax_ledger_period ON tax_ledger_entries;
DROP TRIGGER IF EXISTS trg_tax_withholding_period ON tax_withholding_records;
DROP TRIGGER IF EXISTS trg_tax_documents_period ON tax_documents;
DROP TRIGGER IF EXISTS trg_tax_exports_immutable ON tax_exports;
DROP TRIGGER IF EXISTS trg_tax_ledger_immutable ON tax_ledger_entries;
DROP TRIGGER IF EXISTS trg_tax_withholding_immutable ON tax_withholding_records;
DROP TRIGGER IF EXISTS trg_tax_document_events_immutable ON tax_document_events;
DROP TRIGGER IF EXISTS trg_tax_documents_immutable ON tax_documents;
DROP FUNCTION IF EXISTS guard_locked_tax_period();
DROP FUNCTION IF EXISTS reject_tax_immutable_change();
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE name LIKE 'tax.%');
DELETE FROM permissions WHERE name LIKE 'tax.%';
DROP TABLE IF EXISTS tax_audit_events;
DROP TABLE IF EXISTS tax_exports;
DROP TABLE IF EXISTS tax_export_schemas;
DROP TABLE IF EXISTS tax_ledger_entries;
DROP TABLE IF EXISTS tax_account_mappings;
DROP TABLE IF EXISTS tax_withholding_records;
DROP TABLE IF EXISTS tax_document_events;
DROP INDEX IF EXISTS uq_ar_invoice_tax_document;
DROP INDEX IF EXISTS uq_ar_invoice_faktur_number;
ALTER TABLE ar_invoices DROP COLUMN IF EXISTS replacement_of_tax_document_id,
    DROP COLUMN IF EXISTS faktur_status, DROP COLUMN IF EXISTS faktur_vat_amount,
    DROP COLUMN IF EXISTS faktur_taxable_base, DROP COLUMN IF EXISTS buyer_tax_id,
    DROP COLUMN IF EXISTS faktur_issue_date, DROP COLUMN IF EXISTS faktur_number,
    DROP COLUMN IF EXISTS tax_document_id, DROP COLUMN IF EXISTS tax_code_id;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS withholding_type_id;
DROP TABLE IF EXISTS tax_documents;
DROP TABLE IF EXISTS tax_periods;
DROP TABLE IF EXISTS tax_invoice_number_ranges;
ALTER TABLE suppliers DROP COLUMN IF EXISTS tax_id;
DROP TABLE IF EXISTS company_tax_identities;
DROP TABLE IF EXISTS tax_codes;
DROP TABLE IF EXISTS tax_withholding_types;
DROP TABLE IF EXISTS tax_vat_rates;
DROP TABLE IF EXISTS tax_rule_versions;
