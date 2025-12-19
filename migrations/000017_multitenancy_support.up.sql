-- Migration 017: Multi-tenancy Support
-- Add company_id dimension to tables that need tenant isolation

-- ============================================================================
-- MASTER DATA TABLES
-- ============================================================================

-- suppliers (currently global, should be per-company)
ALTER TABLE suppliers
    ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE;

-- products (currently global, should be per-company)
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE;

-- categories (currently global, should be per-company)  
ALTER TABLE categories
    ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE;

-- units (keep global - shared across companies)
-- taxes (keep global - shared across companies)

-- ============================================================================
-- PROCUREMENT TABLES
-- ============================================================================

-- prs (purchase requisitions)
ALTER TABLE prs
    ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE;

-- pos (purchase orders)
ALTER TABLE pos
    ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE;

-- grns (goods receipt notes)
ALTER TABLE grns
    ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE;

-- ap_invoices
ALTER TABLE ap_invoices
    ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE;

-- ============================================================================
-- INVENTORY TABLES
-- ============================================================================

-- inventory_tx
ALTER TABLE inventory_tx
    ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE;

-- ============================================================================
-- ACCOUNTING TABLES
-- ============================================================================

-- accounts (Chart of Accounts per company)
ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE;

-- account_mappings
ALTER TABLE account_mappings
    ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE;

-- ============================================================================
-- CREATE INDEXES FOR MULTI-TENANT QUERIES
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_suppliers_company ON suppliers(company_id);
CREATE INDEX IF NOT EXISTS idx_products_company ON products(company_id);
CREATE INDEX IF NOT EXISTS idx_categories_company ON categories(company_id);
CREATE INDEX IF NOT EXISTS idx_prs_company ON prs(company_id);
CREATE INDEX IF NOT EXISTS idx_pos_company ON pos(company_id);
CREATE INDEX IF NOT EXISTS idx_grns_company ON grns(company_id);
CREATE INDEX IF NOT EXISTS idx_ap_invoices_company ON ap_invoices(company_id);
CREATE INDEX IF NOT EXISTS idx_inventory_tx_company ON inventory_tx(company_id);
CREATE INDEX IF NOT EXISTS idx_accounts_company ON accounts(company_id);
CREATE INDEX IF NOT EXISTS idx_account_mappings_company ON account_mappings(company_id);

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON COLUMN suppliers.company_id IS 'Tenant isolation: company that owns this supplier';
COMMENT ON COLUMN products.company_id IS 'Tenant isolation: company that owns this product';
COMMENT ON COLUMN accounts.company_id IS 'Tenant isolation: company-specific Chart of Accounts';
