CREATE TYPE bank_statement_status AS ENUM ('DRAFT', 'RECONCILED');
CREATE TYPE bank_line_status AS ENUM ('UNMATCHED', 'SUGGESTED', 'MATCHED');

CREATE TABLE bank_statements (
    id BIGSERIAL PRIMARY KEY,
    bank_account_id BIGINT NOT NULL REFERENCES bank_accounts(id) ON DELETE CASCADE,
    statement_date DATE NOT NULL,
    starting_balance NUMERIC(14,2) NOT NULL DEFAULT 0,
    ending_balance NUMERIC(14,2) NOT NULL DEFAULT 0,
    status bank_statement_status NOT NULL DEFAULT 'DRAFT',
    imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE bank_statement_lines (
    id BIGSERIAL PRIMARY KEY,
    statement_id BIGINT NOT NULL REFERENCES bank_statements(id) ON DELETE CASCADE,
    trx_date DATE NOT NULL,
    description TEXT NOT NULL,
    amount NUMERIC(14,2) NOT NULL,
    reference_number VARCHAR(100),
    status bank_line_status NOT NULL DEFAULT 'UNMATCHED',
    matched_doc_type VARCHAR(50),
    matched_doc_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bank_statement_lines_stmt ON bank_statement_lines(statement_id);
CREATE INDEX idx_bank_statement_lines_status ON bank_statement_lines(status);
