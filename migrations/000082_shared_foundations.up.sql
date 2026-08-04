-- Phase 0: Shared Foundations for Documents, CMMS, and QMS
-- Migration 000082: Shared object storage and module foundations

-- ============================================================================
-- SHARED OBJECT STORAGE TABLES
-- ============================================================================

-- Storage blobs: Binary content metadata (actual content in S3/local storage)
CREATE TABLE storage_blobs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    storage_key VARCHAR(512) NOT NULL,          -- Opaque key from storage backend
    storage_driver VARCHAR(20) NOT NULL,        -- 'local' or 's3'
    bucket VARCHAR(255),                        -- S3 bucket (if S3)
    size_bytes BIGINT NOT NULL,
    checksum_sha256 VARCHAR(64) NOT NULL,
    declared_content_type VARCHAR(100),
    detected_content_type VARCHAR(100),
    encryption_metadata JSONB DEFAULT '{}',
    malware_scan_status VARCHAR(20) NOT NULL DEFAULT 'PENDING',  -- PENDING, CLEAN, INFECTED, ERROR, SKIPPED
    malware_scan_details JSONB,
    metadata JSONB DEFAULT '{}',
    reference_count INT NOT NULL DEFAULT 0,     -- Number of document versions referencing this blob
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE (company_id, storage_key)
);

CREATE INDEX idx_storage_blobs_company ON storage_blobs(company_id);
CREATE INDEX idx_storage_blobs_checksum ON storage_blobs(company_id, checksum_sha256);
CREATE INDEX idx_storage_blobs_malware ON storage_blobs(company_id, malware_scan_status)
WHERE malware_scan_status IN ('PENDING', 'INFECTED', 'ERROR');

-- ============================================================================
-- DOCUMENT MANAGEMENT TABLES
-- ============================================================================

-- Document classifications (company-scoped)
CREATE TABLE document_classifications (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    code VARCHAR(50) NOT NULL,                  -- e.g., 'PUBLIC', 'INTERNAL', 'CONFIDENTIAL', 'RESTRICTED'
    name VARCHAR(100) NOT NULL,
    description TEXT,
    default_retention_policy_id BIGINT,         -- FK to retention_policies (added later)
    requires_approval BOOLEAN NOT NULL DEFAULT FALSE,
    requires_signature BOOLEAN NOT NULL DEFAULT FALSE,
    allowed_extensions TEXT[],                  -- e.g., '{pdf,docx,xlsx}'
    max_size_bytes BIGINT,
    sort_order INT NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE (company_id, code)
);

CREATE INDEX idx_document_classifications_company ON document_classifications(company_id, active);

-- Document categories (company-scoped, hierarchical)
CREATE TABLE document_categories (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    parent_id BIGINT REFERENCES document_categories(id),
    code VARCHAR(50) NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    default_classification_id BIGINT REFERENCES document_classifications(id),
    numbering_rule_id BIGINT,                   -- FK to numbering_rules (added later)
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE (company_id, parent_id, code)
);

CREATE INDEX idx_document_categories_company ON document_categories(company_id, parent_id);

-- Document numbering rules
CREATE TABLE document_numbering_rules (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    code VARCHAR(50) NOT NULL,
    name VARCHAR(100) NOT NULL,
    prefix VARCHAR(20),
    suffix VARCHAR(20),
    pattern VARCHAR(100) NOT NULL,              -- e.g., '{PREFIX}-{YYYY}-{SEQ:04d}'
    sequence_start INT NOT NULL DEFAULT 1,
    sequence_current INT NOT NULL DEFAULT 1,
    scope VARCHAR(20) NOT NULL DEFAULT 'COMPANY', -- COMPANY, CATEGORY, CLASSIFICATION
    scope_id BIGINT,                            -- Category or classification ID when scope != COMPANY
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE (company_id, code)
);

CREATE INDEX idx_document_numbering_rules_company ON document_numbering_rules(company_id, active);

-- Documents: Stable identity, multiple immutable versions
CREATE TABLE documents (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    category_id BIGINT REFERENCES document_categories(id),
    classification_id BIGINT NOT NULL REFERENCES document_classifications(id),
    numbering_rule_id BIGINT REFERENCES document_numbering_rules(id),
    document_number VARCHAR(100) NOT NULL,      -- Generated from numbering rule
    title VARCHAR(500) NOT NULL,
    description TEXT,
    owner_id BIGINT NOT NULL REFERENCES users(id),
    status VARCHAR(30) NOT NULL DEFAULT 'DRAFT', -- DRAFT, ACTIVE, SUPERSEDED, OBSOLETE, ARCHIVED, DISPOSED
    effective_from TIMESTAMPTZ,
    effective_to TIMESTAMPTZ,
    current_version_id BIGINT,                  -- FK to document_versions (added after versions table)
    migration_source VARCHAR(50),               -- e.g., 'portal_documents', 'boardpack'
    migration_source_id VARCHAR(100),           -- Original record ID
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE (company_id, document_number),
    UNIQUE (company_id, migration_source, migration_source_id)
);

CREATE INDEX idx_documents_company ON documents(company_id, status);
CREATE INDEX idx_documents_category ON documents(company_id, category_id);
CREATE INDEX idx_documents_classification ON documents(company_id, classification_id);
CREATE INDEX idx_documents_owner ON documents(company_id, owner_id);
CREATE INDEX idx_documents_migration ON documents(company_id, migration_source, migration_source_id);

-- Document versions: Immutable, one per document at a time
CREATE TABLE document_versions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    document_id BIGINT NOT NULL REFERENCES documents(id),
    version_number INT NOT NULL,                -- 1, 2, 3...
    version_label VARCHAR(50),                  -- e.g., '1.0', '2.0-RC1'
    blob_id BIGINT NOT NULL REFERENCES storage_blobs(id),
    status VARCHAR(30) NOT NULL DEFAULT 'DRAFT', -- DRAFT, IN_REVIEW, APPROVED, EFFECTIVE, SUPERSEDED, OBSOLETE, ARCHIVED, REJECTED, QUARANTINED, DISPOSED
    classification_id BIGINT NOT NULL REFERENCES document_classifications(id),
    change_summary TEXT,
    review_snapshot JSONB,                      -- Snapshot of review state at submission
    approved_by BIGINT REFERENCES users(id),
    approved_at TIMESTAMPTZ,
    effective_at TIMESTAMPTZ,
    superseded_at TIMESTAMPTZ,
    superseded_by_version_id BIGINT REFERENCES document_versions(id),
    disposed_at TIMESTAMPTZ,
    disposed_by BIGINT REFERENCES users(id),
    disposition_request_id BIGINT,              -- FK to disposition_requests (added later)
    migration_source VARCHAR(50),
    migration_source_id VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE (company_id, document_id, version_number)
);

CREATE INDEX idx_document_versions_document ON document_versions(company_id, document_id, version_number);
CREATE INDEX idx_document_versions_status ON document_versions(company_id, status);
CREATE INDEX idx_document_versions_blob ON document_versions(blob_id);
CREATE INDEX idx_document_versions_effective ON document_versions(company_id, effective_at)
WHERE status = 'EFFECTIVE';

-- Add FK from documents to current_version
ALTER TABLE documents ADD CONSTRAINT fk_documents_current_version
    FOREIGN KEY (current_version_id) REFERENCES document_versions(id);

-- Document ACLs: Role/user grants and explicit denies
CREATE TABLE document_acls (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    document_id BIGINT REFERENCES documents(id),           -- NULL = classification default
    classification_id BIGINT REFERENCES document_classifications(id), -- For defaults
    principal_type VARCHAR(20) NOT NULL,                   -- 'ROLE' or 'USER'
    principal_id BIGINT NOT NULL,                          -- role_id or user_id
    permission VARCHAR(50) NOT NULL,                       -- view, upload, version, review, approve, sign, share, retention.manage, hold.manage, dispose, admin
    effect VARCHAR(10) NOT NULL DEFAULT 'ALLOW',           -- ALLOW or DENY
    granted_by BIGINT NOT NULL REFERENCES users(id),
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    CHECK (document_id IS NOT NULL OR classification_id IS NOT NULL),
    CHECK (effect IN ('ALLOW', 'DENY')),
    UNIQUE (company_id, document_id, classification_id, principal_type, principal_id, permission)
);

CREATE INDEX idx_document_acls_document ON document_acls(company_id, document_id);
CREATE INDEX idx_document_acls_classification ON document_acls(company_id, classification_id);
CREATE INDEX idx_document_acls_principal ON document_acls(company_id, principal_type, principal_id);

-- Document links: Allow-listed links to domain records
CREATE TABLE document_links (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id),
    target_module VARCHAR(50) NOT NULL,                    -- 'assets', 'work_orders', 'suppliers', 'purchase_orders', 'inspections', 'ncrs', 'capas', etc.
    target_id BIGINT NOT NULL,
    target_company_id BIGINT NOT NULL,                     -- Must match company_id
    link_type VARCHAR(50) NOT NULL DEFAULT 'REFERENCE',    -- REFERENCE, EVIDENCE, ATTACHMENT, SOURCE
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE (company_id, document_version_id, target_module, target_id)
);

CREATE INDEX idx_document_links_target ON document_links(company_id, target_module, target_id);
CREATE INDEX idx_document_links_version ON document_links(document_version_id);

-- Document review and approval steps
CREATE TABLE document_review_steps (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id),
    step_order INT NOT NULL,
    name VARCHAR(100) NOT NULL,
    reviewer_role_id BIGINT REFERENCES roles(id),
    reviewer_user_id BIGINT REFERENCES users(id),
    required_approvals INT NOT NULL DEFAULT 1,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',         -- PENDING, IN_PROGRESS, APPROVED, REJECTED, SKIPPED
    due_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, document_version_id, step_order)
);

CREATE INDEX idx_document_review_steps_version ON document_review_steps(document_version_id, step_order);

-- Document review decisions
CREATE TABLE document_review_decisions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    review_step_id BIGINT NOT NULL REFERENCES document_review_steps(id),
    reviewer_id BIGINT NOT NULL REFERENCES users(id),
    decision VARCHAR(20) NOT NULL,                         -- APPROVE, REJECT
    comment TEXT,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (review_step_id, reviewer_id)
);

-- Signature challenges (one-time reauthentication)
CREATE TABLE document_signature_challenges (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    challenge_id UUID NOT NULL UNIQUE,
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id),
    signer_id BIGINT NOT NULL REFERENCES users(id),
    meaning VARCHAR(200) NOT NULL,                         -- e.g., 'Approved as effective version'
    policy_version INT NOT NULL,
    expiry TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_document_signature_challenges_expiry ON document_signature_challenges(expiry);
CREATE INDEX idx_document_signature_challenges_version ON document_signature_challenges(document_version_id);

-- Electronic signatures
CREATE TABLE document_signatures (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id),
    challenge_id UUID NOT NULL REFERENCES document_signature_challenges(challenge_id),
    signer_id BIGINT NOT NULL REFERENCES users(id),
    record_version VARCHAR(50) NOT NULL,                   -- Version at time of signing
    record_hash VARCHAR(64) NOT NULL,                      -- SHA-256 of canonical snapshot
    meaning VARCHAR(200) NOT NULL,
    policy_version INT NOT NULL,
    auth_method VARCHAR(50) NOT NULL,                      -- 'PASSWORD', '2FA', 'SSO'
    signed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_document_signatures_version ON document_signatures(document_version_id);
CREATE INDEX idx_document_signatures_signer ON document_signatures(company_id, signer_id);

-- Retention policies
CREATE TABLE retention_policies (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    code VARCHAR(50) NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    trigger_event VARCHAR(50) NOT NULL,                    -- APPROVAL, SUPERSESSION, WORK_ORDER_CLOSURE, QUALITY_CASE_CLOSURE, CONTRACT_EXPIRY, CREATION
    retention_period_days INT NOT NULL,
    classification_ids BIGINT[],                           -- Applies to these classifications
    category_ids BIGINT[],                                 -- Applies to these categories
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE (company_id, code)
);

CREATE INDEX idx_retention_policies_company ON retention_policies(company_id, active);

-- Document retention calculations
CREATE TABLE document_retention (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id),
    policy_id BIGINT NOT NULL REFERENCES retention_policies(id),
    trigger_date TIMESTAMPTZ NOT NULL,
    expiry_date TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',          -- ACTIVE, EXPIRED, HELD, DISPOSED
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (document_version_id, policy_id)
);

CREATE INDEX idx_document_retention_company ON document_retention(company_id, status);
CREATE INDEX idx_document_retention_expiry ON document_retention(expiry_date)
WHERE status = 'ACTIVE';

-- Legal holds
CREATE TABLE legal_holds (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    name VARCHAR(200) NOT NULL,
    description TEXT,
    issued_by BIGINT NOT NULL REFERENCES users(id),
    issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ,
    released_by BIGINT REFERENCES users(id),
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',          -- ACTIVE, RELEASED
    scope_type VARCHAR(20) NOT NULL,                       -- DOCUMENT_VERSION, DOCUMENT, CLASSIFICATION, CATEGORY, COMPANY
    scope_id BIGINT NOT NULL,                              -- ID based on scope_type
    UNIQUE (company_id, name)
);

CREATE INDEX idx_legal_holds_scope ON legal_holds(company_id, scope_type, scope_id);
CREATE INDEX idx_legal_holds_status ON legal_holds(company_id, status);

-- Legal hold references (which documents are held)
CREATE TABLE legal_hold_references (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    legal_hold_id BIGINT NOT NULL REFERENCES legal_holds(id),
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id),
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (legal_hold_id, document_version_id)
);

CREATE INDEX idx_legal_hold_refs_document ON legal_hold_references(document_version_id);

-- Disposition requests
CREATE TABLE disposition_requests (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id),
    requested_by BIGINT NOT NULL REFERENCES users(id),
    reason TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',         -- PENDING, APPROVED, REJECTED, EXECUTED, FAILED
    approved_by BIGINT REFERENCES users(id),
    approved_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ,
    executed_by BIGINT REFERENCES users(id),
    execution_evidence JSONB,                              -- Proof of deletion, etc.
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_disposition_requests_pending_approved
    ON disposition_requests(document_version_id, status)
    WHERE status IN ('PENDING', 'APPROVED');

CREATE INDEX idx_disposition_requests_company ON disposition_requests(company_id, status);

-- Document access events (audit trail for downloads, previews, shares)
CREATE TABLE document_access_events (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id),
    actor_id BIGINT NOT NULL REFERENCES users(id),
    action VARCHAR(30) NOT NULL,                           -- PREVIEW, DOWNLOAD, EXPORT, SHARE_CREATE, SHARE_ACCESS
    ip_address INET,
    user_agent TEXT,
    share_token VARCHAR(100),                              -- If SHARE_CREATE or SHARE_ACCESS
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_document_access_events_version ON document_access_events(document_version_id);
CREATE INDEX idx_document_access_events_actor ON document_access_events(company_id, actor_id, created_at);
CREATE INDEX idx_document_access_events_share ON document_access_events(share_token)
WHERE share_token IS NOT NULL;

-- ============================================================================
-- COMPANY FEATURE FLAGS
-- ============================================================================

CREATE TABLE company_module_features (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    module VARCHAR(50) NOT NULL,                           -- 'documents', 'cmms', 'qms'
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    migration_state VARCHAR(30) NOT NULL DEFAULT 'NOT_STARTED', -- NOT_STARTED, IN_PROGRESS, DUAL_READ, CUTOVER_COMPLETE, ROLLED_BACK
    enforcement_mode VARCHAR(20) NOT NULL DEFAULT 'DISABLED',   -- DISABLED, WARN, ENFORCE
    config JSONB DEFAULT '{}',
    enabled_at TIMESTAMPTZ,
    enabled_by BIGINT REFERENCES users(id),
    UNIQUE (company_id, module)
);

CREATE INDEX idx_company_module_features_company ON company_module_features(company_id);

-- ============================================================================
-- OUTBOX FOR CROSS-MODULE EVENTS
-- ============================================================================

CREATE TABLE outbox_events (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    correlation_id UUID NOT NULL,                          -- Groups related events
    causation_id UUID,                                     -- Links cause to effect
    event_type VARCHAR(100) NOT NULL,                      -- e.g., 'documents.version.effective', 'cmms.work_order.completed'
    aggregate_type VARCHAR(50) NOT NULL,                   -- 'document', 'work_order', 'inspection', etc.
    aggregate_id BIGINT NOT NULL,
    aggregate_version INT,                                 -- Optimistic version
    payload JSONB NOT NULL,
    idempotency_key VARCHAR(100),                          -- For exactly-once processing
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    publish_attempts INT NOT NULL DEFAULT 0,
    last_error TEXT
);

CREATE INDEX idx_outbox_events_unpublished ON outbox_events(company_id, published_at)
WHERE published_at IS NULL;
CREATE INDEX idx_outbox_events_correlation ON outbox_events(correlation_id);
CREATE INDEX idx_outbox_events_causation ON outbox_events(causation_id);
CREATE INDEX idx_outbox_events_idempotency ON outbox_events(company_id, idempotency_key)
WHERE idempotency_key IS NOT NULL;

-- ============================================================================
-- SEED DATA: Default document classifications
-- ============================================================================

INSERT INTO document_classifications (company_id, code, name, description, requires_approval, requires_signature, allowed_extensions, max_size_bytes, sort_order, active, created_by)
SELECT c.id, 'PUBLIC', 'Public', 'Publicly shareable documents', FALSE, FALSE, '{pdf,txt,docx,xlsx,png,jpg}', 52428800, 10, TRUE, 1
FROM companies c WHERE NOT EXISTS (SELECT 1 FROM document_classifications WHERE company_id = c.id AND code = 'PUBLIC');

INSERT INTO document_classifications (company_id, code, name, description, requires_approval, requires_signature, allowed_extensions, max_size_bytes, sort_order, active, created_by)
SELECT c.id, 'INTERNAL', 'Internal', 'Internal use only', FALSE, FALSE, '{pdf,txt,docx,xlsx,png,jpg,dwg}', 104857600, 20, TRUE, 1
FROM companies c WHERE NOT EXISTS (SELECT 1 FROM document_classifications WHERE company_id = c.id AND code = 'INTERNAL');

INSERT INTO document_classifications (company_id, code, name, description, requires_approval, requires_signature, allowed_extensions, max_size_bytes, sort_order, active, created_by)
SELECT c.id, 'CONFIDENTIAL', 'Confidential', 'Restricted to authorized personnel', TRUE, TRUE, '{pdf,docx,xlsx}', 52428800, 30, TRUE, 1
FROM companies c WHERE NOT EXISTS (SELECT 1 FROM document_classifications WHERE company_id = c.id AND code = 'CONFIDENTIAL');

INSERT INTO document_classifications (company_id, code, name, description, requires_approval, requires_signature, allowed_extensions, max_size_bytes, sort_order, active, created_by)
SELECT c.id, 'RESTRICTED', 'Restricted', 'Highly sensitive, limited distribution', TRUE, TRUE, '{pdf}', 10485760, 40, TRUE, 1
FROM companies c WHERE NOT EXISTS (SELECT 1 FROM document_classifications WHERE company_id = c.id AND code = 'RESTRICTED');

-- Default numbering rule
INSERT INTO document_numbering_rules (company_id, code, name, prefix, pattern, sequence_start, sequence_current, scope, active, created_by)
SELECT c.id, 'DEFAULT', 'Default Document Numbering', 'DOC', 'DOC-{YYYY}-{SEQ:06d}', 1, 1, 'COMPANY', TRUE, 1
FROM companies c WHERE NOT EXISTS (SELECT 1 FROM document_numbering_rules WHERE company_id = c.id AND code = 'DEFAULT');

-- Default category
INSERT INTO document_categories (company_id, code, name, description, active, created_by)
SELECT c.id, 'GENERAL', 'General Documents', 'General purpose documents', TRUE, 1
FROM companies c WHERE NOT EXISTS (SELECT 1 FROM document_categories WHERE company_id = c.id AND code = 'GENERAL');