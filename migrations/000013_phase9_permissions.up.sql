-- Phase 9: Sales & Delivery RBAC Permissions
-- Insert permissions for customers, quotations, sales orders, and delivery orders

-- ============================================================================
-- CUSTOMER PERMISSIONS
-- ============================================================================

INSERT INTO permissions (name, description)
VALUES
    ('sales.customer.view', 'View customer records'),
    ('sales.customer.create', 'Create new customers'),
    ('sales.customer.edit', 'Edit customer information'),
    ('sales.customer.delete', 'Delete or deactivate customers')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

-- ============================================================================
-- QUOTATION PERMISSIONS
-- ============================================================================

INSERT INTO permissions (name, description)
VALUES
    ('sales.quotation.view', 'View sales quotations'),
    ('sales.quotation.create', 'Create new quotations'),
    ('sales.quotation.edit', 'Edit draft quotations'),
    ('sales.quotation.approve', 'Approve submitted quotations'),
    ('sales.quotation.reject', 'Reject quotations'),
    ('sales.quotation.convert', 'Convert quotations to sales orders')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

-- ============================================================================
-- SALES ORDER PERMISSIONS
-- ============================================================================

INSERT INTO permissions (name, description)
VALUES
    ('sales.order.view', 'View sales orders'),
    ('sales.order.create', 'Create new sales orders'),
    ('sales.order.edit', 'Edit draft sales orders'),
    ('sales.order.confirm', 'Confirm sales orders'),
    ('sales.order.cancel', 'Cancel sales orders')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

-- ============================================================================
-- DELIVERY ORDER PERMISSIONS
-- ============================================================================

INSERT INTO permissions (name, description)
VALUES
    ('delivery.order.view', 'View delivery orders'),
    ('delivery.order.create', 'Create new delivery orders'),
    ('delivery.order.edit', 'Edit draft delivery orders'),
    ('delivery.order.confirm', 'Confirm delivery orders for picking'),
    ('delivery.order.ship', 'Mark delivery orders as shipped'),
    ('delivery.order.complete', 'Complete delivery orders'),
    ('delivery.order.cancel', 'Cancel delivery orders'),
    ('delivery.order.print', 'Print packing lists and delivery documents')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

-- ============================================================================
-- DEFAULT ROLE ASSIGNMENTS
-- ============================================================================

-- Assign basic sales permissions to existing roles (if they exist)
-- These are examples - adjust based on your organization's role structure

-- Sales Manager role gets all sales and delivery permissions
DO $$
DECLARE
    v_role_id BIGINT;
    v_perm_id BIGINT;
BEGIN
    -- Check if 'Sales Manager' role exists, if not create it
    SELECT id INTO v_role_id FROM roles WHERE name = 'Sales Manager';
    IF v_role_id IS NULL THEN
        INSERT INTO roles (name, description)
        VALUES ('Sales Manager', 'Full access to sales and delivery operations')
        RETURNING id INTO v_role_id;
    END IF;

    -- Assign all sales and delivery permissions to Sales Manager
    FOR v_perm_id IN
        SELECT id FROM permissions
        WHERE name LIKE 'sales.%' OR name LIKE 'delivery.%'
    LOOP
        INSERT INTO role_permissions (role_id, permission_id)
        VALUES (v_role_id, v_perm_id)
        ON CONFLICT DO NOTHING;
    END LOOP;
END$$;

-- Sales Staff role gets basic sales permissions
DO $$
DECLARE
    v_role_id BIGINT;
    v_perm_id BIGINT;
BEGIN
    -- Check if 'Sales Staff' role exists, if not create it
    SELECT id INTO v_role_id FROM roles WHERE name = 'Sales Staff';
    IF v_role_id IS NULL THEN
        INSERT INTO roles (name, description)
        VALUES ('Sales Staff', 'Basic sales operations without approval rights')
        RETURNING id INTO v_role_id;
    END IF;

    -- Assign view and create permissions to Sales Staff
    FOR v_perm_id IN
        SELECT id FROM permissions
        WHERE name IN (
            'sales.customer.view',
            'sales.customer.create',
            'sales.quotation.view',
            'sales.quotation.create',
            'sales.quotation.edit',
            'sales.order.view',
            'sales.order.create',
            'sales.order.edit'
        )
    LOOP
        INSERT INTO role_permissions (role_id, permission_id)
        VALUES (v_role_id, v_perm_id)
        ON CONFLICT DO NOTHING;
    END LOOP;
END$$;

-- Warehouse Staff role gets delivery permissions
DO $$
DECLARE
    v_role_id BIGINT;
    v_perm_id BIGINT;
BEGIN
    -- Check if 'Warehouse Staff' role exists, if not create it
    SELECT id INTO v_role_id FROM roles WHERE name = 'Warehouse Staff';
    IF v_role_id IS NULL THEN
        INSERT INTO roles (name, description)
        VALUES ('Warehouse Staff', 'Warehouse and delivery operations')
        RETURNING id INTO v_role_id;
    END IF;

    -- Assign delivery permissions to Warehouse Staff
    FOR v_perm_id IN
        SELECT id FROM permissions
        WHERE name IN (
            'delivery.order.view',
            'delivery.order.confirm',
            'delivery.order.ship',
            'delivery.order.complete',
            'delivery.order.print'
        )
    LOOP
        INSERT INTO role_permissions (role_id, permission_id)
        VALUES (v_role_id, v_perm_id)
        ON CONFLICT DO NOTHING;
    END LOOP;
END$$;

-- ============================================================================
-- PERMISSION VERIFICATION VIEW
-- ============================================================================

-- Create a view to easily verify role-permission assignments
CREATE OR REPLACE VIEW v_sales_delivery_permissions AS
SELECT
    r.name AS role_name,
    r.description AS role_description,
    p.name AS permission_name,
    p.description AS permission_description,
    CASE
        WHEN p.name LIKE 'sales.customer.%' THEN 'Customer'
        WHEN p.name LIKE 'sales.quotation.%' THEN 'Quotation'
        WHEN p.name LIKE 'sales.order.%' THEN 'Sales Order'
        WHEN p.name LIKE 'delivery.order.%' THEN 'Delivery Order'
        ELSE 'Other'
    END AS module
FROM roles r
JOIN role_permissions rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id
WHERE p.name LIKE 'sales.%' OR p.name LIKE 'delivery.%'
ORDER BY r.name, module, p.name;

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON VIEW v_sales_delivery_permissions IS 'Shows all sales and delivery permission assignments by role for easy verification';
