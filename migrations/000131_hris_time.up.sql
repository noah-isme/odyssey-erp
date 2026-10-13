-- HRIS expansion P1.1: holidays, shifts, shift assignments, overtime requests,
-- leave half-day flags, and schedule/overtime permissions.

CREATE TABLE hr_holidays (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    holiday_date DATE NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, holiday_date)
);

CREATE TABLE hr_shifts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    break_minutes INTEGER NOT NULL DEFAULT 0 CHECK (break_minutes >= 0),
    work_days SMALLINT[] NOT NULL DEFAULT '{1,2,3,4,5}',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, code)
);

CREATE TABLE hr_employee_shifts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    shift_id BIGINT NOT NULL REFERENCES hr_shifts(id) ON DELETE RESTRICT,
    valid_from DATE NOT NULL,
    valid_to DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (valid_to IS NULL OR valid_to >= valid_from),
    UNIQUE (employee_id, valid_from)
);

CREATE TABLE hr_overtime_requests (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    worked_on DATE NOT NULL,
    minutes INTEGER NOT NULL CHECK (minutes > 0 AND minutes <= 720),
    reason TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','APPROVED','REJECTED','CANCELLED')),
    approval_request_id BIGINT NULL REFERENCES approval_requests(id) ON DELETE SET NULL,
    apply_error TEXT NOT NULL DEFAULT '',
    decided_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    decided_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_hr_overtime_requests_employee_worked_on ON hr_overtime_requests(employee_id, worked_on);
CREATE INDEX idx_hr_overtime_requests_company_status ON hr_overtime_requests(company_id, status);
CREATE UNIQUE INDEX uq_hr_overtime_requests_active ON hr_overtime_requests(employee_id, worked_on)
    WHERE status IN ('PENDING','APPROVED');

ALTER TABLE hr_leave_requests
    ADD COLUMN start_half_day BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN end_half_day BOOLEAN NOT NULL DEFAULT FALSE;

INSERT INTO permissions (name, description) VALUES
    ('hr.schedule.admin', 'Manage holidays, shifts, and employee shift assignments'),
    ('hr.overtime.request', 'Submit overtime requests'),
    ('hr.overtime.admin', 'Administer and retry overtime requests')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
  AND p.name IN ('hr.schedule.admin', 'hr.overtime.request', 'hr.overtime.admin')
ON CONFLICT DO NOTHING;
