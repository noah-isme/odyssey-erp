-- Procurement sourcing foundation: RFQs, supplier bids, comparisons, awards,
-- and the permissions shared by the sourcing and future logistics modules.

CREATE TABLE rfqs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    number TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','ISSUED','CLOSED','AWARDED','CANCELLED')),
    currency TEXT NOT NULL,
    response_due_at TIMESTAMPTZ NOT NULL,
    commercial_terms TEXT NOT NULL DEFAULT '',
    price_weight INTEGER NOT NULL DEFAULT 50 CHECK (price_weight BETWEEN 0 AND 100),
    lead_time_weight INTEGER NOT NULL DEFAULT 20 CHECK (lead_time_weight BETWEEN 0 AND 100),
    terms_weight INTEGER NOT NULL DEFAULT 10 CHECK (terms_weight BETWEEN 0 AND 100),
    supplier_rating_weight INTEGER NOT NULL DEFAULT 20 CHECK (supplier_rating_weight BETWEEN 0 AND 100),
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    issued_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    awarded_at TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, number),
    CHECK (price_weight + lead_time_weight + terms_weight + supplier_rating_weight = 100)
);
CREATE INDEX idx_rfqs_company_status ON rfqs(company_id, status, response_due_at);

CREATE TABLE rfq_lines (
    id BIGSERIAL PRIMARY KEY,
    rfq_id BIGINT NOT NULL REFERENCES rfqs(id) ON DELETE CASCADE,
    pr_line_id BIGINT REFERENCES pr_lines(id) ON DELETE SET NULL,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
    note TEXT NOT NULL DEFAULT '',
    line_order INTEGER NOT NULL DEFAULT 0,
    UNIQUE (rfq_id, line_order)
);

CREATE TABLE rfq_suppliers (
    id BIGSERIAL PRIMARY KEY,
    rfq_id BIGINT NOT NULL REFERENCES rfqs(id) ON DELETE CASCADE,
    supplier_id BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    invited_at TIMESTAMPTZ,
    issued_message_ref TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'INVITED' CHECK (status IN ('INVITED','SENT','DECLINED','RESPONDED')),
    UNIQUE (rfq_id, supplier_id)
);

CREATE TABLE rfq_bids (
    id BIGSERIAL PRIMARY KEY,
    rfq_id BIGINT NOT NULL REFERENCES rfqs(id) ON DELETE CASCADE,
    supplier_id BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','SUBMITTED','WITHDRAWN','DISQUALIFIED')),
    currency TEXT NOT NULL,
    fx_rate NUMERIC(20,10) NOT NULL CHECK (fx_rate > 0),
    fx_rate_date DATE NOT NULL,
    payment_terms TEXT NOT NULL DEFAULT '',
    source_reference TEXT NOT NULL DEFAULT '',
    valid_until DATE,
    submitted_at TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (rfq_id, supplier_id)
);
CREATE INDEX idx_rfq_bids_rfq_status ON rfq_bids(rfq_id, status);

CREATE TABLE rfq_bid_lines (
    id BIGSERIAL PRIMARY KEY,
    bid_id BIGINT NOT NULL REFERENCES rfq_bids(id) ON DELETE CASCADE,
    rfq_line_id BIGINT NOT NULL REFERENCES rfq_lines(id) ON DELETE RESTRICT,
    quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
    unit_price NUMERIC(20,4) NOT NULL CHECK (unit_price >= 0),
    unit_price_base NUMERIC(20,4) NOT NULL CHECK (unit_price_base >= 0),
    tax_amount NUMERIC(20,4) NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    freight_amount NUMERIC(20,4) NOT NULL DEFAULT 0 CHECK (freight_amount >= 0),
    tax_amount_base NUMERIC(20,4) NOT NULL DEFAULT 0 CHECK (tax_amount_base >= 0),
    freight_amount_base NUMERIC(20,4) NOT NULL DEFAULT 0 CHECK (freight_amount_base >= 0),
    minimum_order_quantity NUMERIC(18,4) NOT NULL DEFAULT 0 CHECK (minimum_order_quantity >= 0),
    lead_time_days INTEGER NOT NULL DEFAULT 0 CHECK (lead_time_days >= 0),
    commercial_score INTEGER NOT NULL DEFAULT 0 CHECK (commercial_score BETWEEN 0 AND 100),
    supplier_rating_score INTEGER NOT NULL DEFAULT 0 CHECK (supplier_rating_score BETWEEN 0 AND 100),
    note TEXT NOT NULL DEFAULT '',
    UNIQUE (bid_id, rfq_line_id)
);

CREATE TABLE rfq_comparison_snapshots (
    id BIGSERIAL PRIMARY KEY,
    rfq_id BIGINT NOT NULL REFERENCES rfqs(id) ON DELETE CASCADE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    rfq_version INTEGER NOT NULL,
    comparison JSONB NOT NULL,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_rfq_comparison_snapshots_rfq ON rfq_comparison_snapshots(rfq_id, created_at DESC);

CREATE TABLE rfq_awards (
    id BIGSERIAL PRIMARY KEY,
    rfq_id BIGINT NOT NULL UNIQUE REFERENCES rfqs(id) ON DELETE RESTRICT,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','APPROVAL','APPROVED','REJECTED')),
    expected_warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    approved_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE rfq_award_lines (
    id BIGSERIAL PRIMARY KEY,
    award_id BIGINT NOT NULL REFERENCES rfq_awards(id) ON DELETE CASCADE,
    rfq_line_id BIGINT NOT NULL REFERENCES rfq_lines(id) ON DELETE RESTRICT,
    bid_line_id BIGINT NOT NULL REFERENCES rfq_bid_lines(id) ON DELETE RESTRICT,
    supplier_id BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
    unit_price NUMERIC(20,4) NOT NULL CHECK (unit_price >= 0),
    tax_amount NUMERIC(20,4) NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    freight_amount NUMERIC(20,4) NOT NULL DEFAULT 0 CHECK (freight_amount >= 0),
    po_id BIGINT REFERENCES pos(id) ON DELETE SET NULL,
    po_line_id BIGINT REFERENCES po_lines(id) ON DELETE SET NULL,
    UNIQUE (award_id, rfq_line_id, bid_line_id)
);
CREATE INDEX idx_rfq_award_lines_supplier ON rfq_award_lines(award_id, supplier_id);

ALTER TABLE pos ADD COLUMN rfq_award_id BIGINT REFERENCES rfq_awards(id) ON DELETE SET NULL;
ALTER TABLE po_lines ADD COLUMN rfq_award_line_id BIGINT REFERENCES rfq_award_lines(id) ON DELETE SET NULL;
CREATE INDEX idx_pos_rfq_award ON pos(rfq_award_id);
CREATE INDEX idx_po_lines_rfq_award_line ON po_lines(rfq_award_line_id);

INSERT INTO permissions (name, description) VALUES
    ('procurement.rfq.view', 'View RFQs, bids, comparisons, and awards'),
    ('procurement.rfq.manage', 'Create, issue, close, and manage RFQs and bids'),
    ('procurement.rfq.award', 'Create and submit RFQ awards'),
    ('procurement.contract.view', 'View supplier contracts'),
    ('procurement.contract.manage', 'Manage supplier contracts and overrides'),
    ('procurement.supplier_rating.view', 'View supplier ratings'),
    ('procurement.supplier_rating.manage', 'Publish supplier ratings'),
    ('logistics.carrier.view', 'View carriers and services'),
    ('logistics.carrier.manage', 'Manage carriers and services'),
    ('logistics.fleet.view', 'View fleet and drivers'),
    ('logistics.fleet.manage', 'Manage fleet and drivers'),
    ('logistics.plan.view', 'View distribution plans'),
    ('logistics.plan.manage', 'Manage distribution plans'),
    ('logistics.dispatch.manage', 'Dispatch and receive logistics movements'),
    ('logistics.freight.view', 'View freight estimates and bills'),
    ('logistics.freight.manage', 'Manage freight estimates, bills, and variances')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name IN (
    'procurement.rfq.view', 'procurement.rfq.manage', 'procurement.rfq.award',
    'procurement.contract.view', 'procurement.contract.manage',
    'procurement.supplier_rating.view', 'procurement.supplier_rating.manage',
    'logistics.carrier.view', 'logistics.carrier.manage', 'logistics.fleet.view',
    'logistics.fleet.manage', 'logistics.plan.view', 'logistics.plan.manage',
    'logistics.dispatch.manage', 'logistics.freight.view', 'logistics.freight.manage'
)
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
ON CONFLICT DO NOTHING;
