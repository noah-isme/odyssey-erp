CREATE TABLE departments (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, code)
);

CREATE TABLE cost_centers (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    department_id BIGINT REFERENCES departments(id) ON DELETE SET NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, code)
);

CREATE INDEX idx_cost_centers_department ON cost_centers(department_id);

ALTER TABLE journal_lines
    ADD COLUMN department_id BIGINT REFERENCES departments(id) ON DELETE SET NULL,
    ADD COLUMN cost_center_id BIGINT REFERENCES cost_centers(id) ON DELETE SET NULL;

CREATE INDEX idx_journal_lines_department ON journal_lines(department_id);
CREATE INDEX idx_journal_lines_cost_center ON journal_lines(cost_center_id);
