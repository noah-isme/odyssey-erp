-- Durable idempotency records for production receipts. The event and all
-- related inventory movements are committed in the same transaction.
CREATE TABLE IF NOT EXISTS mrp_production_events (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    work_order_id BIGINT NOT NULL REFERENCES mrp_work_orders(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL CHECK (event_type IN ('COMPLETE')),
    idempotency_key TEXT NOT NULL,
    quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
    response JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, work_order_id, event_type, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_mrp_production_events_work_order
    ON mrp_production_events(company_id, work_order_id, created_at);
