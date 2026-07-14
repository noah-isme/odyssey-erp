-- Migration 016: Schema Standardization
-- Fix type inconsistencies and add missing timestamps

-- This view depends on warehouses.id, whose type is widened below.
DROP VIEW IF EXISTS vw_delivery_orders_detail;

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

CREATE VIEW vw_delivery_orders_detail AS
SELECT
    dord.id,
    dord.doc_number,
    dord.company_id,
    dord.sales_order_id,
    so.doc_number AS sales_order_number,
    dord.warehouse_id,
    w.name AS warehouse_name,
    dord.customer_id,
    c.name AS customer_name,
    dord.delivery_date,
    dord.status,
    dord.driver_name,
    dord.vehicle_number,
    dord.tracking_number,
    dord.notes,
    dord.created_by,
    u_created.email AS created_by_name,
    dord.confirmed_by,
    u_confirmed.email AS confirmed_by_name,
    dord.confirmed_at,
    dord.delivered_at,
    dord.created_at,
    dord.updated_at,
    COUNT(dol.id) AS line_count,
    SUM(dol.quantity_to_deliver) AS total_quantity
FROM delivery_orders dord
INNER JOIN sales_orders so ON so.id = dord.sales_order_id
INNER JOIN warehouses w ON w.id = dord.warehouse_id
INNER JOIN customers c ON c.id = dord.customer_id
INNER JOIN users u_created ON u_created.id = dord.created_by
LEFT JOIN users u_confirmed ON u_confirmed.id = dord.confirmed_by
LEFT JOIN delivery_order_lines dol ON dol.delivery_order_id = dord.id
GROUP BY
    dord.id, dord.doc_number, dord.company_id, dord.sales_order_id, so.doc_number,
    dord.warehouse_id, w.name, dord.customer_id, c.name, dord.delivery_date,
    dord.status, dord.driver_name, dord.vehicle_number, dord.tracking_number,
    dord.notes, dord.created_by, u_created.email, dord.confirmed_by,
    u_confirmed.email, dord.confirmed_at, dord.delivered_at,
    dord.created_at, dord.updated_at;

COMMENT ON VIEW vw_delivery_orders_detail IS
'Enriched view of delivery orders with related entity details';
