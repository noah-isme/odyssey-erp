-- Migration Rollback: Phase 9.2 - Delivery Order & Fulfillment
-- Description: Remove delivery order tables, triggers, and functions

-- ============================================================================
-- DROP VIEWS
-- ============================================================================

DROP VIEW IF EXISTS vw_delivery_orders_detail;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS trg_do_line_update_so_status ON delivery_order_lines;
DROP TRIGGER IF EXISTS trg_do_line_update_so_qty ON delivery_order_lines;
DROP TRIGGER IF EXISTS trg_delivery_order_lines_updated_at ON delivery_order_lines;
DROP TRIGGER IF EXISTS trg_delivery_orders_updated_at ON delivery_orders;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS update_sales_order_status_from_delivery();
DROP FUNCTION IF EXISTS update_so_line_quantity_delivered();
DROP FUNCTION IF EXISTS generate_delivery_order_number(BIGINT, DATE);

-- ============================================================================
-- DROP INDEXES
-- ============================================================================

-- Delivery Order Lines indexes
DROP INDEX IF EXISTS idx_delivery_order_lines_product;
DROP INDEX IF EXISTS idx_delivery_order_lines_sol;
DROP INDEX IF EXISTS idx_delivery_order_lines_do;

-- Delivery Orders indexes
DROP INDEX IF EXISTS idx_delivery_orders_created_at;
DROP INDEX IF EXISTS idx_delivery_orders_doc_number;
DROP INDEX IF EXISTS idx_delivery_orders_date;
DROP INDEX IF EXISTS idx_delivery_orders_customer;
DROP INDEX IF EXISTS idx_delivery_orders_warehouse;
DROP INDEX IF EXISTS idx_delivery_orders_so;
DROP INDEX IF EXISTS idx_delivery_orders_company_status;

-- ============================================================================
-- DROP TABLES
-- ============================================================================

DROP TABLE IF EXISTS delivery_order_lines;
DROP TABLE IF EXISTS delivery_orders;

-- ============================================================================
-- DROP ENUMS
-- ============================================================================

DROP TYPE IF EXISTS delivery_order_status;

-- ============================================================================
-- NOTES
-- ============================================================================

-- This migration removes all delivery order functionality
-- Sales orders will remain but quantity_delivered will no longer be updated
-- Any existing delivery data will be lost
