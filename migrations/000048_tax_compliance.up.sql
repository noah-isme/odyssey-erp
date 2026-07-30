-- Phase 5: immutable tax documents and reconcilable tax ledgers.
-- Regulatory values are intentionally not seeded. A rule becomes usable only
-- after its source, checksum, reviewer and review timestamp are recorded.

CREATE TABLE tax_rule_versions (
    id BIGSERIAL PRIMARY KEY,
    rule_kind TEXT NOT NULL CHECK (rule_kind IN ('VAT','WITHHOLDING','TAX_CODE')),
    version_code TEXT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE,
    source_url TEXT NOT NULL,
    source_checksum TEXT NOT NULL,
    reviewed_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (effective_to IS NULL OR effective_to >= effective_from),
    CHECK ((reviewed_at IS NULL) = (reviewed_by IS NULL)),
    UNIQUE (rule_kind, version_code)
);

ALTER TABLE tax_rule_versions ADD CONSTRAINT tax_rule_versions_no_reviewed_overlap
    EXCLUDE USING gist (rule_kind WITH =, daterange(effective_from,effective_to,'[]') WITH &&)
    WHERE (reviewed_at IS NOT NULL);

CREATE TABLE tax_vat_rates (
    id BIGSERIAL PRIMARY KEY,
    rule_version_id BIGINT NOT NULL REFERENCES tax_rule_versions(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    rate_bps INTEGER NOT NULL CHECK (rate_bps BETWEEN 0 AND 10000),
    dpp_numerator INTEGER NOT NULL DEFAULT 1 CHECK (dpp_numerator > 0),
    dpp_denominator INTEGER NOT NULL DEFAULT 1 CHECK (dpp_denominator > 0),
    luxury_only BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE(rule_version_id, code)
);

CREATE TABLE tax_withholding_types (
    id BIGSERIAL PRIMARY KEY,
    rule_version_id BIGINT NOT NULL REFERENCES tax_rule_versions(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    article TEXT NOT NULL CHECK (article IN ('PPh23','PPh4(2)')),
    name TEXT NOT NULL,
    recognition_event TEXT NOT NULL CHECK (recognition_event IN ('INVOICE','PAYMENT')),
    rate_bps INTEGER NOT NULL CHECK (rate_bps BETWEEN 0 AND 10000),
    tax_base TEXT NOT NULL DEFAULT 'GROSS' CHECK (tax_base IN ('GROSS','DPP')),
    UNIQUE(rule_version_id, code)
);

CREATE TABLE tax_codes (
    id BIGSERIAL PRIMARY KEY,
    rule_version_id BIGINT NOT NULL REFERENCES tax_rule_versions(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    tax_kind TEXT NOT NULL CHECK (tax_kind IN ('VAT_OUTPUT','VAT_INPUT','PPh23','PPh4(2)')),
    official_object_code TEXT,
    vat_rate_id BIGINT REFERENCES tax_vat_rates(id) ON DELETE RESTRICT,
    withholding_type_id BIGINT REFERENCES tax_withholding_types(id) ON DELETE RESTRICT,
    UNIQUE(rule_version_id, code),
    CHECK ((vat_rate_id IS NOT NULL)::int + (withholding_type_id IS NOT NULL)::int <= 1)
);

CREATE TABLE company_tax_identities (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    legal_name TEXT NOT NULL,
    npwp TEXT NOT NULL,
    nitku TEXT NOT NULL,
    pkp_number TEXT,
    registered_address TEXT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (effective_to IS NULL OR effective_to >= effective_from)
);
ALTER TABLE company_tax_identities ADD CONSTRAINT company_tax_identities_no_overlap
    EXCLUDE USING gist (company_id WITH =, daterange(effective_from,effective_to,'[]') WITH &&);

ALTER TABLE suppliers ADD COLUMN tax_id TEXT NOT NULL DEFAULT '';

CREATE TABLE tax_invoice_number_ranges (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    prefix TEXT NOT NULL,
    range_start BIGINT NOT NULL,
    range_end BIGINT NOT NULL,
    next_number BIGINT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (range_start > 0 AND range_end >= range_start),
    CHECK (next_number BETWEEN range_start AND range_end + 1),
    CHECK (effective_to IS NULL OR effective_to >= effective_from),
    UNIQUE(company_id, prefix, range_start, range_end)
);

CREATE TABLE tax_periods (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    accounting_period_id BIGINT NOT NULL REFERENCES accounting_periods(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','LOCKED')),
    locked_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    locked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, accounting_period_id),
    CHECK ((status='LOCKED') = (locked_at IS NOT NULL))
);

CREATE TABLE tax_documents (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    tax_period_id BIGINT NOT NULL REFERENCES tax_periods(id) ON DELETE RESTRICT,
    source_type TEXT NOT NULL CHECK (source_type IN ('AR_INVOICE','AR_CREDIT_NOTE','AP_INVOICE','AP_DEBIT_NOTE')),
    source_id BIGINT NOT NULL,
    source_number TEXT NOT NULL,
    source_posted_at TIMESTAMPTZ NOT NULL,
    document_kind TEXT NOT NULL CHECK (document_kind IN ('INVOICE','CREDIT_NOTE','DEBIT_NOTE','REPLACEMENT')),
    direction TEXT NOT NULL CHECK (direction IN ('OUTPUT','INPUT')),
    tax_number TEXT,
    issue_date DATE NOT NULL,
    counterparty_name TEXT NOT NULL,
    counterparty_tax_id TEXT NOT NULL,
    taxable_base NUMERIC(18,2) NOT NULL,
    vat_amount NUMERIC(18,2) NOT NULL,
    gross_amount NUMERIC(18,2) NOT NULL,
    sign SMALLINT NOT NULL CHECK (sign IN (-1,1)),
    tax_code_id BIGINT REFERENCES tax_codes(id) ON DELETE RESTRICT,
    rule_version_id BIGINT NOT NULL REFERENCES tax_rule_versions(id) ON DELETE RESTRICT,
    replacement_of_id BIGINT REFERENCES tax_documents(id) ON DELETE RESTRICT,
    source_hash TEXT NOT NULL,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source_type, source_id),
    UNIQUE(company_id, tax_number),
    CHECK (taxable_base >= 0 AND vat_amount >= 0 AND gross_amount >= 0),
    CHECK (document_kind <> 'REPLACEMENT' OR replacement_of_id IS NOT NULL)
);
CREATE INDEX idx_tax_documents_period ON tax_documents(tax_period_id, direction, issue_date);

-- Regulated faktur metadata is mirrored on AR for operational reads, while
-- tax_documents remains the immutable source of truth used by ledgers/exports.
ALTER TABLE ar_invoices
    ADD COLUMN tax_code_id BIGINT REFERENCES tax_codes(id) ON DELETE RESTRICT,
    ADD COLUMN tax_document_id BIGINT REFERENCES tax_documents(id) ON DELETE RESTRICT,
    ADD COLUMN faktur_number TEXT,
    ADD COLUMN faktur_issue_date DATE,
    ADD COLUMN buyer_tax_id TEXT,
    ADD COLUMN faktur_taxable_base NUMERIC(18,2),
    ADD COLUMN faktur_vat_amount NUMERIC(18,2),
    ADD COLUMN faktur_status TEXT CHECK (faktur_status IN ('ISSUED','CANCELLED','REPLACED')),
    ADD COLUMN replacement_of_tax_document_id BIGINT REFERENCES tax_documents(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX uq_ar_invoice_faktur_number ON ar_invoices(faktur_number) WHERE faktur_number IS NOT NULL;
CREATE UNIQUE INDEX uq_ar_invoice_tax_document ON ar_invoices(tax_document_id) WHERE tax_document_id IS NOT NULL;

ALTER TABLE ap_invoices
    ADD COLUMN withholding_type_id BIGINT REFERENCES tax_withholding_types(id) ON DELETE RESTRICT;

CREATE TABLE tax_document_events (
    id BIGSERIAL PRIMARY KEY,
    tax_document_id BIGINT NOT NULL REFERENCES tax_documents(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL CHECK (event_type IN ('ISSUED','CANCELLED','REPLACED')),
    reason TEXT NOT NULL DEFAULT '',
    replacement_document_id BIGINT REFERENCES tax_documents(id) ON DELETE RESTRICT,
    actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tax_document_id, event_type),
    CHECK (event_type <> 'REPLACED' OR replacement_document_id IS NOT NULL)
);

CREATE TABLE tax_withholding_records (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    tax_period_id BIGINT NOT NULL REFERENCES tax_periods(id) ON DELETE RESTRICT,
    ap_invoice_id BIGINT NOT NULL REFERENCES ap_invoices(id) ON DELETE RESTRICT,
    ap_payment_id BIGINT REFERENCES ap_payments(id) ON DELETE RESTRICT,
    source_event TEXT NOT NULL CHECK (source_event IN ('INVOICE','PAYMENT')),
    withholding_type_id BIGINT NOT NULL REFERENCES tax_withholding_types(id) ON DELETE RESTRICT,
    tax_code_id BIGINT REFERENCES tax_codes(id) ON DELETE RESTRICT,
    recognition_date DATE NOT NULL,
    taxable_base NUMERIC(18,2) NOT NULL CHECK (taxable_base >= 0),
    withheld_amount NUMERIC(18,2) NOT NULL CHECK (withheld_amount >= 0),
    supplier_tax_id TEXT NOT NULL,
    source_hash TEXT NOT NULL,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX uq_tax_withholding_invoice_event ON tax_withholding_records(withholding_type_id,ap_invoice_id,source_event) WHERE ap_payment_id IS NULL;
CREATE UNIQUE INDEX uq_tax_withholding_payment_event ON tax_withholding_records(withholding_type_id,ap_invoice_id,ap_payment_id,source_event) WHERE ap_payment_id IS NOT NULL;

CREATE TABLE tax_account_mappings (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    category TEXT NOT NULL CHECK (category IN ('VAT_OUTPUT','VAT_INPUT','PPh23','PPh4(2)')),
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    effective_from DATE NOT NULL,
    effective_to DATE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (effective_to IS NULL OR effective_to >= effective_from)
);
ALTER TABLE tax_account_mappings ADD CONSTRAINT tax_account_mappings_no_overlap
    EXCLUDE USING gist (company_id WITH =, category WITH =, daterange(effective_from,effective_to,'[]') WITH &&);

CREATE TABLE tax_ledger_entries (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    tax_period_id BIGINT NOT NULL REFERENCES tax_periods(id) ON DELETE RESTRICT,
    category TEXT NOT NULL CHECK (category IN ('VAT_OUTPUT','VAT_INPUT','PPh23','PPh4(2)')),
    tax_document_id BIGINT REFERENCES tax_documents(id) ON DELETE RESTRICT,
    withholding_record_id BIGINT REFERENCES tax_withholding_records(id) ON DELETE RESTRICT,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    source_type TEXT NOT NULL,
    source_id BIGINT NOT NULL,
    source_date DATE NOT NULL,
    taxable_base NUMERIC(18,2) NOT NULL,
    tax_amount NUMERIC(18,2) NOT NULL,
    sign SMALLINT NOT NULL CHECK (sign IN (-1,1)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(category, source_type, source_id),
    CHECK ((tax_document_id IS NOT NULL)::int + (withholding_record_id IS NOT NULL)::int = 1)
);
CREATE INDEX idx_tax_ledger_recap ON tax_ledger_entries(company_id,tax_period_id,category,account_id);

CREATE TABLE tax_export_schemas (
    id BIGSERIAL PRIMARY KEY,
    export_kind TEXT NOT NULL CHECK (export_kind IN ('CORETAX_OUTPUT_VAT','CORETAX_WITHHOLDING')),
    version_code TEXT NOT NULL,
    media_type TEXT NOT NULL DEFAULT 'application/xml',
    schema_body TEXT NOT NULL,
    official_source_url TEXT NOT NULL,
    official_checksum TEXT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE,
    reviewed_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(export_kind,version_code),
    CHECK (effective_to IS NULL OR effective_to >= effective_from),
    CHECK ((reviewed_at IS NULL) = (reviewed_by IS NULL))
);

CREATE TABLE tax_exports (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    tax_period_id BIGINT NOT NULL REFERENCES tax_periods(id) ON DELETE RESTRICT,
    schema_id BIGINT NOT NULL REFERENCES tax_export_schemas(id) ON DELETE RESTRICT,
    content_hash TEXT NOT NULL,
    record_count INTEGER NOT NULL CHECK (record_count >= 0),
    taxable_base NUMERIC(18,2) NOT NULL,
    tax_amount NUMERIC(18,2) NOT NULL,
    generated_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id,tax_period_id,schema_id,content_hash)
);

CREATE TABLE tax_audit_events (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    entity_type TEXT NOT NULL,
    entity_id BIGINT NOT NULL,
    action TEXT NOT NULL,
    actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE OR REPLACE FUNCTION reject_tax_immutable_change() RETURNS TRIGGER AS $$
BEGIN RAISE EXCEPTION 'tax compliance records are immutable'; END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trg_tax_documents_immutable BEFORE UPDATE OR DELETE ON tax_documents FOR EACH ROW EXECUTE FUNCTION reject_tax_immutable_change();
CREATE TRIGGER trg_tax_document_events_immutable BEFORE UPDATE OR DELETE ON tax_document_events FOR EACH ROW EXECUTE FUNCTION reject_tax_immutable_change();
CREATE TRIGGER trg_tax_withholding_immutable BEFORE UPDATE OR DELETE ON tax_withholding_records FOR EACH ROW EXECUTE FUNCTION reject_tax_immutable_change();
CREATE TRIGGER trg_tax_ledger_immutable BEFORE UPDATE OR DELETE ON tax_ledger_entries FOR EACH ROW EXECUTE FUNCTION reject_tax_immutable_change();
CREATE TRIGGER trg_tax_exports_immutable BEFORE UPDATE OR DELETE ON tax_exports FOR EACH ROW EXECUTE FUNCTION reject_tax_immutable_change();

CREATE OR REPLACE FUNCTION guard_locked_tax_period() RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM tax_periods WHERE id=NEW.tax_period_id AND status='LOCKED') THEN
        RAISE EXCEPTION 'tax period is locked';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trg_tax_documents_period BEFORE INSERT ON tax_documents FOR EACH ROW EXECUTE FUNCTION guard_locked_tax_period();
CREATE TRIGGER trg_tax_withholding_period BEFORE INSERT ON tax_withholding_records FOR EACH ROW EXECUTE FUNCTION guard_locked_tax_period();
CREATE TRIGGER trg_tax_ledger_period BEFORE INSERT ON tax_ledger_entries FOR EACH ROW EXECUTE FUNCTION guard_locked_tax_period();

CREATE VIEW v_tax_document_status AS
SELECT d.*, CASE
    WHEN EXISTS (SELECT 1 FROM tax_document_events e WHERE e.tax_document_id=d.id AND e.event_type='CANCELLED') THEN 'CANCELLED'
    WHEN EXISTS (SELECT 1 FROM tax_document_events e WHERE e.tax_document_id=d.id AND e.event_type='REPLACED') THEN 'REPLACED'
    ELSE 'ISSUED' END AS status
FROM tax_documents d;

INSERT INTO permissions(name,description) VALUES
 ('tax.view','View tax documents, ledgers, and recaps'),
 ('tax.config.manage','Manage reviewed tax configuration'),
 ('tax.period.lock','Lock tax reporting periods'),
 ('tax.document.correct','Cancel or replace tax documents'),
 ('tax.report.export','Generate tax authority exports')
ON CONFLICT(name) DO UPDATE SET description=EXCLUDED.description;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE LOWER(r.name) IN ('admin','administrator') AND p.name LIKE 'tax.%'
ON CONFLICT DO NOTHING;
