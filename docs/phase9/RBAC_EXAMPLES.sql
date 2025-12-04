-- ============================================================================
-- RBAC EXAMPLES - Sales & Delivery Permissions
-- ============================================================================
-- Common SQL scripts for setting up users, roles, and permissions
-- Run these after applying migration 000013_phase9_permissions.up.sql

-- ============================================================================
-- SECTION 1: VERIFICATION QUERIES
-- ============================================================================

-- Verify all permissions were created (should return 23)
SELECT COUNT(*) as permission_count
FROM permissions
WHERE name LIKE 'sales.%' OR name LIKE 'delivery.%';

-- View all sales & delivery permissions
SELECT name, description
FROM permissions
WHERE name LIKE 'sales.%' OR name LIKE 'delivery.%'
ORDER BY name;

-- View default roles created by migration
SELECT id, name, description
FROM roles
WHERE name IN ('Sales Manager', 'Sales Staff', 'Warehouse Staff');

-- View all role-permission assignments
SELECT * FROM v_sales_delivery_permissions
ORDER BY role_name, module, permission_name;

-- ============================================================================
-- SECTION 2: USER ROLE ASSIGNMENT
-- ============================================================================

-- Assign "Sales Manager" role to user
INSERT INTO user_roles (user_id, role_id)
SELECT
    5,  -- Replace with actual user ID
    id
FROM roles
WHERE name = 'Sales Manager'
ON CONFLICT DO NOTHING;

-- Assign "Sales Staff" role to user
INSERT INTO user_roles (user_id, role_id)
SELECT
    10,  -- Replace with actual user ID
    id
FROM roles
WHERE name = 'Sales Staff'
ON CONFLICT DO NOTHING;

-- Assign "Warehouse Staff" role to user
INSERT INTO user_roles (user_id, role_id)
SELECT
    15,  -- Replace with actual user ID
    id
FROM roles
WHERE name = 'Warehouse Staff'
ON CONFLICT DO NOTHING;

-- Assign multiple roles to one user
DO $$
DECLARE
    v_user_id BIGINT := 20;  -- Replace with actual user ID
BEGIN
    INSERT INTO user_roles (user_id, role_id)
    SELECT v_user_id, id FROM roles WHERE name = 'Sales Staff'
    ON CONFLICT DO NOTHING;

    INSERT INTO user_roles (user_id, role_id)
    SELECT v_user_id, id FROM roles WHERE name = 'Warehouse Staff'
    ON CONFLICT DO NOTHING;
END$$;

-- ============================================================================
-- SECTION 3: CUSTOM ROLE CREATION
-- ============================================================================

-- Example 1: Order Entry Clerk (can view and create, but not approve)
DO $$
DECLARE
    v_role_id BIGINT;
BEGIN
    INSERT INTO roles (name, description)
    VALUES ('Order Entry Clerk', 'Create and edit orders without approval rights')
    RETURNING id INTO v_role_id;

    -- Assign permissions
    INSERT INTO role_permissions (role_id, permission_id)
    SELECT v_role_id, id FROM permissions
    WHERE name IN (
        'sales.customer.view',
        'sales.quotation.view',
        'sales.quotation.create',
        'sales.quotation.edit',
        'sales.order.view',
        'sales.order.create',
        'sales.order.edit'
    );
END$$;

-- Example 2: Fulfillment Supervisor (warehouse + some sales oversight)
DO $$
DECLARE
    v_role_id BIGINT;
BEGIN
    INSERT INTO roles (name, description)
    VALUES ('Fulfillment Supervisor', 'Oversee delivery operations with sales visibility')
    RETURNING id INTO v_role_id;

    -- Assign all delivery permissions
    INSERT INTO role_permissions (role_id, permission_id)
    SELECT v_role_id, id FROM permissions
    WHERE name LIKE 'delivery.order.%';

    -- Add sales order view permission
    INSERT INTO role_permissions (role_id, permission_id)
    SELECT v_role_id, id FROM permissions
    WHERE name = 'sales.order.view';
END$$;

-- Example 3: Customer Service Rep (view-only + customer edits)
DO $$
DECLARE
    v_role_id BIGINT;
BEGIN
    INSERT INTO roles (name, description)
    VALUES ('Customer Service Rep', 'Customer support with limited edit rights')
    RETURNING id INTO v_role_id;

    INSERT INTO role_permissions (role_id, permission_id)
    SELECT v_role_id, id FROM permissions
    WHERE name IN (
        'sales.customer.view',
        'sales.customer.edit',
        'sales.quotation.view',
        'sales.order.view',
        'delivery.order.view'
    );
END$$;

-- Example 4: Sales Approver (can approve but not create)
DO $$
DECLARE
    v_role_id BIGINT;
BEGIN
    INSERT INTO roles (name, description)
    VALUES ('Sales Approver', 'Approve quotations and confirm orders')
    RETURNING id INTO v_role_id;

    INSERT INTO role_permissions (role_id, permission_id)
    SELECT v_role_id, id FROM permissions
    WHERE name IN (
        'sales.customer.view',
        'sales.quotation.view',
        'sales.quotation.approve',
        'sales.quotation.reject',
        'sales.order.view',
        'sales.order.confirm',
        'sales.order.cancel'
    );
END$$;

-- ============================================================================
-- SECTION 4: PERMISSION MANAGEMENT
-- ============================================================================

-- Grant additional permission to existing role
INSERT INTO role_permissions (role_id, permission_id)
SELECT
    (SELECT id FROM roles WHERE name = 'Sales Staff'),
    (SELECT id FROM permissions WHERE name = 'sales.customer.edit')
ON CONFLICT DO NOTHING;

-- Remove permission from role
DELETE FROM role_permissions
WHERE role_id = (SELECT id FROM roles WHERE name = 'Sales Staff')
  AND permission_id = (SELECT id FROM permissions WHERE name = 'sales.order.cancel');

-- Copy permissions from one role to another
INSERT INTO role_permissions (role_id, permission_id)
SELECT
    (SELECT id FROM roles WHERE name = 'New Role'),
    permission_id
FROM role_permissions
WHERE role_id = (SELECT id FROM roles WHERE name = 'Sales Staff')
ON CONFLICT DO NOTHING;

-- ============================================================================
-- SECTION 5: USER QUERIES
-- ============================================================================

-- View all permissions for a specific user
SELECT DISTINCT p.name, p.description
FROM user_roles ur
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE ur.user_id = 5  -- Replace with actual user ID
ORDER BY p.name;

-- View all roles assigned to a user
SELECT r.id, r.name, r.description
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = 5  -- Replace with actual user ID;

-- Check if user has specific permission
SELECT EXISTS(
    SELECT 1
    FROM user_roles ur
    JOIN role_permissions rp ON rp.role_id = ur.role_id
    JOIN permissions p ON p.id = rp.permission_id
    WHERE ur.user_id = 5  -- Replace with actual user ID
      AND p.name = 'delivery.order.create'
) as has_permission;

-- Find all users with a specific permission
SELECT DISTINCT u.id, u.username, u.email
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE p.name = 'delivery.order.confirm'
ORDER BY u.username;

-- Find users with multiple specific permissions (AND logic)
SELECT u.id, u.username, u.email
FROM users u
WHERE EXISTS (
    SELECT 1 FROM user_roles ur
    JOIN role_permissions rp ON rp.role_id = ur.role_id
    JOIN permissions p ON p.id = rp.permission_id
    WHERE ur.user_id = u.id AND p.name = 'delivery.order.create'
)
AND EXISTS (
    SELECT 1 FROM user_roles ur
    JOIN role_permissions rp ON rp.role_id = ur.role_id
    JOIN permissions p ON p.id = rp.permission_id
    WHERE ur.user_id = u.id AND p.name = 'delivery.order.confirm'
)
ORDER BY u.username;

-- Find users without any roles (orphaned users)
SELECT id, username, email
FROM users
WHERE id NOT IN (SELECT DISTINCT user_id FROM user_roles)
ORDER BY username;

-- ============================================================================
-- SECTION 6: ROLE STATISTICS
-- ============================================================================

-- Count users per role
SELECT
    r.name as role_name,
    COUNT(DISTINCT ur.user_id) as user_count
FROM roles r
LEFT JOIN user_roles ur ON ur.role_id = r.id
WHERE r.name LIKE 'Sales%' OR r.name LIKE 'Warehouse%'
GROUP BY r.id, r.name
ORDER BY user_count DESC, r.name;

-- Count permissions per role
SELECT
    r.name as role_name,
    COUNT(rp.permission_id) as permission_count
FROM roles r
LEFT JOIN role_permissions rp ON rp.role_id = r.id
GROUP BY r.id, r.name
ORDER BY permission_count DESC;

-- Most common permissions (granted to most roles)
SELECT
    p.name,
    p.description,
    COUNT(rp.role_id) as role_count
FROM permissions p
LEFT JOIN role_permissions rp ON rp.permission_id = p.id
WHERE p.name LIKE 'sales.%' OR p.name LIKE 'delivery.%'
GROUP BY p.id, p.name, p.description
ORDER BY role_count DESC, p.name;

-- ============================================================================
-- SECTION 7: BULK OPERATIONS
-- ============================================================================

-- Assign role to multiple users at once
INSERT INTO user_roles (user_id, role_id)
SELECT
    u.id,
    (SELECT id FROM roles WHERE name = 'Sales Staff')
FROM users u
WHERE u.email LIKE '%@sales.company.com'
  AND u.id NOT IN (SELECT user_id FROM user_roles WHERE role_id = (SELECT id FROM roles WHERE name = 'Sales Staff'))
ON CONFLICT DO NOTHING;

-- Remove specific role from all users
DELETE FROM user_roles
WHERE role_id = (SELECT id FROM roles WHERE name = 'Old Role Name');

-- Grant permission to all roles that have another permission
-- (e.g., if role can view, also grant create)
INSERT INTO role_permissions (role_id, permission_id)
SELECT DISTINCT
    rp.role_id,
    (SELECT id FROM permissions WHERE name = 'sales.order.create')
FROM role_permissions rp
JOIN permissions p ON p.id = rp.permission_id
WHERE p.name = 'sales.order.view'
ON CONFLICT DO NOTHING;

-- ============================================================================
-- SECTION 8: CLEANUP OPERATIONS
-- ============================================================================

-- Remove user from all roles
DELETE FROM user_roles WHERE user_id = 999;  -- Replace with actual user ID

-- Delete empty roles (no users and no permissions)
DELETE FROM roles
WHERE id NOT IN (SELECT DISTINCT role_id FROM user_roles)
  AND id NOT IN (SELECT DISTINCT role_id FROM role_permissions)
  AND name NOT IN ('Sales Manager', 'Sales Staff', 'Warehouse Staff');  -- Protect defaults

-- Remove orphaned permission assignments (role doesn't exist)
DELETE FROM role_permissions
WHERE role_id NOT IN (SELECT id FROM roles);

-- ============================================================================
-- SECTION 9: AUDIT QUERIES
-- ============================================================================

-- Recent role changes (requires audit table - add if needed)
-- This is a template - adjust based on your audit setup
/*
SELECT
    a.created_at,
    a.actor,
    a.action,
    a.entity_type,
    a.entity_id,
    a.details
FROM audit_logs a
WHERE a.entity_type IN ('user_roles', 'role_permissions')
ORDER BY a.created_at DESC
LIMIT 50;
*/

-- Users with elevated permissions (multiple roles)
SELECT
    u.id,
    u.username,
    u.email,
    COUNT(DISTINCT ur.role_id) as role_count,
    STRING_AGG(DISTINCT r.name, ', ') as roles
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN roles r ON r.id = ur.role_id
GROUP BY u.id, u.username, u.email
HAVING COUNT(DISTINCT ur.role_id) > 1
ORDER BY role_count DESC, u.username;

-- Permissions granted through multiple roles (redundant assignments)
SELECT
    u.username,
    p.name as permission,
    COUNT(DISTINCT ur.role_id) as granted_by_role_count,
    STRING_AGG(DISTINCT r.name, ', ') as roles
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
JOIN roles r ON r.id = ur.role_id
GROUP BY u.id, u.username, p.name
HAVING COUNT(DISTINCT ur.role_id) > 1
ORDER BY u.username, p.name;

-- ============================================================================
-- SECTION 10: TESTING SCENARIOS
-- ============================================================================

-- Test Scenario 1: Verify Sales Staff cannot approve
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1
            FROM role_permissions rp
            JOIN permissions p ON p.id = rp.permission_id
            WHERE rp.role_id = (SELECT id FROM roles WHERE name = 'Sales Staff')
              AND p.name = 'sales.quotation.approve'
        ) THEN 'FAIL: Sales Staff should not have approve permission'
        ELSE 'PASS: Sales Staff correctly lacks approve permission'
    END as test_result;

-- Test Scenario 2: Verify Sales Manager has all delivery permissions
SELECT
    CASE
        WHEN COUNT(*) = 8 THEN 'PASS: Sales Manager has all 8 delivery permissions'
        ELSE 'FAIL: Sales Manager missing delivery permissions (has ' || COUNT(*) || ' of 8)'
    END as test_result
FROM role_permissions rp
JOIN permissions p ON p.id = rp.permission_id
WHERE rp.role_id = (SELECT id FROM roles WHERE name = 'Sales Manager')
  AND p.name LIKE 'delivery.order.%';

-- Test Scenario 3: Verify Warehouse Staff has no edit customer permission
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1
            FROM role_permissions rp
            JOIN permissions p ON p.id = rp.permission_id
            WHERE rp.role_id = (SELECT id FROM roles WHERE name = 'Warehouse Staff')
              AND p.name = 'sales.customer.edit'
        ) THEN 'FAIL: Warehouse Staff should not edit customers'
        ELSE 'PASS: Warehouse Staff correctly cannot edit customers'
    END as test_result;

-- ============================================================================
-- NOTES
-- ============================================================================
--
-- Replace placeholder user IDs (5, 10, 15, etc.) with actual user IDs
-- from your users table before running these scripts.
--
-- Always test permission changes in a non-production environment first.
--
-- Use transactions when making bulk changes:
--   BEGIN;
--   -- Your changes here
--   COMMIT;  -- or ROLLBACK if something goes wrong
--
-- ============================================================================
