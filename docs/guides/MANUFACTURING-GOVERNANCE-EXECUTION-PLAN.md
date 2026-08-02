# Manufacturing Governance Execution Plan - Detailed Implementation Guide

**Status:** Planning phase complete. Ready for Phase 1 implementation.

**Last Updated:** 2026-08-03

---

## Executive Summary

This document breaks down the manufacturing governance execution plan from `docs/guides/manufacturing-governance-plan.md` into 33 concrete, sequenced implementation tasks across 7 phases.

**Key Milestones:**
- **Phase 1:** Schema, domain models, SQL queries (5 tasks)
- **Phase 2:** Compliance gate, signature challenges, snapshots (3 tasks)
- **Phase 3:** Mandatory prerequisites for 8 decision points (8 tasks)
- **Phase 4:** Quality lifecycle state machines (2 tasks)
- **Phase 5:** RBAC permissions, routes, SSR templates (3 tasks)
- **Phase 6:** Comprehensive test suites (8 tasks)
- **Phase 7:** Staging deployment, evidence collection, sign-off (4 tasks)

**Estimated effort:** 3-4 weeks for core implementation + 2 weeks staging/validation.

---

## Phase 1: Schema & Models (Critical Path)

### 1-1: Migration Files
**Files:**
- `migrations/000081_manufacturing_governance.up.sql`
- `migrations/000081_manufacturing_governance.down.sql`

**Schema tables required:**

```sql
-- Policy versions with versioned, effective-dated rules
CREATE TABLE policy_versions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    record_type VARCHAR(50) NOT NULL,           -- BOM, WorkOrder, Operation, etc.
    decision_name VARCHAR(50) NOT NULL,         -- Approve, Release, Close, etc.
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    enforcement_mode VARCHAR(20) NOT NULL,      -- DISABLED, WARN, ENFORCE
    signature_required BOOLEAN NOT NULL,
    approver_roles TEXT[] NOT NULL,             -- Array of required roles
    separation_of_duties BOOLEAN,               -- Creator != releaser
    required_evidence TEXT[] NOT NULL,          -- Types of evidence needed
    retention_period_days INT,
    version INT NOT NULL,
    status VARCHAR(20) NOT NULL,                -- DRAFT, ACTIVE, RETIRED
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL,
    UNIQUE(company_id, record_type, decision_name, effective_from, version)
);

-- Governance decisions made through compliance gate
CREATE TABLE compliance_decisions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    policy_version_id BIGINT NOT NULL REFERENCES policy_versions(id),
    record_type VARCHAR(50) NOT NULL,
    record_id BIGINT NOT NULL,
    action VARCHAR(50) NOT NULL,
    actor_id BIGINT NOT NULL REFERENCES users(id),
    reason TEXT,
    decision_id UUID NOT NULL UNIQUE,           -- Immutable identifier
    record_version VARCHAR(50),                 -- Snapshot version
    record_hash VARCHAR(64),                    -- SHA-256 of canonical snapshot
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id),
    INDEX idx_company_record (company_id, record_type, record_id)
);

-- One-time signature challenges
CREATE TABLE signature_challenges (
    id BIGSERIAL PRIMARY KEY,
    challenge_id UUID NOT NULL UNIQUE,
    policy_version_id BIGINT NOT NULL REFERENCES policy_versions(id),
    record_id BIGINT NOT NULL,
    record_version VARCHAR(50),
    expiry TIMESTAMPTZ NOT NULL,
    reauthentication_required BOOLEAN NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    INDEX idx_expiry (expiry)
);

-- Evidence records linked to decisions
CREATE TABLE evidence_records (
    id BIGSERIAL PRIMARY KEY,
    decision_id BIGINT NOT NULL REFERENCES compliance_decisions(id),
    evidence_type VARCHAR(50),                  -- Inspection, Hold, Signature, etc.
    content JSONB NOT NULL,                     -- Snapshot/verification data
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Immutable: no updates allowed after creation
    INDEX idx_decision (decision_id)
);

-- Immutable audit events with causation tracking
CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    correlation_id UUID NOT NULL,              -- Groups related decisions
    causation_id UUID,                         -- Links cause → effect
    decision_id BIGINT REFERENCES compliance_decisions(id),
    entity_type VARCHAR(50),
    entity_id BIGINT,
    action VARCHAR(50),
    actor_id BIGINT REFERENCES users(id),
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Immutable: no updates or deletes allowed
    INDEX idx_company_causation (company_id, causation_id),
    INDEX idx_entity (entity_type, entity_id)
);

-- Quality record lifecycles
CREATE TABLE quality_inspections (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    work_order_id BIGINT,
    operation_id BIGINT,
    inspection_plan_id BIGINT,
    status VARCHAR(20) NOT NULL,               -- PENDING, PASSED, FAILED, HOLD, RELEASED
    result_snapshot JSONB,
    result_version VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id)
);

CREATE TABLE quality_holds (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    inspection_id BIGINT REFERENCES quality_inspections(id),
    record_type VARCHAR(50),                   -- Work order, operation, etc.
    record_id BIGINT,
    status VARCHAR(20) NOT NULL,               -- OPEN, RELEASED
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id)
);

CREATE TABLE quality_ncrs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    number VARCHAR(50) NOT NULL,
    status VARCHAR(30) NOT NULL,               -- OPEN, INVESTIGATING, DISPOSITIONED, CLOSED
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id),
    UNIQUE(company_id, number)
);

CREATE TABLE quality_capas (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    number VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,               -- OPEN, IN_PROGRESS, VERIFICATION, CLOSED
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id),
    UNIQUE(company_id, number)
);

CREATE TABLE subcontract_receipts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    work_order_id BIGINT NOT NULL,
    operation_id BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL,               -- SENT, RECEIVED, INSPECTING, ACCEPTED, CLOSED
    sent_qty NUMERIC(12,4) NOT NULL,
    received_qty NUMERIC(12,4),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (company_id) REFERENCES companies(id)
);
```

### 1-2: Go Domain Types
**File:** `internal/mrp/governance_domain.go`

Key structs:
- `DecisionRequest` – what the user requests
- `DecisionGrant` – what the gate returns after validation
- `PolicyVersion` – versioned rules with effective dates
- `ComplianceDecision` – recorded decision with proof
- `SignatureChallenge` – one-time challenge for reauthentication
- `EvidenceRecord` – linked proof of decision
- `AuditEvent` – immutable causation tracking

### 1-3: SQL Queries
**File:** `sql/queries/mrp_governance.sql`

Query patterns:
- `CreatePolicyVersion`, `GetActivePolicy`, `ListPoliciesByCompany`
- `CreateComplianceDecision`, `GetDecision`, `ListDecisions`
- `CreateChallenge`, `GetChallenge`, `MarkChallengeUsed`
- `CreateEvidenceRecord`, `GetEvidence`
- `CreateAuditEvent`, `ListAuditEvents` (read-only)

### 1-4: sqlc Code Generation
Run `make sqlc-gen` to generate `internal/sqlc/mrp_governance.go`.

---

## Phase 2: Compliance Gate (Critical Path)

### 2-1: ComplianceGate Service
**File:** `internal/mrp/compliance_gate.go`

The 8-step flow in `DecideDecision(ctx, req) -> (grant, error)`:

1. **Lock & load** – SELECT ... FOR UPDATE on policy_versions; fail if no effective policy
2. **Snapshot** – Generate canonical JSON of the record being governed
3. **Hash** – SHA-256 of canonical snapshot (server-side, not client-supplied)
4. **Validate actor** – Check permissions, approver roles, actor ≠ creator if SOD required
5. **Verify challenge** – Check reauthentication challenge if policy requires it
6. **Store decision** – Insert into compliance_decisions with reason, hash, policy_version_id in same transaction
7. **Execute transition** – Perform the governed action (approve BOM, release WO, etc.)
8. **Audit event** – Append immutable audit_events row with correlation & causation IDs

All 8 steps in a single database transaction; if any step fails, entire transaction rolls back.

### 2-2: Signature Challenge Service
**File:** `internal/mrp/signature_challenge.go`

```go
func (s *ChallengeService) GenerateChallenge(ctx context.Context, policyVersionID, recordID int64, recordVersion string) (string, error)
func (s *ChallengeService) VerifyChallenge(ctx context.Context, challengeID string, reauthToken string) (bool, error)
```

- Challenge expiry: 5 minutes (enforced on verification)
- One-time use: marked as `used=true` after verification
- No password storage: only verify against existing auth service
- Reauthentication: use existing password verification endpoint

### 2-3: Record Snapshot Service
**File:** `internal/mrp/record_snapshot.go`

Functions to generate canonical snapshots:
- `SnapshotBOM(ctx, bomID)` – JSON of BOM + all lines
- `SnapshotWorkOrder(ctx, woID)` – JSON of WO + routing + operations
- `SnapshotOperation(ctx, opID)` – JSON of operation + schedules
- `SnapshotHold(ctx, holdID)` – JSON of hold + linked records
- `SnapshotNCR(ctx, ncrID)` – JSON of NCR + investigation data
- `SnapshotCAPE(ctx, capaID)` – JSON of CAPA + verification status

Each returns `(json []byte, hash string, error)`.

---

## Phase 3: Mandatory Prerequisites (8 Decision Points)

Each prerequisite validator returns `(passed bool, errors []string, error)`.

### 3-1: BOM Approval Prerequisites
**File:** `internal/mrp/prerequisites_bom.go`

Checks when approving a BOM revision:
- Status must be DRAFT
- All lines complete (no nil quantities or products)
- EffectiveFrom date is valid (not in past)
- No self-reference or circular BOM
- Manager signature exists if policy requires it

### 3-2: Work Order Release Prerequisites
**File:** `internal/mrp/prerequisites_workorder.go`

Checks when releasing (scheduled) a work order:
- BOM revision is APPROVED and effective
- Routing snapshot exists and is valid
- WIP locations are configured
- Work centers have sufficient capacity
- Required inspection plans are resolved
- No blocking compliance exception

### 3-3: Operation Completion Prerequisites
**File:** `internal/mrp/prerequisites_operation.go`

Checks when completing an operation:
- Prior operations in sequence are completed
- All mandatory inspections passed (no FAILED or HOLD)
- No quality hold on operation or work order
- Good/scrap quantities are valid (≥ 0, sum ≤ planned)
- Time entries are valid
- Operator/supervisor signature exists if policy requires

### 3-4: Finished Goods Receipt Prerequisites
**File:** `internal/mrp/prerequisites_receipt.go`

Checks when receiving finished goods and closing work order:
- All required operations completed
- All final inspections passed
- No open quality holds or blocking NCRs
- Issued WIP covers the receipt quantity
- Lot/serial genealogy is complete
- Receipt and close are idempotent (can retry without error)

### 3-5: Schedule Override Prerequisites
**File:** `internal/mrp/prerequisites_override.go`

Checks when manually rescheduling or splitting an operation:
- Override reason is provided
- Manager approval or signature if policy requires
- Original schedule is preserved (write before/after audit event)

### 3-6: Hold Release Prerequisites
**File:** `internal/mrp/prerequisites_hold.go`

Checks when releasing a quality hold:
- Disposition is provided (root cause, action)
- Actor has quality.approve permission
- Creator ≠ releaser if separation of duties is active

### 3-7: Quality Disposition Prerequisites
**File:** `internal/mrp/prerequisites_quality.go`

Checks when dispositioned a failed inspection or NCR:
- Failed mandatory inspections have a linked hold or NCR
- CAPA closure verified by someone other than the opener (if SOD active)

### 3-8: Subcontract Acceptance Prerequisites
**File:** `internal/mrp/prerequisites_subcontract.go`

Checks when accepting received subcontract goods:
- Received quantity ≤ sent quantity
- Mandatory incoming inspection passed
- Failed inspection creates or links a hold/NCR
- Supplier, quantity, inspection, and acceptance remain traceable

---

## Phase 4: Quality Lifecycle

### 4-1: State Machines
**File:** `internal/mrp/quality_lifecycle.go`

Define state transition rules:
- **Inspection:** PENDING → {PASSED, FAILED, HOLD} → RELEASED
- **Hold:** OPEN → RELEASED
- **NCR:** OPEN → INVESTIGATING → DISPOSITIONED → CLOSED
- **CAPA:** OPEN → IN_PROGRESS → VERIFICATION → CLOSED
- **Subcontract:** SENT → RECEIVED → INSPECTING → {ACCEPTED, CLOSED}

Each transition enforces:
- Valid state pair (no jumps)
- Required actors/permissions
- Prerequisite checks
- Immutable audit trail

### 4-2: Versioned Snapshots
**File:** `internal/mrp/quality_snapshots.go`

- Store inspection-plan criteria snapshots with version and timestamp
- Store result snapshots with record version (immutable after creation)
- Link historical versions to records; never mutate old snapshots

---

## Phase 5: RBAC & Interfaces

### 5-1: Permissions
**File:** `internal/mrp/governance_permissions.go`

Replace broad `mrp.manage` with:
- `mrp.planner` – Create and release work orders, manage schedules
- `mrp.operator` – Complete operations, report production
- `mrp.quality.inspect` – Create and record inspections
- `mrp.quality.approve` – Approve/release holds, disposition NCRs
- `mrp.manager` – Sign off decisions requiring manager approval
- `mrp.compliance.admin` – Activate policies, export evidence

### 5-2: Routes
**File:** `internal/mrp/governance_routes.go`

```
GET  /mrp/compliance/policies          – List active policies
POST /mrp/compliance/policies          – Create/activate new policy (compliance.admin)
GET  /mrp/compliance/challenges        – List challenges (debug)
GET  /mrp/compliance/decisions         – Search decisions (audit.view)
GET  /mrp/compliance/evidence          – Export evidence as CSV (compliance.admin)

GET  /mrp/quality/inspections          – List inspections
POST /mrp/quality/inspections          – Create inspection (quality.inspect)
POST /mrp/quality/inspections/:id/approve – Pass/fail inspection (quality.inspect)
POST /mrp/quality/inspections/:id/hold    – Create hold (quality.inspect)

GET  /mrp/quality/holds                – List holds
POST /mrp/quality/holds/:id/release    – Release hold with disposition (quality.approve)

GET  /mrp/quality/nonconformances      – List NCRs
POST /mrp/quality/nonconformances      – Create NCR (quality.inspect)
POST /mrp/quality/nonconformances/:id/disposition – Disposition (quality.approve)

GET  /mrp/quality/capas                – List CAPAs
POST /mrp/quality/capas                – Create CAPA (quality.approve)
POST /mrp/quality/capas/:id/verify     – Verify CAPA (quality.approve, non-creator)
```

### 5-3: SSR Templates
**Directory:** `web/templates/mrp/compliance/` and `web/templates/mrp/quality/`

Templates render:
- Policy CRUD and activation status
- Challenge details (expiry, usage)
- Decision history with audit trail
- Evidence export UI (CSV download)
- Inspection record form and results
- Hold creation and release form
- NCR workflow and disposition tracking
- CAPA tracking and verification

---

## Phase 6: Testing & Validation (8 Test Suites)

### 6-1: Policy Resolution Tests
**File:** `internal/mrp/governance_test.go`

Table-driven tests for:
- Load effective policy by company, record type, action, effective date
- Correct version resolution (only ACTIVE versions apply)
- Enforcement mode (DISABLED, WARN, ENFORCE) affects gate behavior
- Expired policies are skipped

### 6-2: Compliance Gate Tests
**File:** `internal/mrp/compliance_gate_test.go`

- Successful decision with all prerequisites met
- Fails when prerequisite check fails
- Fails when actor lacks required permission
- Fails when approval signature missing but required
- Fails when separation of duties violated
- Gateway blocks invalid state transitions

### 6-3: Signature Challenge Tests
**File:** `internal/mrp/signature_challenge_test.go`

- Challenge expires after 5 minutes
- Challenge marked `used` after one verification
- Replay attempt fails (used challenges rejected)
- Tampering detected (modified challenge_id rejected)
- Record version mismatch detected
- Reauthentication failure blocks decision

### 6-4: RBAC & Separation-of-Duties Tests
**File:** `internal/mrp/rbac_test.go`

- Route 401/403 for missing/wrong permissions
- SOD enforced: creator cannot release own hold (for configured policies)
- SOD enforced: CAPA opener cannot verify own CAPA
- All roles tested on all governed decision points

### 6-5: Quality Lifecycle Tests
**File:** `internal/mrp/quality_lifecycle_test.go`

- Inspection: PENDING → PASSED, PENDING → FAILED → HOLD
- Hold: OPEN → RELEASED (with disposition)
- NCR: OPEN → INVESTIGATING → DISPOSITIONED → CLOSED
- CAPA: OPEN → IN_PROGRESS → VERIFICATION → CLOSED
- Subcontract: SENT → RECEIVED → INSPECTING → ACCEPTED
- Invalid transitions rejected

### 6-6: Concurrency Tests
**File:** `internal/mrp/concurrency_test.go`

- Failed prerequisite check rolls back entire transaction (no partial updates)
- Concurrent work order close/receipt requests post inventory exactly once
- Concurrent CAPA verification doesn't double-verify
- Idempotent receipt and close (can retry successfully)

### 6-7: Company Isolation Tests
**File:** `internal/mrp/isolation_test.go`

- Company A policies never affect Company B decisions
- CSV evidence export only includes Company A records
- Cross-company queries return empty results
- Database constraints enforce company_id scoping

### 6-8: Security & Audit Tests
**File:** `internal/mrp/security_test.go`

- SSR form validation prevents injection
- CSRF tokens checked on POST/PUT/DELETE
- Safe error messages (no internal details leaked)
- Audit events are immutable (no updates/deletes)
- Correlation and causation IDs track event chains

---

## Phase 7: Staging & Rollout

### 7-1: Deploy to Staging (WARN Mode)
- Run fresh schema migration against staging database
- Backfill existing policies with `ENFORCE` mode = false (WARN)
- Route all governed transitions through compliance gate
- Observe and fix missing prerequisites in business processes
- No production traffic affected

### 7-2: Switch to ENFORCE
- Move staging company to `ENFORCE` mode = true
- Run comprehensive negative-path tests
- Run concurrency/recovery tests
- Measure performance impact
- Rehearse rollback

### 7-3: Collect Evidence
Mandatory documentation for sign-off:
- Fresh schema migration success logs
- Rollback rehearsal (backup restore) success
- Permission matrix (role → decision points)
- Test evidence: all controlled decisions succeed with proof, fail without
- Audit immutability: audit events cannot be updated/deleted
- Company isolation: cross-company queries return empty
- CSV export completeness and correctness
- Existing non-regulated companies continue operating

### 7-4: Obtain Sign-Off & Production Enablement
- Manufacturing manager approval
- Quality owner approval
- Security review approval
- Operations approval

Then:
- Enable `ENFORCE` per production company (one at a time, no global cutover)
- Monitor for 1 week per company
- Adjust policies based on feedback
- Document runbooks for policy maintenance

---

## Risk Mitigation

**Rollback plan:**
- Schema is backward-compatible (new tables, no breaking changes to existing tables)
- Compliance gate is guarded by policy activation (company-level feature flag)
- Non-regulated companies bypass gate entirely until their profile is activated
- Rollback: disable policies → gate becomes no-op → existing logic path still works

**Performance:**
- Signature challenges have 5-minute expiry; stale entries cleaned on read
- Policy version locks are row-level and short-lived (decision duration)
- Audit events use append-only inserts (no deletes/updates)
- Indexes on company_id and record_type for fast lookups

**Data integrity:**
- All decision logic in single transaction (no partial state)
- Snapshot hashes prevent silent tampering
- Immutable audit trail enables forensics
- Concurrent receipt/close uses database locking (FOR UPDATE)

---

## Checkpoints

| Checkpoint | Gate | Feedback Loop |
|---|---|---|
| Phase 1 complete | Schema migrates, sqlc compiles, all types build | `make sqlc-gen && go build ./...` |
| Phase 2 complete | Unit tests pass for gate, challenges, snapshots | `go test ./internal/mrp -run TestGate` |
| Phase 3 complete | Prerequisite tests pass for all 8 decision types | `go test ./internal/mrp -run TestPrerequisite` |
| Phase 4 complete | Lifecycle state machine tests pass | `go test ./internal/mrp -run TestLifecycle` |
| Phase 5 complete | Routes render, permissions enforced, templates valid | `go test ./internal/mrp -run TestRBAC && make lint` |
| Phase 6 complete | All tests pass, coverage >80% for gate & prereqs | `make test && go test ./... -cover` |
| Phase 7 complete | Staging ENFORCE mode stable, evidence collected, sign-offs obtained | Manual verification + sign-off checklist |

---

## Next Steps

1. Create Phase 1 migration files (schema)
2. Define Go domain types (governance_domain.go)
3. Write SQL queries (mrp_governance.sql)
4. Run `make sqlc-gen` and verify compilation
5. Implement ComplianceGate service with full 8-step flow
6. Implement prerequisite validators for each decision point
7. Add SSR routes and templates
8. Write comprehensive test suites
9. Deploy to staging in WARN mode
10. Collect evidence and obtain sign-offs
11. Enable ENFORCE per production company

