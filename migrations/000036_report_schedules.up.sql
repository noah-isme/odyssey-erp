CREATE TABLE report_schedules (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE,
    report_type TEXT NOT NULL CHECK (report_type IN ('PNL', 'BUDGET_VS_ACTUAL')),
    recipients TEXT[] NOT NULL CHECK (cardinality(recipients) > 0),
    frequency TEXT NOT NULL CHECK (frequency IN ('DAILY', 'WEEKLY', 'MONTHLY')),
    department_id BIGINT REFERENCES departments(id) ON DELETE SET NULL,
    cost_center_id BIGINT REFERENCES cost_centers(id) ON DELETE SET NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    last_sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_report_schedules_due ON report_schedules(is_active, frequency, last_sent_at);
