-- =============================================================================
-- QMS: Inspections and Complaints
-- =============================================================================

CREATE TABLE qms_inspections (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    name VARCHAR(200) NOT NULL,
    description TEXT,
    reference_module VARCHAR(50), -- E.g. 'MRP', 'INVENTORY', 'PROCUREMENT'
    reference_id BIGINT,
    status VARCHAR(30) NOT NULL DEFAULT 'PLANNED', -- PLANNED, IN_PROGRESS, PASSED, FAILED, HOLD, CLOSED
    inspector_id BIGINT REFERENCES users(id),
    scheduled_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by BIGINT NOT NULL REFERENCES users(id)
);

CREATE INDEX idx_qms_inspections_company_status ON qms_inspections(company_id, status);

CREATE TABLE qms_inspection_results (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    inspection_id BIGINT NOT NULL REFERENCES qms_inspections(id) ON DELETE CASCADE,
    characteristic_name VARCHAR(200) NOT NULL,
    expected_value VARCHAR(200),
    actual_value VARCHAR(200),
    is_conforming BOOLEAN NOT NULL,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id)
);

CREATE INDEX idx_qms_insp_results_insp ON qms_inspection_results(inspection_id);

CREATE TABLE customer_complaints (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    complaint_number VARCHAR(50) NOT NULL,
    customer_id BIGINT NOT NULL, -- references a hypothetical customers table or is an opaque ID
    title VARCHAR(200) NOT NULL,
    description TEXT NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'RECEIVED', -- RECEIVED, TRIAGED, INVESTIGATING, RESPONDED, CLOSED
    severity VARCHAR(20) NOT NULL DEFAULT 'LOW',
    assigned_to BIGINT REFERENCES users(id),
    response_evidence TEXT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE (company_id, complaint_number)
);

CREATE INDEX idx_customer_complaints_company_status ON customer_complaints(company_id, status);

