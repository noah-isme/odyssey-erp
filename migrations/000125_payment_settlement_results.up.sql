-- Durable bank-file/provider result inbox and idempotency boundary.
-- Financial effects are applied by the application settlement-effects port;
-- this table records the immutable result before and across worker retries.
CREATE TABLE payment_settlement_results (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    result_id TEXT NOT NULL,
    connection_id BIGINT NOT NULL REFERENCES connector_connections(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    instruction_type TEXT NOT NULL,
    instruction_id TEXT NOT NULL,
    provider_object_type TEXT NOT NULL DEFAULT '',
    provider_object_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('PARTIALLY_SETTLED', 'SETTLED', 'FAILED', 'CANCELLED')),
    state TEXT NOT NULL CHECK (state IN ('PARTIALLY_SETTLED', 'SETTLED', 'FAILED', 'CANCELLED')),
    effect_applied BOOLEAN NOT NULL DEFAULT FALSE,
    fingerprint TEXT NOT NULL,
    payload JSONB NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_settlement_results_reference_not_blank CHECK (
        btrim(result_id) <> '' AND btrim(provider) <> '' AND
        btrim(instruction_type) <> '' AND btrim(instruction_id) <> '' AND
        btrim(fingerprint) <> ''
    ),
    CONSTRAINT payment_settlement_results_result_key UNIQUE (company_id, result_id)
);

CREATE INDEX idx_payment_settlement_results_company_status
    ON payment_settlement_results (company_id, state, recorded_at DESC);
CREATE INDEX idx_payment_settlement_results_instruction
    ON payment_settlement_results (company_id, connection_id, instruction_type, instruction_id);

-- The effect key is independent from the provider result key so one result
-- can be joined to the exact AP/GL/bank/reconciliation mutation set. The
-- application writes this row in the same transaction as those effects.
CREATE TABLE payment_settlement_effects (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    effect_key TEXT NOT NULL,
    result_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('PARTIALLY_SETTLED', 'SETTLED')),
    fingerprint TEXT NOT NULL,
    payload JSONB NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_settlement_effects_key UNIQUE (company_id, effect_key),
    CONSTRAINT payment_settlement_effects_fingerprint_not_blank CHECK (btrim(fingerprint) <> '')
);

CREATE INDEX idx_payment_settlement_effects_result
    ON payment_settlement_effects (company_id, result_id);
