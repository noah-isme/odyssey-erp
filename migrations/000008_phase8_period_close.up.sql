CREATE TYPE accounting_period_status AS ENUM ('OPEN','SOFT_CLOSED','HARD_CLOSED');
CREATE TYPE period_close_run_status AS ENUM ('DRAFT','IN_PROGRESS','COMPLETED','CANCELLED');
CREATE TYPE period_close_checklist_status AS ENUM ('PENDING','IN_PROGRESS','DONE','SKIPPED');

CREATE TABLE accounting_periods (
    id BIGSERIAL PRIMARY KEY,
    period_id BIGINT NOT NULL UNIQUE REFERENCES periods(id) ON DELETE CASCADE,
    company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    status accounting_period_status NOT NULL DEFAULT 'OPEN',
    soft_closed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    soft_closed_at TIMESTAMPTZ,
    closed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    closed_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_accounting_period_name UNIQUE (company_id, name),
    CONSTRAINT chk_accounting_period_dates CHECK (start_date <= end_date)
);
CREATE INDEX idx_accounting_periods_company_id_status ON accounting_periods(company_id, status);
CREATE INDEX idx_accounting_periods_company_id_dates ON accounting_periods(company_id, start_date, end_date);

INSERT INTO accounting_periods (period_id, company_id, name, start_date, end_date, status, created_at, updated_at)
SELECT
    p.id,
    NULL,
    p.code,
    p.start_date,
    p.end_date,
    (
        CASE
            WHEN p.status = 'LOCKED' THEN 'HARD_CLOSED'
            WHEN p.status = 'CLOSED' THEN 'SOFT_CLOSED'
            ELSE 'OPEN'
        END
    )::accounting_period_status,
    p.created_at,
    p.updated_at
FROM periods p;

CREATE TABLE period_close_runs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    period_id BIGINT NOT NULL REFERENCES accounting_periods(id) ON DELETE CASCADE,
    status period_close_run_status NOT NULL DEFAULT 'DRAFT',
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    notes TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_period_close_runs_company_id_period_id ON period_close_runs(company_id, period_id);

CREATE TABLE period_close_checklist_items (
    id BIGSERIAL PRIMARY KEY,
    period_close_run_id BIGINT NOT NULL REFERENCES period_close_runs(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    label TEXT NOT NULL,
    status period_close_checklist_status NOT NULL DEFAULT 'PENDING',
    assigned_to BIGINT REFERENCES users(id) ON DELETE SET NULL,
    completed_at TIMESTAMPTZ,
    comment TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_period_close_checklist_code UNIQUE (period_close_run_id, code)
);
CREATE INDEX idx_period_close_checklist_period_close_run_id ON period_close_checklist_items(period_close_run_id);
