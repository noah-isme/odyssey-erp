# Accounting Period Policy – Phase 4.2

Status: Finalised

This policy defines lifecycle management, permissions, and audit requirements for accounting periods in Odyssey ERP.

## Period Lifecycle
1. **Draft (implicit)** – Period record created via UI/API but not yet opened. Validated for non-overlap and chronological order.
2. **OPEN** – Default operational state. Journal postings and reversals allowed. System ensures journal dates fall within `start_d
   ate` and `end_date`.
3. **CLOSED** – Month-end review completed. New postings are blocked, but existing entries may be voided/reversed by users with `f
   inance.gl.edit`. Reversal automatically posts into the earliest subsequent OPEN period.
4. **LOCKED** – Financial statements published. Only users with `finance.override.lock` may reopen/override. Locking captures tim
   e, actor, and justification in audit log.

Transitions:
* Draft → OPEN (requires `finance.period.close`). System stores opener ID/timestamp.
* OPEN → CLOSED (requires `finance.period.close`). Validation ensures all source modules have completed (procurement receipts app
  roved, AP invoices posted, inventory adjustments reconciled). Integrity job must pass with zero imbalances before closing.
* CLOSED → OPEN (reopen) allowed with justification; requires `finance.period.close` and records audit entry with before/after st
  atus.
* CLOSED → LOCKED (requires `finance.period.close`). Optional manual approval step may be toggled via configuration.
* LOCKED → CLOSED (override) only with `finance.override.lock`, after multi-factor confirmation. Audit log stores reason and link
  s to approval ticket.

## Validation Rules
* Period date ranges may not overlap and must progress sequentially (previous period end + 1 day == next period start).
* Journal posting service validates period status atomically within transaction; race conditions mitigated via `SELECT ... FOR UP
  DATE` on period row.
* GL postings referencing historical periods must pass override permission checks.
* Source modules respect `periods.current_open` pointer to default period when not explicitly provided.

## Audit Trail
* All state transitions create entries in `audit_logs` with `entity = 'period'` and JSON metadata `{ "from": "OPEN", "to": "CLO
  SED", "period_id": <id>, "reason": "<text>" }`.
* UI requires operator to enter free-text reason for closing, reopening, or locking.
* Reports include last lock timestamp and actor for transparency.

## Operational Procedures
* Nightly Asynq job `jobs/gl_integrity.go` validates that per-period debits equal credits, no orphaned source links exist, and al
  l postings fall inside active periods.
* Prior to closing, finance runs `make reports-demo` to generate TB/P&L/BS snapshots and reviews anomalies.
* After locking, any discovered errors require override process: request approval, unlock, post adjustments in corrective period,
  relock.

## Future Enhancements
* Support fiscal calendars with 4-4-5 or custom periods by storing `fiscal_year` and `sequence` columns.
* Introduce segregation-of-duties automation: separate permissions for preparer vs approver.
* Add webhook notifications on period status changes to alert BI systems.
