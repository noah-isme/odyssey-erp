CREATE TABLE IF NOT EXISTS mrp_inspection_plans (
 id BIGSERIAL PRIMARY KEY, company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
 product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
 routing_operation_id BIGINT REFERENCES mrp_routing_operations(id) ON DELETE CASCADE,
 name TEXT NOT NULL, required BOOLEAN NOT NULL DEFAULT TRUE, active BOOLEAN NOT NULL DEFAULT TRUE,
 created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE TABLE IF NOT EXISTS mrp_inspections (
 id BIGSERIAL PRIMARY KEY, company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
 plan_id BIGINT REFERENCES mrp_inspection_plans(id) ON DELETE RESTRICT, work_order_id BIGINT NOT NULL REFERENCES mrp_work_orders(id) ON DELETE RESTRICT,
 operation_id BIGINT REFERENCES mrp_work_order_operations(id) ON DELETE RESTRICT,
 status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','PASSED','FAILED','HOLD','RELEASED')),
 result JSONB NOT NULL DEFAULT '{}'::JSONB, defect_code TEXT, disposition TEXT, inspector_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
 approved_by BIGINT REFERENCES users(id) ON DELETE RESTRICT, approved_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE TABLE IF NOT EXISTS mrp_quality_holds (
 id BIGSERIAL PRIMARY KEY, company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
 work_order_id BIGINT NOT NULL REFERENCES mrp_work_orders(id) ON DELETE RESTRICT, operation_id BIGINT REFERENCES mrp_work_order_operations(id) ON DELETE RESTRICT,
 inspection_id BIGINT REFERENCES mrp_inspections(id) ON DELETE RESTRICT, reason TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'OPEN' CHECK(status IN ('OPEN','RELEASED')),
 created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT, released_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,released_at TIMESTAMPTZ,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE TABLE IF NOT EXISTS mrp_nonconformances (id BIGSERIAL PRIMARY KEY,company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,inspection_id BIGINT REFERENCES mrp_inspections(id) ON DELETE RESTRICT,number TEXT NOT NULL,description TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'OPEN',owner_id BIGINT REFERENCES users(id),created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),UNIQUE(company_id,number));
CREATE TABLE IF NOT EXISTS mrp_capas (id BIGSERIAL PRIMARY KEY,company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,ncr_id BIGINT REFERENCES mrp_nonconformances(id) ON DELETE RESTRICT,action TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'OPEN',owner_id BIGINT REFERENCES users(id),due_date DATE,closed_by BIGINT REFERENCES users(id),closed_at TIMESTAMPTZ);
CREATE TABLE IF NOT EXISTS mrp_genealogy (id BIGSERIAL PRIMARY KEY,company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,work_order_id BIGINT NOT NULL REFERENCES mrp_work_orders(id) ON DELETE RESTRICT,operation_id BIGINT REFERENCES mrp_work_order_operations(id) ON DELETE RESTRICT,component_product_id BIGINT REFERENCES products(id),consumed_lot_id BIGINT REFERENCES inventory_lots(id),consumed_serial_id BIGINT REFERENCES inventory_serials(id),produced_lot_id BIGINT REFERENCES inventory_lots(id),produced_serial_id BIGINT REFERENCES inventory_serials(id),quantity NUMERIC(18,4) NOT NULL DEFAULT 0,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE TABLE IF NOT EXISTS mrp_subcontract_operations (id BIGSERIAL PRIMARY KEY,company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,operation_id BIGINT NOT NULL REFERENCES mrp_work_order_operations(id) ON DELETE RESTRICT,supplier_id BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,status TEXT NOT NULL DEFAULT 'SENT' CHECK(status IN ('SENT','RECEIVED','INSPECTING','CLOSED')),sent_quantity NUMERIC(18,4) NOT NULL, sent_cost NUMERIC(18,4) NOT NULL DEFAULT 0,sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),received_quantity NUMERIC(18,4) NOT NULL DEFAULT 0,received_at TIMESTAMPTZ,inspection_id BIGINT REFERENCES mrp_inspections(id));
CREATE INDEX IF NOT EXISTS idx_mrp_quality_holds_open ON mrp_quality_holds(company_id,work_order_id) WHERE status='OPEN';
