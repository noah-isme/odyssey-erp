-- Durable provider-neutral payment execution snapshots. The JSONB payload is
-- the complete coordinator record; the duplicated columns are the canonical,
-- company-scoped lookup and optimistic-concurrency boundary.
CREATE TABLE payment_executions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES connector_connections(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN (
        'PROPOSED', 'APPROVED', 'SUBMITTED', 'EXPORTED', 'AMBIGUOUS',
        'PARTIALLY_SETTLED', 'SETTLED', 'CANCELLED', 'FAILED'
    )),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_executions_reference_not_blank CHECK (
        btrim(provider) <> '' AND btrim(object_type) <> '' AND btrim(object_id) <> ''
    ),
    CONSTRAINT payment_executions_reference_key
        UNIQUE (company_id, connection_id, provider, object_type, object_id)
);

CREATE INDEX idx_payment_executions_company_state
    ON payment_executions (company_id, state, updated_at DESC);
