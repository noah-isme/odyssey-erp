-- HRIS expansion P4.1: performance review cycles, participants and goals, the
-- company_id column on hr_performance_reviews, and the performance and HR
-- reports permissions (hr.reports.view is consumed by phase 5).

CREATE TABLE hr_performance_cycles (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    self_review_due DATE NOT NULL,
    manager_review_due DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','ACTIVE','CLOSED')),
    created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (period_end >= period_start),
    UNIQUE (company_id, name)
);
CREATE INDEX idx_hr_performance_cycles_company_status ON hr_performance_cycles(company_id, status);

CREATE TABLE hr_performance_participants (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    cycle_id BIGINT NOT NULL REFERENCES hr_performance_cycles(id) ON DELETE CASCADE,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    reviewer_employee_id BIGINT NULL REFERENCES hr_employees(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'ENROLLED' CHECK (status IN ('ENROLLED','SELF_SUBMITTED','MANAGER_SUBMITTED','COMPLETED')),
    self_rating INT NULL CHECK (self_rating BETWEEN 1 AND 5),
    self_comments TEXT NOT NULL DEFAULT '',
    manager_rating INT NULL CHECK (manager_rating BETWEEN 1 AND 5),
    manager_comments TEXT NOT NULL DEFAULT '',
    final_rating INT NULL CHECK (final_rating BETWEEN 1 AND 5),
    self_submitted_at TIMESTAMPTZ NULL,
    manager_submitted_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    review_id BIGINT NULL REFERENCES hr_performance_reviews(id) ON DELETE SET NULL,
    UNIQUE (cycle_id, employee_id)
);
CREATE INDEX idx_hr_performance_participants_company_cycle ON hr_performance_participants(company_id, cycle_id);
CREATE INDEX idx_hr_performance_participants_reviewer ON hr_performance_participants(reviewer_employee_id, status);
CREATE INDEX idx_hr_performance_participants_employee ON hr_performance_participants(employee_id);

CREATE TABLE hr_performance_goals (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    participant_id BIGINT NOT NULL REFERENCES hr_performance_participants(id) ON DELETE CASCADE,
    seq INT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    kpi_target TEXT NOT NULL DEFAULT '',
    weight NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (weight BETWEEN 0 AND 100),
    self_score INT NULL CHECK (self_score BETWEEN 1 AND 5),
    manager_score INT NULL CHECK (manager_score BETWEEN 1 AND 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (participant_id, seq)
);

-- Add company_id nullable, backfill from the employee, then enforce NOT NULL.
ALTER TABLE hr_performance_reviews
    ADD COLUMN company_id BIGINT NULL REFERENCES companies(id);

UPDATE hr_performance_reviews pr
SET company_id = e.company_id
FROM hr_employees e
WHERE e.id = pr.employee_id;

ALTER TABLE hr_performance_reviews
    ALTER COLUMN company_id SET NOT NULL;
CREATE INDEX idx_hr_performance_reviews_company ON hr_performance_reviews(company_id);

INSERT INTO permissions (name, description) VALUES
    ('hr.performance.view', 'View performance review cycles and reviews'),
    ('hr.performance.admin', 'Administer performance review cycles'),
    ('hr.reports.view', 'View HR dashboard and reports')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
  AND p.name IN ('hr.performance.view', 'hr.performance.admin', 'hr.reports.view')
ON CONFLICT DO NOTHING;
