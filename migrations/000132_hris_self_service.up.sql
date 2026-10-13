-- HRIS expansion P2.1: employee self-service expense claims, profile change
-- requests, and their admin permissions.

CREATE TABLE hr_expense_claims (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    claim_date DATE NOT NULL,
    category TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    amount NUMERIC(18,2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL DEFAULT 'IDR',
    document_id BIGINT NULL REFERENCES documents(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','APPROVED','REJECTED','CANCELLED')),
    approval_request_id BIGINT NULL REFERENCES approval_requests(id) ON DELETE SET NULL,
    apply_error TEXT NOT NULL DEFAULT '',
    decided_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    decided_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_hr_expense_claims_company_status ON hr_expense_claims(company_id, status);
CREATE INDEX idx_hr_expense_claims_employee_claim_date ON hr_expense_claims(employee_id, claim_date);

CREATE TABLE hr_profile_change_requests (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    change_type TEXT NOT NULL CHECK (change_type IN ('ADDRESS','BANK','EMERGENCY_CONTACT')),
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','APPROVED','REJECTED','CANCELLED')),
    approval_request_id BIGINT NULL REFERENCES approval_requests(id) ON DELETE SET NULL,
    apply_error TEXT NOT NULL DEFAULT '',
    applied_at TIMESTAMPTZ NULL,
    decided_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_hr_profile_change_requests_company_status ON hr_profile_change_requests(company_id, status);

INSERT INTO permissions (name, description) VALUES
    ('hr.expense.admin', 'Administer and retry employee expense claims'),
    ('hr.profile_change.admin', 'Administer and retry employee profile change requests')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
  AND p.name IN ('hr.expense.admin', 'hr.profile_change.admin')
ON CONFLICT DO NOTHING;
