CREATE TABLE bank_connections (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    connection_ref TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE', -- ACTIVE, DISCONNECTED, REQUIRES_CONSENT
    consent_expires_at TIMESTAMPTZ,
    health_status TEXT NOT NULL DEFAULT 'HEALTHY',
    error_details TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, provider_id, connection_ref)
);

CREATE TABLE bank_connection_accounts (
    id BIGSERIAL PRIMARY KEY,
    connection_id BIGINT NOT NULL REFERENCES bank_connections(id) ON DELETE CASCADE,
    bank_account_id BIGINT NOT NULL REFERENCES bank_accounts(id) ON DELETE CASCADE,
    external_account_id TEXT NOT NULL,
    cursor TEXT,
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(connection_id, external_account_id)
);

CREATE TABLE bank_feed_sync_runs (
    id BIGSERIAL PRIMARY KEY,
    connection_id BIGINT NOT NULL REFERENCES bank_connections(id) ON DELETE CASCADE,
    status TEXT NOT NULL, -- PENDING, COMPLETED, FAILED
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    error_details TEXT
);

CREATE TABLE bank_feed_events (
    id BIGSERIAL PRIMARY KEY,
    provider_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING', -- PENDING, PROCESSED, FAILED
    error_details TEXT,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bank_feed_events_status ON bank_feed_events(status);
