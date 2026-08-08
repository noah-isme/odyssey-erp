CREATE TABLE payment_intents (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    connection_id BIGINT NOT NULL REFERENCES connector_connections(id),
    source_type VARCHAR(100) NOT NULL, -- e.g., 'ar_invoice', 'pos_order'
    source_id BIGINT NOT NULL,
    amount NUMERIC(18,2) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'CREATED', -- CREATED, PENDING, AUTHORIZED, CAPTURED, SETTLED, EXPIRED, FAILED, CANCELLED, REFUNDED, DISPUTED
    provider_reference VARCHAR(255),
    checkout_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payment_intents_company_status ON payment_intents(company_id, status);
CREATE INDEX idx_payment_intents_provider_ref ON payment_intents(connection_id, provider_reference);

CREATE TABLE payment_intent_transitions (
    id BIGSERIAL PRIMARY KEY,
    payment_intent_id BIGINT NOT NULL REFERENCES payment_intents(id),
    from_status VARCHAR(50) NOT NULL,
    to_status VARCHAR(50) NOT NULL,
    provider_event_id VARCHAR(255),
    occurred_at TIMESTAMPTZ NOT NULL,
    raw_payload JSONB
);

CREATE INDEX idx_payment_intent_transitions_intent ON payment_intent_transitions(payment_intent_id, occurred_at DESC);

CREATE TABLE payment_refunds (
    id BIGSERIAL PRIMARY KEY,
    payment_intent_id BIGINT NOT NULL REFERENCES payment_intents(id),
    amount NUMERIC(18,2) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    reason TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    provider_reference VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE payment_disputes (
    id BIGSERIAL PRIMARY KEY,
    payment_intent_id BIGINT NOT NULL REFERENCES payment_intents(id),
    amount NUMERIC(18,2) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    reason TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'OPEN',
    provider_reference VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
