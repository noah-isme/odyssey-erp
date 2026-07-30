ALTER TABLE approval_policy_steps DROP CONSTRAINT IF EXISTS approval_policy_steps_check;
ALTER TABLE approval_policy_steps ADD COLUMN approver_manager BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE approval_policy_steps ADD CONSTRAINT approval_policy_steps_approver_check CHECK (
    (approver_user_id IS NOT NULL)::INTEGER +
    (approver_role_id IS NOT NULL)::INTEGER +
    approver_manager::INTEGER = 1
);

CREATE TABLE hr_departments (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, code)
);
CREATE TABLE hr_positions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, code)
);
CREATE TABLE hr_employees (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NULL UNIQUE REFERENCES users(id) ON DELETE SET NULL,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    employee_number TEXT NOT NULL,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    department_id BIGINT NULL REFERENCES hr_departments(id) ON DELETE SET NULL,
    position_id BIGINT NULL REFERENCES hr_positions(id) ON DELETE SET NULL,
    manager_id BIGINT NULL REFERENCES hr_employees(id) ON DELETE SET NULL,
    hire_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK(status IN ('ACTIVE','INACTIVE')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, employee_number)
);
CREATE INDEX idx_hr_employees_manager ON hr_employees(manager_id);

CREATE TABLE hr_leave_types (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    default_days NUMERIC(8,2) NOT NULL DEFAULT 0 CHECK(default_days >= 0),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE(company_id, code)
);
CREATE TABLE hr_leave_balances (
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    leave_type_id BIGINT NOT NULL REFERENCES hr_leave_types(id) ON DELETE CASCADE,
    year INTEGER NOT NULL,
    entitled NUMERIC(8,2) NOT NULL DEFAULT 0,
    used NUMERIC(8,2) NOT NULL DEFAULT 0,
    pending NUMERIC(8,2) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(employee_id, leave_type_id, year),
    CHECK(entitled >= 0 AND used >= 0 AND pending >= 0)
);
CREATE TABLE hr_leave_requests (
    id BIGSERIAL PRIMARY KEY,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    leave_type_id BIGINT NOT NULL REFERENCES hr_leave_types(id) ON DELETE RESTRICT,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    days NUMERIC(8,2) NOT NULL CHECK(days > 0),
    reason TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK(status IN ('DRAFT','PENDING','APPROVED','REJECTED','CANCELLED')),
    approval_request_id BIGINT NULL REFERENCES approval_requests(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK(end_date >= start_date)
);
CREATE INDEX idx_hr_leave_employee ON hr_leave_requests(employee_id, start_date DESC);

CREATE TABLE hr_attendance_imports (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    filename TEXT NOT NULL,
    imported_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    total_rows INTEGER NOT NULL DEFAULT 0,
    accepted_rows INTEGER NOT NULL DEFAULT 0,
    rejected_rows INTEGER NOT NULL DEFAULT 0,
    errors JSONB NOT NULL DEFAULT '[]'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE hr_attendance (
    id BIGSERIAL PRIMARY KEY,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    attendance_date DATE NOT NULL,
    check_in TIMESTAMPTZ NULL,
    check_out TIMESTAMPTZ NULL,
    status TEXT NOT NULL DEFAULT 'PRESENT' CHECK(status IN ('PRESENT','ABSENT','LEAVE')),
    source TEXT NOT NULL DEFAULT 'CSV',
    import_id BIGINT NULL REFERENCES hr_attendance_imports(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(employee_id, attendance_date),
    CHECK(check_out IS NULL OR check_in IS NULL OR check_out >= check_in)
);

INSERT INTO permissions(name,description) VALUES
 ('hr.employee.view','View employee directory'),('hr.employee.admin','Manage employee records'),
 ('hr.leave.request','Create and view own leave requests'),('hr.leave.admin','Manage leave types and balances'),
 ('hr.attendance.import','Import attendance CSV files')
ON CONFLICT(name) DO UPDATE SET description=EXCLUDED.description;
INSERT INTO role_permissions(role_id,permission_id) SELECT r.id,p.id FROM roles r CROSS JOIN permissions p WHERE r.name='Admin' AND p.name LIKE 'hr.%' ON CONFLICT DO NOTHING;
