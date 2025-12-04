-- Phase 9.1: Sales Quotation & Sales Order - ROLLBACK

-- Drop triggers
DROP TRIGGER IF EXISTS trg_update_sales_order_status ON sales_order_lines;
DROP TRIGGER IF EXISTS trg_update_sales_order_totals ON sales_order_lines;
DROP TRIGGER IF EXISTS trg_update_quotation_totals ON quotation_lines;

-- Drop functions
DROP FUNCTION IF EXISTS update_sales_order_status();
DROP FUNCTION IF EXISTS update_sales_order_totals();
DROP FUNCTION IF EXISTS update_quotation_totals();
DROP FUNCTION IF EXISTS calculate_quotation_line_total(NUMERIC, NUMERIC, NUMERIC, NUMERIC);
DROP FUNCTION IF EXISTS generate_sales_order_number(BIGINT, DATE);
DROP FUNCTION IF EXISTS generate_quotation_number(BIGINT, DATE);
DROP FUNCTION IF EXISTS generate_customer_code(BIGINT);

-- Drop indexes
DROP INDEX IF EXISTS idx_sales_order_lines_product;
DROP INDEX IF EXISTS idx_sales_order_lines_so;
DROP INDEX IF EXISTS idx_sales_orders_order_date;
DROP INDEX IF EXISTS idx_sales_orders_created_by;
DROP INDEX IF EXISTS idx_sales_orders_quotation;
DROP INDEX IF EXISTS idx_sales_orders_customer;
DROP INDEX IF EXISTS idx_sales_orders_company_status;

DROP INDEX IF EXISTS idx_quotation_lines_product;
DROP INDEX IF EXISTS idx_quotation_lines_quotation;
DROP INDEX IF EXISTS idx_quotations_quote_date;
DROP INDEX IF EXISTS idx_quotations_created_by;
DROP INDEX IF EXISTS idx_quotations_customer;
DROP INDEX IF EXISTS idx_quotations_company_status;

DROP INDEX IF EXISTS idx_customers_active;
DROP INDEX IF EXISTS idx_customers_code;
DROP INDEX IF EXISTS idx_customers_company;

-- Drop tables
DROP TABLE IF EXISTS sales_order_lines;
DROP TABLE IF EXISTS sales_orders;
DROP TABLE IF EXISTS quotation_lines;
DROP TABLE IF EXISTS quotations;
DROP TABLE IF EXISTS customers;

-- Drop types
DROP TYPE IF EXISTS sales_order_status;
DROP TYPE IF EXISTS quotation_status;
