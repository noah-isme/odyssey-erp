-- Q2: Matching Engine and Policy Versions Schema

CREATE TABLE IF NOT EXISTS ap_matching_policies (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    company_id BIGINT, -- If null, applies globally
    supplier_id BIGINT, -- If null, applies to all suppliers
    category_id BIGINT, -- If null, applies to all categories
    
    qty_tolerance_pct NUMERIC(10,4) NOT NULL DEFAULT 0,
    price_tolerance_pct NUMERIC(10,4) NOT NULL DEFAULT 0,
    tax_tolerance_pct NUMERIC(10,4) NOT NULL DEFAULT 0,
    freight_tolerance_pct NUMERIC(10,4) NOT NULL DEFAULT 0,
    total_tolerance_amt NUMERIC(15,4) NOT NULL DEFAULT 0,
    
    effective_from DATE NOT NULL DEFAULT CURRENT_DATE,
    effective_to DATE,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT ap_matching_policies_dates CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE TABLE IF NOT EXISTS ap_matching_runs (
    id BIGSERIAL PRIMARY KEY,
    ap_invoice_id BIGINT NOT NULL REFERENCES ap_invoices(id),
    policy_id BIGINT REFERENCES ap_matching_policies(id),
    status TEXT NOT NULL CHECK (status IN ('MATCHED', 'WITHIN_TOLERANCE', 'EXCEPTION', 'DUPLICATE_REVIEW')),
    
    -- Snapshots of document totals at match time
    invoice_total NUMERIC(15,4) NOT NULL,
    po_total NUMERIC(15,4),
    grn_total NUMERIC(15,4),
    
    reasons TEXT[],
    action_recommended TEXT NOT NULL,
    
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    run_by BIGINT
);

CREATE TABLE IF NOT EXISTS ap_matching_run_lines (
    id BIGSERIAL PRIMARY KEY,
    ap_matching_run_id BIGINT NOT NULL REFERENCES ap_matching_runs(id) ON DELETE CASCADE,
    ap_invoice_line_id BIGINT NOT NULL REFERENCES ap_invoice_lines(id),
    
    -- Context
    po_line_id BIGINT REFERENCES po_lines(id),
    grn_line_id BIGINT REFERENCES grn_lines(id),
    
    -- Fact snapshots
    invoice_qty NUMERIC(15,4) NOT NULL,
    invoice_price NUMERIC(15,4) NOT NULL,
    po_qty NUMERIC(15,4),
    po_price NUMERIC(15,4),
    grn_qty NUMERIC(15,4),
    
    status TEXT NOT NULL CHECK (status IN ('MATCHED', 'WITHIN_TOLERANCE', 'EXCEPTION')),
    reasons TEXT[]
);
