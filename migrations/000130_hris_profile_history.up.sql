-- HRIS expansion P0.3: employee profile, emergency contacts, job history,
-- compensation audit columns, and HR permissions.

ALTER TABLE hr_employees
    ADD COLUMN date_of_birth DATE NULL,
    ADD COLUMN gender TEXT NULL CHECK (gender IN ('MALE','FEMALE','OTHER')),
    ADD COLUMN national_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN tax_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN marital_status TEXT NULL CHECK (marital_status IN ('SINGLE','MARRIED','DIVORCED','WIDOWED')),
    ADD COLUMN dependents_count INTEGER NOT NULL DEFAULT 0 CHECK (dependents_count >= 0),
    ADD COLUMN phone TEXT NOT NULL DEFAULT '',
    ADD COLUMN address_line1 TEXT NOT NULL DEFAULT '',
    ADD COLUMN address_line2 TEXT NOT NULL DEFAULT '',
    ADD COLUMN city TEXT NOT NULL DEFAULT '',
    ADD COLUMN province TEXT NOT NULL DEFAULT '',
    ADD COLUMN postal_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN country TEXT NOT NULL DEFAULT '',
    ADD COLUMN employment_type TEXT NOT NULL DEFAULT 'PERMANENT' CHECK (employment_type IN ('PERMANENT','CONTRACT','PROBATION','INTERN')),
    ADD COLUMN contract_start_date DATE NULL,
    ADD COLUMN contract_end_date DATE NULL,
    ADD COLUMN probation_end_date DATE NULL,
    ADD COLUMN termination_date DATE NULL,
    ADD COLUMN termination_reason TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT hr_employees_contract_dates_check
        CHECK (contract_end_date IS NULL OR contract_start_date IS NULL OR contract_end_date >= contract_start_date);

CREATE TABLE hr_employee_emergency_contacts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    relationship TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_hr_employee_emergency_contacts_employee ON hr_employee_emergency_contacts(employee_id);

CREATE TABLE hr_employee_job_history (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    department_id BIGINT NULL REFERENCES hr_departments(id) ON DELETE SET NULL,
    position_id BIGINT NULL REFERENCES hr_positions(id) ON DELETE SET NULL,
    manager_id BIGINT NULL REFERENCES hr_employees(id) ON DELETE SET NULL,
    status TEXT NOT NULL CHECK (status IN ('ACTIVE','INACTIVE')),
    employment_type TEXT NOT NULL CHECK (employment_type IN ('PERMANENT','CONTRACT','PROBATION','INTERN')),
    grade TEXT NOT NULL DEFAULT '',
    valid_from DATE NOT NULL,
    valid_to DATE NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (valid_to IS NULL OR valid_to >= valid_from),
    UNIQUE (employee_id, valid_from)
);
CREATE INDEX idx_hr_employee_job_history_employee_valid_from ON hr_employee_job_history(employee_id, valid_from DESC);

INSERT INTO hr_employee_job_history
    (company_id, employee_id, department_id, position_id, manager_id, status, employment_type, grade, valid_from, reason)
SELECT e.company_id, e.id, e.department_id, e.position_id, e.manager_id, e.status, e.employment_type, '', e.hire_date, 'Initial record'
FROM hr_employees e;

ALTER TABLE payroll_compensation_assignments
    ADD COLUMN reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;

INSERT INTO permissions (name, description) VALUES
    ('hr.compensation.view', 'View employee compensation and bank details'),
    ('hr.compensation.admin', 'Change employee compensation and bank details'),
    ('hr.employee.sensitive', 'View and edit sensitive employee profile fields')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
  AND p.name IN ('hr.compensation.view', 'hr.compensation.admin', 'hr.employee.sensitive')
ON CONFLICT DO NOTHING;
