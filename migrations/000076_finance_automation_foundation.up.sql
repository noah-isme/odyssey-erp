-- Core Finance Automation F0: company-scoped controls and a durable command outbox.

CREATE TABLE IF NOT EXISTS finance_automation_settings (
    company_id BIGINT PRIMARY KEY REFERENCES companies(id) ON DELETE CASCADE,
    bank_feed_auto_sync_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    cash_forecast_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    payment_scheduling_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    payment_execution_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    p2p_auto_post_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    asset_operations_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    forecast_horizon_weeks SMALLINT NOT NULL DEFAULT 13 CHECK (forecast_horizon_weeks BETWEEN 1 AND 26),
    bank_feed_sync_interval_minutes INTEGER NOT NULL DEFAULT 1440 CHECK (bank_feed_sync_interval_minutes BETWEEN 15 AND 10080),
    payment_maker_checker_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    payment_executor_separation_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (NOT payment_execution_enabled OR payment_scheduling_enabled)
);

INSERT INTO finance_automation_settings (company_id)
SELECT id FROM companies
ON CONFLICT (company_id) DO NOTHING;

CREATE OR REPLACE FUNCTION ensure_finance_automation_settings() RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO finance_automation_settings (company_id)
    VALUES (NEW.id)
    ON CONFLICT (company_id) DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_company_finance_automation_settings ON companies;
CREATE TRIGGER trg_company_finance_automation_settings
AFTER INSERT ON companies
FOR EACH ROW EXECUTE FUNCTION ensure_finance_automation_settings();

CREATE TABLE IF NOT EXISTS finance_automation_outbox (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    topic TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    causation_id TEXT,
    idempotency_key TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::JSONB,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PROCESSING', 'COMPLETED', 'DEAD_LETTERED', 'CANCELLED')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 10 CHECK (max_attempts BETWEEN 1 AND 100),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    locked_by TEXT,
    last_error TEXT,
    completed_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    replayed_from_id BIGINT REFERENCES finance_automation_outbox(id) ON DELETE SET NULL,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((status = 'COMPLETED') = (completed_at IS NOT NULL)),
    CHECK ((status = 'DEAD_LETTERED') = (dead_lettered_at IS NOT NULL)),
    UNIQUE (company_id, operation, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_finance_automation_outbox_pending
    ON finance_automation_outbox (available_at, id)
    WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_finance_automation_outbox_company_status
    ON finance_automation_outbox (company_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_finance_automation_outbox_correlation
    ON finance_automation_outbox (company_id, correlation_id, id);

INSERT INTO permissions (name, description) VALUES
    ('finance.automation.manage', 'Manage finance automation settings and feature flags'),
    ('finance.bank_feed.manage', 'Manage bank feed connections and synchronization'),
    ('finance.forecast.view', 'View cash forecast scenarios and results'),
    ('finance.forecast.manage', 'Manage cash forecast scenarios and adjustments'),
    ('finance.payment.propose', 'Create and revise payment proposals'),
    ('finance.payment.approve', 'Approve or reject payment proposals'),
    ('finance.payment.export', 'Export approved payment instructions'),
    ('finance.payment.execute', 'Submit or confirm payment execution'),
    ('procurement.p2p_exception.view', 'View purchase-to-pay matching exceptions'),
    ('procurement.p2p_exception.resolve', 'Resolve purchase-to-pay matching exceptions'),
    ('fixedassets.location.manage', 'Manage fixed asset locations and custody'),
    ('fixedassets.transfer.manage', 'Manage fixed asset transfers'),
    ('fixedassets.maintenance.manage', 'Manage fixed asset maintenance work'),
    ('fixedassets.warranty.manage', 'Manage fixed asset warranties and claims')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name IN (
    'finance.automation.manage', 'finance.bank_feed.manage',
    'finance.forecast.view', 'finance.forecast.manage',
    'finance.payment.propose', 'finance.payment.approve',
    'finance.payment.export', 'finance.payment.execute',
    'procurement.p2p_exception.view', 'procurement.p2p_exception.resolve',
    'fixedassets.location.manage', 'fixedassets.transfer.manage',
    'fixedassets.maintenance.manage', 'fixedassets.warranty.manage'
)
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name IN ('finance.forecast.view', 'procurement.p2p_exception.view')
WHERE LOWER(TRIM(r.name)) IN ('finance user', 'finance manager')
ON CONFLICT DO NOTHING;
