-- Phase 9.1: Sales Quotation & Sales Order
-- Customers, Quotations, and Sales Orders

-- ============================================================================
-- CUSTOMERS (drop and recreate with extended schema)
-- ============================================================================

DROP TABLE IF EXISTS customers CASCADE;
CREATE TABLE customers (
    id BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    email TEXT,
    phone TEXT,
    tax_id TEXT,
    credit_limit NUMERIC(18,2) NOT NULL DEFAULT 0,
    payment_terms_days INT NOT NULL DEFAULT 30,
    address_line1 TEXT,
    address_line2 TEXT,
    city TEXT,
    state TEXT,
    postal_code TEXT,
    country TEXT NOT NULL DEFAULT 'ID',
    is_active BOOLEAN NOT NULL DEFAULT true,
    notes TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_code_company UNIQUE(company_id, code)
);

CREATE INDEX idx_customers_company ON customers(company_id);
CREATE INDEX idx_customers_code ON customers(code);
CREATE INDEX idx_customers_active ON customers(company_id, is_active);

-- ============================================================================
-- QUOTATIONS
-- ============================================================================

CREATE TYPE quotation_status AS ENUM ('DRAFT','SUBMITTED','APPROVED','REJECTED','CONVERTED');

CREATE TABLE quotations (
    id BIGSERIAL PRIMARY KEY,
    doc_number TEXT NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    quote_date DATE NOT NULL,
    valid_until DATE NOT NULL,
    status quotation_status NOT NULL DEFAULT 'DRAFT',
    currency TEXT NOT NULL DEFAULT 'IDR',
    subtotal NUMERIC(18,2) NOT NULL DEFAULT 0,
    tax_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    total_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    notes TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    approved_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ,
    rejected_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    rejected_at TIMESTAMPTZ,
    rejection_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_quotation_dates CHECK (quote_date <= valid_until),
    CONSTRAINT chk_quotation_amounts CHECK (subtotal >= 0 AND tax_amount >= 0 AND total_amount >= 0)
);

CREATE TABLE quotation_lines (
    id BIGSERIAL PRIMARY KEY,
    quotation_id BIGINT NOT NULL REFERENCES quotations(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    description TEXT,
    quantity NUMERIC(14,4) NOT NULL CHECK (quantity > 0),
    uom TEXT NOT NULL DEFAULT 'PCS',
    unit_price NUMERIC(18,2) NOT NULL CHECK (unit_price >= 0),
    discount_percent NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (discount_percent >= 0 AND discount_percent <= 100),
    discount_amount NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    tax_percent NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (tax_percent >= 0),
    tax_amount NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    line_total NUMERIC(18,2) NOT NULL,
    notes TEXT,
    line_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_quotations_company_status ON quotations(company_id, status);
CREATE INDEX idx_quotations_customer ON quotations(customer_id);
CREATE INDEX idx_quotations_created_by ON quotations(created_by);
CREATE INDEX idx_quotations_quote_date ON quotations(quote_date);
CREATE INDEX idx_quotation_lines_quotation ON quotation_lines(quotation_id);
CREATE INDEX idx_quotation_lines_product ON quotation_lines(product_id);

-- ============================================================================
-- SALES ORDERS
-- ============================================================================

CREATE TYPE sales_order_status AS ENUM ('DRAFT','CONFIRMED','PROCESSING','COMPLETED','CANCELLED');

CREATE TABLE sales_orders (
    id BIGSERIAL PRIMARY KEY,
    doc_number TEXT NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    quotation_id BIGINT REFERENCES quotations(id) ON DELETE SET NULL,
    order_date DATE NOT NULL,
    expected_delivery_date DATE,
    status sales_order_status NOT NULL DEFAULT 'DRAFT',
    currency TEXT NOT NULL DEFAULT 'IDR',
    subtotal NUMERIC(18,2) NOT NULL DEFAULT 0,
    tax_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    total_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    notes TEXT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    confirmed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    confirmed_at TIMESTAMPTZ,
    cancelled_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    cancelled_at TIMESTAMPTZ,
    cancellation_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_sales_order_amounts CHECK (subtotal >= 0 AND tax_amount >= 0 AND total_amount >= 0)
);

CREATE TABLE sales_order_lines (
    id BIGSERIAL PRIMARY KEY,
    sales_order_id BIGINT NOT NULL REFERENCES sales_orders(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    description TEXT,
    quantity NUMERIC(14,4) NOT NULL CHECK (quantity > 0),
    quantity_delivered NUMERIC(14,4) NOT NULL DEFAULT 0 CHECK (quantity_delivered >= 0),
    quantity_invoiced NUMERIC(14,4) NOT NULL DEFAULT 0 CHECK (quantity_invoiced >= 0),
    uom TEXT NOT NULL DEFAULT 'PCS',
    unit_price NUMERIC(18,2) NOT NULL CHECK (unit_price >= 0),
    discount_percent NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (discount_percent >= 0 AND discount_percent <= 100),
    discount_amount NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    tax_percent NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (tax_percent >= 0),
    tax_amount NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    line_total NUMERIC(18,2) NOT NULL,
    notes TEXT,
    line_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_qty_delivered CHECK (quantity_delivered <= quantity),
    CONSTRAINT chk_qty_invoiced CHECK (quantity_invoiced <= quantity)
);

CREATE INDEX idx_sales_orders_company_status ON sales_orders(company_id, status);
CREATE INDEX idx_sales_orders_customer ON sales_orders(customer_id);
CREATE INDEX idx_sales_orders_quotation ON sales_orders(quotation_id);
CREATE INDEX idx_sales_orders_created_by ON sales_orders(created_by);
CREATE INDEX idx_sales_orders_order_date ON sales_orders(order_date);
CREATE INDEX idx_sales_order_lines_so ON sales_order_lines(sales_order_id);
CREATE INDEX idx_sales_order_lines_product ON sales_order_lines(product_id);

-- ============================================================================
-- HELPER FUNCTIONS
-- ============================================================================

-- Function to generate customer code
CREATE OR REPLACE FUNCTION generate_customer_code(p_company_id BIGINT)
RETURNS TEXT AS $$
DECLARE
    v_count INT;
    v_code TEXT;
BEGIN
    SELECT COUNT(*) INTO v_count FROM customers WHERE company_id = p_company_id;
    v_code := 'CUST-' || LPAD((v_count + 1)::TEXT, 6, '0');
    RETURN v_code;
END;
$$ LANGUAGE plpgsql;

-- Function to generate quotation number
CREATE OR REPLACE FUNCTION generate_quotation_number(p_company_id BIGINT, p_quote_date DATE)
RETURNS TEXT AS $$
DECLARE
    v_count INT;
    v_doc_number TEXT;
    v_year TEXT;
    v_month TEXT;
BEGIN
    v_year := TO_CHAR(p_quote_date, 'YYYY');
    v_month := TO_CHAR(p_quote_date, 'MM');

    SELECT COUNT(*) INTO v_count
    FROM quotations
    WHERE company_id = p_company_id
      AND EXTRACT(YEAR FROM quote_date) = EXTRACT(YEAR FROM p_quote_date)
      AND EXTRACT(MONTH FROM quote_date) = EXTRACT(MONTH FROM p_quote_date);

    v_doc_number := 'QUO-' || v_year || v_month || '-' || LPAD((v_count + 1)::TEXT, 4, '0');
    RETURN v_doc_number;
END;
$$ LANGUAGE plpgsql;

-- Function to generate sales order number
CREATE OR REPLACE FUNCTION generate_sales_order_number(p_company_id BIGINT, p_order_date DATE)
RETURNS TEXT AS $$
DECLARE
    v_count INT;
    v_doc_number TEXT;
    v_year TEXT;
    v_month TEXT;
BEGIN
    v_year := TO_CHAR(p_order_date, 'YYYY');
    v_month := TO_CHAR(p_order_date, 'MM');

    SELECT COUNT(*) INTO v_count
    FROM sales_orders
    WHERE company_id = p_company_id
      AND EXTRACT(YEAR FROM order_date) = EXTRACT(YEAR FROM p_order_date)
      AND EXTRACT(MONTH FROM order_date) = EXTRACT(MONTH FROM p_order_date);

    v_doc_number := 'SO-' || v_year || v_month || '-' || LPAD((v_count + 1)::TEXT, 4, '0');
    RETURN v_doc_number;
END;
$$ LANGUAGE plpgsql;

-- Function to calculate quotation line total
CREATE OR REPLACE FUNCTION calculate_quotation_line_total(
    p_quantity NUMERIC,
    p_unit_price NUMERIC,
    p_discount_percent NUMERIC,
    p_tax_percent NUMERIC
)
RETURNS TABLE(discount_amount NUMERIC, tax_amount NUMERIC, line_total NUMERIC) AS $$
DECLARE
    v_subtotal NUMERIC;
    v_discount_amount NUMERIC;
    v_taxable_amount NUMERIC;
    v_tax_amount NUMERIC;
    v_line_total NUMERIC;
BEGIN
    -- Calculate subtotal
    v_subtotal := p_quantity * p_unit_price;

    -- Calculate discount
    v_discount_amount := ROUND(v_subtotal * p_discount_percent / 100, 2);

    -- Calculate taxable amount
    v_taxable_amount := v_subtotal - v_discount_amount;

    -- Calculate tax
    v_tax_amount := ROUND(v_taxable_amount * p_tax_percent / 100, 2);

    -- Calculate line total
    v_line_total := v_taxable_amount + v_tax_amount;

    RETURN QUERY SELECT v_discount_amount, v_tax_amount, v_line_total;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Trigger to update quotation totals when lines change
CREATE OR REPLACE FUNCTION update_quotation_totals()
RETURNS TRIGGER AS $$
DECLARE
    v_quotation_id BIGINT;
BEGIN
    -- Get quotation_id from the affected row
    IF TG_OP = 'DELETE' THEN
        v_quotation_id := OLD.quotation_id;
    ELSE
        v_quotation_id := NEW.quotation_id;
    END IF;

    -- Recalculate totals
    UPDATE quotations
    SET
        subtotal = COALESCE((
            SELECT SUM((quantity * unit_price) - discount_amount)
            FROM quotation_lines
            WHERE quotation_id = v_quotation_id
        ), 0),
        tax_amount = COALESCE((
            SELECT SUM(tax_amount)
            FROM quotation_lines
            WHERE quotation_id = v_quotation_id
        ), 0),
        total_amount = COALESCE((
            SELECT SUM(line_total)
            FROM quotation_lines
            WHERE quotation_id = v_quotation_id
        ), 0),
        updated_at = NOW()
    WHERE id = v_quotation_id;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_quotation_totals
AFTER INSERT OR UPDATE OR DELETE ON quotation_lines
FOR EACH ROW
EXECUTE FUNCTION update_quotation_totals();

-- Trigger to update sales order totals when lines change
CREATE OR REPLACE FUNCTION update_sales_order_totals()
RETURNS TRIGGER AS $$
DECLARE
    v_sales_order_id BIGINT;
BEGIN
    -- Get sales_order_id from the affected row
    IF TG_OP = 'DELETE' THEN
        v_sales_order_id := OLD.sales_order_id;
    ELSE
        v_sales_order_id := NEW.sales_order_id;
    END IF;

    -- Recalculate totals
    UPDATE sales_orders
    SET
        subtotal = COALESCE((
            SELECT SUM((quantity * unit_price) - discount_amount)
            FROM sales_order_lines
            WHERE sales_order_id = v_sales_order_id
        ), 0),
        tax_amount = COALESCE((
            SELECT SUM(tax_amount)
            FROM sales_order_lines
            WHERE sales_order_id = v_sales_order_id
        ), 0),
        total_amount = COALESCE((
            SELECT SUM(line_total)
            FROM sales_order_lines
            WHERE sales_order_id = v_sales_order_id
        ), 0),
        updated_at = NOW()
    WHERE id = v_sales_order_id;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_sales_order_totals
AFTER INSERT OR UPDATE OR DELETE ON sales_order_lines
FOR EACH ROW
EXECUTE FUNCTION update_sales_order_totals();

-- Trigger to update sales order status based on delivery/invoice progress
CREATE OR REPLACE FUNCTION update_sales_order_status()
RETURNS TRIGGER AS $$
DECLARE
    v_total_qty NUMERIC;
    v_total_delivered NUMERIC;
    v_sales_order_id BIGINT;
    v_current_status sales_order_status;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_sales_order_id := OLD.sales_order_id;
    ELSE
        v_sales_order_id := NEW.sales_order_id;
    END IF;

    -- Get current status
    SELECT status INTO v_current_status FROM sales_orders WHERE id = v_sales_order_id;

    -- Only update if order is CONFIRMED or PROCESSING
    IF v_current_status IN ('CONFIRMED', 'PROCESSING') THEN
        SELECT
            COALESCE(SUM(quantity), 0),
            COALESCE(SUM(quantity_delivered), 0)
        INTO v_total_qty, v_total_delivered
        FROM sales_order_lines
        WHERE sales_order_id = v_sales_order_id;

        -- Update status
        IF v_total_delivered = 0 THEN
            UPDATE sales_orders SET status = 'CONFIRMED', updated_at = NOW() WHERE id = v_sales_order_id;
        ELSIF v_total_delivered >= v_total_qty THEN
            UPDATE sales_orders SET status = 'COMPLETED', updated_at = NOW() WHERE id = v_sales_order_id;
        ELSE
            UPDATE sales_orders SET status = 'PROCESSING', updated_at = NOW() WHERE id = v_sales_order_id;
        END IF;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_sales_order_status
AFTER UPDATE OF quantity_delivered ON sales_order_lines
FOR EACH ROW
EXECUTE FUNCTION update_sales_order_status();
