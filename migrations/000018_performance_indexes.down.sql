-- Rollback Migration 018: Performance Optimizations

-- Drop composite indexes
DROP INDEX IF EXISTS idx_sales_orders_company_status_date;
DROP INDEX IF EXISTS idx_quotations_company_status_date;
DROP INDEX IF EXISTS idx_delivery_orders_company_status_date;
DROP INDEX IF EXISTS idx_journal_entries_period_status;
DROP INDEX IF EXISTS idx_journal_entries_date_status;
DROP INDEX IF EXISTS idx_journal_lines_account_je;

-- Drop audit indexes
DROP INDEX IF EXISTS idx_audit_logs_occurred_at;
DROP INDEX IF EXISTS idx_audit_logs_actor_time;
DROP INDEX IF EXISTS idx_audit_logs_entity_history;

-- Drop inventory indexes
DROP INDEX IF EXISTS idx_inventory_tx_company_warehouse_date;
DROP INDEX IF EXISTS idx_inventory_balances_qty;
DROP INDEX IF EXISTS idx_inventory_cards_lookup_desc;

-- Drop procurement indexes
DROP INDEX IF EXISTS idx_pos_company_status_date;
DROP INDEX IF EXISTS idx_grns_company_status_date;

-- Drop partial indexes
DROP INDEX IF EXISTS idx_products_active;
DROP INDEX IF EXISTS idx_suppliers_active;
DROP INDEX IF EXISTS idx_customers_active;
DROP INDEX IF EXISTS idx_accounting_periods_open;
DROP INDEX IF EXISTS idx_journal_entries_posted;
