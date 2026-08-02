# Manufacturing Governance Execution Plan

**Status:** Planned. Existing manufacturing compliance and quality records remain
available, but mandatory controlled-decision enforcement and staging certification are
not yet implemented.

## Summary

Close two manufacturing governance gaps:

- Enforce controlled-record policies at every governed manufacturing decision.
- Add a formal staging certification gate before production enablement.

Quality remains embedded within manufacturing execution. This plan strengthens
inspections, holds, NCRs, CAPAs, subcontract acceptance, and genealogy, but does not
create a standalone QMS. The separate enterprise QMS and controlled-document migration
are defined in the
[`CMMS, QMS, and Document Management Execution Plan`](missing-modules-cmms-qms-documents-plan.md).

## 1. Policy and decision model

Extend the existing controlled-record policy into versioned, effective-dated rules
with:

- Company, record type, decision/action, effective period, and enforcement mode.
- `DISABLED`, `WARN`, and `ENFORCE` modes.
- Required signature meaning and permitted approver roles.
- Reauthentication requirement.
- Separation-of-duties rules.
- Required evidence and prerequisite checks.
- Retention period and policy version.
- A `DRAFT → ACTIVE → RETIRED` lifecycle; activated versions are immutable.

Govern these decision points:

| Record | Controlled decision |
|---|---|
| BOM revision | Approve |
| Work order | Release and close/final receipt |
| Operation | Complete, manually reschedule, or split |
| Quality hold | Release |
| NCR | Disposition and close |
| CAPA | Verify and close |
| Subcontract operation | Accept received quantity |
| Manufacturing exception | Override or dismiss a compliance-related exception |

When a company activates its manufacturing governance profile, these decisions fail
closed if no effective policy exists.

## 2. Central compliance gate

Add a manufacturing `ComplianceGate` used by services rather than handlers:

```go
type DecisionRequest struct {
    CompanyID   int64
    RecordType  string
    RecordID    int64
    Action      string
    ActorID     int64
    Reason      string
    ChallengeID string
}

type DecisionGrant struct {
    PolicyVersionID int64
    RecordVersion   string
    RecordHash      string
    DecisionID      int64
}
```

The gate must:

1. Load and lock the effective policy.
2. Generate the record snapshot and version on the server.
3. Check actor permission, approver role, and separation of duties.
4. Validate action-specific prerequisites.
5. Verify a one-time reauthentication challenge when required.
6. Store the decision, signature, reason, and evidence references.
7. Execute the governed state transition in the same transaction.
8. Append an immutable audit event with correlation and causation IDs.

No handler may update governed manufacturing statuses directly after this change.

## 3. Electronic signatures

Replace arbitrary client-supplied record JSON with server-generated canonical
snapshots.

- Add one-time signature challenges with a five-minute expiry.
- Reauthenticate using the existing password/authentication service for v1.
- Store the method, verification timestamp, record version, canonical SHA-256 hash,
  signature meaning, policy version, and signer.
- Never persist passwords or free-form reauthentication evidence.
- Prevent signature reuse across records, actions, or versions.
- Keep historical signatures immutable when a record receives a later revision.
- Deprecate direct use of `POST /mrp/compliance/signatures`; expose challenge and
  decision-confirmation endpoints instead.

This provides internal controlled-record assurance without claiming
regulator-specific electronic-signature certification.

## 4. Mandatory manufacturing prerequisites

Enforce the following through the compliance gate.

### BOM approval

- Draft revision, complete component lines, valid effective date, and change reason.
- No self-reference or detected BOM cycle.
- Manager signature when the active policy requires it.
- Superseding a revision cannot alter existing work-order snapshots.

### Work-order release

- Approved and effective BOM revision.
- Valid routing snapshot and WIP location mapping.
- Active work centers and sufficient capacity configuration.
- Required inspection plans resolved for product/routing operations.
- No blocking compliance exception.

### Operation completion

- Prior operations completed.
- All mandatory inspections for the operation passed.
- No open work-order or operation quality hold.
- Reported good/scrap quantities and time are valid.
- Any policy-required operator or supervisor signature exists.

### Finished-goods receipt and work-order close

- Required operations and final inspections completed.
- No open hold or unresolved blocking NCR.
- Issued WIP covers the receipt.
- Required lot/serial genealogy is complete.
- Final receipt and close remain idempotent and transactional.

### Schedule override

- Manual reschedule/split requires an override reason.
- Policy may require manager approval or signature.
- Preserve the original schedule and write an immutable before/after audit event.

### Hold release and quality disposition

- Release requires a disposition and quality-approval permission.
- Enforce creator/releaser separation when configured.
- Failed mandatory inspections must have a linked hold or NCR.
- CAPA closure requires verification by someone other than the action owner when
  separation of duties is active.

### Subcontract acceptance

- Received quantity cannot exceed sent quantity.
- Mandatory incoming inspection must pass before acceptance or operation continuation.
- Failed inspection creates or links a hold/NCR.
- Supplier, quantity, cost, inspection, and acceptance decision remain traceable.

## 5. Manufacturing quality boundary

Strengthen the current manufacturing-quality records with explicit lifecycles:

- Inspection: `PENDING → PASSED/FAILED/HOLD → RELEASED`.
- Hold: `OPEN → RELEASED`.
- NCR: `OPEN → INVESTIGATING → DISPOSITIONED → CLOSED`.
- CAPA: `OPEN → IN_PROGRESS → VERIFICATION → CLOSED`.
- Subcontract receipt: `SENT → RECEIVED → INSPECTING → ACCEPTED/CLOSED`.

Add versioned inspection-plan criteria and immutable result snapshots. Quality records
remain linked to products, work orders, operations, subcontract receipts, lots, and
serials.

Explicitly exclude from this program:

- Enterprise document-control and training management.
- Calibration and metrology.
- Statistical process control.
- Customer complaints and supplier-quality management.
- Enterprise audit management.
- General-purpose QMS workflows unrelated to production.

These exclusions are planned as a separate top-level QMS rather than being added to
the MRP ownership boundary; see the
[`CMMS, QMS, and Document Management Execution Plan`](missing-modules-cmms-qms-documents-plan.md).

## 6. RBAC and interfaces

Replace the current broad `mrp.manage` route guard with dedicated permissions:

- `mrp.planner`.
- `mrp.operator`.
- `mrp.quality.inspect`.
- `mrp.quality.approve`.
- `mrp.manager`.
- New `mrp.compliance.admin` for policy activation and evidence export.

Add administration and evidence interfaces under:

- `/mrp/compliance/policies`.
- `/mrp/compliance/challenges`.
- `/mrp/compliance/decisions`.
- `/mrp/compliance/evidence`.
- `/mrp/quality/inspections`.
- `/mrp/quality/holds`.
- `/mrp/quality/nonconformances`.
- `/mrp/quality/capas`.

The existing manufacturing quality page remains the operational workbench; do not
introduce a top-level QMS module.

## 7. Staging validation and rollout

### Deployment sequence

1. Add policy versions, decisions, challenges, evidence, and lifecycle constraints.
2. Backfill existing policies into `WARN` mode.
3. Route governed transitions through the compliance gate.
4. Run production-like staging in `WARN` mode and resolve missing prerequisites.
5. Switch the staging company to `ENFORCE`.
6. Complete negative-path, concurrency, audit, and recovery testing.
7. Obtain manufacturing-manager, quality-owner, security, and operations sign-off.
8. Enable `ENFORCE` per production company; do not use a global cutover.

### Mandatory staging evidence

- Fresh-schema and upgrade migration success.
- Rollback rehearsal against a staging backup.
- Permission matrix for planner, operator, quality inspector, quality approver,
  manager, and compliance administrator.
- Every controlled decision succeeds with valid evidence and fails without it.
- Expired, reused, wrong-record, and wrong-version signature challenges are rejected.
- Separation-of-duties violations are rejected.
- Required inspections and open holds block operation completion and receipt.
- Concurrent close/receipt requests post inventory and accounting exactly once.
- Record hashes can be regenerated and matched.
- Audit events cannot be updated or deleted.
- CSV evidence export is complete and company-isolated.
- Existing non-regulated companies continue operating until their profile is activated.

## Test plan

- Table-driven policy resolution tests for company, action, effective date, version,
  and enforcement mode.
- Service tests for every controlled decision and missing prerequisite.
- Signature challenge expiry, replay, tampering, record-version mismatch, and
  reauthentication failure.
- RBAC and separation-of-duties tests.
- Inspection, hold, NCR, CAPA, and subcontract lifecycle tests.
- Transaction rollback tests proving failed governance checks do not partially mutate
  production, inventory, or accounting.
- Idempotency and concurrent work-order completion tests.
- Company-isolation tests for policies, decisions, signatures, quality records, and
  exports.
- SSR/API validation, CSRF, safe-error, and audit-event tests.
- Full verification with `go test ./...`, `go vet ./...`, `make lint`, and
  `make docs-check`.

## Assumptions

- Staging validation means deployment and operational certification in a staging
  environment.
- Password reauthentication is the v1 signature mechanism; 2FA/SSO can replace it
  later.
- Policies are configurable and regulator-neutral; implementation does not itself
  certify ISO, GMP, FDA, or other regulatory compliance.
- Existing quality records remain manufacturing-owned and are not advertised as a
  standalone QMS.
