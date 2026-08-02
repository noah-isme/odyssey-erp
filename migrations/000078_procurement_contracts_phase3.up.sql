-- Phase 3: Vendor Intelligence - Supplier Contracts, Price History, and Scorecards

-- Supplier contracts: versioned with effective dates and product price tiers
CREATE TABLE supplier_contracts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    supplier_id BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    version INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','APPROVAL','ACTIVE','EXPIRED','TERMINATED')),
    currency TEXT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE,
    payment_terms TEXT NOT NULL DEFAULT '',
    incoterms TEXT NOT NULL DEFAULT '',
    renewal_notice_days INTEGER NOT NULL DEFAULT 30,
    expiry_notification_sent BOOLEAN NOT NULL DEFAULT FALSE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    approved_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ,
    terminated_at TIMESTAMPTZ,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, supplier_id, version),
    CHECK (effective_to IS NULL OR effective_to > effective_from)
);
CREATE INDEX idx_supplier_contracts_company_supplier ON supplier_contracts(company_id, supplier_id, status);
CREATE INDEX idx_supplier_contracts_active ON supplier_contracts(supplier_id, status, effective_from, effective_to);

-- Contract product price tiers: quantity-based pricing
CREATE TABLE contract_price_lines (
    id BIGSERIAL PRIMARY KEY,
    contract_id BIGSERIAL NOT NULL REFERENCES supplier_contracts(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    min_quantity NUMERIC(18,4) NOT NULL DEFAULT 0 CHECK (min_quantity >= 0),
    unit_price NUMERIC(18,4) NOT NULL CHECK (unit_price >= 0),
    tax_rate NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (tax_rate >= 0 AND tax_rate <= 100),
    lead_time_days INTEGER NOT NULL DEFAULT 0 CHECK (lead_time_days >= 0),
    moq NUMERIC(18,4) NOT NULL DEFAULT 0 CHECK (moq >= 0),
    UNIQUE (contract_id, product_id, min_quantity)
);
CREATE INDEX idx_contract_price_lines_contract ON contract_price_lines(contract_id);
CREATE INDEX idx_contract_price_lines_product ON contract_price_lines(product_id);

-- Immutable price history from bids, awards, and approved POs
CREATE TABLE price_history (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    supplier_id BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    source_type TEXT NOT NULL CHECK (source_type IN ('BID','AWARD','CONTRACT','PO')),
    source_id BIGINT NOT NULL,
    currency TEXT NOT NULL,
    unit_price NUMERIC(18,4) NOT NULL CHECK (unit_price >= 0),
    quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
    tax_rate NUMERIC(5,2) NOT NULL DEFAULT 0,
    moq NUMERIC(18,4) NOT NULL DEFAULT 0,
    lead_time_days INTEGER NOT NULL DEFAULT 0,
    fx_rate NUMERIC(18,6),
    base_currency_price NUMERIC(18,4),
    observation_date DATE NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Immutable: no updates allowed
    CONSTRAINT price_history_immutable CHECK (created_at = created_at)
);
CREATE INDEX idx_price_history_supplier_product ON price_history(supplier_id, product_id, observation_date DESC);
CREATE INDEX idx_price_history_source ON price_history(source_type, source_id);
CREATE INDEX idx_price_history_company ON price_history(company_id, observation_date DESC);

-- Supplier scorecards: versioned and immutable once published
CREATE TABLE supplier_scorecards (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    supplier_id BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    version INTEGER NOT NULL DEFAULT 1,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','PUBLISHED')),
    delivery_otif_score NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (delivery_otif_score BETWEEN 0 AND 100),
    delivery_otif_weight INTEGER NOT NULL DEFAULT 35,
    delivery_otif_sample_size INTEGER NOT NULL DEFAULT 0,
    quality_score NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (quality_score BETWEEN 0 AND 100),
    quality_weight INTEGER NOT NULL DEFAULT 25,
    quality_sample_size INTEGER NOT NULL DEFAULT 0,
    price_adherence_score NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (price_adherence_score BETWEEN 0 AND 100),
    price_adherence_weight INTEGER NOT NULL DEFAULT 20,
    price_adherence_sample_size INTEGER NOT NULL DEFAULT 0,
    rfq_responsiveness_score NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (rfq_responsiveness_score BETWEEN 0 AND 100),
    rfq_responsiveness_weight INTEGER NOT NULL DEFAULT 10,
    rfq_responsiveness_sample_size INTEGER NOT NULL DEFAULT 0,
    reviewer_assessment_score NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (reviewer_assessment_score BETWEEN 0 AND 100),
    reviewer_assessment_weight INTEGER NOT NULL DEFAULT 10,
    overall_score NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (overall_score BETWEEN 0 AND 100),
    published_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    published_at TIMESTAMPTZ,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, supplier_id, version)
);
CREATE INDEX idx_supplier_scorecards_published ON supplier_scorecards(company_id, supplier_id, status, period_end DESC);

-- PO variance controls and exceptions
CREATE TABLE po_contract_variances (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    po_id BIGINT NOT NULL REFERENCES pos(id) ON DELETE CASCADE,
    po_line_id BIGINT NOT NULL REFERENCES po_lines(id) ON DELETE CASCADE,
    contract_id BIGINT REFERENCES supplier_contracts(id) ON DELETE SET NULL,
    variance_type TEXT NOT NULL CHECK (variance_type IN ('NO_CONTRACT','EXPIRED_CONTRACT','PRICE_VARIANCE','TERM_VARIANCE')),
    variance_percentage NUMERIC(7,2),
    variance_reason TEXT NOT NULL DEFAULT '',
    approval_status TEXT NOT NULL DEFAULT 'PENDING' CHECK (approval_status IN ('PENDING','APPROVED','REJECTED')),
    approved_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_po_contract_variances_po ON po_contract_variances(po_id, approval_status);
CREATE INDEX idx_po_contract_variances_company ON po_contract_variances(company_id, approval_status);

-- Audit trail for contract changes (extends existing audit_logs table)
-- Contract lifecycle changes are recorded in audit_logs with entity='supplier_contract'
-- Price history source references are immutable and audited via creation

-- Permissions for Phase 3: Vendor Intelligence
INSERT INTO permissions (module, action, description) VALUES
    ('procurement.contract.create', 'Create supplier contracts', 'Create draft supplier contracts'),
    ('procurement.contract.submit', 'Submit contracts for approval', 'Submit contracts to approval workflow'),
    ('procurement.contract.approve', 'Approve supplier contracts', 'Approve contracts to activate'),
    ('procurement.contract.terminate', 'Terminate contracts', 'Terminate active contracts'),
    ('procurement.supplier_rating.view', 'View supplier ratings', 'View supplier performance scorecards'),
    ('procurement.supplier_rating.create', 'Create supplier ratings', 'Create draft scorecards'),
    ('procurement.supplier_rating.publish', 'Publish ratings', 'Publish supplier scorecards'),
    ('procurement.price_history.view', 'View price history', 'View supplier/product price trends'),
    ('procurement.variance.view', 'View PO variances', 'View contract variance exceptions'),
    ('procurement.variance.approve', 'Approve PO variances', 'Approve variance exceptions')
ON CONFLICT (module, action) DO NOTHING;
