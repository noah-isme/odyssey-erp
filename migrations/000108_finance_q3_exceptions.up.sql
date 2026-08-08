-- Q3: Exception Workbench Schema

CREATE TABLE IF NOT EXISTS ap_exceptions (
    id BIGSERIAL PRIMARY KEY,
    ap_invoice_id BIGINT NOT NULL REFERENCES ap_invoices(id),
    ap_matching_run_id BIGINT REFERENCES ap_matching_runs(id),
    
    exception_type TEXT NOT NULL CHECK (exception_type IN ('MISMATCH', 'MISSING_MAPPING', 'SUPPLIER_HOLD', 'CLOSED_PERIOD', 'OVERDUE_RECEIPT', 'DUPLICATE')),
    severity TEXT NOT NULL CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    status TEXT NOT NULL CHECK (status IN ('OPEN', 'IN_REVIEW', 'RESOLVED', 'REJECTED')),
    
    owner_id BIGINT, -- The user responsible for resolving the exception
    sla_due_at TIMESTAMPTZ,
    
    reason TEXT NOT NULL,
    evidence TEXT,
    comments TEXT[],
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    resolved_by BIGINT
);

CREATE INDEX idx_ap_exceptions_status ON ap_exceptions(status);
CREATE INDEX idx_ap_exceptions_owner ON ap_exceptions(owner_id) WHERE owner_id IS NOT NULL;
CREATE INDEX idx_ap_exceptions_invoice ON ap_exceptions(ap_invoice_id);
