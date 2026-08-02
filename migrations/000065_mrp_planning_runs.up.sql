CREATE TABLE IF NOT EXISTS mrp_product_planning_policies (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    order_type TEXT NOT NULL CHECK (order_type IN ('BUY','MAKE')),
    lead_days INT NOT NULL DEFAULT 0 CHECK (lead_days >= 0),
    safety_stock NUMERIC(18,4) NOT NULL DEFAULT 0 CHECK (safety_stock >= 0),
    lot_sizing TEXT NOT NULL DEFAULT 'LOT_FOR_LOT' CHECK (lot_sizing IN ('LOT_FOR_LOT','MINIMUM','FIXED','MULTIPLE')),
    lot_quantity NUMERIC(18,4) NOT NULL DEFAULT 0 CHECK (lot_quantity >= 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, product_id, warehouse_id)
);

CREATE TABLE IF NOT EXISTS mrp_planning_runs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    as_of_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'COMPLETED' CHECK (status IN ('COMPLETED','FAILED')),
    input_snapshot JSONB NOT NULL,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS mrp_planning_recommendations (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES mrp_planning_runs(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    order_type TEXT NOT NULL CHECK (order_type IN ('BUY','MAKE')),
    quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
    release_date DATE NOT NULL,
    due_date DATE NOT NULL,
    demand_source_ref TEXT NOT NULL,
    late BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL DEFAULT 'PLANNED' CHECK (status IN ('PLANNED','FIRMED','DISMISSED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_mrp_planning_policies_company_active
    ON mrp_product_planning_policies(company_id, active);
CREATE INDEX IF NOT EXISTS idx_mrp_planning_runs_company_created
    ON mrp_planning_runs(company_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mrp_planning_recommendations_run_status
    ON mrp_planning_recommendations(run_id, status);
