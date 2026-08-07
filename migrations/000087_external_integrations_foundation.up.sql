CREATE TABLE connector_connections (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    provider VARCHAR(100) NOT NULL,
    type VARCHAR(100) NOT NULL,
    name VARCHAR(255) NOT NULL,
    secret_ref VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'disabled',
    last_sync TIMESTAMPTZ,
    last_error TEXT,
    token_expiry TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_connector_connections_company_id ON connector_connections(company_id);

CREATE TABLE connector_outbox_commands (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    connection_id BIGINT NOT NULL REFERENCES connector_connections(id),
    command_type VARCHAR(100) NOT NULL,
    correlation_id VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    state VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    next_attempt TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_connector_outbox_company_state ON connector_outbox_commands(company_id, state);
CREATE INDEX idx_connector_outbox_next_attempt ON connector_outbox_commands(next_attempt) WHERE state IN ('pending', 'processing');
CREATE UNIQUE INDEX idx_connector_outbox_correlation ON connector_outbox_commands(connection_id, correlation_id);

CREATE TABLE connector_inbox_events (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    connection_id BIGINT NOT NULL REFERENCES connector_connections(id),
    provider_event_id VARCHAR(255) NOT NULL,
    raw_payload JSONB NOT NULL,
    processed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_connector_inbox_dedupe ON connector_inbox_events(connection_id, provider_event_id);
CREATE INDEX idx_connector_inbox_unprocessed ON connector_inbox_events(company_id, connection_id) WHERE processed = false;

CREATE TABLE connector_canonical_events (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    connection_id BIGINT NOT NULL REFERENCES connector_connections(id),
    event_type VARCHAR(100) NOT NULL,
    event_time TIMESTAMPTZ NOT NULL,
    correlation_id VARCHAR(255) NOT NULL,
    causation_id VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_connector_canonical_company ON connector_canonical_events(company_id, event_time);
