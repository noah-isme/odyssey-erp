-- Phase 7 consolidation core schema and RBAC additions

CREATE TABLE consol_groups (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    reporting_currency TEXT NOT NULL,
    fx_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE consol_group_accounts (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES consol_groups(id) ON DELETE CASCADE,
    code VARCHAR(20) NOT NULL,
    name VARCHAR(120) NOT NULL,
    type account_type NOT NULL,
    parent_id BIGINT REFERENCES consol_group_accounts(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_consol_group_accounts UNIQUE (group_id, code)
);
CREATE INDEX idx_consol_group_accounts_group ON consol_group_accounts(group_id);

CREATE TABLE consol_members (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES consol_groups(id) ON DELETE CASCADE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_consol_members UNIQUE (group_id, company_id)
);
CREATE INDEX idx_consol_members_group ON consol_members(group_id);
CREATE INDEX idx_consol_members_company ON consol_members(company_id);

CREATE TABLE account_map (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES consol_groups(id) ON DELETE CASCADE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    local_account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    group_account_id BIGINT NOT NULL REFERENCES consol_group_accounts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_account_map UNIQUE (group_id, company_id, local_account_id)
);
CREATE INDEX idx_account_map_group_account ON account_map(group_account_id);

CREATE TABLE ic_rules (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES consol_groups(id) ON DELETE CASCADE,
    src_group_acc BIGINT NOT NULL REFERENCES consol_group_accounts(id) ON DELETE CASCADE,
    dst_group_acc BIGINT NOT NULL REFERENCES consol_group_accounts(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('AR_AP', 'REV_COGS')),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ic_rules UNIQUE (group_id, src_group_acc, dst_group_acc, type)
);

CREATE TABLE elimination_journal_headers (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES consol_groups(id) ON DELETE CASCADE,
    period_id BIGINT NOT NULL REFERENCES periods(id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('DRAFT', 'POSTED')),
    source_link TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    posted_at TIMESTAMPTZ
);
CREATE INDEX idx_elimination_headers_group_period ON elimination_journal_headers(group_id, period_id);
CREATE UNIQUE INDEX idx_elimination_headers_source ON elimination_journal_headers(group_id, period_id, source_link) WHERE source_link IS NOT NULL;

CREATE TABLE elimination_journal_lines (
    id BIGSERIAL PRIMARY KEY,
    header_id BIGINT NOT NULL REFERENCES elimination_journal_headers(id) ON DELETE CASCADE,
    group_account_id BIGINT NOT NULL REFERENCES consol_group_accounts(id) ON DELETE RESTRICT,
    debit NUMERIC(14,2) NOT NULL DEFAULT 0,
    credit NUMERIC(14,2) NOT NULL DEFAULT 0,
    memo TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_elimination_journal_lines_positive CHECK (debit >= 0 AND credit >= 0)
);
CREATE INDEX idx_elimination_lines_header ON elimination_journal_lines(header_id);
CREATE INDEX idx_elimination_lines_group_account ON elimination_journal_lines(group_account_id);

CREATE TABLE fx_rates (
    as_of_date DATE NOT NULL,
    pair TEXT NOT NULL,
    average_rate NUMERIC(18,6) NOT NULL CHECK (average_rate > 0),
    closing_rate NUMERIC(18,6) NOT NULL CHECK (closing_rate > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (as_of_date, pair)
);

CREATE TABLE fx_policy (
    group_account_id BIGINT PRIMARY KEY REFERENCES consol_group_accounts(id) ON DELETE CASCADE,
    method TEXT NOT NULL CHECK (method IN ('AVERAGE', 'CLOSING')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS ic_flag BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_accounts_ic_flag ON accounts(ic_flag);

ALTER TABLE journal_lines
    ADD COLUMN IF NOT EXISTS ic_party_id BIGINT REFERENCES companies(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_journal_lines_ic_party ON journal_lines(ic_party_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum e
        JOIN pg_type t ON t.oid = e.enumtypid
        WHERE t.typname = 'period_status' AND e.enumlabel = 'OPEN_CONSOL'
    ) THEN
        ALTER TYPE period_status ADD VALUE 'OPEN_CONSOL';
    END IF;
END$$;

INSERT INTO permissions (name, description)
VALUES
    ('finance.view_consolidation', 'View consolidated ledger and reports'),
    ('finance.post_elimination', 'Post consolidation elimination journals'),
    ('finance.manage_consolidation', 'Manage consolidation configuration and refresh'),
    ('finance.export_consolidation', 'Export consolidated reports')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

CREATE MATERIALIZED VIEW mv_consol_balances AS
WITH base AS (
    SELECT
        je.period_id,
        cm.group_id,
        am.group_account_id,
        cm.company_id,
        SUM(jl.debit - jl.credit) AS local_amt
    FROM journal_lines jl
    JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
    JOIN consol_members cm ON cm.company_id = jl.dim_company_id AND cm.enabled
    JOIN account_map am ON am.group_id = cm.group_id
        AND am.company_id = cm.company_id
        AND am.local_account_id = jl.account_id
    GROUP BY je.period_id, cm.group_id, am.group_account_id, cm.company_id
)
SELECT
    b.period_id,
    b.group_id,
    b.group_account_id,
    SUM(b.local_amt) AS local_ccy_amt,
    SUM(b.local_amt) AS group_ccy_amt,
    jsonb_agg(
        jsonb_build_object(
            'company_id', b.company_id,
            'local_ccy_amt', b.local_amt
        )
        ORDER BY b.company_id
    ) AS members
FROM base b
GROUP BY b.period_id, b.group_id, b.group_account_id;

CREATE INDEX idx_mv_consol_balances_period ON mv_consol_balances(period_id);
CREATE INDEX idx_mv_consol_balances_group ON mv_consol_balances(group_id);
