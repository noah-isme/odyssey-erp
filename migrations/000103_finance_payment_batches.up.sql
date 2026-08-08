CREATE TABLE IF NOT EXISTS treasury_payment_batches (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    reference_code VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'DRAFT', -- DRAFT, PENDING_APPROVAL, APPROVED, EXPORTED, SETTLED, CANCELLED
    currency VARCHAR(3) NOT NULL,
    total_amount NUMERIC(19,4) NOT NULL DEFAULT 0,
    revision_number INT NOT NULL DEFAULT 1,
    proposed_by BIGINT NOT NULL,
    approved_by BIGINT,
    approved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_treasury_payment_batches_ref ON treasury_payment_batches(company_id, reference_code);

CREATE TABLE IF NOT EXISTS treasury_payment_batch_items (
    id BIGSERIAL PRIMARY KEY,
    batch_id BIGINT NOT NULL REFERENCES treasury_payment_batches(id) ON DELETE CASCADE,
    supplier_id BIGINT NOT NULL,
    bank_account_id BIGINT NOT NULL REFERENCES treasury_supplier_bank_accounts(id),
    amount NUMERIC(19,4) NOT NULL,
    ap_invoice_id BIGINT,
    status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE', -- ACTIVE, REMOVED
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
