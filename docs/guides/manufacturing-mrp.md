# Manufacturing / MRP

## Current status

**Implemented manufacturing planning and execution workflow, with compliance-control foundations.** Routes are mounted under `/mrp`; local acceptance evidence is linked from [`horizon-mvp.md`](horizon-mvp.md).

Mandatory decision enforcement and staging certification are specified in the
[`Manufacturing Governance Execution Plan`](manufacturing-governance-plan.md).

## Supported scope

- Company-scoped, effective-dated BOM revisions with `DRAFT`, `APPROVED`, and `SUPERSEDED` states. Approved revisions and lines are immutable; approval records an approver, time, and change reason.
- New BOM revisions copy the source component lines. Work orders and MRP firming require an approved effective revision.
- Work centers and ordered routing operations; release snapshots routing steps onto the work order and marks the first operation ready.
- Operation reporting captures setup/run time, good and scrap quantities, operator, and operation state (`PENDING`, `READY`, `IN_PROGRESS`, `COMPLETED`, `BLOCKED`).
- WIP mappings pair a source warehouse with a WIP warehouse, with optional work-center-specific overrides and one default mapping per source warehouse.
- Idempotent material issue and return move BOM components between raw stock and WIP. Finished-goods receipt consumes issued WIP material only, records component-level receipt cost, and receipts FG at the reconciled WIP cost.
- Warehouse-scoped work orders, including approved-effective BOM validation at creation and release.
- Per-product, per-warehouse planning policies (BUY/MAKE, lead time, safety stock, and lot sizing).
- Planning runs that net confirmed sales-order demand, on-hand stock, and approved purchase-order supply.
- Multi-level BOM explosion for MAKE policies, including scrap, lead-time offsets, and cycle detection.
- Firming a recommendation into one draft work order or one draft purchase request; retries return the original linked document.
- Work-center shifts plus holiday, maintenance, and capacity-override calendars. The finite-capacity scheduler honours routing-operation dependencies, preserves manual schedules, and records late or missing-capacity exceptions.
- Persistent MRP exceptions with deduplicated open fingerprints, severity, linked records, structured planner explanations, ownership, comments, resolution history, and firm/dismiss/reschedule actions.
- Inspection plans and results, quality holds/releases, NCRs, CAPAs, and subcontract-operation send/receive records. Open holds prevent operation completion and finished-goods receipt.
- Lot/serial references on material issue and finished-goods receipt, with component-to-finished-good genealogy exposed through the MRP API.
- Live manufacturing analytics for operation yield/scrap/time variance, WIP value/aging inputs, schedule adherence, and work-center utilization, including CSV export.
- Controlled-record policies, captured electronic signatures, immutable manufacturing audit events, retention settings, audit CSV export, and dedicated manufacturing planner/operator/quality/manager permissions.
- Dispatcher APIs and screens for BOM revisions, WIP-location administration, work-order execution, scheduling, exceptions, quality, and manufacturing analytics.
- Explicit fulfillment warehouse on every newly created or updated sales-order line. Delivery orders may use only lines assigned to their header warehouse.

## Not currently documented as supported

- Automatic enforcement of every controlled-record policy at each approval, quality disposition, schedule override, subcontract receipt, and work-order-close decision point.
- Re-authentication, signature-to-record-version hashing, and regulator-specific electronic-signature validation.
- Automated retention/export jobs and a full compliance-reporting suite.
- Advanced quality workflows such as statistical process control, calibration, and supplier-quality scorecards.

## Operating constraints

- Historical sales-order lines whose warehouse cannot be inferred remain unassigned; they are excluded from MRP demand and cannot be placed on a new delivery order until assigned.
- An approved purchase order contributes supply only when it has an explicit expected receiving warehouse and unreceived quantity.
- A BUY recommendation retains its target warehouse on the MRP recommendation and in the generated PR note; the purchase-order conversion must select that same receiving warehouse.
- A WIP issue or return is limited to a BOM component of its work order. A completion fails atomically if its issued-minus-returned WIP quantity cannot cover the receipt.
- Scheduling is a planning aid: dispatchers may manually reschedule or split an operation, and the next scheduler run preserves manual schedules.
- Quality holds block operation completion and finished-goods receipt until an authorized release. Material issue and receipt references are retained for genealogy.
- Electronic signatures and immutable audit events are captured as compliance foundations; organizations must configure and enforce their own approval policy before relying on them for regulated certification.
