# RBAC Quick Start Guide - Sales & Delivery

## TL;DR - Get Started in 5 Minutes

### 1. Apply Migration

```bash
cd odyssey-erp
migrate -path ./migrations -database "postgresql://user:pass@localhost:5432/odyssey?sslmode=disable" up
```

### 2. Verify Installation

```sql
-- Check permissions were created (should show 23 rows)
SELECT COUNT(*) FROM permissions 
WHERE name LIKE 'sales.%' OR name LIKE 'delivery.%';

-- Check roles were created (should show 3 rows)
SELECT name FROM roles 
WHERE name IN ('Sales Manager', 'Sales Staff', 'Warehouse Staff');
```

### 3. Assign Roles to Users

```sql
-- Example: Make user #5 a Sales Manager
INSERT INTO user_roles (user_id, role_id)
SELECT 5, id FROM roles WHERE name = 'Sales Manager';

-- Example: Make user #10 Warehouse Staff
INSERT INTO user_roles (user_id, role_id)
SELECT 10, id FROM roles WHERE name = 'Warehouse Staff';
```

### 4. Test Access

Login as the user and try accessing:
- `/delivery-orders` - Should work if user has `delivery.order.view`
- `/delivery-orders/new` - Should work if user has `delivery.order.create`

---

## Common Tasks

### View User's Permissions

```sql
SELECT DISTINCT p.name, p.description
FROM user_roles ur
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE ur.user_id = 123  -- Replace with actual user ID
ORDER BY p.name;
```

### View All Role Assignments

```sql
SELECT * FROM v_sales_delivery_permissions
ORDER BY role_name, module;
```

### Create Custom Role

```sql
-- 1. Create the role
INSERT INTO roles (name, description)
VALUES ('Custom Role', 'Description here')
RETURNING id;

-- 2. Assign permissions (replace 999 with role ID from step 1)
INSERT INTO role_permissions (role_id, permission_id)
SELECT 999, id FROM permissions 
WHERE name IN (
    'sales.order.view',
    'delivery.order.view',
    'delivery.order.create'
);
```

### Remove User from Role

```sql
DELETE FROM user_roles
WHERE user_id = 123 AND role_id = (
    SELECT id FROM roles WHERE name = 'Sales Staff'
);
```

### Grant Additional Permission to Role

```sql
INSERT INTO role_permissions (role_id, permission_id)
SELECT 
    (SELECT id FROM roles WHERE name = 'Warehouse Staff'),
    (SELECT id FROM permissions WHERE name = 'delivery.order.create')
ON CONFLICT DO NOTHING;
```

---

## Default Roles Cheat Sheet

### Sales Manager (Full Access)
✅ Everything in sales and delivery modules

**Best for:** Department heads, operations managers

### Sales Staff (No Approvals)
✅ View/create/edit customers, quotations, orders  
❌ Cannot approve, confirm, or cancel

**Best for:** Sales reps, account managers

### Warehouse Staff (Fulfillment Only)
✅ View/confirm/ship/complete deliveries  
❌ Cannot create sales orders or edit customers

**Best for:** Warehouse workers, shipping clerks

---

## Permission Quick Reference

### Most Common Combinations

**Read-Only Auditor:**
- `sales.customer.view`
- `sales.quotation.view`
- `sales.order.view`
- `delivery.order.view`

**Order Entry Clerk:**
- `sales.customer.view`
- `sales.order.view`
- `sales.order.create`
- `sales.order.edit`

**Warehouse Manager:**
- All `delivery.order.*` permissions
- `sales.order.view` (to see what needs shipping)

**Customer Service Rep:**
- `sales.customer.view`
- `sales.customer.edit`
- `sales.order.view`
- `delivery.order.view`

---

## Troubleshooting

### User Gets 403 Forbidden

**Quick Fix:**
```sql
-- Check if user has ANY roles
SELECT r.name 
FROM user_roles ur 
JOIN roles r ON r.id = ur.role_id 
WHERE ur.user_id = 123;

-- If no results, assign a role (see "Assign Roles to Users" above)
```

### Permission Denied Despite Having Role

**Quick Fix:**
```sql
-- Verify the role has the required permission
SELECT p.name 
FROM role_permissions rp
JOIN permissions p ON p.id = rp.permission_id
WHERE rp.role_id = (SELECT id FROM roles WHERE name = 'Sales Staff');

-- If missing, grant it (see "Grant Additional Permission to Role" above)
```

### Can't Find a Role/Permission

**Quick Fix:**
```sql
-- List all sales/delivery permissions
SELECT name, description FROM permissions 
WHERE name LIKE 'sales.%' OR name LIKE 'delivery.%'
ORDER BY name;

-- List all roles
SELECT name, description FROM roles ORDER BY name;
```

---

## Permission Naming Convention

Format: `<module>.<entity>.<action>`

**Modules:**
- `sales` - Sales operations
- `delivery` - Fulfillment operations

**Entities:**
- `customer` - Customer records
- `quotation` - Sales quotations
- `order` - Sales orders
- `order` (in delivery context) - Delivery orders

**Actions:**
- `view` - Read access
- `create` - Create new records
- `edit` - Modify existing records
- `approve` - Approve/authorize
- `confirm` - Confirm for processing
- `ship` - Mark as shipped
- `complete` - Finalize/close
- `cancel` - Cancel/void
- `print` - Generate documents
- `delete` - Remove/deactivate

---

## Security Checklist

- [ ] All users have appropriate roles assigned
- [ ] No users have unnecessary admin permissions
- [ ] Sales staff cannot approve their own quotations
- [ ] Warehouse staff cannot modify order quantities
- [ ] Sensitive actions (confirm, approve, cancel) are restricted
- [ ] Test access with non-admin accounts
- [ ] Review permissions quarterly
- [ ] Audit logs are being collected

---

## One-Liners

```sql
-- Grant user full delivery access
INSERT INTO user_roles (user_id, role_id)
SELECT 456, id FROM roles WHERE name = 'Sales Manager';

-- Revoke all user roles
DELETE FROM user_roles WHERE user_id = 456;

-- List users with a specific permission
SELECT DISTINCT u.id, u.username, u.email
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE p.name = 'delivery.order.confirm';

-- Count users per role
SELECT r.name, COUNT(ur.user_id) as user_count
FROM roles r
LEFT JOIN user_roles ur ON ur.role_id = r.id
WHERE r.name IN ('Sales Manager', 'Sales Staff', 'Warehouse Staff')
GROUP BY r.name;

-- Find orphaned users (no roles)
SELECT id, username, email 
FROM users 
WHERE id NOT IN (SELECT user_id FROM user_roles);
```

---

## Next Steps

1. **Read Full Documentation:** See `RBAC_SETUP.md` for detailed information
2. **Plan Role Structure:** Map your org chart to roles
3. **Test in Staging:** Verify permissions before production
4. **Train Users:** Document which role users need for their job
5. **Monitor Access:** Review audit logs for unauthorized attempts

---

**Need Help?** Check the full RBAC documentation in `RBAC_SETUP.md`
