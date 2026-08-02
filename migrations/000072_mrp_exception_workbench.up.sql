CREATE TABLE IF NOT EXISTS mrp_exceptions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    fingerprint TEXT NOT NULL,
    exception_type TEXT NOT NULL CHECK(exception_type IN ('MATERIAL_SHORTAGE','LATE_SUPPLY','LATE_WORK_ORDER','MISSING_CAPACITY','EXCESS_RECOMMENDATION','INVALID_MASTER_DATA')),
    severity TEXT NOT NULL CHECK(severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
    product_id BIGINT REFERENCES products(id) ON DELETE RESTRICT,
    warehouse_id BIGINT REFERENCES warehouses(id) ON DELETE RESTRICT,
    work_order_id BIGINT REFERENCES mrp_work_orders(id) ON DELETE RESTRICT,
    operation_id BIGINT REFERENCES mrp_work_order_operations(id) ON DELETE RESTRICT,
    recommendation_id BIGINT REFERENCES mrp_planning_recommendations(id) ON DELETE RESTRICT,
    due_date DATE,
    explanation JSONB NOT NULL DEFAULT '{}'::JSONB,
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK(status IN ('OPEN','ASSIGNED','RESOLVED','DISMISSED')),
    owner_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    resolved_action TEXT,
    comment TEXT NOT NULL DEFAULT '',
    resolved_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mrp_exceptions_open_fingerprint ON mrp_exceptions(company_id,fingerprint) WHERE status IN ('OPEN','ASSIGNED');
CREATE INDEX IF NOT EXISTS idx_mrp_exceptions_workbench ON mrp_exceptions(company_id,status,severity,due_date);
