# Phase 9: Sales & Delivery RBAC Permissions Setup

## Overview

This document describes the Role-Based Access Control (RBAC) permissions implementation for the Sales & Delivery modules in Odyssey ERP. The RBAC system provides granular access control for all sales and delivery operations, ensuring proper segregation of duties and security compliance.

## Architecture

### Permission Structure

Permissions follow a hierarchical naming convention:

```
<module>.<entity>.<action>
```

Examples:
- `sales.customer.view` - View customer records
- `delivery.order.create` - Create delivery orders
- `sales.quotation.approve` - Approve quotations

### Modules

1. **Sales Module** - Customer management, quotations, and sales orders
2. **Delivery Module** - Delivery order processing and fulfillment

## Permission Catalog

### Customer Permissions

| Permission | Description | Use Case |
|------------|-------------|----------|
| `sales.customer.view` | View customer records | Access customer list and details |
| `sales.customer.create` | Create new customers | Register new customers |
| `sales.customer.edit` | Edit customer information | Update customer details, credit limits |
| `sales.customer.delete` | Delete or deactivate customers | Mark customers as inactive |

### Quotation Permissions

| Permission | Description | Use Case |
|------------|-------------|----------|
| `sales.quotation.view` | View sales quotations | Access quotation list and details |
| `sales.quotation.create` | Create new quotations | Draft quotations for customers |
| `sales.quotation.edit` | Edit draft quotations | Modify quotation lines and prices |
| `sales.quotation.approve` | Approve submitted quotations | Authorize quotations for conversion |
| `sales.quotation.reject` | Reject quotations | Decline quotations with reasons |
| `sales.quotation.convert` | Convert quotations to sales orders | Generate SO from approved quotations |

### Sales Order Permissions

| Permission | Description | Use Case |
|------------|-------------|----------|
| `sales.order.view` | View sales orders | Access sales order list and details |
| `sales.order.create` | Create new sales orders | Create direct sales orders |
| `sales.order.edit` | Edit draft sales orders | Modify order lines and details |
| `sales.order.confirm` | Confirm sales orders | Lock orders for fulfillment |
| `sales.order.cancel` | Cancel sales orders | Cancel orders with reasons |

### Delivery Order Permissions

| Permission | Description | Use Case |
|------------|-------------|----------|
| `delivery.order.view` | View delivery orders | Access delivery order list and details |
| `delivery.order.create` | Create new delivery orders | Create DOs from sales orders |
| `delivery.order.edit` | Edit draft delivery orders | Modify delivery lines and quantities |
| `delivery.order.confirm` | Confirm delivery orders for picking | Authorize picking operations |
| `delivery.order.ship` | Mark delivery orders as shipped | Record shipment with tracking |
| `delivery.order.complete` | Complete delivery orders | Finalize delivery and update inventory |
| `delivery.order.cancel` | Cancel delivery orders | Cancel unfulfilled deliveries |
| `delivery.order.print` | Print packing lists and delivery documents | Generate delivery documentation |

## Default Roles

The system includes three pre-configured roles for common organizational structures:

### 1. Sales Manager

**Purpose:** Full access to all sales and delivery operations with supervisory capabilities.

**Permissions:**
- All customer permissions (view, create, edit, delete)
- All quotation permissions (view, create, edit, approve, reject, convert)
- All sales order permissions (view, create, edit, confirm, cancel)
- All delivery order permissions (view, create, edit, confirm, ship, complete, cancel, print)

**Typical Users:** Sales Directors, Sales Managers, Operations Managers

### 2. Sales Staff

**Purpose:** Basic sales operations without approval or cancellation rights.

**Permissions:**
- `sales.customer.view`
- `sales.customer.create`
- `sales.quotation.view`
- `sales.quotation.create`
- `sales.quotation.edit`
- `sales.order.view`
- `sales.order.create`
- `sales.order.edit`

**Typical Users:** Sales Representatives, Account Executives, Sales Coordinators

### 3. Warehouse Staff

**Purpose:** Warehouse and delivery operations focused on fulfillment.

**Permissions:**
- `delivery.order.view`
- `delivery.order.confirm`
- `delivery.order.ship`
- `delivery.order.complete`
- `delivery.order.print`

**Typical Users:** Warehouse Workers, Shipping Clerks, Logistics Coordinators

## Implementation

### Database Schema

Permissions are stored in the `permissions` table:

```sql
CREATE TABLE permissions (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT
);
```

Role-permission mappings are stored in `role_permissions`:

```sql
CREATE TABLE role_permissions (
    role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id BIGINT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);
```

### Code Constants

All permissions are defined as constants in `internal/shared/authz_sales_delivery.go`:

```go
const (
    PermCustomerView   = "sales.customer.view"
    PermCustomerCreate = "sales.customer.create"
    // ... etc
)
```

Helper functions are provided:
- `SalesScopes()` - Returns all sales permissions
- `DeliveryScopes()` - Returns all delivery permissions
- `AllSalesDeliveryScopes()` - Returns combined list

### Middleware Usage

Handlers use RBAC middleware for authorization:

```go
// Require ANY of the listed permissions
r.Group(func(r chi.Router) {
    r.Use(h.rbac.RequireAny(shared.PermDeliveryOrderView))
    r.Get("/delivery-orders", h.listDeliveryOrders)
})

// Require ALL of the listed permissions
r.Group(func(r chi.Router) {
    r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderConfirm))
    r.Post("/delivery-orders/{id}/confirm", h.confirmDeliveryOrder)
})
```

## Setup Instructions

### 1. Run Migration

Apply the permissions migration:

```bash
migrate -path ./migrations -database "postgres://..." up
```

This will:
- Create all 23 sales and delivery permissions
- Create 3 default roles (Sales Manager, Sales Staff, Warehouse Staff)
- Assign appropriate permissions to each role
- Create a verification view `v_sales_delivery_permissions`

### 2. Verify Permissions

Check that permissions were created:

```sql
SELECT name, description 
FROM permissions 
WHERE name LIKE 'sales.%' OR name LIKE 'delivery.%'
ORDER BY name;
```

View role-permission assignments:

```sql
SELECT * FROM v_sales_delivery_permissions
ORDER BY role_name, module, permission_name;
```

### 3. Assign Roles to Users

Assign roles to users using the RBAC service:

```sql
-- Assign "Sales Manager" role to user ID 5
INSERT INTO user_roles (user_id, role_id)
SELECT 5, id FROM roles WHERE name = 'Sales Manager';

-- Assign "Warehouse Staff" role to user ID 10
INSERT INTO user_roles (user_id, role_id)
SELECT 10, id FROM roles WHERE name = 'Warehouse Staff';
```

Or through the Go service:

```go
err := rbacService.AssignRoleToUser(ctx, userID, roleID)
```

### 4. Test Access Control

Verify that permissions are enforced:

```bash
# User with delivery.order.view can access list
curl -H "Cookie: session=..." http://localhost:8080/delivery-orders

# User without delivery.order.create gets 403
curl -X POST -H "Cookie: session=..." http://localhost:8080/delivery-orders
```

## Permission Matrix

### Read-Only User

Minimal view-only access:

- ✅ `sales.customer.view`
- ✅ `sales.quotation.view`
- ✅ `sales.order.view`
- ✅ `delivery.order.view`

### Standard Sales User

Can create and edit, but not approve or confirm:

- ✅ `sales.customer.view`, `sales.customer.create`, `sales.customer.edit`
- ✅ `sales.quotation.view`, `sales.quotation.create`, `sales.quotation.edit`
- ✅ `sales.order.view`, `sales.order.create`, `sales.order.edit`
- ❌ `sales.quotation.approve`, `sales.order.confirm`

### Fulfillment User

Can process deliveries but not create sales:

- ✅ `sales.order.view` (to see what needs fulfillment)
- ✅ All `delivery.order.*` permissions
- ❌ `sales.order.create`, `sales.order.edit`

### Admin User

Full access to all operations:

- ✅ All permissions in both modules

## Best Practices

### Segregation of Duties

1. **Separate Sales and Approval** - Sales staff should not approve their own quotations
2. **Separate Order Creation and Confirmation** - Different users should create vs. confirm orders
3. **Warehouse Independence** - Warehouse staff should not modify sales order quantities

### Permission Granularity

- Use `RequireAny()` for OR logic (e.g., view from multiple modules)
- Use `RequireAll()` for AND logic (e.g., both read and write needed)
- Prefer specific permissions over wildcard grants

### Role Design

- Create roles based on job functions, not individual users
- Keep roles simple and focused
- Use role hierarchies for complex organizations
- Document role purpose and typical users

### Audit Trail

All permission-protected actions are logged:
- User ID and username
- Action performed
- Timestamp
- Entity affected
- Result (success/failure)

## Troubleshooting

### User Gets 403 Forbidden

**Symptom:** User receives HTTP 403 when accessing a page

**Possible Causes:**
1. User not logged in (session expired)
2. User has no roles assigned
3. User's roles lack required permission
4. Permission name mismatch (check constants)

**Resolution:**
```sql
-- Check user's effective permissions
SELECT p.name 
FROM user_roles ur
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE ur.user_id = <user_id>;
```

### Permission Not Found

**Symptom:** "Permission not found" error in logs

**Resolution:**
1. Verify migration was applied: `SELECT * FROM permissions WHERE name = 'delivery.order.view'`
2. Check constant spelling: `shared.PermDeliveryOrderView`
3. Re-run migration if needed

### Role Assignment Fails

**Symptom:** Cannot assign role to user

**Resolution:**
1. Verify role exists: `SELECT * FROM roles WHERE name = 'Sales Manager'`
2. Check user exists: `SELECT * FROM users WHERE id = <user_id>`
3. Verify no constraint violations in `user_roles`

## Migration

### From String Literals to Constants

If you previously used string literals in handlers:

```go
// Before
r.Use(h.rbac.RequireAny("delivery.order.view"))

// After
r.Use(h.rbac.RequireAny(shared.PermDeliveryOrderView))
```

### Custom Permission Setup

To create custom roles beyond the defaults:

```sql
-- Create custom role
INSERT INTO roles (name, description)
VALUES ('Regional Manager', 'Regional sales oversight');

-- Get role and permission IDs
SELECT id FROM roles WHERE name = 'Regional Manager';
SELECT id FROM permissions WHERE name = 'sales.quotation.approve';

-- Assign permissions
INSERT INTO role_permissions (role_id, permission_id)
VALUES (<role_id>, <permission_id>);
```

## API Reference

### Go Functions

**Check User Permissions:**
```go
perms, err := rbacService.EffectivePermissions(ctx, userID)
```

**Assign Role:**
```go
err := rbacService.AssignRoleToUser(ctx, userID, roleID)
```

**Create Permission:**
```go
perm, err := rbacService.CreatePermission(ctx, name, description)
```

### SQL Queries

**List User Roles:**
```sql
SELECT r.name, r.description
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = ?;
```

**Check Permission:**
```sql
SELECT EXISTS(
    SELECT 1
    FROM user_roles ur
    JOIN role_permissions rp ON rp.role_id = ur.role_id
    JOIN permissions p ON p.id = rp.permission_id
    WHERE ur.user_id = ? AND p.name = ?
);
```

## Security Considerations

1. **Session Management** - All RBAC checks require valid session
2. **Permission Caching** - Permissions are not cached; checked on each request
3. **SQL Injection** - All queries use parameterized statements
4. **Audit Logging** - All authorization failures are logged
5. **Least Privilege** - Grant minimum permissions needed for job function

## Future Enhancements

- [ ] Row-level security (company/branch filtering)
- [ ] Time-based permissions (temporary access grants)
- [ ] Permission delegation (acting on behalf of another user)
- [ ] Dynamic permission evaluation (based on document state)
- [ ] Permission request workflow
- [ ] Role templates and quick-start wizards
- [ ] Permission analytics and reporting

## References

- RBAC Middleware: `internal/rbac/middleware.go`
- Permission Constants: `internal/shared/authz_sales_delivery.go`
- Migration: `migrations/000013_phase9_permissions.up.sql`
- Handler Implementation: `internal/delivery/handler.go`
- Database Schema: `migrations/000001_init.up.sql` (RBAC tables)

## Change Log

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2024-01 | Initial RBAC setup for Phase 9.2 |
| | | - 23 permissions defined |
| | | - 3 default roles created |
| | | - Delivery order handlers secured |
| | | - Documentation completed |

---

**Document Owner:** Engineering Team  
**Last Updated:** Phase 9.2 Implementation  
**Next Review:** Phase 10 or as needed