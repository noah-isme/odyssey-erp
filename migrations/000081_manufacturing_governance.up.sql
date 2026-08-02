-- Manufacturing Governance: Policy versions, decisions, challenges, evidence, audit events

-- Policy Versions: Versioned, effective-dated governance rules
CREATE TABLE policy_versions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    record_type VARCHAR(50) NOT NULL,           -- BOM, WorkOrder, Operation, etc.
    decision_name VARCHAR(50) NOT NULL,         -- Approve, Release, Complete, etc.
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    enforcement_mode VARCHAR(20) NOT NULL,      -- DISABLED, WARN, ENFORCE
    signature_required BOOLEAN NOT NULL,
    approver_roles TEXT[] NOT NULL,             -- Array of required role names
    separation_of_duties BOOLEAN,               -- Creator != releaser requirement
    required_evidence TEXT[] NOT NULL,          -- Types of evidence needed
    retention_period_days INT,
    version INT NOT NULL,
    status VARCHAR(20) NOT NULL,                -- DRAFT, ACTIVE, RETIRED
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL,
    FOREIGN KEY (company_id) REFERENCES companies(id),
    FOREIGN KEY (created_by) REFERENCES users(id),
    UNIQUE(company_id, record_type, decision_name, effective_from, version)
);

CREATE INDEX idx_policy_versions_company_active ON policy_versions(company_id, status)
WHERE status = 'ACTIVE';
CREATE INDEX idx_policy_versions_effective ON policy_versions(company_id, record_type, decision_name, effective_from);

-- Compliance Decisions: Decisions made through governance gate
CREATE TABLE compliance_decisions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    policy_version_id BIGINT NOT NULL,
    record_type VARCHAR(50) NOT NULL,
    record_id BIGINT NOT NULL,
    action VARCHAR(50) NOT NULL,
    actor_id BIGINT NOT NULL,
    reason TEXT,
    decision_id UUID NOT NULL UNIQUE,           -- Immutable identifier
    record_version VARCHAR(50),                 -- Snapshot version
    record_hash VARCHAR(64),                    -- SHA-256 of canonical snapshot
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id),
    FOREIGN KEY (policy_version_id) REFERENCES policy_versions(id),
    FOREIGN KEY (actor_id) REFERENCES users(id),
    INDEX idx_company_record (company_id, record_type, record_id),
    INDEX idx_decision_id (decision_id)
);

-- Signature Challenges: One-time challenges with expiry
CREATE TABLE signature_challenges (
    id BIGSERIAL PRIMARY KEY,
    challenge_id UUID NOT NULL UNIQUE,
    policy_version_id BIGINT NOT NULL,
    record_id BIGINT NOT NULL,
    record_version VARCHAR(50),
    expiry TIMESTAMPTZ NOT NULL,
    reauthentication_required BOOLEAN NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (policy_version_id) REFERENCES policy_versions(id),
    INDEX idx_expiry (expiry),
    INDEX idx_challenge_id (challenge_id)
);

-- Evidence Records: Immutable proof linked to decisions
CREATE TABLE evidence_records (
    id BIGSERIAL PRIMARY KEY,
    decision_id BIGINT NOT NULL,
    evidence_type VARCHAR(50),                  -- Inspection, Hold, Signature, etc.
    content JSONB NOT NULL,                     -- Snapshot/verification data
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (decision_id) REFERENCES compliance_decisions(id),
    INDEX idx_decision (decision_id)
);

-- Audit Events: Immutable causation tracking
CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    correlation_id UUID NOT NULL,              -- Groups related decisions
    causation_id UUID,                         -- Links cause → effect
    decision_id BIGINT,
    entity_type VARCHAR(50),
    entity_id BIGINT,
    action VARCHAR(50),
    actor_id BIGINT,
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id),
    FOREIGN KEY (decision_id) REFERENCES compliance_decisions(id),
    FOREIGN KEY (actor_id) REFERENCES users(id),
    INDEX idx_company_causation (company_id, causation_id),
    INDEX idx_entity (entity_type, entity_id),
    INDEX idx_correlation (correlation_id)
);

-- Quality Inspections: Inspection records with explicit lifecycle
CREATE TABLE quality_inspections (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    work_order_id BIGINT,
    operation_id BIGINT,
    inspection_plan_id BIGINT,
    status VARCHAR(20) NOT NULL,               -- PENDING, PASSED, FAILED, HOLD, RELEASED
    result_snapshot JSONB,
    result_version VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id),
    INDEX idx_company_status (company_id, status),
    INDEX idx_work_order (work_order_id),
    INDEX idx_operation (operation_id)
);

-- Quality Holds: Hold records with explicit lifecycle
CREATE TABLE quality_holds (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    inspection_id BIGINT,
    record_type VARCHAR(50),                   -- Work order, operation, etc.
    record_id BIGINT,
    status VARCHAR(20) NOT NULL,               -- OPEN, RELEASED
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id),
    FOREIGN KEY (inspection_id) REFERENCES quality_inspections(id),
    FOREIGN KEY (created_by) REFERENCES users(id),
    INDEX idx_company_status (company_id, status),
    INDEX idx_record (record_type, record_id)
);

-- Quality NCRs: Nonconformance records with explicit lifecycle
CREATE TABLE quality_ncrs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    number VARCHAR(50) NOT NULL,
    status VARCHAR(30) NOT NULL,               -- OPEN, INVESTIGATING, DISPOSITIONED, CLOSED
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id),
    FOREIGN KEY (created_by) REFERENCES users(id),
    UNIQUE(company_id, number),
    INDEX idx_company_status (company_id, status)
);

-- Quality CAPAs: Corrective/preventive action records with explicit lifecycle
CREATE TABLE quality_capas (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    number VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,               -- OPEN, IN_PROGRESS, VERIFICATION, CLOSED
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id),
    FOREIGN KEY (created_by) REFERENCES users(id),
    UNIQUE(company_id, number),
    INDEX idx_company_status (company_id, status)
);

-- Subcontract Receipts: Received goods tracking with explicit lifecycle
CREATE TABLE subcontract_receipts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    work_order_id BIGINT NOT NULL,
    operation_id BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL,               -- SENT, RECEIVED, INSPECTING, ACCEPTED, CLOSED
    sent_qty NUMERIC(12,4) NOT NULL,
    received_qty NUMERIC(12,4),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id),
    INDEX idx_company_status (company_id, status),
    INDEX idx_work_order (work_order_id),
    INDEX idx_operation (operation_id)
);
