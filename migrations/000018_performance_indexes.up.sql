-- Migration 018: Performance Optimizations
-- Add missing indexes and optimize large tables

-- ============================================================================
-- COMPOSITE INDEXES FOR COMMON QUERY PATTERNS
-- ============================================================================

-- Sales Orders: Company + Status + Date (common list filter)
CREATE INDEX IF NOT EXISTS idx_sales_orders_company_status_date 
    ON sales_orders(company_id, status, order_date);

-- Quotations: Company + Status + Date
CREATE INDEX IF NOT EXISTS idx_quotations_company_status_date 
    ON quotations(company_id, status, quote_date);

-- Delivery Orders: Company + Status + Date
CREATE INDEX IF NOT EXISTS idx_delivery_orders_company_status_date 
    ON delivery_orders(company_id, status, delivery_date);

-- Journal Entries: Period + Status (common for GL queries)
CREATE INDEX IF NOT EXISTS idx_journal_entries_period_status 
    ON journal_entries(period_id, status);

-- Journal Entries: Date + Status (for date-range queries)
CREATE INDEX IF NOT EXISTS idx_journal_entries_date_status 
    ON journal_entries(date, status);

-- Journal Lines: Account + JE (for account ledger queries)
CREATE INDEX IF NOT EXISTS idx_journal_lines_account_je 
    ON journal_lines(account_id, je_id);

-- ============================================================================
-- AUDIT LOGS PERFORMANCE
-- ============================================================================

-- audit_logs: occurred_at for time-based queries
CREATE INDEX IF NOT EXISTS idx_audit_logs_occurred_at 
    ON audit_logs(occurred_at DESC);

-- audit_logs: actor + occurred_at (user activity)
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_time 
    ON audit_logs(actor_id, occurred_at DESC);

-- audit_logs: entity + entity_id + occurred_at (entity history)
CREATE INDEX IF NOT EXISTS idx_audit_logs_entity_history 
    ON audit_logs(entity, entity_id, occurred_at DESC);

-- ============================================================================
-- INVENTORY PERFORMANCE
-- ============================================================================

-- inventory_tx: company + warehouse + posted_at
CREATE INDEX IF NOT EXISTS idx_inventory_tx_company_warehouse_date 
    ON inventory_tx(company_id, warehouse_id, posted_at);

-- inventory_balances: warehouse + product (already PK, but add qty for covering)
CREATE INDEX IF NOT EXISTS idx_inventory_balances_qty 
    ON inventory_balances(warehouse_id, product_id, qty);

-- inventory_cards: lookup by warehouse/product/date
CREATE INDEX IF NOT EXISTS idx_inventory_cards_lookup_desc 
    ON inventory_cards(warehouse_id, product_id, posted_at DESC);

-- ============================================================================
-- PROCUREMENT PERFORMANCE
-- ============================================================================

-- pos: company + status + created_at
CREATE INDEX IF NOT EXISTS idx_pos_company_status_date 
    ON pos(company_id, status, created_at);

-- grns: company + status + received_at
CREATE INDEX IF NOT EXISTS idx_grns_company_status_date 
    ON grns(company_id, status, received_at);

-- ============================================================================
-- PARTIAL INDEXES FOR COMMON FILTERS
-- ============================================================================

-- Active products only
CREATE INDEX IF NOT EXISTS idx_products_active 
    ON products(company_id, sku) 
    WHERE is_active = true AND deleted_at IS NULL;

-- Active suppliers only
CREATE INDEX IF NOT EXISTS idx_suppliers_active 
    ON suppliers(company_id, code) 
    WHERE is_active = true;

-- Active customers only
CREATE INDEX IF NOT EXISTS idx_customers_active 
    ON customers(company_id, code) 
    WHERE is_active = true;

-- Open accounting periods
CREATE INDEX IF NOT EXISTS idx_accounting_periods_open 
    ON accounting_periods(company_id, start_date) 
    WHERE status = 'OPEN';

-- Posted journal entries only
CREATE INDEX IF NOT EXISTS idx_journal_entries_posted 
    ON journal_entries(period_id, date) 
    WHERE status = 'POSTED';

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON INDEX idx_sales_orders_company_status_date IS 'Optimize sales order list queries by company/status/date';
COMMENT ON INDEX idx_audit_logs_occurred_at IS 'Optimize audit log time-based queries';
COMMENT ON INDEX idx_products_active IS 'Partial index for active products only';
