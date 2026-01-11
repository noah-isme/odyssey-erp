# RBAC Deployment Checklist - Phase 9.2

## Pre-Deployment

### Environment Preparation
- [ ] Staging environment available and matches production
- [ ] Database backup completed and verified
- [ ] Rollback plan documented and reviewed
- [ ] Deployment window scheduled (recommended: low-traffic period)
- [ ] On-call team notified and available
- [ ] Stakeholders informed of deployment timeline

### Code and Build Verification
- [ ] Latest code pulled from main branch
- [ ] Build successful: `go build ./cmd/server`
- [ ] No compiler errors in delivery module
- [ ] Handler code compiles: `go build ./internal/delivery/handler.go`
- [ ] Constants file compiles: `go build ./internal/shared/authz_sales_delivery.go`
- [ ] All non-handler tests passing (80/80 for delivery module)

### Migration Files Review
- [ ] Up migration exists: `migrations/000013_phase9_permissions.up.sql` (184 lines)
- [ ] Down migration exists: `migrations/000013_phase9_permissions.down.sql` (78 lines)
- [ ] SQL syntax validated (no syntax errors)
- [ ] Migration tested in staging environment
- [ ] Rollback tested in staging environment

### Documentation Review
- [ ] `RBAC_SETUP.md` reviewed (458 lines)
- [ ] `RBAC_QUICK_START.md` available for admins (279 lines)
- [ ] `RBAC_EXAMPLES.sql` tested (434 lines)
- [ ] `RBAC_TESTING_CHECKLIST.md` completed (484 lines)
- [ ] `PHASE_9_2_RBAC_SUMMARY.md` reviewed (656 lines)

---

## Staging Deployment

### Step 1: Apply Migration to Staging
```bash
migrate -path ./migrations \
        -database "postgresql://user:pass@staging-db:5432/odyssey?sslmode=disable" \
        up
```

- [ ] Migration executed successfully
- [ ] No errors in migration output
- [ ] Migration logged in `schema_migrations` table

### Step 2: Verify Migration Results

```sql
-- Check permissions count (should be 23)
SELECT COUNT(*) FROM permissions 
WHERE name LIKE 'sales.%' OR name LIKE 'delivery.%';

-- Check roles created (should be 3)
SELECT id, name FROM roles 
WHERE name IN ('Sales Manager', 'Sales Staff', 'Warehouse Staff');

-- Verify view exists
SELECT COUNT(*) FROM v_sales_delivery_permissions;
```

- [ ] 23 permissions created
- [ ] 3 default roles created
- [ ] Role-permission assignments correct
- [ ] Verification view accessible

### Step 3: Create Test Users

```sql
-- Create test users (adjust IDs as needed)
INSERT INTO user_roles (user_id, role_id)
SELECT 101, id FROM roles WHERE name = 'Sales Manager';

INSERT INTO user_roles (user_id, role_id)
SELECT 102, id FROM roles WHERE name = 'Sales Staff';

INSERT INTO user_roles (user_id, role_id)
SELECT 103, id FROM roles WHERE name = 'Warehouse Staff';
```

- [ ] Test admin user assigned Sales Manager role
- [ ] Test sales user assigned Sales Staff role
- [ ] Test warehouse user assigned Warehouse Staff role
- [ ] Test user with no roles created

### Step 4: Functional Testing in Staging

#### Test Admin User (Sales Manager)
- [ ] Can view delivery orders: `GET /delivery-orders` → 200 OK
- [ ] Can view delivery detail: `GET /delivery-orders/{id}` → 200 OK
- [ ] Can access create form: `GET /delivery-orders/new` → 200 OK
- [ ] Can create delivery order: `POST /delivery-orders` → Success
- [ ] Can access edit form: `GET /delivery-orders/{id}/edit` → 200 OK
- [ ] Can confirm delivery: `POST /delivery-orders/{id}/confirm` → Success
- [ ] Can ship delivery: `POST /delivery-orders/{id}/ship` → Success
- [ ] Can complete delivery: `POST /delivery-orders/{id}/complete` → Success
- [ ] Can cancel delivery: `POST /delivery-orders/{id}/cancel` → Success

#### Test Warehouse User
- [ ] Can view delivery orders: `GET /delivery-orders` → 200 OK
- [ ] Can confirm delivery: `POST /delivery-orders/{id}/confirm` → Success
- [ ] Can ship delivery: `POST /delivery-orders/{id}/ship` → Success
- [ ] Can complete delivery: `POST /delivery-orders/{id}/complete` → Success
- [ ] CANNOT access create form: `GET /delivery-orders/new` → 403 Forbidden
- [ ] CANNOT cancel delivery: `POST /delivery-orders/{id}/cancel` → 403 Forbidden

#### Test Sales User
- [ ] CANNOT view delivery orders: `GET /delivery-orders` → 403 Forbidden
- [ ] CANNOT access any delivery operations → All 403 Forbidden

#### Test User Without Roles
- [ ] CANNOT access any protected route → All 403 Forbidden

### Step 5: Performance Testing

```bash
# Run load test (adjust URL and auth token)
ab -n 1000 -c 10 -H "Cookie: session=..." http://staging/delivery-orders
```

- [ ] Average response time < 100ms
- [ ] No 500 errors
- [ ] No permission check timeouts
- [ ] Database CPU < 70%
- [ ] Memory usage stable

### Step 6: Rollback Test in Staging

```bash
migrate -path ./migrations \
        -database "postgresql://...staging..." \
        down 1
```

- [ ] Rollback executed successfully
- [ ] 23 permissions removed
- [ ] 3 roles removed (if no users assigned)
- [ ] View dropped
- [ ] No errors or warnings
- [ ] Application still functional

```bash
# Re-apply migration
migrate -path ./migrations \
        -database "postgresql://...staging..." \
        up
```

- [ ] Migration re-applied successfully
- [ ] All permissions recreated
- [ ] All roles recreated
- [ ] Ready for production deployment

---

## Production Deployment

### Pre-Production Final Checks
- [ ] All staging tests passed
- [ ] Deployment window confirmed
- [ ] Database backup completed (< 1 hour old)
- [ ] Backup verified and restorable
- [ ] Monitoring alerts configured
- [ ] On-call team on standby
- [ ] Rollback procedure ready

### Step 1: Maintenance Mode (Optional)
```bash
# If deploying during business hours, consider maintenance mode
```
- [ ] Users notified of brief maintenance
- [ ] Maintenance page activated (if applicable)

### Step 2: Database Backup
```bash
pg_dump -h production-db -U postgres odyssey > odyssey_backup_$(date +%Y%m%d_%H%M%S).sql
```
- [ ] Backup completed successfully
- [ ] Backup file size reasonable (matches expected)
- [ ] Backup stored in secure location

### Step 3: Apply Migration to Production
```bash
migrate -path ./migrations \
        -database "postgresql://user:pass@production-db:5432/odyssey?sslmode=disable" \
        up
```

- [ ] Migration executed successfully
- [ ] Execution time recorded: ________ seconds
- [ ] No errors in output
- [ ] Logged in deployment log

### Step 4: Verify Migration in Production

```sql
-- Critical verification queries
SELECT COUNT(*) FROM permissions 
WHERE name LIKE 'sales.%' OR name LIKE 'delivery.%';
-- Expected: 23

SELECT COUNT(*) FROM roles 
WHERE name IN ('Sales Manager', 'Sales Staff', 'Warehouse Staff');
-- Expected: 3

SELECT COUNT(*) FROM v_sales_delivery_permissions;
-- Expected: > 0 (should show assignments)
```

- [ ] 23 permissions verified
- [ ] 3 roles verified
- [ ] View accessible
- [ ] No orphaned records

### Step 5: Deploy Application Code
```bash
# Deploy updated application with RBAC constants
cd /path/to/odyssey-erp
git pull origin main
go build -o /opt/odyssey/server ./cmd/server
sudo systemctl restart odyssey-server
```

- [ ] Code deployed successfully
- [ ] Application started without errors
- [ ] Health check passed: `curl http://localhost:8080/health`
- [ ] Logs show no startup errors

### Step 6: Smoke Tests in Production

#### Test 1: Admin Access
- [ ] Login as admin user
- [ ] Access `/delivery-orders` → 200 OK
- [ ] View single delivery order → 200 OK
- [ ] No JavaScript errors in console

#### Test 2: Permission Enforcement
- [ ] Login as regular user (non-admin)
- [ ] Verify appropriate 403 errors where expected
- [ ] No system errors or crashes

#### Test 3: Database Queries
```sql
-- Verify permission checks are working
SELECT COUNT(*) FROM user_roles;
SELECT COUNT(*) FROM role_permissions;
```
- [ ] Queries execute quickly (< 10ms)
- [ ] No blocking locks
- [ ] No slow query warnings

### Step 7: Assign Roles to Production Users

```sql
-- Identify users who need roles
SELECT id, username, email FROM users 
WHERE id NOT IN (SELECT user_id FROM user_roles)
ORDER BY username;

-- Assign roles as appropriate
-- Example for a sales manager:
INSERT INTO user_roles (user_id, role_id)
SELECT <user_id>, id FROM roles WHERE name = 'Sales Manager'
ON CONFLICT DO NOTHING;
```

- [ ] Key users identified
- [ ] Roles assigned to department heads
- [ ] Roles assigned to sales team
- [ ] Roles assigned to warehouse team
- [ ] Users notified of new permissions

### Step 8: Resume Normal Operations
- [ ] Maintenance mode disabled (if enabled)
- [ ] Users notified of completion
- [ ] Application fully accessible

---

## Post-Deployment Monitoring

### First Hour
- [ ] Monitor error logs for 403 errors
- [ ] Check application logs for RBAC errors
- [ ] Monitor database CPU and memory
- [ ] Verify no spike in 500 errors
- [ ] Check response times (should be normal)

### First Day
- [ ] Review audit logs for authorization failures
- [ ] Monitor 403 error rate (should be < 0.5%)
- [ ] Check for any user complaints
- [ ] Verify all workflows functional
- [ ] Document any issues encountered

### First Week
- [ ] Gather user feedback
- [ ] Review permission requests
- [ ] Adjust roles if needed
- [ ] Monitor for performance degradation
- [ ] Update documentation based on feedback

### Metrics to Track

| Metric | Target | Actual | Status |
|--------|--------|--------|--------|
| 403 error rate | < 0.1% | ______% | [ ] |
| Avg permission check time | < 10ms | ______ms | [ ] |
| Support tickets (RBAC) | < 5/week | _______ | [ ] |
| Users without roles | 0 | _______ | [ ] |
| Application uptime | 99.9% | ______% | [ ] |

---

## Rollback Procedure (If Needed)

### When to Rollback
- Critical permission errors preventing normal operations
- Database performance degradation > 50%
- Multiple users unable to access required functions
- Security vulnerability discovered

### Rollback Steps

1. **Stop Application (Optional)**
```bash
sudo systemctl stop odyssey-server
```

2. **Rollback Database Migration**
```bash
migrate -path ./migrations \
        -database "postgresql://...production..." \
        down 1
```

3. **Verify Rollback**
```sql
-- Verify permissions removed
SELECT COUNT(*) FROM permissions 
WHERE name LIKE 'sales.%' OR name LIKE 'delivery.%';
-- Expected: 0
```

4. **Restore Previous Application Version**
```bash
git checkout <previous-version>
go build -o /opt/odyssey/server ./cmd/server
sudo systemctl start odyssey-server
```

5. **Verify Application**
- [ ] Application started successfully
- [ ] Users can access system normally
- [ ] No permission errors
- [ ] All features working

6. **Document Rollback**
- [ ] Rollback reason documented
- [ ] Time of rollback recorded
- [ ] Issues logged for analysis
- [ ] Stakeholders notified

---

## Troubleshooting

### Issue: Users Getting 403 Forbidden

**Diagnosis:**
```sql
SELECT p.name FROM user_roles ur
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE ur.user_id = <user_id>;
```

**Resolution:**
- Assign appropriate role to user
- Verify role has required permissions
- Check session is valid

### Issue: Migration Fails

**Diagnosis:**
- Check migration logs
- Verify database connectivity
- Check for conflicting permissions

**Resolution:**
- Review error message
- Fix conflicts manually if needed
- Re-run migration
- Consider rollback if unrecoverable

### Issue: Performance Degradation

**Diagnosis:**
```sql
-- Check slow queries
SELECT * FROM pg_stat_statements 
WHERE query LIKE '%permissions%'
ORDER BY total_time DESC LIMIT 10;
```

**Resolution:**
- Verify indexes on role_permissions and user_roles
- Check for table locks
- Review query plans
- Consider caching if needed

---

## Communication

### Pre-Deployment Announcement

**Subject:** Scheduled Deployment - RBAC Permissions (Phase 9.2)

**Body:**
```
We will be deploying Role-Based Access Control permissions for the 
Sales & Delivery modules on [DATE] at [TIME].

Expected downtime: < 5 minutes (if any)

New Features:
- Granular access control for delivery orders
- Improved security and audit trail
- Role-based permissions for sales team

Users may need to log out and log back in after deployment.

Contact: [SUPPORT EMAIL/PHONE]
```

### Post-Deployment Announcement

**Subject:** RBAC Deployment Complete - Action Required

**Body:**
```
The RBAC permission system has been successfully deployed.

Action Required:
- Sales managers: Verify you can access all delivery functions
- Warehouse staff: Check you can process deliveries
- If you encounter "403 Forbidden" errors, contact IT

Documentation: [LINK TO RBAC_QUICK_START.md]

Support: [SUPPORT EMAIL/PHONE]
```

---

## Sign-Off

### Deployment Completed By
- **Name:** _______________________
- **Date:** _______________________
- **Time:** _______________________

### Verification Completed By
- **Name:** _______________________
- **Date:** _______________________
- **Time:** _______________________

### Approved By
- **Engineering Lead:** _______________________ Date: __________
- **Operations Lead:** ________________________ Date: __________
- **Security Lead:** __________________________ Date: __________

---

## Deployment Summary

**Status:** [ ] Success [ ] Partial Success [ ] Rolled Back

**Deployment Duration:** ________ minutes

**Issues Encountered:**
```
(List any issues and resolutions)
```

**Rollback Required:** [ ] Yes [ ] No

**Notes:**
```
(Additional notes or observations)
```

---

## Next Steps

- [ ] Monitor for 7 days
- [ ] Collect user feedback
- [ ] Schedule follow-up review meeting
- [ ] Update documentation based on learnings
- [ ] Plan for next phase (9.3 - AR Invoice)

---

**Document Version:** 1.0  
**Last Updated:** Phase 9.2 RBAC Implementation  
**Owner:** DevOps Team  
**Status:** Ready for Use