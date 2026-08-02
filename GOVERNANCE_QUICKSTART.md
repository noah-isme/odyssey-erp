# Manufacturing Governance System - Quick Start Guide

## Prerequisites
- PostgreSQL 12+
- Go 1.21+
- Docker & Docker Compose
- Playwright (optional, for E2E tests)

## 1. Database Setup

### Apply Migration
```bash
cd /home/noah/project/odyssey-erp
make migrate-up
```

### Verify Tables Created
```bash
psql $PG_DSN -c "\dt mrp_*" | grep governance
```

Expected output: 10 tables with governance prefix

### Load Seed Data
```bash
psql $PG_DSN < scripts/seed_governance.sql
```

Verify seed data:
```sql
SELECT COUNT(*) FROM mrp_policy_versions;      -- Should be 5
SELECT COUNT(*) FROM mrp_actor_roles;          -- Should be 6
SELECT COUNT(*) FROM mrp_compliance_decisions; -- Should be 4
SELECT COUNT(*) FROM mrp_audit_events;         -- Should be 9
```

## 2. Build & Compile

```bash
# Build the application
make build

# Run linter
make lint

# Run all tests (should see 43/43 passing)
make test
```

## 3. Start Development Environment

### Option A: Full Stack with Docker
```bash
docker compose up -d
# Wait for services to start (10-15 seconds)
curl http://localhost:8080/health
```

### Option B: Hot Reload with Air
```bash
~/go/bin/air
# Application restarts on file changes
```

## 4. Test Governance API

### Submit a Decision
```bash
curl -X POST http://localhost:8080/mrp/decisions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
  -d '{
    "record_type": "BOM",
    "record_id": 1,
    "company_id": 1,
    "actor_id": 100,
    "actor_role": "QUALITY_LEAD",
    "action": "Approve",
    "reason": "BOM structure verified and complete"
  }'
```

Expected response:
```json
{
  "success": true,
  "message": "BOM ready for decision gate",
  "challenge_id": "challenge-1-100",
  "challenge_text": "Approve BOM-001 for production"
}
```

### Verify Challenge Signature
```bash
curl -X POST http://localhost:8080/mrp/decisions/1/verify-challenge \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
  -d '{
    "challenge_response": "signature-data-from-actor",
    "verifying_actor_id": 100,
    "verifying_company_id": 1
  }'
```

### Get Audit Log
```bash
curl http://localhost:8080/mrp/decisions/audit-log?page=1 \
  -H "Authorization: Bearer YOUR_AUTH_TOKEN"
```

## 5. Access Governance UI

Once server is running, navigate to:

### Decision Submission Form
```
http://localhost:8080/mrp/decisions/form
```
- Select record type (BOM, WorkOrder, QualityHold, etc.)
- Choose action (Approve, Release, Complete, etc.)
- Enter reason
- Submit for gate processing

### Audit Log Viewer
```
http://localhost:8080/mrp/decisions/audit
```
- View all governance decisions
- Filter by status (PENDING, APPROVED, REJECTED)
- Paginate through results
- Export to CSV for compliance

### Gate Status Display
```
http://localhost:8080/mrp/gates/BOM/status
http://localhost:8080/mrp/gates/WorkOrder/status
http://localhost:8080/mrp/gates/Hold/status
```
- View required actors for each gate
- See signature status
- Track approval progress

## 6. Run Tests

### Unit Tests
```bash
go test ./internal/mrp -v
```

Expected: 43/43 tests passing

### Integration Tests
```bash
go test ./internal/mrp -v -run "Integration"
```

Expected: 7 integration tests passing

### E2E Tests (requires Playwright)
```bash
# Install Playwright
npx playwright install

# Run E2E tests
npx playwright test e2e/odyssey.spec.ts
```

Expected: 10 governance E2E tests passing

## 7. Verify Routes Registered

```bash
# List all registered routes
curl http://localhost:8080/_routes | jq '.[] | select(.Pattern | contains("decisions"))'
```

Expected routes:
```
POST   /mrp/decisions
POST   /mrp/decisions/{id}/verify-challenge
GET    /mrp/decisions/audit-log
GET    /mrp/decisions/form
GET    /mrp/decisions/audit
GET    /mrp/gates/{gateType}/status
```

## 8. Test Multi-Actor Workflow

### Step 1: QUALITY_LEAD Submits Decision
```bash
curl -X POST http://localhost:8080/mrp/decisions \
  -d '{"record_type":"BOM","record_id":1,"actor_id":100,"actor_role":"QUALITY_LEAD",...}'
```

### Step 2: QUALITY_LEAD Signs Challenge
```bash
curl -X POST http://localhost:8080/mrp/decisions/1/verify-challenge \
  -d '{"challenge_response":"sig-100-1"}'
```

### Step 3: ENGINEERING Reviews & Signs
```bash
curl -X POST http://localhost:8080/mrp/decisions/1/verify-challenge \
  -d '{"challenge_response":"sig-101-1","verifying_actor_id":101,"verifying_company_id":1}'
```

### Step 4: Gate Completes
```bash
curl http://localhost:8080/mrp/decisions/audit-log
# See gate_completed event with status APPROVED
```

## 9. Troubleshooting

### Database Connection Error
```bash
# Verify PostgreSQL is running
docker compose logs db

# Check connection string
echo $PG_DSN
```

### Route Not Found
```bash
# Ensure migrations ran
make migrate-up

# Rebuild application
make build
```

### Test Failures
```bash
# Run with verbose output
go test ./internal/mrp -v -run "TestName"

# Check database state
psql $PG_DSN -c "SELECT COUNT(*) FROM mrp_compliance_decisions;"
```

## 10. Common Development Tasks

### Reset Database
```bash
# Rollback migrations
make migrate-down

# Reapply migrations
make migrate-up

# Reload seed data
psql $PG_DSN < scripts/seed_governance.sql
```

### Add New Decision Type
1. Add validator in `internal/mrp/validators.go`
2. Create staging gate in `internal/mrp/staging_gates.go`
3. Update handler routing in `internal/mrp/handler.go`
4. Add test cases in `internal/mrp/*_test.go`
5. Run `make test` to verify

### Export Audit Trail
```bash
# CSV export for compliance
curl http://localhost:8080/mrp/decisions/audit-log/export \
  -o governance_audit_$(date +%Y%m%d).csv
```

## Production Deployment

### Pre-Deployment Checklist
- [ ] All tests passing (43/43)
- [ ] Database migrations applied
- [ ] Seed data loaded
- [ ] API endpoints verified
- [ ] UI templates rendering
- [ ] E2E tests passing (10/10)
- [ ] Documentation reviewed

### Deployment Steps
```bash
# 1. Build optimized binary
make build

# 2. Apply database migrations
make migrate-up

# 3. Load initial seed data (optional)
psql $PG_DSN < scripts/seed_governance.sql

# 4. Start application
docker compose up -d

# 5. Verify health
curl http://localhost:8080/health

# 6. Test key endpoints
curl http://localhost:8080/mrp/decisions/audit-log
```

### Monitoring
```bash
# Check application logs
docker compose logs odyssey -f

# Monitor database
psql $PG_DSN -c "WATCH 'SELECT COUNT(*) FROM mrp_compliance_decisions;'"

# Performance stats
curl http://localhost:8080/_metrics
```

---

**For detailed documentation, see:**
- `GOVERNANCE_SUMMARY.md` - Complete system overview
- `DEPLOYMENT_CHECKLIST.md` - Validation procedures
- Git commit history - Phase-by-phase implementation
