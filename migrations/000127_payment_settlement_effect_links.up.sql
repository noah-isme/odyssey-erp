-- Durable links from one confirmed settlement effect to the financial records
-- created by the accounting boundary. The effect claim and these links are
-- inserted in the same transaction as AP/GL/bank mutations by the adapter.
CREATE TABLE payment_settlement_effect_links (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    effect_key TEXT NOT NULL,
    result_id TEXT NOT NULL,
    link_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    amount NUMERIC,
    currency TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_settlement_effect_links_effect_fk
        FOREIGN KEY (company_id, effect_key)
        REFERENCES payment_settlement_effects (company_id, effect_key)
        ON DELETE CASCADE,
    CONSTRAINT payment_settlement_effect_links_identity_not_blank CHECK (
        btrim(effect_key) <> '' AND btrim(result_id) <> '' AND
        btrim(link_type) <> '' AND btrim(entity_type) <> '' AND btrim(entity_id) <> ''
    ),
    CONSTRAINT payment_settlement_effect_links_amount_currency CHECK (
        (amount IS NULL AND currency IS NULL) OR
        (amount IS NOT NULL AND amount > 0 AND currency ~ '^[A-Z]{3}$')
    ),
    CONSTRAINT payment_settlement_effect_links_metadata_object CHECK (
        jsonb_typeof(metadata) = 'object'
    ),
    CONSTRAINT payment_settlement_effect_links_unique UNIQUE (
        company_id, effect_key, link_type, entity_type, entity_id
    )
);

CREATE INDEX idx_payment_settlement_effect_links_result
    ON payment_settlement_effect_links (company_id, result_id);
CREATE INDEX idx_payment_settlement_effect_links_entity
    ON payment_settlement_effect_links (company_id, entity_type, entity_id);
