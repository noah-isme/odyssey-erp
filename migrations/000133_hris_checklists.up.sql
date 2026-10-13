-- HRIS expansion P3.1: onboarding/offboarding checklists, asset custody, and
-- the checklist admin permission.

CREATE TABLE hr_checklist_templates (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('ONBOARDING','OFFBOARDING')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, code)
);

CREATE TABLE hr_checklist_template_items (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    template_id BIGINT NOT NULL REFERENCES hr_checklist_templates(id) ON DELETE CASCADE,
    seq INT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    assignee_role TEXT NOT NULL CHECK (assignee_role IN ('HR','IT','MANAGER')),
    default_assignee_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    due_offset_days INT NOT NULL DEFAULT 0,
    action_key TEXT NULL CHECK (action_key IN ('DEACTIVATE_USER','RETURN_ASSETS','CLOSE_LEAVE','CLOSE_BENEFITS')),
    UNIQUE (template_id, seq)
);

CREATE TABLE hr_employee_checklists (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    template_id BIGINT NOT NULL REFERENCES hr_checklist_templates(id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (kind IN ('ONBOARDING','OFFBOARDING')),
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','COMPLETED','CANCELLED')),
    started_on DATE NOT NULL DEFAULT CURRENT_DATE,
    termination_date DATE NULL,
    termination_reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    completed_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX uq_hr_employee_checklists_open ON hr_employee_checklists(employee_id, kind) WHERE status = 'OPEN';
CREATE INDEX idx_hr_employee_checklists_company_status ON hr_employee_checklists(company_id, status);

CREATE TABLE hr_employee_checklist_tasks (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    checklist_id BIGINT NOT NULL REFERENCES hr_employee_checklists(id) ON DELETE CASCADE,
    seq INT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    assignee_role TEXT NOT NULL CHECK (assignee_role IN ('HR','IT','MANAGER')),
    assignee_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    due_date DATE NULL,
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','DONE','SKIPPED')),
    action_key TEXT NULL CHECK (action_key IN ('DEACTIVATE_USER','RETURN_ASSETS','CLOSE_LEAVE','CLOSE_BENEFITS')),
    notes TEXT NOT NULL DEFAULT '',
    completed_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    completed_at TIMESTAMPTZ NULL
);
CREATE INDEX idx_hr_employee_checklist_tasks_assignee_status ON hr_employee_checklist_tasks(assignee_user_id, status);
CREATE INDEX idx_hr_employee_checklist_tasks_status_due ON hr_employee_checklist_tasks(status, due_date);
CREATE INDEX idx_hr_employee_checklist_tasks_checklist ON hr_employee_checklist_tasks(checklist_id);

ALTER TABLE fixed_assets
    ADD COLUMN custodian_employee_id BIGINT NULL REFERENCES hr_employees(id) ON DELETE SET NULL;
CREATE INDEX idx_fixed_assets_custodian ON fixed_assets(custodian_employee_id);

INSERT INTO permissions (name, description) VALUES
    ('hr.checklist.admin', 'Administer onboarding and offboarding checklists')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
  AND p.name IN ('hr.checklist.admin')
ON CONFLICT DO NOTHING;
