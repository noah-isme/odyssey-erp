CREATE TABLE IF NOT EXISTS accounting_budgets (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    period_year INT NOT NULL,
    period_month INT NOT NULL,
    amount NUMERIC(14,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(account_id, period_year, period_month)
);

CREATE INDEX IF NOT EXISTS idx_accounting_budgets_period ON accounting_budgets(period_year, period_month);
