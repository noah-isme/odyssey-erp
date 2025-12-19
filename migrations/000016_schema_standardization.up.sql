-- Migration 016: Schema Standardization
-- Fix type inconsistencies and add missing timestamps

-- ============================================================================
-- ADD MISSING TIMESTAMPS
-- ============================================================================

-- branches
ALTER TABLE branches
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- warehouses
ALTER TABLE warehouses
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- units
ALTER TABLE units
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- categories
ALTER TABLE categories
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- ============================================================================
-- FIX ID TYPES: SERIAL → BIGSERIAL (via ALTER SEQUENCE)
-- PostgreSQL does not allow direct ALTER COLUMN TYPE for serial.
-- We change the sequence to BIGINT and alter the column type.
-- ============================================================================

-- companies
ALTER TABLE companies ALTER COLUMN id TYPE BIGINT;
ALTER SEQUENCE companies_id_seq AS BIGINT;

-- branches
ALTER TABLE branches ALTER COLUMN id TYPE BIGINT;
ALTER SEQUENCE branches_id_seq AS BIGINT;
ALTER TABLE branches ALTER COLUMN company_id TYPE BIGINT;

-- warehouses
ALTER TABLE warehouses ALTER COLUMN id TYPE BIGINT;
ALTER SEQUENCE warehouses_id_seq AS BIGINT;
ALTER TABLE warehouses ALTER COLUMN branch_id TYPE BIGINT;

-- units
ALTER TABLE units ALTER COLUMN id TYPE BIGINT;
ALTER SEQUENCE units_id_seq AS BIGINT;

-- taxes
ALTER TABLE taxes ALTER COLUMN id TYPE BIGINT;
ALTER SEQUENCE taxes_id_seq AS BIGINT;

-- categories
ALTER TABLE categories ALTER COLUMN id TYPE BIGINT;
ALTER SEQUENCE categories_id_seq AS BIGINT;
ALTER TABLE categories ALTER COLUMN parent_id TYPE BIGINT;

-- suppliers
ALTER TABLE suppliers ALTER COLUMN id TYPE BIGINT;
ALTER SEQUENCE suppliers_id_seq AS BIGINT;

-- products
ALTER TABLE products ALTER COLUMN id TYPE BIGINT;
ALTER SEQUENCE products_id_seq AS BIGINT;
ALTER TABLE products ALTER COLUMN category_id TYPE BIGINT;
ALTER TABLE products ALTER COLUMN unit_id TYPE BIGINT;
ALTER TABLE products ALTER COLUMN tax_id TYPE BIGINT;

-- ============================================================================
-- FIX FK REFERENCES IN OTHER TABLES
-- ============================================================================

-- inventory_tx
ALTER TABLE inventory_tx ALTER COLUMN warehouse_id TYPE BIGINT;

-- inventory_tx_lines
ALTER TABLE inventory_tx_lines ALTER COLUMN product_id TYPE BIGINT;
ALTER TABLE inventory_tx_lines ALTER COLUMN src_warehouse_id TYPE BIGINT;
ALTER TABLE inventory_tx_lines ALTER COLUMN dst_warehouse_id TYPE BIGINT;

-- inventory_balances
ALTER TABLE inventory_balances ALTER COLUMN warehouse_id TYPE BIGINT;
ALTER TABLE inventory_balances ALTER COLUMN product_id TYPE BIGINT;

-- inventory_cards
ALTER TABLE inventory_cards ALTER COLUMN warehouse_id TYPE BIGINT;
ALTER TABLE inventory_cards ALTER COLUMN product_id TYPE BIGINT;

-- prs (purchase requisitions)
ALTER TABLE prs ALTER COLUMN supplier_id TYPE BIGINT;

-- pr_lines
ALTER TABLE pr_lines ALTER COLUMN product_id TYPE BIGINT;

-- pos (purchase orders)
ALTER TABLE pos ALTER COLUMN supplier_id TYPE BIGINT;

-- po_lines
ALTER TABLE po_lines ALTER COLUMN product_id TYPE BIGINT;
ALTER TABLE po_lines ALTER COLUMN tax_id TYPE BIGINT;

-- grns
ALTER TABLE grns ALTER COLUMN supplier_id TYPE BIGINT;
ALTER TABLE grns ALTER COLUMN warehouse_id TYPE BIGINT;

-- grn_lines
ALTER TABLE grn_lines ALTER COLUMN product_id TYPE BIGINT;

-- ap_invoices
ALTER TABLE ap_invoices ALTER COLUMN supplier_id TYPE BIGINT;

-- quotation_lines
ALTER TABLE quotation_lines ALTER COLUMN product_id TYPE BIGINT;

-- sales_order_lines
ALTER TABLE sales_order_lines ALTER COLUMN product_id TYPE BIGINT;

-- delivery_order_lines
ALTER TABLE delivery_order_lines ALTER COLUMN product_id TYPE BIGINT;

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON COLUMN companies.id IS 'Primary key (BIGINT for consistency)';
COMMENT ON COLUMN branches.id IS 'Primary key (BIGINT for consistency)';
COMMENT ON COLUMN warehouses.id IS 'Primary key (BIGINT for consistency)';
