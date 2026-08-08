CREATE TABLE IF NOT EXISTS forecast_scenarios (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    policy_version VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS forecast_runs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    scenario_id BIGINT NOT NULL REFERENCES forecast_scenarios(id),
    status VARCHAR(50) NOT NULL, -- e.g., PENDING, COMPLETED, INCOMPLETE, FAILED
    fx_snapshot JSONB,
    completed_at TIMESTAMPTZ,
    error_details TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS forecast_daily_buckets (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES forecast_runs(id) ON DELETE CASCADE,
    bank_account_id BIGINT, -- Can be null if aggregated by currency rather than account
    currency VARCHAR(3) NOT NULL,
    bucket_date DATE NOT NULL,
    opening_balance NUMERIC(19,4) NOT NULL DEFAULT 0,
    total_inflow NUMERIC(19,4) NOT NULL DEFAULT 0,
    total_outflow NUMERIC(19,4) NOT NULL DEFAULT 0,
    closing_balance NUMERIC(19,4) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS forecast_source_lines (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES forecast_runs(id) ON DELETE CASCADE,
    daily_bucket_id BIGINT NOT NULL REFERENCES forecast_daily_buckets(id) ON DELETE CASCADE,
    source_type VARCHAR(50) NOT NULL,
    source_ref VARCHAR(255) NOT NULL,
    amount NUMERIC(19,4) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    expected_date DATE NOT NULL,
    certainty VARCHAR(50) NOT NULL, -- e.g., COMMITTED, PROBABLE
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS forecast_adjustments (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    scenario_id BIGINT NOT NULL REFERENCES forecast_scenarios(id),
    adjustment_type VARCHAR(50) NOT NULL, -- e.g., MANUAL, RECURRING
    description TEXT NOT NULL,
    amount NUMERIC(19,4) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    expected_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
