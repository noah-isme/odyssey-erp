-- Migration 000083: CMMS and QMS core tables
-- Phase 3 & 5: Asset registry, work orders, NCR, CAPA, Audits

-- ============================================================================
-- CMMS TABLES
-- ============================================================================

-- Locations (physical locations for assets and work orders)
CREATE TABLE locations (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    code VARCHAR(50) NOT NULL,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    parent_id BIGINT REFERENCES locations(id),
    address TEXT,
    gps_lat DOUBLE PRECISION,
    gps_lng DOUBLE PRECISION,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, code)
);

CREATE INDEX idx_locations_company ON locations(company_id, active);
CREATE INDEX idx_locations_parent ON locations(company_id, parent_id);

-- Assets (maintainable equipment, facilities, vehicles, tools)
CREATE TABLE assets (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    code VARCHAR(50) NOT NULL,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    asset_type VARCHAR(30) NOT NULL DEFAULT 'EQUIPMENT', -- EQUIPMENT, FACILITY, VEHICLE, TOOL, INFRASTRUCTURE
    parent_id BIGINT REFERENCES assets(id),
    location_id BIGINT REFERENCES locations(id),
    fixed_asset_id BIGINT,                               -- Optional link to financial fixed asset
    manufacturer VARCHAR(200),
    model VARCHAR(200),
    serial_number VARCHAR(100),
    tag_number VARCHAR(100),
    install_date DATE,
    warranty_expiry DATE,
    status VARCHAR(30) NOT NULL DEFAULT 'ACTIVE',        -- ACTIVE, INACTIVE, DECOMMISSIONED, SCRAPPED
    criticality VARCHAR(5) NOT NULL DEFAULT 'B',         -- A, B, C, D (A = most critical)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, code)
);

CREATE INDEX idx_assets_company ON assets(company_id, status);
CREATE INDEX idx_assets_location ON assets(company_id, location_id);
CREATE INDEX idx_assets_parent ON assets(company_id, parent_id);
CREATE INDEX idx_assets_criticality ON assets(company_id, criticality);

-- Work orders
CREATE TABLE work_orders (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    number VARCHAR(50) NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    asset_id BIGINT REFERENCES assets(id),
    location_id BIGINT REFERENCES locations(id),
    priority VARCHAR(20) NOT NULL DEFAULT 'MEDIUM',      -- LOW, MEDIUM, HIGH, CRITICAL
    status VARCHAR(30) NOT NULL DEFAULT 'DRAFT',         -- DRAFT, PLANNED, SCHEDULED, IN_PROGRESS, ON_HOLD, COMPLETED, CANCELLED, CLOSED
    category VARCHAR(30) NOT NULL DEFAULT 'CORRECTIVE',  -- PREVENTIVE, CORRECTIVE, PREDICTIVE, INSPECTION, EMERGENCY, CALIBRATION
    requester_id BIGINT NOT NULL REFERENCES users(id),
    assignee_id BIGINT REFERENCES users(id),
    planned_start TIMESTAMPTZ,
    planned_end TIMESTAMPTZ,
    actual_start TIMESTAMPTZ,
    actual_end TIMESTAMPTZ,
    estimated_hours DOUBLE PRECISION NOT NULL DEFAULT 0,
    actual_hours DOUBLE PRECISION NOT NULL DEFAULT 0,
    pm_schedule_id BIGINT,                               -- FK to pm_schedules (added after)
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, number)
);

CREATE INDEX idx_work_orders_company ON work_orders(company_id, status);
CREATE INDEX idx_work_orders_asset ON work_orders(company_id, asset_id);
CREATE INDEX idx_work_orders_assignee ON work_orders(company_id, assignee_id);
CREATE INDEX idx_work_orders_planned ON work_orders(company_id, planned_start);

-- Work order tasks (checklist items)
CREATE TABLE work_order_tasks (
    id BIGSERIAL PRIMARY KEY,
    work_order_id BIGINT NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    sequence INT NOT NULL DEFAULT 1,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
    assignee_id BIGINT REFERENCES users(id),
    estimated_hours DOUBLE PRECISION NOT NULL DEFAULT 0,
    actual_hours DOUBLE PRECISION NOT NULL DEFAULT 0,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_work_order_tasks_wo ON work_order_tasks(work_order_id, sequence);

-- Task templates for reusable procedures
CREATE TABLE task_templates (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    name VARCHAR(200) NOT NULL,
    description TEXT,
    category VARCHAR(50),
    estimated_hours DOUBLE PRECISION NOT NULL DEFAULT 0,
    instructions TEXT,
    safety_notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_templates_company ON task_templates(company_id);

-- Task template steps
CREATE TABLE task_template_steps (
    id BIGSERIAL PRIMARY KEY,
    task_template_id BIGINT NOT NULL REFERENCES task_templates(id) ON DELETE CASCADE,
    sequence INT NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    estimated_hours DOUBLE PRECISION NOT NULL DEFAULT 0,
    instructions TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_template_steps_template ON task_template_steps(task_template_id, sequence);

-- Preventive maintenance schedules
CREATE TABLE pm_schedules (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    asset_id BIGINT NOT NULL REFERENCES assets(id),
    name VARCHAR(200) NOT NULL,
    description TEXT,
    frequency_type VARCHAR(30) NOT NULL,                 -- DAILY, WEEKLY, MONTHLY, QUARTERLY, SEMI_ANNUAL, ANNUAL, METER_BASED
    frequency_value INT NOT NULL DEFAULT 1,
    meter_reading_type VARCHAR(30),                      -- HOURS, CYCLES, DISTANCE (when METER_BASED)
    task_template_id BIGINT REFERENCES task_templates(id),
    next_due_date DATE,
    next_due_meter DOUBLE PRECISION,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',        -- DRAFT, ACTIVE, SUSPENDED, RETIRED
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pm_schedules_company ON pm_schedules(company_id, active);
CREATE INDEX idx_pm_schedules_asset ON pm_schedules(asset_id);
CREATE INDEX idx_pm_schedules_due ON pm_schedules(company_id, next_due_date) WHERE active = TRUE;

-- Add FK from work_orders to pm_schedules
ALTER TABLE work_orders ADD CONSTRAINT fk_work_orders_pm_schedule
    FOREIGN KEY (pm_schedule_id) REFERENCES pm_schedules(id);

-- Meter readings for assets
CREATE TABLE meter_readings (
    id BIGSERIAL PRIMARY KEY,
    asset_id BIGINT NOT NULL REFERENCES assets(id),
    reading_type VARCHAR(30) NOT NULL,                   -- HOURS, CYCLES, DISTANCE, TEMPERATURE, PRESSURE, VOLTAGE
    value DOUBLE PRECISION NOT NULL,
    reading_date DATE NOT NULL,
    entered_by BIGINT NOT NULL REFERENCES users(id),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_meter_readings_asset ON meter_readings(asset_id, reading_type, reading_date DESC);

-- Spare parts catalog
CREATE TABLE spare_parts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    code VARCHAR(50) NOT NULL,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    category VARCHAR(100),
    unit_of_measure VARCHAR(30) NOT NULL DEFAULT 'EA',
    min_quantity DOUBLE PRECISION NOT NULL DEFAULT 0,
    max_quantity DOUBLE PRECISION NOT NULL DEFAULT 0,
    reorder_point DOUBLE PRECISION NOT NULL DEFAULT 0,
    lead_time_days INT NOT NULL DEFAULT 0,
    unit_cost DOUBLE PRECISION NOT NULL DEFAULT 0,
    critical_spare BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, code)
);

CREATE INDEX idx_spare_parts_company ON spare_parts(company_id);

-- Work order spare parts usage
CREATE TABLE work_order_spare_parts (
    id BIGSERIAL PRIMARY KEY,
    work_order_id BIGINT NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    spare_part_id BIGINT NOT NULL REFERENCES spare_parts(id),
    quantity DOUBLE PRECISION NOT NULL,
    unit_cost DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_cost DOUBLE PRECISION NOT NULL DEFAULT 0,
    issued_at TIMESTAMPTZ,
    issued_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_work_order_spare_parts_wo ON work_order_spare_parts(work_order_id);

-- ============================================================================
-- QMS TABLES
-- ============================================================================

-- Non-conformance reports
CREATE TABLE ncrs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    number VARCHAR(50) NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    source_type VARCHAR(30) NOT NULL DEFAULT 'INTERNAL',  -- INTERNAL, SUPPLIER, CUSTOMER, AUDIT, PRODUCTION
    source_id BIGINT,
    source_reference VARCHAR(100),
    category VARCHAR(30) NOT NULL DEFAULT 'MATERIAL',     -- MATERIAL, PROCESS, PRODUCT, DOCUMENTATION, SERVICE
    severity VARCHAR(20) NOT NULL DEFAULT 'MAJOR',        -- MINOR, MAJOR, CRITICAL
    status VARCHAR(30) NOT NULL DEFAULT 'OPEN',           -- OPEN, UNDER_REVIEW, DISPOSITIONED, CLOSED, CANCELLED
    detected_by BIGINT NOT NULL REFERENCES users(id),
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    detected_location TEXT,
    responsible_party_id BIGINT REFERENCES users(id),
    assigned_to BIGINT REFERENCES users(id),
    target_closure_date DATE,
    actual_closure_date DATE,
    root_cause TEXT,
    containment_action TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, number)
);

CREATE INDEX idx_ncrs_company ON ncrs(company_id, status);
CREATE INDEX idx_ncrs_assigned ON ncrs(company_id, assigned_to);
CREATE INDEX idx_ncrs_detected ON ncrs(company_id, detected_at DESC);

-- NCR dispositions
CREATE TABLE ncr_dispositions (
    id BIGSERIAL PRIMARY KEY,
    ncr_id BIGINT NOT NULL REFERENCES ncrs(id),
    disposition_type VARCHAR(30) NOT NULL,               -- REWORK, REPAIR, USE_AS_IS, SCRAP, RETURN_TO_SUPPLIER
    description TEXT,
    approved_by BIGINT NOT NULL REFERENCES users(id),
    approved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ncr_dispositions_ncr ON ncr_dispositions(ncr_id);

-- Corrective and preventive actions
CREATE TABLE capas (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    number VARCHAR(50) NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    source_type VARCHAR(30) NOT NULL DEFAULT 'INTERNAL',  -- NCR, AUDIT, CUSTOMER_COMPLAINT, REGULATORY, INTERNAL
    source_id BIGINT,
    source_reference VARCHAR(100),
    status VARCHAR(30) NOT NULL DEFAULT 'OPEN',           -- OPEN, IN_PROGRESS, VERIFYING, EFFECTIVE, CLOSED
    priority VARCHAR(20) NOT NULL DEFAULT 'MEDIUM',       -- LOW, MEDIUM, HIGH, CRITICAL
    owner_id BIGINT NOT NULL REFERENCES users(id),
    team_members BIGINT[],
    root_cause TEXT,
    root_cause_method VARCHAR(30),                        -- FIVE_WHYS, FISHBONE, FAULT_TREE, PARETO
    corrective_action TEXT,
    preventive_action TEXT,
    verification_method TEXT,
    verification_result TEXT,
    effectiveness_check TEXT,
    target_date DATE,
    completion_date DATE,
    effectiveness_date DATE,
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, number)
);

CREATE INDEX idx_capas_company ON capas(company_id, status);
CREATE INDEX idx_capas_owner ON capas(company_id, owner_id);
CREATE INDEX idx_capas_created ON capas(company_id, created_at DESC);

-- Quality audits
CREATE TABLE audits (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    number VARCHAR(50) NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    audit_type VARCHAR(30) NOT NULL DEFAULT 'INTERNAL',   -- INTERNAL, SUPPLIER, REGULATORY, CERTIFICATION, PROCESS, PRODUCT
    status VARCHAR(30) NOT NULL DEFAULT 'PLANNED',        -- PLANNED, IN_PROGRESS, COMPLETED, REPORTED, CLOSED
    standard VARCHAR(50),                                 -- ISO9001, ISO13485, IATF16949, AS9100, CUSTOM
    scope TEXT,
    lead_auditor_id BIGINT NOT NULL REFERENCES users(id),
    audit_team_ids BIGINT[],
    auditee_id BIGINT REFERENCES users(id),
    planned_start DATE,
    planned_end DATE,
    actual_start DATE,
    actual_end DATE,
    report_number VARCHAR(100),
    report_date DATE,
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, number)
);

CREATE INDEX idx_audits_company ON audits(company_id, status);
CREATE INDEX idx_audits_lead ON audits(company_id, lead_auditor_id);
CREATE INDEX idx_audits_planned ON audits(company_id, planned_start);

-- Audit findings
CREATE TABLE audit_findings (
    id BIGSERIAL PRIMARY KEY,
    audit_id BIGINT NOT NULL REFERENCES audits(id) ON DELETE CASCADE,
    finding_number VARCHAR(50),
    category VARCHAR(30) NOT NULL DEFAULT 'MINOR',        -- MAJOR, MINOR, OBSERVATION, OPPORTUNITY
    clause VARCHAR(100),
    description TEXT NOT NULL,
    evidence TEXT,
    requirement TEXT,
    risk_level VARCHAR(10) NOT NULL DEFAULT 'LOW',        -- HIGH, MEDIUM, LOW
    status VARCHAR(20) NOT NULL DEFAULT 'OPEN',           -- OPEN, IN_PROGRESS, CLOSED
    response TEXT,
    response_due_date DATE,
    response_date DATE,
    assigned_to BIGINT REFERENCES users(id),
    verified_by BIGINT REFERENCES users(id),
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_findings_audit ON audit_findings(audit_id);

-- Supplier quality records
CREATE TABLE supplier_quality (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    supplier_id BIGINT NOT NULL,                          -- References procurement suppliers
    status VARCHAR(30) NOT NULL DEFAULT 'APPROVED',       -- APPROVED, CONDITIONAL, REJECTED, ON_HOLD
    quality_rating DOUBLE PRECISION NOT NULL DEFAULT 100,  -- 0–100
    risk_level VARCHAR(10) NOT NULL DEFAULT 'LOW',        -- LOW, MEDIUM, HIGH, CRITICAL
    approved_date DATE,
    expiry_date DATE,
    last_audit_date DATE,
    next_audit_date DATE,
    notes TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_supplier_quality_company ON supplier_quality(company_id, status);
CREATE INDEX idx_supplier_quality_supplier ON supplier_quality(company_id, supplier_id);

-- Supplier audits
CREATE TABLE supplier_audits (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    supplier_id BIGINT NOT NULL,
    audit_number VARCHAR(50),
    audit_type VARCHAR(30) NOT NULL DEFAULT 'INITIAL',    -- INITIAL, SURVEILLANCE, REQUALIFICATION, FOR_CAUSE
    status VARCHAR(30) NOT NULL DEFAULT 'PLANNED',
    standard VARCHAR(50),
    planned_date DATE,
    actual_date DATE,
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    lead_auditor_id BIGINT NOT NULL REFERENCES users(id),
    report_number VARCHAR(100),
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_supplier_audits_company ON supplier_audits(company_id, supplier_id);

-- Quality objectives
CREATE TABLE quality_objectives (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    name VARCHAR(200) NOT NULL,
    description TEXT,
    metric_type VARCHAR(50) NOT NULL,                     -- DPPM, FPY, COQ, OTD, CUSTOMER_COMPLAINTS
    target_value DOUBLE PRECISION NOT NULL DEFAULT 0,
    current_value DOUBLE PRECISION NOT NULL DEFAULT 0,
    unit VARCHAR(50),
    frequency VARCHAR(20) NOT NULL DEFAULT 'MONTHLY',     -- DAILY, WEEKLY, MONTHLY, QUARTERLY, ANNUAL
    owner_id BIGINT NOT NULL REFERENCES users(id),
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',         -- ACTIVE, INACTIVE, ACHIEVED
    start_date DATE NOT NULL DEFAULT CURRENT_DATE,
    end_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_quality_objectives_company ON quality_objectives(company_id, status);

-- Quality objective measurements
CREATE TABLE quality_objective_measurements (
    id BIGSERIAL PRIMARY KEY,
    objective_id BIGINT NOT NULL REFERENCES quality_objectives(id) ON DELETE CASCADE,
    value DOUBLE PRECISION NOT NULL,
    measurement_date DATE NOT NULL,
    notes TEXT,
    recorded_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_quality_obj_measurements_objective ON quality_objective_measurements(objective_id, measurement_date DESC);

-- ============================================================================
-- CMMS PERMISSIONS SEED
-- ============================================================================

INSERT INTO permissions (name, description) VALUES
    ('cmms.asset.view',         'View CMMS assets and locations'),
    ('cmms.asset.manage',       'Create and edit CMMS assets'),
    ('cmms.request.create',     'Create maintenance requests'),
    ('cmms.request.triage',     'Triage and schedule maintenance requests'),
    ('cmms.plan.view',          'View preventive maintenance plans'),
    ('cmms.plan.manage',        'Create and edit preventive maintenance plans'),
    ('cmms.work_order.view',    'View work orders'),
    ('cmms.work_order.release', 'Release work orders for execution'),
    ('cmms.work_order.execute', 'Record labor, parts, and complete tasks'),
    ('cmms.work_order.close',   'Close completed work orders'),
    ('cmms.cost.view',          'View maintenance costs'),
    ('cmms.cost.approve',       'Approve cost exceptions'),
    ('cmms.admin',              'Administer CMMS module settings')
ON CONFLICT (name) DO NOTHING;

-- ============================================================================
-- QMS PERMISSIONS SEED
-- ============================================================================

INSERT INTO permissions (name, description) VALUES
    ('qms.specification.view',     'View quality specifications and inspection plans'),
    ('qms.specification.manage',   'Create and manage quality specifications'),
    ('qms.inspection.view',        'View inspections and results'),
    ('qms.inspection.execute',     'Record inspection results'),
    ('qms.hold.view',              'View quality holds'),
    ('qms.hold.manage',            'Create and release quality holds'),
    ('qms.ncr.view',               'View non-conformance reports'),
    ('qms.ncr.create',             'Create non-conformance reports'),
    ('qms.ncr.manage',             'Manage NCR disposition and closure'),
    ('qms.capa.view',              'View corrective actions'),
    ('qms.capa.create',            'Create corrective actions'),
    ('qms.capa.manage',            'Manage CAPA lifecycle'),
    ('qms.capa.verify',            'Verify CAPA effectiveness'),
    ('qms.audit.view',             'View quality audits'),
    ('qms.audit.manage',           'Plan and manage quality audits'),
    ('qms.complaint.view',         'View customer complaints'),
    ('qms.complaint.manage',       'Manage complaint investigations'),
    ('qms.supplier_quality.view',  'View supplier quality records'),
    ('qms.supplier_quality.manage','Manage supplier quality ratings'),
    ('qms.admin',                  'Administer QMS module settings')
ON CONFLICT (name) DO NOTHING;

-- ============================================================================
-- DOCUMENTS PERMISSIONS SEED
-- ============================================================================

INSERT INTO permissions (name, description) VALUES
    ('documents.view',             'View managed documents'),
    ('documents.upload',           'Upload new documents'),
    ('documents.version',          'Create new document versions'),
    ('documents.review',           'Review and comment on documents'),
    ('documents.approve',          'Approve document versions'),
    ('documents.sign',             'Electronically sign documents'),
    ('documents.share',            'Create and manage document shares'),
    ('documents.retention.manage', 'Manage document retention policies'),
    ('documents.hold.manage',      'Create and release legal holds'),
    ('documents.dispose',          'Execute approved document dispositions'),
    ('documents.admin',            'Administer document management settings')
ON CONFLICT (name) DO NOTHING;

-- Grant permissions to Administrator role (role_id = 1)
INSERT INTO role_permissions (role_id, permission_id)
SELECT 1, id FROM permissions
WHERE name LIKE 'cmms.%' OR name LIKE 'qms.%' OR name LIKE 'documents.%'
ON CONFLICT DO NOTHING;
