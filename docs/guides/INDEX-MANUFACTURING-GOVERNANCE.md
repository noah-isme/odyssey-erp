# Manufacturing Governance - Documentation Index

**Status:** Planning Phase Complete ✓  
**Date:** 2026-08-03  
**Ready for:** Phase 1 Implementation

---

## Quick Navigation

### For Executives & Decision Makers
→ Start here: **[MFG-GOVERNANCE-QUICK-START.md](MFG-GOVERNANCE-QUICK-START.md)**
- 1-page executive summary
- Scope, architecture, risks at a glance
- Open questions for stakeholder alignment

### For Technical Leads & Architects
→ Start here: **[MANUFACTURING-GOVERNANCE-EXECUTION-MAP.md](MANUFACTURING-GOVERNANCE-EXECUTION-MAP.md)**
- Visual flowchart with mermaid diagrams
- Timeline and effort estimates
- File inventory by phase
- Validation gates and dependencies

### For Developers & Implementation Team
→ Start here: **[MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md](MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md)**
- 33 concrete tasks across 7 phases
- Full SQL schema design with column definitions
- Service architecture and design patterns
- HTTP routes and permission model
- Test strategy and staging procedure
- Risk mitigation strategies

### For Project Managers
→ Use: **jcode todo system**
- 33 tracked tasks with file targets
- Confidence levels and dependencies
- Progress tracking capability
- `jcode todo list` to view all tasks

---

## Document Breakdown

| Document | Lines | Purpose | Audience |
|----------|-------|---------|----------|
| [MFG-GOVERNANCE-QUICK-START.md](MFG-GOVERNANCE-QUICK-START.md) | 302 | 1-page reference | Executives, managers, security |
| [MANUFACTURING-GOVERNANCE-EXECUTION-MAP.md](MANUFACTURING-GOVERNANCE-EXECUTION-MAP.md) | 269 | Visual roadmap | PMs, architects, leads |
| [MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md](MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md) | 583 | Detailed breakdown | Developers, architects |
| [INDEX-MANUFACTURING-GOVERNANCE.md](INDEX-MANUFACTURING-GOVERNANCE.md) | 150 | Navigation hub | Everyone |
| **TOTAL** | **1,304** | **Complete plan** | **All teams** |

---

## What's Being Built

### Scope: 8 Governed Manufacturing Decisions
1. **BOM Approval** – Draft→Approved with manager signature
2. **Work Order Release** – Approved BOM + routing + capacity
3. **Operation Completion** – Prior ops + inspections + no holds
4. **Finished Goods Receipt** – All ops + inspections + genealogy
5. **Schedule Override** – Reason + manager approval
6. **Hold Release** – Disposition + quality.approve
7. **NCR Disposition** – Investigation + linked holds/CAPAs
8. **Subcontract Acceptance** – Qty validation + inspection

### Architecture: Central Compliance Gate (8-Step Atomic Flow)
```
Request → Lock Policy → Snapshot → Hash → Validate Actor 
        → Verify Challenge → Store Decision → Execute → Audit Event
        → All in single database transaction
```

### Implementation: 33 Tasks Across 7 Phases
- **Phase 1:** Schema & Models (5 tasks)
- **Phase 2:** Compliance Gate (3 critical tasks)
- **Phase 3:** Prerequisites (8 validators)
- **Phase 4:** Quality Lifecycle (2 tasks)
- **Phase 5:** RBAC & Interfaces (3 tasks)
- **Phase 6:** Testing (8 test suites)
- **Phase 7:** Staging & Rollout (4 steps)

**Total Effort:** ~31 days (3-4 weeks) + 10 days staging

---

## Key Metrics

| Metric | Value |
|--------|-------|
| Governed decisions | 8 |
| Implementation tasks | 33 |
| Phases | 7 |
| New database tables | 6 |
| New RBAC permissions | 6 |
| Go services to create | 14 |
| Test suites | 8 |
| SSR templates | 8 |
| Validation gates | 13 |
| Files to create | 33 total |
| Timeline | 3-4 weeks implementation |
| Staging | 2 weeks (parallel) |

---

## Implementation Phases

### Phase 1: Schema & Models (CRITICAL PATH)
**Duration:** 3 days | **Tasks:** 5
- Create migrations (000081_manufacturing_governance.up/down.sql)
- Define Go types (governance_domain.go)
- Write SQL queries (mrp_governance.sql)
- Run sqlc-gen
- **Gate:** Schema migrates, sqlc compiles, types build

### Phase 2: Compliance Gate (CRITICAL PATH)
**Duration:** 4 days | **Tasks:** 3
- ComplianceGate service (8-step atomic flow)
- SignatureChallenge service (expiry, replay enforcement)
- RecordSnapshot service (snapshots + SHA-256)
- **Gate:** Gate tests pass, challenges expire, audit immutable

### Phase 3: Prerequisites
**Duration:** 4 days | **Tasks:** 8 (parallelizable)
- BOM approval, work order release, operation completion
- Finished goods receipt, schedule override, hold release
- Quality disposition, subcontract acceptance
- **Gate:** All prerequisites enforced, decisions fail without proof

### Phase 4: Quality Lifecycle
**Duration:** 2 days | **Tasks:** 2
- Lifecycle state machines (inspection, hold, NCR, CAPA, subcontract)
- Versioned snapshots (immutable results)
- **Gate:** Lifecycle tests pass, transitions enforced

### Phase 5: RBAC & Interfaces
**Duration:** 3 days | **Tasks:** 3
- Define 6 permissions (planner, operator, quality.*, manager, compliance.admin)
- Add HTTP routes with permission guards
- Create SSR templates (policies, challenges, decisions, evidence, quality records)
- **Gate:** Permissions enforced, routes guarded, templates render

### Phase 6: Testing
**Duration:** 5 days | **Tasks:** 8 test suites
- Policy resolution, gate operation, challenge security
- RBAC & SOD, lifecycle transitions, concurrency
- Company isolation, security & audit
- **Gate:** go test ./... passes, coverage >80%

### Phase 7: Staging & Rollout
**Duration:** 10 days | **Tasks:** 4 steps
- Deploy to staging in WARN mode (violations logged)
- Switch to ENFORCE mode (actively enforced)
- Collect evidence (migration, rollback, permissions, tests)
- Obtain sign-offs (Manufacturing, Quality, Security, Operations)
- **Gate:** Staging stable, evidence complete, sign-offs obtained

---

## Database Schema (6 New Tables)

```
policy_versions
├─ Versioned rules with effective dates
├─ DRAFT→ACTIVE→RETIRED lifecycle
└─ 1 row per (company, record_type, decision, version)

compliance_decisions
├─ Decisions made through gate
├─ Contains: policy_version_id, record_snapshot, hash, actor_id
└─ Immutable after creation

signature_challenges
├─ One-time challenges with 5-minute expiry
├─ Prevents replay attacks
└─ Marked used after verification

evidence_records
├─ Immutable proof linked to decisions
├─ Contains: evidence_type, content (JSONB)
└─ No updates allowed

audit_events
├─ Immutable causation tracking
├─ Correlation IDs group related decisions
├─ Causation IDs link effect to cause
└─ Append-only, no deletes

quality_*
├─ Inspection, Hold, NCR, CAPA, Subcontract
├─ Explicit lifecycle states
└─ Versioned snapshots with immutability
```

---

## RBAC Permissions (6 New)

| Permission | Capability |
|-----------|-----------|
| `mrp.planner` | Create/release work orders, manage schedules |
| `mrp.operator` | Complete operations, report production |
| `mrp.quality.inspect` | Create/record inspections |
| `mrp.quality.approve` | Hold release, NCR disposition |
| `mrp.manager` | Sign off decisions requiring manager approval |
| `mrp.compliance.admin` | Activate policies, export evidence |

---

## Validation Gates (13 Total)

### Hard Gates (Must Pass Before Production)
1. Schema migration runs without error
2. make sqlc-gen produces valid Go code
3. go build ./... succeeds
4. Compliance gate unit tests pass
5. All prerequisite tests pass (8 types)
6. RBAC tests pass
7. Concurrency tests pass
8. Company isolation verified
9. Full test suite passes (go test ./...)
10. Coverage >80% for gate and prerequisites
11. Staging ENFORCE mode stable
12. All mandatory evidence collected
13. Sign-offs obtained (4 stakeholders)

### Soft Gates (Warning Only)
- Staging WARN mode (violations logged)
- Performance <100ms per decision

---

## Risk Mitigation

### Rollback is SAFE
- Schema backward-compatible (new tables only)
- Gate guarded by company-level policy activation
- Non-regulated companies bypass gate until activated
- Disable policies → gate becomes no-op → existing path works

### Performance is ACCEPTABLE
- Challenges expire after 5 min (stale cleaned on read)
- Policy locks are row-level and short-lived
- Audit events are append-only (no deletes/updates)
- Indexes on company_id, record_type

### Data Integrity is GUARANTEED
- All decision logic in single transaction
- Snapshot hashes prevent tampering
- Immutable audit trail enables forensics
- FOR UPDATE locking on concurrent operations

---

## How to Track Progress

### View all 33 tasks
```bash
jcode todo list
```

### View tasks by phase
```bash
jcode todo list | grep "Phase 1"
jcode todo list | grep "Phase 2"
# etc.
```

### Mark task complete
```bash
jcode todo complete 1-1-migration-up
```

### View task details
```bash
jcode todo show 1-1-migration-up
```

---

## Open Questions for Stakeholders

Before starting Phase 1, clarify these with manufacturing, quality, security, operations:

1. **Policy activation for existing companies:**
   - [ ] Bypass governance until profile activated?
   - [ ] Start in WARN mode and graduate to ENFORCE?

2. **Reauthentication method (v1):**
   - [ ] Use existing password service?
   - [ ] Implement separate SMS/2FA challenge?

3. **Audit retention period:**
   - [ ] How long to keep audit events? (Recommended: 7 years)

4. **Performance SLA:**
   - [ ] Target latency per decision? (Recommended: <100ms)

5. **Rollback testing frequency:**
   - [ ] Automated or manual? How often? (Recommended: quarterly)

---

## Next Steps

### This Week
1. Review the 3 documentation files
2. Present to manufacturing, quality, security, operations
3. Answer 5 open questions above
4. Get sign-off on scope and architecture

### Week 1-2 (Phase 1 & 2)
1. Start Phase 1: Create migrations, types, SQL queries
2. Validate: make sqlc-gen && go build ./...
3. Start Phase 2: Implement ComplianceGate service
4. Track progress: Update todo items

### Week 3-6 (Phases 3-6)
1. Implement prerequisites (8 validators)
2. Implement quality lifecycle
3. Implement RBAC and routes
4. Write comprehensive tests

### Week 7-8 (Phase 7)
1. Deploy to staging (WARN mode)
2. Collect evidence and run tests
3. Obtain sign-offs
4. Enable ENFORCE per production company

---

## Documentation Map

```
docs/guides/
├── manufacturing-governance-plan.md
│   └── Original plan from user (source document)
│
├── INDEX-MANUFACTURING-GOVERNANCE.md (THIS FILE)
│   └── Navigation hub for all three documents
│
├── MFG-GOVERNANCE-QUICK-START.md
│   └── 1-page executive reference
│       • For: Executives, managers, security
│       • Contains: Scope, architecture, risks
│
├── MANUFACTURING-GOVERNANCE-EXECUTION-MAP.md
│   └── Visual roadmap with flowcharts
│       • For: PMs, architects, technical leads
│       • Contains: Flowchart, timeline, file inventory
│
└── MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md
    └── Detailed phase-by-phase breakdown
        • For: Developers, architects
        • Contains: Schema, services, routes, tests, staging
```

---

## Summary

**What:** Detailed execution plan for manufacturing governance implementation  
**When:** Planning complete, ready to start Phase 1  
**Where:** docs/guides/ (4 files) + jcode todo system (33 tasks)  
**Who:** 33 concrete tasks tracked and ready for team  
**Why:** Close two governance gaps in MRP module  
**How:** 7 phases, 33 tasks, 3-4 weeks implementation

**Status: PLANNING PHASE COMPLETE ✓**

---

## Getting Help

- **Questions about scope?** → See [MFG-GOVERNANCE-QUICK-START.md](MFG-GOVERNANCE-QUICK-START.md)
- **Questions about timeline?** → See [MANUFACTURING-GOVERNANCE-EXECUTION-MAP.md](MANUFACTURING-GOVERNANCE-EXECUTION-MAP.md)
- **Questions about implementation?** → See [MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md](MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md)
- **Track your progress?** → Use `jcode todo list`
- **Need to clarify anything?** → See Open Questions section above

---

*Last Updated: 2026-08-03*  
*Next Review: After Phase 1 completion*
