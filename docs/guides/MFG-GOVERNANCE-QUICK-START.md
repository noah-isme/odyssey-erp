# Manufacturing Governance - Quick Start Guide

**Status:** Planning complete. Ready for Phase 1 implementation.
**Created:** 2026-08-03
**Effort Estimate:** 3–4 weeks implementation + 2 weeks staging/validation

---

## What's Being Built

Manufacturing governance closes two gaps in the MRP module:

1. **Enforce controlled-record policies** at every governed manufacturing decision
2. **Add formal staging certification** before production enablement

This is NOT a standalone QMS. Quality remains embedded within manufacturing execution. The separate enterprise QMS is planned separately.

---

## Scope: 8 Governed Decision Points

| Decision | Record | Enforcement |
|----------|--------|-------------|
| Approve BOM revision | BOM | Draft→Approved (manager signature) |
| Release work order | Work order | Approved BOM + routing validated |
| Complete operation | Operation | Prior ops done + inspections passed + no holds |
| Receive finished goods | Work order | All ops/inspections done + genealogy complete |
| Override schedule | Operation | Reason required + manager approval |
| Release quality hold | Hold | Disposition + quality.approve permission |
| Disposition NCR | NCR | Investigation + disposition decision |
| Accept subcontract | Subcontract op | Qty validated + mandatory inspection passed |

---

## Architecture: 3 Core Components

### 1. Compliance Gate (Central Service)
- **File:** `internal/mrp/compliance_gate.go`
- **Method:** `DecideDecision(ctx, request) → (grant, error)`
- **Flow (8 steps, atomic transaction):**
  1. Lock & load effective policy for company/record/action
  2. Generate server-side canonical snapshot of record
  3. Hash snapshot (SHA-256)
  4. Validate actor permission, approver role, separation of duties
  5. Verify one-time reauthentication challenge (if required)
  6. Store decision with reason, hash, policy version
  7. Execute the governed state transition
  8. Append immutable audit event with correlation/causation IDs

All 8 steps in single database transaction; rollback on any failure.

### 2. Signature Challenge (One-Time Proof)
- **File:** `internal/mrp/signature_challenge.go`
- **Challenge generation:** 5-minute expiry, one-time use
- **Reauthentication:** Password verification via existing auth service (v1)
- **No password storage:** Only verify against live auth service

### 3. Record Snapshot (Server-Generated)
- **File:** `internal/mrp/record_snapshot.go`
- **Replace:** Client-supplied JSON → server canonical snapshots
- **Content:** Full record + all related lines as immutable JSON
- **Hash:** SHA-256 of canonical form prevents tampering
- **Keep:** Historical snapshots immutable when record versioned

---

## Database Schema (6 New Tables)

```sql
policy_versions         -- Versioned, effective-dated rules (DRAFT→ACTIVE→RETIRED)
compliance_decisions    -- Decisions made through gate with proof
signature_challenges    -- One-time challenges with 5-min expiry
evidence_records        -- Immutable proof linked to decisions
audit_events           -- Immutable causation tracking (correlation + causation IDs)
quality_*              -- Inspection, Hold, NCR, CAPA, Subcontract lifecycle tables
```

See detailed schema in `docs/guides/MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md`.

---

## RBAC: 6 New Permissions

| Permission | Can Do |
|------------|--------|
| `mrp.planner` | Create/release work orders, manage schedules |
| `mrp.operator` | Complete operations, report production |
| `mrp.quality.inspect` | Create/record inspections |
| `mrp.quality.approve` | Approve/release holds, disposition NCRs |
| `mrp.manager` | Sign off decisions requiring manager approval |
| `mrp.compliance.admin` | Activate policies, export evidence |

Replaces broad `mrp.manage` route guard with specific decision-level permissions.

---

## Implementation Phases

### Phase 1: Schema & Models (5 tasks, ~3 days)
- Create migration with 6 new tables
- Define Go types (DecisionRequest, DecisionGrant, PolicyVersion, etc.)
- Write SQL queries for policy/decision/challenge/evidence CRUD
- Run `make sqlc-gen`
- **Validation:** `make sqlc-gen && go build ./...`

### Phase 2: Compliance Gate (3 tasks, ~4 days)
- Implement ComplianceGate service (8-step atomic flow)
- Implement SignatureChallenge service (expiry, replay enforcement)
- Implement RecordSnapshot service (canonical snapshots + SHA-256)
- **Validation:** Unit tests pass for gate, challenges, audit immutability

### Phase 3: Prerequisites (8 tasks, ~4 days)
- BOM approval (draft, complete lines, no cycles)
- Work order release (approved BOM, routing, capacity)
- Operation completion (prior ops, inspections, no holds)
- Finished goods receipt (operations done, genealogy complete)
- Schedule override (reason required, manager approval)
- Hold release (disposition, separation of duties)
- Quality disposition (linked holds/NCRs)
- Subcontract acceptance (qty check, mandatory inspection)
- **Validation:** Each prerequisite validated correctly, decisions fail without proof

### Phase 4: Quality Lifecycle (2 tasks, ~2 days)
- State machines for Inspection, Hold, NCR, CAPA, Subcontract
- Versioned snapshots with immutability enforcement
- **Validation:** Lifecycle tests pass, state transitions enforced

### Phase 5: RBAC & Interfaces (3 tasks, ~3 days)
- Define 6 permissions in permission system
- Add HTTP routes with permission guards
- Create SSR templates for policy admin, challenges, decisions, evidence, quality records
- **Validation:** Routes enforce permissions, templates render, no CSRF issues

### Phase 6: Testing (8 test suites, ~5 days)
- Policy resolution (table-driven by company, action, date, version)
- Compliance gate (succeeds with proof, fails without)
- Signature challenges (expiry, replay, tampering, reauthentication)
- RBAC & separation of duties (all roles, SOD violations rejected)
- Quality lifecycle (state transitions enforced)
- Concurrency (idempotent close/receipt, no race conditions)
- Company isolation (cross-company queries return empty)
- Security & audit (validation, CSRF, safe errors, immutable events)
- **Validation:** `go test ./...` passes, coverage >80%

### Phase 7: Staging & Rollout (4 steps, ~2 weeks)
1. **Deploy to staging (WARN mode):** Policies don't block, violations logged
2. **Switch to ENFORCE:** Policies actively block invalid transitions
3. **Collect evidence:** Migration success, rollback success, all tests passing
4. **Obtain sign-offs:** Manufacturing mgr, Quality owner, Security, Ops
5. **Enable per company:** NO global cutover, one company at a time

---

## File Inventory (33 Total)

### Critical Path Files (Phase 1–2)
```
migrations/000081_manufacturing_governance.up.sql       -- Schema
migrations/000081_manufacturing_governance.down.sql      -- Rollback
internal/mrp/governance_domain.go                        -- Types
internal/mrp/compliance_gate.go                          -- 8-step gate
internal/mrp/signature_challenge.go                      -- Challenge/reauthentication
internal/mrp/record_snapshot.go                          -- Snapshots + hashing
sql/queries/mrp_governance.sql                          -- SQL CRUD
internal/sqlc/mrp_governance.go                         -- Generated (make sqlc-gen)
```

### Prerequisite Validators (Phase 3)
```
internal/mrp/prerequisites_bom.go
internal/mrp/prerequisites_workorder.go
internal/mrp/prerequisites_operation.go
internal/mrp/prerequisites_receipt.go
internal/mrp/prerequisites_override.go
internal/mrp/prerequisites_hold.go
internal/mrp/prerequisites_quality.go
internal/mrp/prerequisites_subcontract.go
```

### Quality Lifecycle (Phase 4)
```
internal/mrp/quality_lifecycle.go                        -- State machines
internal/mrp/quality_snapshots.go                        -- Versioned snapshots
```

### Routes & RBAC (Phase 5)
```
internal/mrp/governance_permissions.go                   -- 6 permissions
internal/mrp/governance_routes.go                        -- HTTP routes
web/templates/mrp/compliance/policies.html
web/templates/mrp/compliance/challenges.html
web/templates/mrp/compliance/decisions.html
web/templates/mrp/compliance/evidence.html
web/templates/mrp/quality/inspections.html
web/templates/mrp/quality/holds.html
web/templates/mrp/quality/nonconformances.html
web/templates/mrp/quality/capas.html
```

### Test Suites (Phase 6)
```
internal/mrp/governance_test.go
internal/mrp/compliance_gate_test.go
internal/mrp/signature_challenge_test.go
internal/mrp/rbac_test.go
internal/mrp/quality_lifecycle_test.go
internal/mrp/concurrency_test.go
internal/mrp/isolation_test.go
internal/mrp/security_test.go
```

### Documentation
```
docs/guides/manufacturing-governance-plan.md                    -- Original plan
docs/guides/MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md         -- Detailed breakdown (NEW)
docs/guides/MANUFACTURING-GOVERNANCE-EXECUTION-MAP.md          -- Visual map (NEW)
docs/guides/MFG-GOVERNANCE-QUICK-START.md                      -- This file
```

---

## Validation Gates

| Gate | Success | Blocker |
|------|---------|---------|
| Schema migration | Runs, no SQL errors | YES |
| sqlc generation | Compiles, all types valid | YES |
| Gate unit tests | Challenges expire, audit immutable | YES |
| Prerequisite tests | All 8 types validated | YES |
| RBAC tests | Permissions enforced | YES |
| Concurrency tests | Idempotent, no races | YES |
| Staging WARN mode | All functions work, violations logged | NO (warning) |
| Staging ENFORCE mode | All rules enforced | YES |
| Evidence collection | All docs complete | YES (for sign-off) |
| Sign-offs | Mgmt, Quality, Security, Ops | YES (for production) |

---

## Risk Mitigation

**Rollback is safe:**
- Schema is backward-compatible (new tables only)
- Compliance gate guarded by company-level policy activation
- Non-regulated companies bypass gate entirely until activated
- Disable policies → gate becomes no-op → existing logic path works

**Performance acceptable:**
- Challenges expire after 5 min (stale entries cleaned on read)
- Policy locks are row-level and short-lived
- Audit events are append-only (no deletes/updates)
- Indexes on company_id, record_type for fast lookup

**Data integrity guaranteed:**
- All decision logic in single transaction (all-or-nothing)
- Snapshot hashes prevent silent tampering
- Immutable audit trail enables forensics
- Concurrent receipt/close uses FOR UPDATE locking

---

## Next Steps

1. **Review** this plan with manufacturing, quality, security, and ops teams
2. **Clarify** 5 open questions (see Appendix below)
3. **Start Phase 1:** Create migration files + Go types
4. **Track progress:** Update todo tasks as you complete each phase
5. **Document runbooks** for policy administration and troubleshooting

---

## Appendix: Open Questions Before Starting

Before beginning Phase 1, clarify these with stakeholders:

1. **Policy activation:** Should existing non-regulated companies:
   - [ ] Bypass governance entirely until profile explicitly activated?
   - [ ] Start in WARN mode and graduate to ENFORCE?

2. **Reauthentication method (v1):**
   - [ ] Use existing password service?
   - [ ] Implement separate SMS/2FA challenge?

3. **Audit retention period:**
   - [ ] How long to keep audit events? (Recommended: 7 years for manufacturing)

4. **Performance SLA:**
   - [ ] Target latency per compliance gate decision? (Recommended: <100ms)

5. **Rollback testing:**
   - [ ] Automated or manual? Test frequency? (Recommended: quarterly)

---

## Contact & Questions

See full documentation:
- **Detailed Plan:** `docs/guides/MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md`
- **Visual Map:** `docs/guides/MANUFACTURING-GOVERNANCE-EXECUTION-MAP.md`
- **Original Plan:** `docs/guides/manufacturing-governance-plan.md`

Track progress in the todo system: `jcode todo list | grep "Phase"`

