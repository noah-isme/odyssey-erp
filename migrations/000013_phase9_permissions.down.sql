-- Phase 9: Sales & Delivery RBAC Permissions Rollback
-- Remove permissions and roles created for sales and delivery modules

-- ============================================================================
-- DROP VERIFICATION VIEW
-- ============================================================================

DROP VIEW IF EXISTS v_sales_delivery_permissions;

-- ============================================================================
-- REMOVE ROLE ASSIGNMENTS
-- ============================================================================

-- Remove permissions from roles before deleting roles
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE name LIKE 'sales.%' OR name LIKE 'delivery.%'
);

-- ============================================================================
-- REMOVE ROLES (OPTIONAL - ONLY IF CREATED BY THIS MIGRATION)
-- ============================================================================

-- These roles might have been created by this migration
-- Only delete if they have no user assignments
DELETE FROM roles
WHERE name IN ('Sales Manager', 'Sales Staff', 'Warehouse Staff')
AND NOT EXISTS (
    SELECT 1 FROM user_roles WHERE role_id = roles.id
);

-- ============================================================================
-- REMOVE PERMISSIONS
-- ============================================================================

-- Delete delivery order permissions
DELETE FROM permissions
WHERE name IN (
    'delivery.order.view',
    'delivery.order.create',
    'delivery.order.edit',
    'delivery.order.confirm',
    'delivery.order.ship',
    'delivery.order.complete',
    'delivery.order.cancel',
    'delivery.order.print'
);

-- Delete sales order permissions
DELETE FROM permissions
WHERE name IN (
    'sales.order.view',
    'sales.order.create',
    'sales.order.edit',
    'sales.order.confirm',
    'sales.order.cancel'
);

-- Delete quotation permissions
DELETE FROM permissions
WHERE name IN (
    'sales.quotation.view',
    'sales.quotation.create',
    'sales.quotation.edit',
    'sales.quotation.approve',
    'sales.quotation.reject',
    'sales.quotation.convert'
);

-- Delete customer permissions
DELETE FROM permissions
WHERE name IN (
    'sales.customer.view',
    'sales.customer.create',
    'sales.customer.edit',
    'sales.customer.delete'
);
