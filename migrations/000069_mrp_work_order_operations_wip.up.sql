ALTER TABLE mrp_work_orders
    ADD COLUMN IF NOT EXISTS routing_id BIGINT REFERENCES mrp_routings(id) ON DELETE RESTRICT;

CREATE TABLE IF NOT EXISTS mrp_work_order_operations (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    work_order_id BIGINT NOT NULL REFERENCES mrp_work_orders(id) ON DELETE CASCADE,
    routing_operation_id BIGINT REFERENCES mrp_routing_operations(id) ON DELETE SET NULL,
    work_center_id BIGINT NOT NULL REFERENCES mrp_work_centers(id) ON DELETE RESTRICT,
    sequence INT NOT NULL CHECK (sequence > 0),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','READY','IN_PROGRESS','COMPLETED','BLOCKED')),
    planned_setup_minutes NUMERIC(10,2) NOT NULL DEFAULT 0,
    planned_run_minutes NUMERIC(10,2) NOT NULL DEFAULT 0,
    actual_setup_minutes NUMERIC(10,2) NOT NULL DEFAULT 0,
    actual_run_minutes NUMERIC(10,2) NOT NULL DEFAULT 0,
    good_quantity NUMERIC(18,4) NOT NULL DEFAULT 0,
    scrap_quantity NUMERIC(18,4) NOT NULL DEFAULT 0,
    operator_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (work_order_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_mrp_work_order_operations_dispatch
    ON mrp_work_order_operations(company_id, work_center_id, status, sequence);

CREATE TABLE IF NOT EXISTS mrp_wip_locations (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    wip_warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    work_center_id BIGINT REFERENCES mrp_work_centers(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    UNIQUE(company_id, warehouse_id, work_center_id),
    CHECK (warehouse_id <> wip_warehouse_id)
);

CREATE TABLE IF NOT EXISTS mrp_material_movements (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    work_order_id BIGINT NOT NULL REFERENCES mrp_work_orders(id) ON DELETE RESTRICT,
    operation_id BIGINT REFERENCES mrp_work_order_operations(id) ON DELETE RESTRICT,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    source_warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    destination_warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
    movement_type TEXT NOT NULL CHECK (movement_type IN ('ISSUE','RETURN')),
    idempotency_key TEXT NOT NULL,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, work_order_id, movement_type, idempotency_key)
);
