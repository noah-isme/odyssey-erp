# Migration Rehearsal Plan: Documents, CMMS, and QMS Modules

## Overview
This plan documents the rehearsal procedures for database migrations, data migrations, and rollback procedures for the three new modules. Each rehearsal must be performed in a staging environment before production deployment.

## Phase 0: Pre-Migration Rehearsals

### 0.1 Fresh Schema Deployment Rehearsal

**Objective**: Verify all new tables, indexes, constraints, and RLS policies deploy cleanly on an empty database.

**Procedure**:
1. Provision clean PostgreSQL instance (matching production version)
2. Run `make migrate-up` targeting Phase 0 migrations
3. Verify all tables exist with correct schema:
   ```sql
   -- Documents module
   \dt documents.* cmms.* qms.*
   
   -- Check RLS policies
   SELECT * FROM pg_policies WHERE schemaname IN ('documents', 'cmms', 'qms');
   
   -- Check indexes
   SELECT indexname, tablename FROM pg_indexes 
   WHERE schemaname IN ('documents', 'cmms', 'qms');
   ```
4. Verify foreign keys reference correct tables
5. Verify enum types created (if any)
6. Run `go test ./...` to verify sqlc generation works

**Success Criteria**:
- All tables created without errors
- All indexes created
- All RLS policies enabled
- All foreign keys valid
- sqlc generates without errors
- Application starts and connects

**Rollback**: `make migrate-down` (verify clean rollback to previous migration)

### 0.2 Upgrade Migration Rehearsal

**Objective**: Verify migrations apply correctly on existing production-like schema with data.

**Procedure**:
1. Restore production schema dump (anonymized) to staging
2. Apply Phase 0 migrations
3. Verify no data loss in existing tables
4. Verify new tables populated with seed data (permissions, default classifications)
5. Run application integration tests
6. Verify backward compatibility (legacy code paths still work)

**Success Criteria**:
- Migrations complete without errors
- Existing data intact
- New tables accessible
- Application functions with feature flags OFF
- No performance regression on existing queries

**Rollback**: `make migrate-down` to previous version, verify application works

### 0.3 Rollback Rehearsal

**Objective**: Verify clean rollback capability at each phase.

**Procedure**:
1. Apply Phase 0 migrations
2. Insert test data in new tables
3. Run `make migrate-down` for each migration in reverse order
4. Verify:
   - Tables dropped cleanly
   - No orphaned foreign keys
   - No data loss in pre-existing tables
   - Application starts on old schema

**Success Criteria**:
- All migrations rollback cleanly
- Zero data loss in existing tables
- Application functional post-rollback

## Phase 1: Document Management Migration Rehearsals

### 1.1 Portal Documents Migration Rehearsal

**Objective**: Migrate `portal_documents` (BYTEA) to managed object storage.

**Source Data**: `portal_documents` table (company_id, user_id, portal_type, filename, content_type, content BYTEA, created_at)

**Target**: Document Management tables (documents, document_versions, blobs, storage)

**Procedure**:
1. Enable dual-read mode (feature flag)
2. Run migration job:
   - For each portal_document:
     - Calculate SHA-256 of content
     - Detect MIME type (libmagic)
     - Scan for malware (ClamAV)
     - Upload to object storage (quarantined)
     - Create document identity
     - Create initial version (QUARANTINED → DRAFT)
     - Link to original portal record
3. Verify reconciliation:
   - Count match: portal_documents = document_versions created
   - Checksum match: all SHA-256 verified
   - Ownership match: company_id, user_id preserved
   - Download test: 100 random documents, compare bytes
4. Switch portal to Document Management service (feature flag)
5. Monitor dual-read for 1 week
6. Stop legacy writes
7. Archive `portal_documents` table (rename, don't drop)

**Reconciliation Queries**:
```sql
-- Count reconciliation
SELECT 
  (SELECT count(*) FROM portal_documents) as portal_count,
  (SELECT count(*) FROM document_versions dv 
   JOIN documents d ON d.id = dv.document_id
   WHERE d.migration_source = 'portal_documents') as dm_count;

-- Checksum reconciliation
SELECT pd.id, pd.filename, 
       encode(sha256(pd.content), 'hex') as portal_sha256,
       b.checksum_sha256 as blob_sha256,
       pd.content_type, b.detected_mime_type
FROM portal_documents pd
JOIN document_versions dv ON dv.migration_source_id = pd.id::text
JOIN blobs b ON b.id = dv.blob_id
WHERE encode(sha256(pd.content), 'hex') != b.checksum_sha256;

-- Ownership reconciliation
SELECT pd.company_id, pd.user_id, d.company_id, dv.created_by
FROM portal_documents pd
JOIN document_versions dv ON dv.migration_source_id = pd.id::text
JOIN documents d ON d.id = dv.document_id
WHERE pd.company_id != d.company_id OR pd.user_id != dv.created_by;
```

**Success Criteria**:
- 100% count match
- 100% checksum match
- 100% ownership match
- 100% download byte-match on sample
- Zero malware detections (or all quarantined with audit trail)
- Portal UI functional with new backend

**Rollback**: Disable feature flag, portal reverts to `portal_documents` table

### 1.2 Board Pack Migration Rehearsal

**Objective**: Move Board Pack storage behind shared storage interface.

**Procedure**:
1. Implement shared storage interface (see storage extraction plan)
2. Update Board Pack to use new interface
3. Verify existing board packs accessible
4. Verify new board packs use shared storage
5. No changes to Board Pack lifecycle or access behavior

**Success Criteria**:
- All existing board packs downloadable
- New board packs stored via shared interface
- No behavior changes for Board Pack users

## Phase 3: CMMS Migration Rehearsals

### 3.1 Fixed Assets Link Migration

**Objective**: Establish CMMS asset ↔ Fixed Asset links without data duplication.

**Procedure**:
1. Create CMMS assets referencing existing fixed assets
2. Verify operational condition separate from financial status
3. Test disposal blocking: disposed fixed asset → no new CMMS work orders
4. Test capitalization workflow: CMMS work order → approval → Fixed Asset creation

**Success Criteria**:
- Links created without data loss
- Disposal blocking enforced
- Capitalization workflow functional

## Phase 5: QMS Migration Rehearsals

### 5.1 MRP Quality Migration Rehearsal

**Objective**: Migrate MRP quality records to QMS as authoritative source.

**Source Tables**: `mrp_inspection_plans`, `quality_inspections`, `quality_holds`, `quality_ncrs`, `quality_capas`

**Target**: QMS tables (specifications, inspections, holds, NCRs, CAPAs)

**Procedure**:
1. Create QMS records with `migration_source` and `migration_source_id`
2. Backfill all data: plans, inspections, holds, NCRs, CAPAs, owners, links, lifecycle state
3. Reconcile:
   - Company ownership
   - Record counts per status
   - Status mapping (MRP → QMS)
   - Evidence links
   - User assignments
4. Switch MRP quality operations to QMS services (feature flag per company)
5. Make MRP quality tables read-only for migrated companies
6. Maintain compatibility views
7. Dual-read reconciliation for 2 weeks

**Reconciliation Queries**:
```sql
-- Inspection count by status
SELECT 
  qi.status as mrp_status,
  COUNT(*) as mrp_count,
  qms_inspections.status as qms_status,
  COUNT(qms_inspections.id) as qms_count
FROM quality_inspections qi
FULL JOIN qms_inspections ON qms_inspections.migration_source_id = qi.id::text
GROUP BY qi.status, qms_inspections.status;

-- Hold blocking verification
SELECT wo.id, wo.status, qh.status as hold_status
FROM work_orders wo
LEFT JOIN quality_holds qh ON qh.record_type = 'work_order' AND qh.record_id = wo.id
WHERE qh.status = 'OPEN';
```

**Success Criteria**:
- 100% record count match per status
- 100% ownership match
- 100% link preservation
- MRP cannot bypass QMS holds after cutover
- Compatibility views functional

**Rollback**: Disable feature flag, MRP reads legacy tables

## Phase 7: Production Rollout Rehearsal

### 7.1 Staging Certification

**Objective**: Full end-to-end rehearsal in staging with production-like data volume.

**Procedure**:
1. Run all Phase 0-6 migrations in sequence
2. Execute all data migrations
3. Run full test suite: `go test ./...`, `go vet ./...`, `make lint`
4. Run integration tests
5. Performance benchmarks (migration time, query latency)
6. Security scan (dependency check, SAST)
7. Documentation check: `make docs-check`
8. Sign-off from: Document Owner, Maintenance Owner, Quality Owner, Security, Finance, Operations

### 7.2 Per-Company Cutover Procedure

**For each company**:
1. Verify staging sign-off complete
2. Record migration checkpoint (timestamp, migration version, data counts)
3. Enable module feature flags for company
4. Run reconciliation queries
5. User acceptance testing
6. Record cutover completion
7. Rollback procedure documented and tested

**Rollback per Company**:
1. Disable feature flags
2. Restore from checkpoint (if data migration run)
3. Verify legacy functionality
4. Document rollback reason

## Rehearsal Schedule

| Rehearsal | Environment | Frequency | Owner |
|-----------|-------------|-----------|-------|
| Fresh schema | CI / Staging | Every PR | Platform |
| Upgrade | Staging | Pre-deploy | Platform |
| Rollback | Staging | Pre-deploy | Platform |
| Portal docs migration | Staging | Pre-Phase 1 | Documents team |
| Board pack migration | Staging | Pre-Phase 1 | Documents team |
| MRP quality migration | Staging | Pre-Phase 5 | QMS team |
| Full staging cert | Staging | Pre-Phase 7 | All leads |
| Per-company cutover | Production | Per company | Module owners |

## Validation Automation

Create rehearsal validation scripts:
```bash
# scripts/migration-rehearsal.sh
#!/bin/bash
set -euo pipefail

PHASE=${1:-0}
ENV=${2:-staging}

case $PHASE in
  0)
    ./scripts/rehearse-fresh-schema.sh $ENV
    ./scripts/rehearse-upgrade.sh $ENV
    ./scripts/rehearse-rollback.sh $ENV
    ;;
  1)
    ./scripts/rehearse-portal-migration.sh $ENV
    ./scripts/rehearse-boardpack-migration.sh $ENV
    ;;
  5)
    ./scripts/rehearse-mrp-quality-migration.sh $ENV
    ;;
  7)
    ./scripts/rehearse-staging-cert.sh $ENV
    ;;
esac
```

## Documentation Requirements

Each rehearsal must produce:
- [ ] Execution log with timestamps
- [ ] Reconciliation query results (saved as CSV/JSON)
- [ ] Performance metrics (migration duration, query latency)
- [ ] Issues found and resolutions
- [ ] Sign-off from responsible owner
- [ ] Updated rollback procedure if changed

## References
- [Missing Modules Plan](../archive/completed-../archive/completed-missing-modules-cmms-qms-documents-plan.md) Section 7, 8
- [ADR-001: Module Boundaries](./adr-001-module-boundaries.md)
- [Threat Model](./threat-model-documents-cmms-qms.md)
- [Permission Matrix](./permission-matrix-documents-cmms-qms.md)