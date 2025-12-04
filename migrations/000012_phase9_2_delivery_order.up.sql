-- Migration: Phase 9.2 - Delivery Order & Fulfillment
-- Description: Add delivery order tables, inventory integration, and SO status tracking

-- ============================================================================
-- DELIVERY ORDER STATUS ENUM
-- ============================================================================

CREATE TYPE delivery_order_status AS ENUM (
    'DRAFT',        -- Initial creation, can be edited
    'CONFIRMED',    -- Confirmed, stock reduced, cannot be edited
    'IN_TRANSIT',   -- Out for delivery
    'DELIVERED',    -- Customer received goods
    'CANCELLED'     -- Cancelled delivery
);

-- ============================================================================
-- DELIVERY ORDERS TABLE
-- ============================================================================

CREATE TABLE delivery_orders (
    id BIGSERIAL PRIMARY KEY,
    doc_number TEXT NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    sales_order_id BIGINT NOT NULL REFERENCES sales_orders(id) ON DELETE CASCADE,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,

    delivery_date DATE NOT NULL,
    status delivery_order_status NOT NULL DEFAULT 'DRAFT',

    -- Logistics information
    driver_name TEXT,
    vehicle_number TEXT,
    tracking_number TEXT,

    notes TEXT,

    -- Audit fields
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    confirmed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    confirmed_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT chk_delivery_order_dates CHECK (
        (confirmed_at IS NULL OR confirmed_at >= created_at) AND
        (delivered_at IS NULL OR delivered_at >= confirmed_at)
    )
);

-- ============================================================================
-- DELIVERY ORDER LINES TABLE
-- ============================================================================

CREATE TABLE delivery_order_lines (
    id BIGSERIAL PRIMARY KEY,
    delivery_order_id BIGINT NOT NULL REFERENCES delivery_orders(id) ON DELETE CASCADE,
    sales_order_line_id BIGINT NOT NULL REFERENCES sales_order_lines(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,

    -- Quantities
    quantity_to_deliver NUMERIC(14,4) NOT NULL CHECK (quantity_to_deliver > 0),
    quantity_delivered NUMERIC(14,4) NOT NULL DEFAULT 0 CHECK (quantity_delivered >= 0),

    -- Reference data from SO line
    uom TEXT NOT NULL,
    unit_price NUMERIC(18,2) NOT NULL,

    notes TEXT,
    line_order INT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT chk_do_line_quantities CHECK (quantity_delivered <= quantity_to_deliver)
);

-- ============================================================================
-- INDEXES FOR PERFORMANCE
-- ============================================================================

-- Delivery Orders indexes
CREATE INDEX idx_delivery_orders_company_status ON delivery_orders(company_id, status);
CREATE INDEX idx_delivery_orders_so ON delivery_orders(sales_order_id);
CREATE INDEX idx_delivery_orders_warehouse ON delivery_orders(warehouse_id);
CREATE INDEX idx_delivery_orders_customer ON delivery_orders(customer_id);
CREATE INDEX idx_delivery_orders_date ON delivery_orders(delivery_date);
CREATE INDEX idx_delivery_orders_doc_number ON delivery_orders(doc_number);
CREATE INDEX idx_delivery_orders_created_at ON delivery_orders(created_at);

-- Delivery Order Lines indexes
CREATE INDEX idx_delivery_order_lines_do ON delivery_order_lines(delivery_order_id);
CREATE INDEX idx_delivery_order_lines_sol ON delivery_order_lines(sales_order_line_id);
CREATE INDEX idx_delivery_order_lines_product ON delivery_order_lines(product_id);

-- ============================================================================
-- HELPER FUNCTIONS
-- ============================================================================

-- Function: Generate delivery order number
-- Format: DO-YYYYMM-#####
CREATE OR REPLACE FUNCTION generate_delivery_order_number(p_company_id BIGINT, p_date DATE)
RETURNS TEXT AS $$
DECLARE
    v_count INT;
    v_year_month TEXT;
    v_doc_number TEXT;
BEGIN
    v_year_month := TO_CHAR(p_date, 'YYYYMM');

    -- Count existing DOs in the same month
    SELECT COUNT(*) INTO v_count
    FROM delivery_orders
    WHERE company_id = p_company_id
      AND DATE_TRUNC('month', delivery_date) = DATE_TRUNC('month', p_date);

    v_doc_number := 'DO-' || v_year_month || '-' || LPAD((v_count + 1)::TEXT, 5, '0');

    RETURN v_doc_number;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION generate_delivery_order_number IS
'Generates unique delivery order document number in format DO-YYYYMM-#####';

-- ============================================================================
-- TRIGGERS
-- ============================================================================

-- Trigger: Auto-update updated_at timestamp on delivery_orders
CREATE TRIGGER trg_delivery_orders_updated_at
    BEFORE UPDATE ON delivery_orders
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Trigger: Auto-update updated_at timestamp on delivery_order_lines
CREATE TRIGGER trg_delivery_order_lines_updated_at
    BEFORE UPDATE ON delivery_order_lines
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- SALES ORDER INTEGRATION FUNCTIONS
-- ============================================================================

-- Function: Update sales_order_lines.quantity_delivered when DO line changes
CREATE OR REPLACE FUNCTION update_so_line_quantity_delivered()
RETURNS TRIGGER AS $$
DECLARE
    v_total_delivered NUMERIC;
BEGIN
    -- Calculate total delivered from all confirmed DOs for this SO line
    SELECT COALESCE(SUM(dol.quantity_delivered), 0)
    INTO v_total_delivered
    FROM delivery_order_lines dol
    INNER JOIN delivery_orders do ON do.id = dol.delivery_order_id
    WHERE dol.sales_order_line_id = NEW.sales_order_line_id
      AND do.status IN ('CONFIRMED', 'IN_TRANSIT', 'DELIVERED');

    -- Update the SO line
    UPDATE sales_order_lines
    SET quantity_delivered = v_total_delivered,
        updated_at = NOW()
    WHERE id = NEW.sales_order_line_id;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_do_line_update_so_qty
    AFTER INSERT OR UPDATE OF quantity_delivered ON delivery_order_lines
    FOR EACH ROW
    EXECUTE FUNCTION update_so_line_quantity_delivered();

COMMENT ON FUNCTION update_so_line_quantity_delivered IS
'Updates sales_order_lines.quantity_delivered based on confirmed delivery orders';

-- Function: Auto-update sales order status based on delivery progress
CREATE OR REPLACE FUNCTION update_sales_order_status_from_delivery()
RETURNS TRIGGER AS $$
DECLARE
    v_so_id BIGINT;
    v_total_ordered NUMERIC;
    v_total_delivered NUMERIC;
    v_has_partial BOOLEAN;
    v_current_status sales_order_status;
BEGIN
    -- Get sales order ID from the delivery order
    SELECT sales_order_id INTO v_so_id
    FROM delivery_orders
    WHERE id = NEW.delivery_order_id;

    -- Get current SO status
    SELECT status INTO v_current_status
    FROM sales_orders
    WHERE id = v_so_id;

    -- Calculate total ordered and delivered quantities
    SELECT
        COALESCE(SUM(quantity), 0),
        COALESCE(SUM(quantity_delivered), 0)
    INTO v_total_ordered, v_total_delivered
    FROM sales_order_lines
    WHERE sales_order_id = v_so_id;

    -- Check if any line is partially delivered
    SELECT EXISTS(
        SELECT 1
        FROM sales_order_lines
        WHERE sales_order_id = v_so_id
          AND quantity_delivered > 0
          AND quantity_delivered < quantity
    ) INTO v_has_partial;

    -- Update SO status based on delivery progress
    IF v_total_delivered >= v_total_ordered THEN
        -- All lines fully delivered
        UPDATE sales_orders
        SET status = 'COMPLETED',
            updated_at = NOW()
        WHERE id = v_so_id
          AND status != 'COMPLETED'
          AND status != 'CANCELLED';

    ELSIF v_total_delivered > 0 OR v_has_partial THEN
        -- Partial delivery
        UPDATE sales_orders
        SET status = 'PROCESSING',
            updated_at = NOW()
        WHERE id = v_so_id
          AND status = 'CONFIRMED';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_do_line_update_so_status
    AFTER INSERT OR UPDATE OF quantity_delivered ON delivery_order_lines
    FOR EACH ROW
    EXECUTE FUNCTION update_sales_order_status_from_delivery();

COMMENT ON FUNCTION update_sales_order_status_from_delivery IS
'Auto-updates sales order status: CONFIRMED → PROCESSING → COMPLETED based on deliveries';

-- ============================================================================
-- HELPER VIEWS
-- ============================================================================

-- View: Delivery orders with enriched details
CREATE OR REPLACE VIEW vw_delivery_orders_detail AS
SELECT
    do.id,
    do.doc_number,
    do.company_id,
    do.sales_order_id,
    so.doc_number AS sales_order_number,
    do.warehouse_id,
    w.name AS warehouse_name,
    do.customer_id,
    c.name AS customer_name,
    do.delivery_date,
    do.status,
    do.driver_name,
    do.vehicle_number,
    do.tracking_number,
    do.notes,
    do.created_by,
    u_created.username AS created_by_name,
    do.confirmed_by,
    u_confirmed.username AS confirmed_by_name,
    do.confirmed_at,
    do.delivered_at,
    do.created_at,
    do.updated_at,
    -- Line counts
    COUNT(dol.id) AS line_count,
    SUM(dol.quantity_to_deliver) AS total_quantity
FROM delivery_orders do
INNER JOIN sales_orders so ON so.id = do.sales_order_id
INNER JOIN warehouses w ON w.id = do.warehouse_id
INNER JOIN customers c ON c.id = do.customer_id
INNER JOIN users u_created ON u_created.id = do.created_by
LEFT JOIN users u_confirmed ON u_confirmed.id = do.confirmed_by
LEFT JOIN delivery_order_lines dol ON dol.delivery_order_id = do.id
GROUP BY
    do.id, do.doc_number, do.company_id, do.sales_order_id, so.doc_number,
    do.warehouse_id, w.name, do.customer_id, c.name, do.delivery_date,
    do.status, do.driver_name, do.vehicle_number, do.tracking_number,
    do.notes, do.created_by, u_created.username, do.confirmed_by,
    u_confirmed.username, do.confirmed_at, do.delivered_at,
    do.created_at, do.updated_at;

COMMENT ON VIEW vw_delivery_orders_detail IS
'Enriched view of delivery orders with related entity details';

-- ============================================================================
-- GRANT PERMISSIONS
-- ============================================================================

-- Grant necessary permissions (adjust based on your application user)
-- GRANT SELECT, INSERT, UPDATE, DELETE ON delivery_orders TO odyssey_app;
-- GRANT SELECT, INSERT, UPDATE, DELETE ON delivery_order_lines TO odyssey_app;
-- GRANT USAGE, SELECT ON SEQUENCE delivery_orders_id_seq TO odyssey_app;
-- GRANT USAGE, SELECT ON SEQUENCE delivery_order_lines_id_seq TO odyssey_app;

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE delivery_orders IS
'Delivery orders for fulfilling sales orders with inventory integration';

COMMENT ON TABLE delivery_order_lines IS
'Line items in delivery orders, linked to sales order lines';

COMMENT ON COLUMN delivery_orders.doc_number IS
'Unique document number in format DO-YYYYMM-#####';

COMMENT ON COLUMN delivery_orders.status IS
'Lifecycle status: DRAFT → CONFIRMED → IN_TRANSIT → DELIVERED or CANCELLED';

COMMENT ON COLUMN delivery_order_lines.quantity_to_deliver IS
'Quantity planned for delivery in this DO';

COMMENT ON COLUMN delivery_order_lines.quantity_delivered IS
'Actual quantity delivered (set on confirmation)';

COMMENT ON COLUMN sales_order_lines.quantity_delivered IS
'Total quantity delivered across all DOs (auto-updated by trigger)';
