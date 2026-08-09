DROP INDEX IF EXISTS idx_cost_centers_company_active;

ALTER TABLE cost_centers
    DROP COLUMN IF EXISTS manager_id,
    DROP COLUMN IF EXISTS gl_account,
    DROP COLUMN IF EXISTS warehouse_id,
    DROP COLUMN IF EXISTS cost_center_type;
