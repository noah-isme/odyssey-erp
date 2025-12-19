-- Rollback Migration 017: Multi-tenancy Support

-- Drop indexes
DROP INDEX IF EXISTS idx_suppliers_company;
DROP INDEX IF EXISTS idx_products_company;
DROP INDEX IF EXISTS idx_categories_company;
DROP INDEX IF EXISTS idx_prs_company;
DROP INDEX IF EXISTS idx_pos_company;
DROP INDEX IF EXISTS idx_grns_company;
DROP INDEX IF EXISTS idx_ap_invoices_company;
DROP INDEX IF EXISTS idx_inventory_tx_company;
DROP INDEX IF EXISTS idx_accounts_company;
DROP INDEX IF EXISTS idx_account_mappings_company;

-- Drop columns
ALTER TABLE suppliers DROP COLUMN IF EXISTS company_id;
ALTER TABLE products DROP COLUMN IF EXISTS company_id;
ALTER TABLE categories DROP COLUMN IF EXISTS company_id;
ALTER TABLE prs DROP COLUMN IF EXISTS company_id;
ALTER TABLE pos DROP COLUMN IF EXISTS company_id;
ALTER TABLE grns DROP COLUMN IF EXISTS company_id;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS company_id;
ALTER TABLE inventory_tx DROP COLUMN IF EXISTS company_id;
ALTER TABLE accounts DROP COLUMN IF EXISTS company_id;
ALTER TABLE account_mappings DROP COLUMN IF EXISTS company_id;
