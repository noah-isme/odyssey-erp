CREATE TYPE elimination_run_status AS ENUM ('DRAFT','SIMULATED','POSTED','FAILED');
CREATE TYPE variance_snapshot_status AS ENUM ('PENDING','IN_PROGRESS','READY','FAILED');

CREATE TABLE elimination_rules (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT REFERENCES consol_groups(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    source_company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    target_company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    account_src TEXT NOT NULL,
    account_tgt TEXT NOT NULL,
    match_criteria JSONB NOT NULL DEFAULT '{}'::JSONB,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_elimination_rules_group ON elimination_rules(group_id);
CREATE INDEX idx_elimination_rules_src_tgt ON elimination_rules(source_company_id, target_company_id);
CREATE INDEX idx_elimination_rules_active ON elimination_rules(is_active) WHERE is_active;

CREATE TABLE elimination_runs (
    id BIGSERIAL PRIMARY KEY,
    period_id BIGINT NOT NULL REFERENCES accounting_periods(id) ON DELETE CASCADE,
    rule_id BIGINT NOT NULL REFERENCES elimination_rules(id) ON DELETE CASCADE,
    status elimination_run_status NOT NULL DEFAULT 'DRAFT',
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    simulated_at TIMESTAMPTZ,
    posted_at TIMESTAMPTZ,
    journal_entry_id BIGINT REFERENCES journal_entries(id) ON DELETE SET NULL,
    summary JSONB
);
CREATE INDEX idx_elimination_runs_period ON elimination_runs(period_id);
CREATE INDEX idx_elimination_runs_rule ON elimination_runs(rule_id);
CREATE INDEX idx_elimination_runs_status ON elimination_runs(status);

CREATE TABLE variance_rules (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    comparison_type TEXT NOT NULL,
    base_period_id BIGINT NOT NULL REFERENCES accounting_periods(id) ON DELETE CASCADE,
    compare_period_id BIGINT REFERENCES accounting_periods(id) ON DELETE SET NULL,
    dimension_filters JSONB NOT NULL DEFAULT '{}'::JSONB,
    threshold_amount NUMERIC(18,2),
    threshold_percent NUMERIC(6,2),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_variance_rules_company ON variance_rules(company_id);
CREATE INDEX idx_variance_rules_active ON variance_rules(is_active) WHERE is_active;

CREATE TABLE variance_snapshots (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL REFERENCES variance_rules(id) ON DELETE CASCADE,
    period_id BIGINT NOT NULL REFERENCES accounting_periods(id) ON DELETE CASCADE,
    status variance_snapshot_status NOT NULL DEFAULT 'PENDING',
    generated_at TIMESTAMPTZ,
    generated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    error_message TEXT,
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_variance_snapshots_rule ON variance_snapshots(rule_id);
CREATE INDEX idx_variance_snapshots_period ON variance_snapshots(period_id);
CREATE INDEX idx_variance_snapshots_status ON variance_snapshots(status);
