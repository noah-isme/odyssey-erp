# Manufacturing Governance - Visual Execution Map

```mermaid
graph TD
    A["🎯 Manufacturing Governance<br/>Start"] --> P1["Phase 1: Schema & Models<br/>5 tasks"]
    
    P1 -->|1-1| M["Create Migration<br/>000081_manufacturing_governance<br/>.up.sql & .down.sql"]
    P1 -->|1-2| DT["Define Go Types<br/>governance_domain.go<br/>DecisionRequest, DecisionGrant, etc."]
    P1 -->|1-3| SQ["Write SQL Queries<br/>mrp_governance.sql<br/>Policy/Decision/Challenge/Evidence CRUD"]
    P1 -->|1-4| SG["Run sqlc-gen<br/>Generate sqlc code"]
    
    M --> P2_CHECK["✓ Schema migrates<br/>✓ Types build<br/>✓ sqlc compiles"]
    DT --> P2_CHECK
    SQ --> P2_CHECK
    SG --> P2_CHECK
    
    P2_CHECK --> P2["Phase 2: Compliance Gate<br/>3 tasks - CRITICAL"]
    
    P2 -->|2-1| CG["ComplianceGate Service<br/>compliance_gate.go<br/>8-step: lock→snap→hash→validate→auth→store→execute→audit"]
    P2 -->|2-2| SC["Signature Challenge<br/>signature_challenge.go<br/>Generate/Verify + expiry/replay enforcement"]
    P2 -->|2-3| RS["Record Snapshot<br/>record_snapshot.go<br/>Canonical JSON + SHA-256"]
    
    CG --> P3_CHECK["✓ Gate unit tests pass<br/>✓ Challenges expire/verify<br/>✓ Audit immutable"]
    SC --> P3_CHECK
    RS --> P3_CHECK
    
    P3_CHECK --> P3["Phase 3: Prerequisites<br/>8 tasks - HIGH"]
    
    P3 -->|3-1| PRE1["BOM Approval<br/>prerequisites_bom.go"]
    P3 -->|3-2| PRE2["Work Order Release<br/>prerequisites_workorder.go"]
    P3 -->|3-3| PRE3["Operation Completion<br/>prerequisites_operation.go"]
    P3 -->|3-4| PRE4["Finished Goods<br/>prerequisites_receipt.go"]
    P3 -->|3-5| PRE5["Schedule Override<br/>prerequisites_override.go"]
    P3 -->|3-6| PRE6["Hold Release<br/>prerequisites_hold.go"]
    P3 -->|3-7| PRE7["Quality Disposition<br/>prerequisites_quality.go"]
    P3 -->|3-8| PRE8["Subcontract Accept<br/>prerequisites_subcontract.go"]
    
    PRE1 --> P4_CHECK["✓ Prereq tests pass<br/>✓ Decisions fail without<br/>✓ Gateway blocks invalid"]
    PRE2 --> P4_CHECK
    PRE3 --> P4_CHECK
    PRE4 --> P4_CHECK
    PRE5 --> P4_CHECK
    PRE6 --> P4_CHECK
    PRE7 --> P4_CHECK
    PRE8 --> P4_CHECK
    
    P4_CHECK --> P4["Phase 4: Quality Lifecycle<br/>2 tasks"]
    
    P4 -->|4-1| LC["State Machines<br/>quality_lifecycle.go<br/>Inspection→Hold→NCR→CAPA→Subcontract"]
    P4 -->|4-2| QS["Versioned Snapshots<br/>quality_snapshots.go<br/>Immutable results with version"]
    
    LC --> P5_CHECK["✓ Lifecycle tests pass<br/>✓ Snapshots immutable"]
    QS --> P5_CHECK
    
    P5_CHECK --> P5["Phase 5: RBAC & Interfaces<br/>3 tasks"]
    
    P5 -->|5-1| PERM["Permissions<br/>governance_permissions.go<br/>planner, operator, quality.*, manager, compliance.admin"]
    P5 -->|5-2| ROUTE["HTTP Routes<br/>governance_routes.go<br/>/mrp/compliance/*, /mrp/quality/*"]
    P5 -->|5-3| TMPL["SSR Templates<br/>web/templates/mrp/<br/>policies, challenges, decisions, evidence, quality records"]
    
    PERM --> P6_CHECK["✓ Permissions enforced<br/>✓ Routes guard properly<br/>✓ Templates render"]
    ROUTE --> P6_CHECK
    TMPL --> P6_CHECK
    
    P6_CHECK --> P6["Phase 6: Tests & Validation<br/>8 test suites"]
    
    P6 -->|6-1| T1["Policy Resolution<br/>governance_test.go"]
    P6 -->|6-2| T2["Compliance Gate<br/>compliance_gate_test.go"]
    P6 -->|6-3| T3["Signature Challenges<br/>signature_challenge_test.go"]
    P6 -->|6-4| T4["RBAC & SOD<br/>rbac_test.go"]
    P6 -->|6-5| T5["Quality Lifecycle<br/>quality_lifecycle_test.go"]
    P6 -->|6-6| T6["Concurrency<br/>concurrency_test.go"]
    P6 -->|6-7| T7["Company Isolation<br/>isolation_test.go"]
    P6 -->|6-8| T8["Security & Audit<br/>security_test.go"]
    
    T1 --> VERIFY["✓ go test ./...<br/>✓ go vet ./...<br/>✓ golangci-lint<br/>✓ Coverage >80%"]
    T2 --> VERIFY
    T3 --> VERIFY
    T4 --> VERIFY
    T5 --> VERIFY
    T6 --> VERIFY
    T7 --> VERIFY
    T8 --> VERIFY
    
    VERIFY --> P7["Phase 7: Staging & Rollout<br/>4 steps"]
    
    P7 -->|7-1| STAGE_WARN["Deploy Staging<br/>WARN Mode<br/>Backfill policies, monitor violations"]
    P7 -->|7-2| STAGE_ENFORCE["Switch to ENFORCE<br/>Run negative-path tests<br/>Run concurrency tests"]
    P7 -->|7-3| EVIDENCE["Collect Evidence<br/>Migration logs, rollback success<br/>Permission matrix, test results<br/>Audit immutability, company isolation"]
    P7 -->|7-4| SIGNOFF["Obtain Sign-Off<br/>Mgmt, Quality, Security, Ops<br/>Enable ENFORCE per company<br/>NO global cutover"]
    
    STAGE_WARN --> FINAL["✅ Production Ready<br/>Governance Active"]
    STAGE_ENFORCE --> FINAL
    EVIDENCE --> FINAL
    SIGNOFF --> FINAL
    
    style A fill:#4CAF50,color:#fff
    style P1 fill:#2196F3,color:#fff
    style P2 fill:#FF9800,color:#fff
    style P3 fill:#FF9800,color:#fff
    style P4 fill:#9C27B0,color:#fff
    style P5 fill:#00BCD4,color:#fff
    style P6 fill:#F44336,color:#fff
    style P7 fill:#009688,color:#fff
    style FINAL fill:#4CAF50,color:#fff
    style P2_CHECK fill:#E8F5E9,color:#333
    style P3_CHECK fill:#E8F5E9,color:#333
    style P4_CHECK fill:#E8F5E9,color:#333
    style P5_CHECK fill:#E8F5E9,color:#333
    style VERIFY fill:#E8F5E9,color:#333
```

## Execution Timeline

```mermaid
timeline
    title Manufacturing Governance Implementation Timeline
    section Week 1
        Day 1-2 : Phase 1: Schema & Models (5 tasks)
        Day 3 : Phase 1 validation (migration + sqlc)
    section Week 2
        Day 1-2 : Phase 2: Compliance Gate (3 tasks)
        Day 3 : Phase 2 unit tests
    section Week 3
        Day 1-3 : Phase 3: Prerequisites (8 tasks)
        Day 4 : Phase 3 prerequisite tests
    section Week 4
        Day 1 : Phase 4: Quality Lifecycle (2 tasks)
        Day 2-3 : Phase 5: RBAC & Routes (3 tasks)
        Day 4 : Phase 5 SSR templates
    section Week 5
        Day 1-4 : Phase 6: Test Suites (8 test files)
        Day 5 : All unit tests passing
    section Week 6-7
        Day 1 : Phase 7.1: Deploy to Staging (WARN mode)
        Day 2-3 : Phase 7.2: Switch to ENFORCE
        Day 4 : Phase 7.3: Collect evidence
        Day 5 : Phase 7.4: Sign-offs
    section Week 8
        Day 1+ : Gradual production rollout (company by company)
```

## Task Dependencies

```
Phase 1 (Schema)
    ↓
Phase 2 (Gate) ← [Requires: Schema, types, SQL]
    ↓
Phase 3 (Prerequisites) ← [Requires: Gate]
    ↓
Phase 4 (Lifecycle) ← [Requires: Schema tables]
    ↓
Phase 5 (RBAC) ← [Requires: All above]
    ↓
Phase 6 (Tests) ← [Requires: All above]
    ↓
Phase 7 (Staging) ← [Requires: All above + tests passing]
```

## File Inventory (33 Total)

### Schema & Migration (2 files)
- `migrations/000081_manufacturing_governance.up.sql`
- `migrations/000081_manufacturing_governance.down.sql`

### Go Services (14 files)
- `internal/mrp/governance_domain.go` – Domain types
- `internal/mrp/compliance_gate.go` – Central gate service
- `internal/mrp/signature_challenge.go` – Challenge/reauthentication
- `internal/mrp/record_snapshot.go` – Snapshot & hashing
- `internal/mrp/prerequisites_bom.go`
- `internal/mrp/prerequisites_workorder.go`
- `internal/mrp/prerequisites_operation.go`
- `internal/mrp/prerequisites_receipt.go`
- `internal/mrp/prerequisites_override.go`
- `internal/mrp/prerequisites_hold.go`
- `internal/mrp/prerequisites_quality.go`
- `internal/mrp/prerequisites_subcontract.go`
- `internal/mrp/quality_lifecycle.go`
- `internal/mrp/quality_snapshots.go`

### Go RBAC & Routes (2 files)
- `internal/mrp/governance_permissions.go`
- `internal/mrp/governance_routes.go`

### SQL Queries (1 file)
- `sql/queries/mrp_governance.sql`

### Generated (1 file)
- `internal/sqlc/mrp_governance.go` – Auto-generated by sqlc

### SSR Templates (8 files)
- `web/templates/mrp/compliance/policies.html`
- `web/templates/mrp/compliance/challenges.html`
- `web/templates/mrp/compliance/decisions.html`
- `web/templates/mrp/compliance/evidence.html`
- `web/templates/mrp/quality/inspections.html`
- `web/templates/mrp/quality/holds.html`
- `web/templates/mrp/quality/nonconformances.html`
- `web/templates/mrp/quality/capas.html`

### Test Files (8 files)
- `internal/mrp/governance_test.go`
- `internal/mrp/compliance_gate_test.go`
- `internal/mrp/signature_challenge_test.go`
- `internal/mrp/rbac_test.go`
- `internal/mrp/quality_lifecycle_test.go`
- `internal/mrp/concurrency_test.go`
- `internal/mrp/isolation_test.go`
- `internal/mrp/security_test.go`

### Documentation (Updated)
- `docs/guides/MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md` – This detailed guide

---

## Key Validation Gates

| Gate | Success Criteria | Blocker? |
|---|---|---|
| **Schema** | Migration runs, no SQL errors | Yes |
| **sqlc** | `make sqlc-gen` produces code, no Go errors | Yes |
| **Gate Unit Tests** | All gate tests pass (challenges, snapshots, audit) | Yes |
| **Prerequisite Tests** | All 8 decision types validated correctly | Yes |
| **Lifecycle Tests** | State machines enforce valid transitions | Yes |
| **RBAC Tests** | Permissions enforced on all routes, SOD violations rejected | Yes |
| **Concurrency Tests** | Concurrent operations idempotent, no race conditions | Yes |
| **Company Isolation** | Cross-company queries return empty, exports correct | Yes |
| **Full Test Suite** | `go test ./...` passes, coverage >80% | Yes |
| **Staging WARN Mode** | All existing functions work, violations logged | No – warning only |
| **Staging ENFORCE Mode** | All governance rules enforced, no data loss | Yes |
| **Evidence Collection** | All mandatory docs completed | Yes – for sign-off |
| **Sign-Offs** | Manufacturing, Quality, Security, Ops approved | Yes – for production |

---

## How to Track Progress

The detailed plan is tracked in the todo system with 33 actionable tasks grouped by phase. Each task has:
- Clear file targets
- Specific acceptance criteria
- Dependency tracking
- Confidence level (95% = well-understood, 80% = likely needs clarification)

**To view progress:**
```bash
# View all tasks
jcode todo list

# View specific phase
jcode todo list | grep "Phase 1"

# Mark task complete
jcode todo complete id-of-task
```

---

## Questions Before Starting?

Before diving into Phase 1 (schema creation), clarify:

1. **Policy activation:** Should existing non-regulated companies bypass governance entirely, or be set to WARN mode initially?
2. **Reauthentication:** Use existing password service for v1, or implement separate challenge mechanism?
3. **Audit retention:** How long should audit events be kept? (Recommended: 7 years for manufacturing)
4. **Performance targets:** Any SLA for compliance gate decisions? (Recommended: <100ms per decision)
5. **Rollback procedure:** Automated or manual? Test required frequency?

