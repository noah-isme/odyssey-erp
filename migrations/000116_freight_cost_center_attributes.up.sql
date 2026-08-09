-- Freight cost centers use the shared accounting dimension table, but freight
-- posting also needs an account and operational ownership metadata.
ALTER TABLE cost_centers
    ADD COLUMN IF NOT EXISTS cost_center_type TEXT NOT NULL DEFAULT 'DEPARTMENT'
        CHECK (cost_center_type IN ('WAREHOUSE', 'DEPARTMENT', 'PROJECT', 'LOCATION')),
    ADD COLUMN IF NOT EXISTS warehouse_id BIGINT REFERENCES warehouses(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS gl_account TEXT,
    ADD COLUMN IF NOT EXISTS manager_id BIGINT REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_cost_centers_company_active
    ON cost_centers(company_id, is_active);
