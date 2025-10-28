-- Phase 4.2 accounting schema
CREATE TYPE account_type AS ENUM ('ASSET','LIABILITY','EQUITY','REVENUE','EXPENSE');
CREATE TYPE period_status AS ENUM ('OPEN','CLOSED','LOCKED');
CREATE TYPE journal_status AS ENUM ('POSTED','VOID');

CREATE SEQUENCE IF NOT EXISTS journal_entries_number_seq START WITH 100000;

CREATE TABLE accounts (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(20) NOT NULL UNIQUE,
    name VARCHAR(120) NOT NULL,
    type account_type NOT NULL,
    parent_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_accounts_parent_id ON accounts(parent_id);

CREATE TABLE periods (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(20) NOT NULL UNIQUE,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    status period_status NOT NULL DEFAULT 'OPEN',
    closed_at TIMESTAMPTZ,
    locked_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_period_dates CHECK (start_date <= end_date)
);
CREATE INDEX idx_periods_status ON periods(status);
CREATE UNIQUE INDEX idx_periods_range ON periods(start_date, end_date);

CREATE TABLE journal_entries (
    id BIGSERIAL PRIMARY KEY,
    number BIGINT NOT NULL DEFAULT nextval('journal_entries_number_seq'),
    period_id BIGINT NOT NULL REFERENCES periods(id) ON DELETE RESTRICT,
    date DATE NOT NULL,
    source_module TEXT NOT NULL,
    source_id UUID,
    memo TEXT,
    posted_by BIGINT,
    posted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status journal_status NOT NULL DEFAULT 'POSTED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_journal_number UNIQUE (number)
);
CREATE INDEX idx_journal_entries_period ON journal_entries(period_id);
CREATE INDEX idx_journal_entries_source ON journal_entries(source_module, source_id);

CREATE TABLE journal_lines (
    id BIGSERIAL PRIMARY KEY,
    je_id BIGINT NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    debit NUMERIC(14,2) NOT NULL DEFAULT 0,
    credit NUMERIC(14,2) NOT NULL DEFAULT 0,
    dim_company_id BIGINT,
    dim_branch_id BIGINT,
    dim_warehouse_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_positive_amount CHECK (debit >= 0 AND credit >= 0)
);
CREATE INDEX idx_journal_lines_account ON journal_lines(account_id);

CREATE TABLE source_links (
    id BIGSERIAL PRIMARY KEY,
    module TEXT NOT NULL,
    ref_id UUID NOT NULL,
    je_id BIGINT NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_source_links UNIQUE (module, ref_id)
);

CREATE TABLE account_mappings (
    id BIGSERIAL PRIMARY KEY,
    module TEXT NOT NULL,
    key TEXT NOT NULL,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_account_mappings UNIQUE (module, key)
);
CREATE INDEX idx_account_mappings_account ON account_mappings(account_id);

CREATE MATERIALIZED VIEW gl_balances AS
SELECT
    a.id AS account_id,
    p.id AS period_id,
    COALESCE((
        SELECT SUM(jl.debit - jl.credit)
        FROM journal_lines jl
        JOIN journal_entries je ON je.id = jl.je_id
        WHERE jl.account_id = a.id
          AND je.date < p.start_date
          AND je.status = 'POSTED'
    ), 0) AS opening,
    COALESCE((
        SELECT SUM(jl.debit)
        FROM journal_lines jl
        JOIN journal_entries je ON je.id = jl.je_id
        WHERE jl.account_id = a.id
          AND je.period_id = p.id
          AND je.status = 'POSTED'
    ), 0) AS debit,
    COALESCE((
        SELECT SUM(jl.credit)
        FROM journal_lines jl
        JOIN journal_entries je ON je.id = jl.je_id
        WHERE jl.account_id = a.id
          AND je.period_id = p.id
          AND je.status = 'POSTED'
    ), 0) AS credit,
    COALESCE((
        SELECT SUM(jl.debit - jl.credit)
        FROM journal_lines jl
        JOIN journal_entries je ON je.id = jl.je_id
        WHERE jl.account_id = a.id
          AND je.date <= p.end_date
          AND je.status = 'POSTED'
    ), 0) AS closing
FROM accounts a
CROSS JOIN periods p
ORDER BY a.code, p.start_date;

CREATE INDEX ON gl_balances(account_id);
CREATE INDEX ON gl_balances(period_id);
