-- Phase 14: transaction-level foreign exchange.  This is deliberately separate
-- from fx_rates, whose monthly rows are used by consolidation.

ALTER TABLE companies
    ADD COLUMN IF NOT EXISTS base_currency CHAR(3) NOT NULL DEFAULT 'IDR';

ALTER TABLE companies
    ADD CONSTRAINT companies_base_currency_iso_chk
    CHECK (base_currency ~ '^[A-Z]{3}$');

CREATE TABLE IF NOT EXISTS fx_daily_rates (
    id BIGSERIAL PRIMARY KEY,
    base_currency CHAR(3) NOT NULL CHECK (base_currency ~ '^[A-Z]{3}$'),
    quote_currency CHAR(3) NOT NULL CHECK (quote_currency ~ '^[A-Z]{3}$'),
    rate_date DATE NOT NULL,
    rate NUMERIC(20,10) NOT NULL CHECK (rate > 0),
    source TEXT NOT NULL,
    source_reference TEXT,
    provider_updated_at TIMESTAMPTZ,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    raw_payload_hash TEXT,
    CHECK (base_currency <> quote_currency),
    UNIQUE (base_currency, quote_currency, rate_date, source)
);

CREATE INDEX IF NOT EXISTS idx_fx_daily_rates_pair_date
    ON fx_daily_rates (base_currency, quote_currency, rate_date);
CREATE INDEX IF NOT EXISTS idx_fx_daily_rates_date_source
    ON fx_daily_rates (rate_date, source);

CREATE TABLE IF NOT EXISTS fx_fetch_runs (
    id BIGSERIAL PRIMARY KEY,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rate_date DATE NOT NULL,
    source TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('SUCCESS','FAILED','PARTIAL')),
    response_hash TEXT,
    error_message TEXT,
    UNIQUE (rate_date, source)
);

-- Posted documents retain the transaction amount and the locked carrying rate.
ALTER TABLE ar_invoices
    ADD COLUMN IF NOT EXISTS base_currency CHAR(3),
    ADD COLUMN IF NOT EXISTS original_currency_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS base_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS fx_rate NUMERIC(20,10),
    ADD COLUMN IF NOT EXISTS fx_rate_date DATE,
    ADD COLUMN IF NOT EXISTS fx_rate_source TEXT,
    ADD COLUMN IF NOT EXISTS fx_rate_locked_at TIMESTAMPTZ;
ALTER TABLE ap_invoices
    ADD COLUMN IF NOT EXISTS base_currency CHAR(3),
    ADD COLUMN IF NOT EXISTS original_currency_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS base_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS fx_rate NUMERIC(20,10),
    ADD COLUMN IF NOT EXISTS fx_rate_date DATE,
    ADD COLUMN IF NOT EXISTS fx_rate_source TEXT,
    ADD COLUMN IF NOT EXISTS fx_rate_locked_at TIMESTAMPTZ;

ALTER TABLE ar_payments
    ADD COLUMN IF NOT EXISTS currency CHAR(3),
    ADD COLUMN IF NOT EXISTS original_currency_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS base_currency CHAR(3),
    ADD COLUMN IF NOT EXISTS base_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS fx_rate NUMERIC(20,10),
    ADD COLUMN IF NOT EXISTS fx_rate_date DATE,
    ADD COLUMN IF NOT EXISTS fx_rate_source TEXT,
    ADD COLUMN IF NOT EXISTS fx_rate_locked_at TIMESTAMPTZ;
ALTER TABLE ap_payments
    ADD COLUMN IF NOT EXISTS currency CHAR(3),
    ADD COLUMN IF NOT EXISTS original_currency_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS base_currency CHAR(3),
    ADD COLUMN IF NOT EXISTS base_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS fx_rate NUMERIC(20,10),
    ADD COLUMN IF NOT EXISTS fx_rate_date DATE,
    ADD COLUMN IF NOT EXISTS fx_rate_source TEXT,
    ADD COLUMN IF NOT EXISTS fx_rate_locked_at TIMESTAMPTZ;

ALTER TABLE ar_payment_allocations
    ADD COLUMN IF NOT EXISTS original_currency_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS base_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS currency CHAR(3),
    ADD COLUMN IF NOT EXISTS base_currency CHAR(3),
    ADD COLUMN IF NOT EXISTS fx_rate NUMERIC(20,10),
    ADD COLUMN IF NOT EXISTS fx_rate_date DATE,
    ADD COLUMN IF NOT EXISTS fx_rate_source TEXT,
    ADD COLUMN IF NOT EXISTS fx_rate_locked_at TIMESTAMPTZ;
ALTER TABLE ap_payment_allocations
    ADD COLUMN IF NOT EXISTS original_currency_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS base_amount NUMERIC(20,2),
    ADD COLUMN IF NOT EXISTS currency CHAR(3),
    ADD COLUMN IF NOT EXISTS base_currency CHAR(3),
    ADD COLUMN IF NOT EXISTS fx_rate NUMERIC(20,10),
    ADD COLUMN IF NOT EXISTS fx_rate_date DATE,
    ADD COLUMN IF NOT EXISTS fx_rate_source TEXT,
    ADD COLUMN IF NOT EXISTS fx_rate_locked_at TIMESTAMPTZ;

-- Existing documents are IDR documents in the pre-Phase-14 schema. Preserve
-- their historical amount while making the carrying valuation explicit.
UPDATE ar_invoices SET base_currency='IDR', original_currency_amount=total,
    base_amount=total, fx_rate=1, fx_rate_source='INTERNAL',
    fx_rate_date=COALESCE(posted_at::date, created_at::date),
    fx_rate_locked_at=COALESCE(posted_at, created_at)
WHERE base_currency IS NULL;
UPDATE ap_invoices SET base_currency='IDR', original_currency_amount=total,
    base_amount=total, fx_rate=1, fx_rate_source='INTERNAL',
    fx_rate_date=COALESCE(posted_at::date, created_at::date),
    fx_rate_locked_at=COALESCE(posted_at, created_at)
WHERE base_currency IS NULL;
UPDATE ar_payments SET currency='IDR', original_currency_amount=amount,
    base_currency='IDR', base_amount=amount, fx_rate=1,
    fx_rate_source='INTERNAL', fx_rate_date=paid_at::date,
    fx_rate_locked_at=COALESCE(updated_at, paid_at)
WHERE base_currency IS NULL;
UPDATE ap_payments SET currency='IDR', original_currency_amount=amount,
    base_currency='IDR', base_amount=amount, fx_rate=1,
    fx_rate_source='INTERNAL', fx_rate_date=paid_at::date,
    fx_rate_locked_at=COALESCE(updated_at, paid_at)
WHERE base_currency IS NULL;
UPDATE ar_payment_allocations SET original_currency_amount=amount, base_amount=amount
WHERE original_currency_amount IS NULL;
UPDATE ap_payment_allocations SET original_currency_amount=amount, base_amount=amount
WHERE original_currency_amount IS NULL;
UPDATE ar_payment_allocations SET currency='IDR', base_currency='IDR', fx_rate=1,
    fx_rate_source='INTERNAL', fx_rate_date=created_at::date, fx_rate_locked_at=created_at
WHERE base_currency IS NULL;
UPDATE ap_payment_allocations SET currency='IDR', base_currency='IDR', fx_rate=1,
    fx_rate_source='INTERNAL', fx_rate_date=created_at::date, fx_rate_locked_at=created_at
WHERE base_currency IS NULL;

CREATE TABLE IF NOT EXISTS fx_revaluations (
    id BIGSERIAL PRIMARY KEY,
    period_id BIGINT NOT NULL REFERENCES periods(id) ON DELETE RESTRICT,
    document_type TEXT NOT NULL CHECK (document_type IN ('AR_INVOICE','AP_INVOICE')),
    document_id BIGINT NOT NULL,
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    original_balance NUMERIC(20,2) NOT NULL,
    previous_base_amount NUMERIC(20,2) NOT NULL,
    closing_base_amount NUMERIC(20,2) NOT NULL,
    difference NUMERIC(20,2) NOT NULL,
    closing_rate NUMERIC(20,10) NOT NULL CHECK (closing_rate > 0),
    rate_date DATE NOT NULL,
    rate_source TEXT NOT NULL,
    journal_entry_id BIGINT REFERENCES journal_entries(id) ON DELETE RESTRICT,
    actor_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    reversed_by_id BIGINT REFERENCES journal_entries(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (period_id, document_type, document_id)
);

-- Payment FX journals use this key before creating a journal. It makes
-- retries safe even when the payment endpoint is submitted more than once.
CREATE TABLE IF NOT EXISTS fx_journal_idempotency (
    source_key TEXT PRIMARY KEY,
    journal_entry_id BIGINT NOT NULL REFERENCES journal_entries(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS fx_revaluation_idempotency (
    period_id BIGINT NOT NULL REFERENCES periods(id) ON DELETE RESTRICT,
    document_type TEXT NOT NULL,
    document_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (period_id, document_type, document_id)
);

CREATE TABLE IF NOT EXISTS fx_revaluation_reversals (
    id BIGSERIAL PRIMARY KEY,
    revaluation_id BIGINT NOT NULL REFERENCES fx_revaluations(id) ON DELETE RESTRICT,
    next_period_id BIGINT NOT NULL REFERENCES periods(id) ON DELETE RESTRICT,
    journal_entry_id BIGINT NOT NULL REFERENCES journal_entries(id) ON DELETE RESTRICT,
    actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (revaluation_id)
);

INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'FX', 'fx.realized.gain', id, NOW(), NOW() FROM accounts WHERE code='4200'
ON CONFLICT (module, key) DO UPDATE SET account_id=EXCLUDED.account_id, updated_at=NOW();
INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'FX', 'fx.realized.loss', id, NOW(), NOW() FROM accounts WHERE code='5200'
ON CONFLICT (module, key) DO UPDATE SET account_id=EXCLUDED.account_id, updated_at=NOW();
INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'FX', 'fx.revaluation.gain', id, NOW(), NOW() FROM accounts WHERE code='4200'
ON CONFLICT (module, key) DO UPDATE SET account_id=EXCLUDED.account_id, updated_at=NOW();
INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'FX', 'fx.revaluation.loss', id, NOW(), NOW() FROM accounts WHERE code='5200'
ON CONFLICT (module, key) DO UPDATE SET account_id=EXCLUDED.account_id, updated_at=NOW();

INSERT INTO permissions (name, description) VALUES
    ('finance.fx.view', 'View transaction FX rates and valuation results'),
    ('finance.fx.manage', 'Manage transaction FX configuration and fetches'),
    ('finance.fx.revalue', 'Execute FX revaluation'),
    ('finance.fx.override', 'Approve manual FX rate overrides')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.name IN ('Admin', 'Finance Manager')
  AND p.name IN ('finance.fx.view', 'finance.fx.manage', 'finance.fx.revalue', 'finance.fx.override')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.name = 'Finance User' AND p.name = 'finance.fx.view'
ON CONFLICT DO NOTHING;
