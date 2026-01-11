# RBAC Testing Checklist - Sales & Delivery Permissions

## Overview

This checklist ensures that Role-Based Access Control (RBAC) for the Sales & Delivery modules is properly configured and functioning correctly before deployment to production.

---

## Pre-Deployment Checklist

### 1. Migration Verification

- [ ] Migration `000013_phase9_permissions.up.sql` applied successfully
- [ ] No errors in migration logs
- [ ] All 23 permissions created in database
- [ ] All 3 default roles created (Sales Manager, Sales Staff, Warehouse Staff)
- [ ] Verification view `v_sales_delivery_permissions` exists
- [ ] Down migration tested in staging environment

**Verification Query:**
```sql
SELECT COUNT(*) FROM permissions 
WHERE name LIKE 'sales.%' OR name LIKE 'delivery.%';
-- Expected: 23
```

### 2. Code Compilation

- [ ] `internal/shared/authz_sales_delivery.go` compiles without errors
- [ ] `internal/delivery/handler.go` compiles without errors
- [ ] No Go linter warnings
- [ ] All tests pass: `go test ./internal/delivery/...`
- [ ] Build successful: `go build ./cmd/server`

### 3. Documentation Review

- [ ] `RBAC_SETUP.md` reviewed and approved
- [ ] `RBAC_QUICK_START.md` accessible to administrators
- [ ] `RBAC_EXAMPLES.sql` tested in staging
- [ ] Runbook updated with RBAC procedures
- [ ] Help desk trained on common permission issues

---

## Functional Testing

### Test User Setup

Create 4 test users with different permission levels:

1. **test_admin** - Sales Manager role (all permissions)
2. **test_sales** - Sales Staff role (no approvals)
3. **test_warehouse** - Warehouse Staff role (fulfillment only)
4. **test_readonly** - No roles (should get 403 errors)

```sql
-- Example user creation
INSERT INTO users (username, email, password_hash) VALUES
('test_admin', 'admin@test.local', 'hash'),
('test_sales', 'sales@test.local', 'hash'),
('test_warehouse', 'warehouse@test.local', 'hash'),
('test_readonly', 'readonly@test.local', 'hash');

-- Assign roles
INSERT INTO user_roles (user_id, role_id)
SELECT 
    (SELECT id FROM users WHERE username = 'test_admin'),
    (SELECT id FROM roles WHERE name = 'Sales Manager');
-- Repeat for other users...
```

---

### Test Suite 1: Delivery Order View Permission

**Permission:** `delivery.order.view`

| User | Action | Expected Result | Pass/Fail |
|------|--------|-----------------|-----------|
| test_admin | GET /delivery-orders | 200 OK, list shown | [ ] |
| test_sales | GET /delivery-orders | 403 Forbidden | [ ] |
| test_warehouse | GET /delivery-orders | 200 OK, list shown | [ ] |
| test_readonly | GET /delivery-orders | 403 Forbidden | [ ] |
| test_admin | GET /delivery-orders/123 | 200 OK or 404 | [ ] |
| test_warehouse | GET /delivery-orders/123 | 200 OK or 404 | [ ] |
| test_sales | GET /delivery-orders/123 | 403 Forbidden | [ ] |

---

### Test Suite 2: Delivery Order Create Permission

**Permission:** `delivery.order.create`

| User | Action | Expected Result | Pass/Fail |
|------|--------|-----------------|-----------|
| test_admin | GET /delivery-orders/new | 200 OK, form shown | [ ] |
| test_sales | GET /delivery-orders/new | 403 Forbidden | [ ] |
| test_warehouse | GET /delivery-orders/new | 403 Forbidden | [ ] |
| test_admin | POST /delivery-orders | 201 Created or validation error | [ ] |
| test_warehouse | POST /delivery-orders | 403 Forbidden | [ ] |

---

### Test Suite 3: Delivery Order Edit Permission

**Permission:** `delivery.order.edit`

| User | Action | Expected Result | Pass/Fail |
|------|--------|-----------------|-----------|
| test_admin | GET /delivery-orders/123/edit | 200 OK, form shown | [ ] |
| test_sales | GET /delivery-orders/123/edit | 403 Forbidden | [ ] |
| test_warehouse | GET /delivery-orders/123/edit | 403 Forbidden | [ ] |
| test_admin | POST /delivery-orders/123/edit | 200 OK or validation error | [ ] |
| test_warehouse | POST /delivery-orders/123/edit | 403 Forbidden | [ ] |

---

### Test Suite 4: Delivery Order Confirm Permission

**Permission:** `delivery.order.confirm`

| User | Action | Expected Result | Pass/Fail |
|------|--------|-----------------|-----------|
| test_admin | POST /delivery-orders/123/confirm | 200 OK or 400 | [ ] |
| test_sales | POST /delivery-orders/123/confirm | 403 Forbidden | [ ] |
| test_warehouse | POST /delivery-orders/123/confirm | 200 OK or 400 | [ ] |

---

### Test Suite 5: Delivery Order Ship Permission

**Permission:** `delivery.order.ship`

| User | Action | Expected Result | Pass/Fail |
|------|--------|-----------------|-----------|
| test_admin | POST /delivery-orders/123/ship | 200 OK or 400 | [ ] |
| test_sales | POST /delivery-orders/123/ship | 403 Forbidden | [ ] |
| test_warehouse | POST /delivery-orders/123/ship | 200 OK or 400 | [ ] |

---

### Test Suite 6: Delivery Order Complete Permission

**Permission:** `delivery.order.complete`

| User | Action | Expected Result | Pass/Fail |
|------|--------|-----------------|-----------|
| test_admin | POST /delivery-orders/123/complete | 200 OK or 400 | [ ] |
| test_sales | POST /delivery-orders/123/complete | 403 Forbidden | [ ] |
| test_warehouse | POST /delivery-orders/123/complete | 200 OK or 400 | [ ] |

---

### Test Suite 7: Delivery Order Cancel Permission

**Permission:** `delivery.order.cancel`

| User | Action | Expected Result | Pass/Fail |
|------|--------|-----------------|-----------|
| test_admin | POST /delivery-orders/123/cancel | 200 OK or 400 | [ ] |
| test_sales | POST /delivery-orders/123/cancel | 403 Forbidden | [ ] |
| test_warehouse | POST /delivery-orders/123/cancel | 403 Forbidden | [ ] |

**Note:** Warehouse staff should NOT be able to cancel deliveries - only managers

---

### Test Suite 8: Sales Order Integration

**Permissions:** `delivery.order.view` OR `sales.order.view`

| User | Action | Expected Result | Pass/Fail |
|------|--------|-----------------|-----------|
| test_admin | GET /sales-orders/456/delivery-orders | 200 OK, list shown | [ ] |
| test_warehouse | GET /sales-orders/456/delivery-orders | 200 OK, list shown | [ ] |
| test_readonly | GET /sales-orders/456/delivery-orders | 403 Forbidden | [ ] |

---

### Test Suite 9: Session and Authentication

| Scenario | Expected Result | Pass/Fail |
|----------|-----------------|-----------|
| No session cookie | 403 Forbidden on all protected routes | [ ] |
| Expired session | Redirect to login or 403 | [ ] |
| Invalid session | Redirect to login or 403 | [ ] |
| Valid session, no roles | 403 Forbidden on all routes | [ ] |

---

### Test Suite 10: Edge Cases

| Scenario | Expected Result | Pass/Fail |
|----------|-----------------|-----------|
| User assigned to deleted role | System handles gracefully (no crash) | [ ] |
| Permission removed from role mid-session | Next request reflects new permissions | [ ] |
| User removed from all roles | All protected routes return 403 | [ ] |
| Multiple roles with overlapping permissions | Access granted (OR logic) | [ ] |
| Direct database role modification | Reflected on next request | [ ] |

---

## Security Testing

### Test Suite 11: Authorization Bypass Attempts

| Attack Vector | Expected Result | Pass/Fail |
|---------------|-----------------|-----------|
| Modify user_id in cookie | Session invalidated, 403 | [ ] |
| SQL injection in permission check | Parameterized query prevents injection | [ ] |
| CSRF token bypass | Request rejected without valid token | [ ] |
| Privilege escalation via role manipulation | Database constraints prevent | [ ] |
| Access with revoked permission | 403 Forbidden | [ ] |

---

### Test Suite 12: Audit Trail

| Action | Audit Log Entry | Pass/Fail |
|--------|-----------------|-----------|
| User accesses /delivery-orders | Request logged with user_id | [ ] |
| User gets 403 error | Authorization failure logged | [ ] |
| Role assigned to user | Change logged in audit table | [ ] |
| Permission granted to role | Change logged in audit table | [ ] |

**Verification Query:**
```sql
SELECT * FROM audit_logs 
WHERE entity_type IN ('user_roles', 'role_permissions')
ORDER BY created_at DESC 
LIMIT 20;
```

---

## Performance Testing

### Test Suite 13: Load Testing

| Metric | Target | Actual | Pass/Fail |
|--------|--------|--------|-----------|
| Permission check latency | < 10ms | ___ms | [ ] |
| Concurrent auth checks (100 users) | No errors | ___ errors | [ ] |
| Database query time (effective permissions) | < 5ms | ___ms | [ ] |
| Memory usage per permission check | < 1KB | ___KB | [ ] |

---

## Database Integrity

### Test Suite 14: Data Integrity

```sql
-- Run these queries and verify results

-- No orphaned role assignments
SELECT COUNT(*) FROM role_permissions rp
WHERE NOT EXISTS (SELECT 1 FROM roles r WHERE r.id = rp.role_id)
   OR NOT EXISTS (SELECT 1 FROM permissions p WHERE p.id = rp.permission_id);
-- Expected: 0

-- No orphaned user roles
SELECT COUNT(*) FROM user_roles ur
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = ur.user_id)
   OR NOT EXISTS (SELECT 1 FROM roles r WHERE r.id = ur.role_id);
-- Expected: 0

-- All default roles exist
SELECT COUNT(*) FROM roles 
WHERE name IN ('Sales Manager', 'Sales Staff', 'Warehouse Staff');
-- Expected: 3

-- All delivery permissions exist
SELECT COUNT(*) FROM permissions WHERE name LIKE 'delivery.order.%';
-- Expected: 8
```

- [ ] No orphaned role_permissions
- [ ] No orphaned user_roles
- [ ] All default roles present
- [ ] All 8 delivery permissions present
- [ ] All 5 sales order permissions present
- [ ] All 6 quotation permissions present
- [ ] All 4 customer permissions present

---

## Integration Testing

### Test Suite 15: End-to-End Workflows

#### Scenario 1: Sales to Delivery Flow

1. [ ] test_sales creates sales order (needs `sales.order.create`)
2. [ ] test_admin confirms sales order (needs `sales.order.confirm`)
3. [ ] test_warehouse views sales order (needs `sales.order.view`)
4. [ ] test_warehouse creates delivery order (needs `delivery.order.create`)
5. [ ] test_warehouse confirms delivery (needs `delivery.order.confirm`)
6. [ ] test_warehouse ships delivery (needs `delivery.order.ship`)
7. [ ] test_warehouse completes delivery (needs `delivery.order.complete`)
8. [ ] Sales order status updates automatically

#### Scenario 2: Permission Denied Flow

1. [ ] test_readonly attempts to view delivery orders → 403
2. [ ] test_sales attempts to confirm delivery → 403
3. [ ] test_warehouse attempts to cancel sales order → 403
4. [ ] All 403 errors logged correctly

---

## Rollback Testing

### Test Suite 16: Rollback Verification

Run down migration:
```bash
migrate -path ./migrations -database "..." down 1
```

- [ ] All 23 permissions removed from database
- [ ] All role_permissions entries removed
- [ ] Default roles removed (if no users assigned)
- [ ] View `v_sales_delivery_permissions` dropped
- [ ] No foreign key violations
- [ ] Application continues to function (other modules unaffected)

Run up migration again:
- [ ] Permissions recreated successfully
- [ ] Roles recreated successfully
- [ ] View recreated successfully
- [ ] All tests pass again

---

## Documentation Verification

- [ ] Admin documentation clear and accurate
- [ ] SQL examples tested and working
- [ ] Error messages documented
- [ ] Common issues and solutions documented
- [ ] Runbook includes RBAC troubleshooting
- [ ] User training materials prepared
- [ ] Help desk procedures updated

---

## Production Readiness

### Final Checklist

- [ ] All test suites passed (100% pass rate required)
- [ ] Security audit completed
- [ ] Performance benchmarks met
- [ ] Documentation complete and reviewed
- [ ] Rollback plan tested and documented
- [ ] On-call team briefed on RBAC system
- [ ] Monitoring alerts configured
- [ ] Backup taken before deployment
- [ ] Stakeholders notified of permission requirements
- [ ] User communication sent (if permissions change)

---

## Post-Deployment Monitoring

### Week 1 Checklist

- [ ] Monitor 403 error rate (should be < 0.1% of requests)
- [ ] Review audit logs for suspicious activity
- [ ] Check for performance degradation
- [ ] Gather user feedback on permission issues
- [ ] Document any issues encountered
- [ ] Adjust roles/permissions if needed

### Metrics to Track

| Metric | Target | Week 1 | Week 2 | Week 4 |
|--------|--------|--------|--------|--------|
| 403 error rate | < 0.1% | _____% | _____% | _____% |
| Support tickets (RBAC) | < 5/week | _____ | _____ | _____ |
| Avg permission check time | < 10ms | ____ms | ____ms | ____ms |
| Users without roles | 0 | _____ | _____ | _____ |

---

## Sign-Off

### Test Results

| Area | Tests Run | Tests Passed | Pass Rate | Tester | Date |
|------|-----------|--------------|-----------|--------|------|
| Functional | ____ | ____ | ___% | _______ | __/__/__ |
| Security | ____ | ____ | ___% | _______ | __/__/__ |
| Performance | ____ | ____ | ___% | _______ | __/__/__ |
| Integration | ____ | ____ | ___% | _______ | __/__/__ |

### Approval

- [ ] QA Lead: __________________ Date: __________
- [ ] Security Lead: _____________ Date: __________
- [ ] Engineering Lead: __________ Date: __________
- [ ] Product Owner: _____________ Date: __________

**Deployment Authorized:** Yes / No

**Notes:**
```

---

## Appendix A: Test Data Setup

### Create Test Sales Order

```sql
-- Create test customer
INSERT INTO customers (code, name, company_id, created_by)
VALUES ('CUST-TEST-001', 'Test Customer', 1, 1);

-- Create test sales order
INSERT INTO sales_orders (
    doc_number, company_id, customer_id, order_date,
    status, currency, created_by
)
VALUES (
    'SO-TEST-001', 1, 
    (SELECT id FROM customers WHERE code = 'CUST-TEST-001'),
    CURRENT_DATE, 'CONFIRMED', 'IDR', 1
);
```

### Create Test Delivery Order

```sql
INSERT INTO delivery_orders (
    doc_number, sales_order_id, warehouse_id, company_id,
    planned_date, status, created_by
)
VALUES (
    'DO-TEST-001',
    (SELECT id FROM sales_orders WHERE doc_number = 'SO-TEST-001'),
    1, 1, CURRENT_DATE, 'DRAFT', 1
);
```

---

## Appendix B: Useful Debug Queries

### Check User's Effective Permissions
```sql
SELECT p.name, p.description
FROM user_roles ur
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE ur.user_id = ?;
```

### Find Users With Specific Permission
```sql
SELECT DISTINCT u.username, u.email
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE p.name = 'delivery.order.confirm';
```

### Audit Failed Authorization Attempts
```sql
SELECT created_at, actor, action, details
FROM audit_logs
WHERE action LIKE '%403%' OR details LIKE '%forbidden%'
ORDER BY created_at DESC
LIMIT 50;
```

---

**Document Version:** 1.0  
**Last Updated:** Phase 9.2 RBAC Implementation  
**Owner:** QA Team  
**Review Frequency:** Every deployment