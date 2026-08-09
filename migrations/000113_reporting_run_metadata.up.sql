-- 000113_reporting_run_metadata.up.sql

CREATE TABLE report_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id),
    dataset_id UUID NOT NULL REFERENCES reporting_datasets(id),
    actor_id UUID NOT NULL REFERENCES users(id),
    status VARCHAR(50) NOT NULL DEFAULT 'QUEUED', -- QUEUED, RUNNING, COMPLETED, FAILED, CANCELLED
    row_count INT,
    error_message TEXT,
    executed_sql TEXT, -- The normalized safe SQL
    execution_time_ms INT,
    query_cost_estimate INT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
