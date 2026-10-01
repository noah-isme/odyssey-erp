-- Operational payment reconciliation evidence, alerts, and connector
-- dead-letter records. These tables are durable so a worker restart cannot
-- erase the recovery history operators need to certify a payment flow.
CREATE TABLE payment_reconciliation_runs (
    id BIGSERIAL PRIMARY KEY,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(30) NOT NULL CHECK (status IN ('SUCCESS', 'PARTIAL', 'FAILED')),
    scanned_count INT NOT NULL DEFAULT 0 CHECK (scanned_count >= 0),
    recovered_count INT NOT NULL DEFAULT 0 CHECK (recovered_count >= 0),
    matched_count INT NOT NULL DEFAULT 0 CHECK (matched_count >= 0),
    unmatched_count INT NOT NULL DEFAULT 0 CHECK (unmatched_count >= 0),
    unsupported_count INT NOT NULL DEFAULT 0 CHECK (unsupported_count >= 0),
    error_count INT NOT NULL DEFAULT 0 CHECK (error_count >= 0),
    refunds_persisted INT NOT NULL DEFAULT 0 CHECK (refunds_persisted >= 0),
    dead_letter_count INT NOT NULL DEFAULT 0 CHECK (dead_letter_count >= 0),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payment_reconciliation_runs_started
    ON payment_reconciliation_runs(started_at DESC);

CREATE TABLE payment_reconciliation_issues (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES connector_connections(id) ON DELETE CASCADE,
    payment_intent_id BIGINT REFERENCES payment_intents(id) ON DELETE CASCADE,
    provider VARCHAR(100) NOT NULL,
    provider_reference VARCHAR(255) NOT NULL,
    issue_type VARCHAR(100) NOT NULL,
    expected_status VARCHAR(50),
    observed_status VARCHAR(100),
    details TEXT NOT NULL DEFAULT '',
    status VARCHAR(30) NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'RESOLVED')),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    alerted_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, connection_id, provider_reference, issue_type)
);

CREATE INDEX idx_payment_reconciliation_issues_open
    ON payment_reconciliation_issues(company_id, last_seen_at DESC)
    WHERE status = 'OPEN';

CREATE TABLE connector_dead_letter_events (
    id BIGSERIAL PRIMARY KEY,
    command_id BIGINT NOT NULL REFERENCES connector_outbox_commands(id) ON DELETE CASCADE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES connector_connections(id) ON DELETE CASCADE,
    command_type VARCHAR(100) NOT NULL,
    correlation_id VARCHAR(255) NOT NULL,
    attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    error_message TEXT NOT NULL,
    dead_lettered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    alerted_at TIMESTAMPTZ,
    replayed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(command_id)
);

CREATE INDEX idx_connector_dead_letters_open
    ON connector_dead_letter_events(company_id, dead_lettered_at DESC)
    WHERE replayed_at IS NULL;
