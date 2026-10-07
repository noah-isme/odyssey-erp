# Odyssey ERP Version and Progress Report

**Reviewed:** 2026-10-07

## How to read the version numbers

`v0.9.1` is the latest named production release in the repository. It is primarily
a UI/UX release dated 2026-05-28. `v0.10.0-rc.10` is the current release candidate
for the post-v0.9.1 platform work; it is not production-certified. It keeps the
`v0.10-core` profile and the `000124` migration ceiling. Its exact candidate
commit, recorded when the annotated tag was cut, is `0745957`. Lineage:
the superseded rc.9 tag `07d2ba2` plus the rc.10 approval-finalization fix and
its remediation runbook (see the rc.10 entry below). The superseded rc.9 and
rc.8 entries below keep their identity records; the historical rc.8 lineage
(merge-base and stabilization chain) is also in the [candidate identity reconciliation](v0.10-candidate-identity-reconciliation.md).

In other words:

- **Latest named production release:** v0.9.1.
- **Current release candidate:** v0.10.0-rc.10 (2026-10-07; tag pending).
- **Latest documented implementation progress:** Phase 10–14 and P7 work, reviewed 2026-08-01.
- **Next final release:** v0.10.0, pending production certification. The release
  gates are tracked in the [Production Release Checklist](production-release-checklist.md).

The roadmap and module catalog describe post-v0.9.1 progress; the candidate release
notes define the packaged scope without claiming production certification.

## At-a-glance comparison

| Version | Date | Main change | What it added or improved | Current relevance |
|---|---|---|---|---|
| v0.7.0 | Earlier Phase 7 release | Finance export reliability | Consolidated P&L/BS exports, warning consistency, streaming CSV, Gotenberg PDF hardening, metrics and rate limits | Foundation for current finance reporting/export operations |
| v0.8.0 | Phase 8 | Board packs | Async executive PDF packs with templates, KPIs, variance sections, lifecycle, storage, and RBAC | Still a current finance/reporting capability |
| v0.9.0 | 2026-01-11 | Sales and AR | Quotations, sales orders, delivery orders, AR invoices, payment allocation, aging | Still the functional baseline for Sales and AR |
| v0.9.1 | 2026-05-28 | Enterprise UI/UX | Standardized forms, filters, tables, responsive layouts, and Midnight Ledger styling across core operations | Latest named release documented in this repository; mostly presentation and usability improvements |
| v0.10.0-rc.1 | 2026-08-10 | Platform foundations and release controls | Advanced documents, CMMS telemetry/prediction foundations, MRP compliance hardening, distribution/finance/connectors work, and production gates | Superseded candidate; staging, provider, and operational certification remain open |
| v0.10.0-rc.2 | 2026-08-10 | Coretax and PPh 21 release-test completion | Fail-closed Coretax transport/validation, export-to-GL contract evidence, and annual last-tax-period PPh 21 reconciliation from a PMK 168/2023 fixture | Superseded candidate; official tax, staging, provider, and operational certification remain open |
| v0.10.0-rc.3 | 2026-08-10 | VPS deployment target and release-gate cleanup | Self-managed VPS runbook, removal of the obsolete hosted blueprint, and evidence-based feature matrix | Superseded candidate; feature, provider, and operational certification remain open |
| v0.10.0-rc.4 | 2026-08-12 | Exact candidate evidence and migration-safe release gates | Executable migration/seed runbook targets, exact tagged-candidate evidence checks, current generated SQLC bindings, and the final lint fix for the release baseline | Superseded candidate; not production-certified; staging, provider, and operational certification remain open |
| v0.10.0-rc.5 | 2026-08-13 | Bounded route-contract and deployment-gate hardening | Tagged E2E route-contract checks, core route-manifest/RBAC seed alignment, and deployment gate hardening | Superseded immutable tag at `1d81938`; the post-tag staging supervision fix is carried by later candidates |
| v0.10.0-rc.6 | 2026-08-14 | Post-tag staging supervision fix | Release-branch head `d8b02b8` adds the deployment supervision fix after immutable rc.5; it preserves the `ec65cc0` application baseline and `000124` migration ceiling | Superseded candidate; the descendant rc.7 candidate carries the release-hygiene correction |
| v0.10.0-rc.7 | 2026-08-26 | Immutable release-hygiene correction | Exact release candidate `5ed11da` adds the portable hygiene scan while preserving the `ec65cc0` application baseline and `000124` migration ceiling | Superseded by v0.10.0-rc.8; never production-certified |
| v0.10.0-rc.8 | 2026-09-06 | Divergent stabilization candidate | Exact release candidate `20cc13a` (annotated tag, tag object `f54a238`) on a line diverging from rc.7 at merge-base `04ebd8a`: staging certification workflow registration, scoped certification gates, enterprise UI hardening, story-driven seed data, lint/CI fixes, migration ceiling sync to `000124`, CSRF/module-UI fixes, and 33 further rc-line commits (POS, theme, MRP, banking, reporting, route profile). Identity reconciled to the tag under the 2026-10-02 Option A ruling; the earlier documented `cdaa910` pointer was superseded when the tag was re-cut | Superseded by v0.10.0-rc.9 before staging certification evidence was collected; never production-certified |
| v0.10.0-rc.9 | 2026-10-04 | Worker idempotency and attribution fixes | Exact release candidate `07d2ba2` (annotated tag, tag object `7c47a3a`): superseded rc.8 tag plus fix commits: payslip delivery row lock (at-least-once) with mail dial/I-O deadlines, AP invoice processing actor binding and dedupe, connector outbox claim-before-execute, bounded retry and TaskID dedupe for variance and board-pack tasks, out-of-profile worker handlers and schedules not registered under `v0.10-core`, worker DB pool default 16 (`PG_MAX_CONNS` knob); no new migration (ceiling `000124`) | Superseded by v0.10.0-rc.10 after its staging deployment and before staging certification ran; never certified |
| v0.10.0-rc.10 | 2026-10-07 (tag pending) | Approval finalization fix | Superseded rc.9 tag plus the approval engine fix: `Decide` returns the post-decision request, so final approvals reach module finalizers as `APPROVED` (LEAVE, PO, RFQ award, payroll), and the step quorum count is scoped to the request; data-remediation runbook for environments that ran rc.1–rc.9; no new migration (ceiling `000124`) | Current candidate; not production-certified; staging, provider, and operational certification remain open |

## Detailed version reports

### v0.7.0 — Finance export reliability

**Primary purpose:** graduate consolidated Profit & Loss and Balance Sheet exporters to
general availability.

**Changes and improvements:**

- Consistent warning propagation across SSR, CSV metadata, and PDF captions.
- Gotenberg v8 PDF client with retries, timeout, and payload-size validation.
- Buffered/streaming CSV exports with metadata headers.
- Finance export runbooks and operational tooling.
- Prometheus metrics, structured logs, and export rate limiting.

**Operational impact:** export automation must have `finance.export_consolidation` and
handle the 10-requests-per-minute limit. Gotenberg is required for PDF output.

See the [v0.7.0 release notes](v0.7.0.md).

### v0.8.0 — Board Pack Generator

**Primary purpose:** package financial results into an executive-ready PDF.

**Changes and improvements:**

- Customizable board-pack templates and sections.
- Asynchronous Asynq generation for longer reports.
- Gotenberg-rendered PDF output.
- Lifecycle states: `DRAFT → GENERATING → COMPLETED/FAILED`.
- Board-pack list, creation, detail, and download pages.
- Dedicated `finance.boardpack` permission and seeded standard template.

**Operational impact:** configure `BOARD_PACK_STORAGE`, Gotenberg, migrations, and the
board-pack permission before using the feature.

See the [v0.8.0 release notes](v0.8.0.md).

### v0.9.0 — Sales and Accounts Receivable

**Primary purpose:** complete the revenue-side document chain.

**Changes and improvements:**

- Customer management and quotation CRUD.
- Quotation approval: `DRAFT → SUBMITTED → APPROVED/REJECTED`.
- Sales orders created from approved quotations.
- Delivery orders with partial delivery and warehouse/stock validation.
- AR invoice lines linked to delivery orders.
- AR posting, voiding, payment allocation, and automatic paid status.
- Aging reports and dedicated AR permissions.

**Operational impact:** this release established the current `/sales`, delivery, and
`/finance/ar` functional baseline. Later work added returns/credit notes and deeper tax
and FX behavior around these documents.

See the [v0.9.0 release notes](v0.9.0.md).

### v0.9.1 — Enterprise UI/UX overhaul

**Primary purpose:** improve usability and visual consistency across existing workflows.

**Changes and improvements:**

- Standardized forms for Sales, Procurement, Inventory, AP, and payment workflows.
- Responsive form grids and reusable form components.
- Unified list filters and action layouts.
- Sticky table headers and improved numeric alignment.
- Consistent truncation, hover states, borders, and Midnight Ledger design tokens.
- Build validation for the updated templates and CSS.

**What this version did not mean:** v0.9.1 did not represent completion of every ERP
module. It mainly improved the UI of existing modules.

See the [v0.9.1 release notes](v0.9.1.md).

### v0.10.0-rc.1 — Platform foundations and release controls

**Primary purpose:** package the post-v0.9.1 implementation work as a reviewable
release candidate while keeping production certification evidence explicit.

See the [v0.10.0-rc.1 release notes](v0.10.0-rc.1.md).

### v0.10.0-rc.2 — Coretax and PPh 21 release-test completion

**Primary purpose:** resolve the skipped tax release tests without treating local
contract evidence as official authority or production certification.

See the [v0.10.0-rc.2 release notes](v0.10.0-rc.2.md).

### v0.10.0-rc.3 — VPS deployment target and release-gate cleanup

**Primary purpose:** align production deployment and release checks with
self-managed VPS operation.

See the [v0.10.0-rc.3 release notes](v0.10.0-rc.3.md).

### v0.10.0-rc.4 — Exact candidate evidence and migration-safe release gates

**Primary purpose:** freeze the v0.10-core staging candidate at the reviewed
`ec65cc0` commit while documenting migration, release identity, and certification
evidence requirements explicitly.

The candidate ends at migration `000124_scoped_rbac_global_compatibility`.
The later v0.11-finance implementation commit `1a8343e` and migration
`000125_payment_settlement_results` are excluded from rc.4 and remain on the
next-release line. This candidate is not production-certified.

See the [v0.10.0-rc.4 release notes](v0.10.0-rc.4.md).

### v0.10.0-rc.5 — Bounded route-contract and deployment-gate hardening

The immutable `v0.10.0-rc.5` tag at `1d81938` added bounded route-contract E2E
checks, route-manifest/RBAC seed alignment, and deployment-gate hardening. It is
superseded and was never production-certified because the staging supervision fix
landed afterward on the release branch.

### v0.10.0-rc.6 — Post-tag staging supervision fix (superseded historical candidate)

**Primary purpose:** certify the exact post-rc.5 release head while keeping the
v0.10-core scope and migration boundary unchanged.

The candidate commit is `d8b02b87fd614edec31e465abc38667ad91f7548`. The reviewed
application baseline remains `ec65cc08639c184030c63e3407791987eee92804`; the
candidate ends at migration `000124_scoped_rbac_global_compatibility`. The later
v0.11-finance implementation commit `1a8343e` and migration
`000125_payment_settlement_results` remain outside the candidate. Staging,
provider, security, and operational evidence are still pending in the [staging
certification record](v0.10-core-staging-certification.md).

This entry is retained only for release lineage; the active certification record
and current candidate are v0.10.0-rc.10 (see the rc.10 entry below).

### v0.10.0-rc.7 — Immutable release-hygiene correction (superseded)

**Primary purpose:** retain the exact v0.10-core candidate identity while making
the release hygiene check portable to environments without ripgrep.

The candidate commit is `5ed11da8aea342708be67284ea7a71224f90ccdc`. It preserved
the reviewed application baseline `ec65cc08639c184030c63e3407791987eee92804`,
the `v0.10-core` scope, and the `000124_scoped_rbac_global_compatibility`
migration ceiling. It was superseded before any staging certification evidence
was collected; the rc.8 candidate (itself superseded by rc.9, which rc.10 superseded) replaced it.

### v0.10.0-rc.8 — Divergent stabilization candidate (superseded)

**Primary purpose:** carry the v0.10-core release hardening — the staging
certification workflow registration, scoped certification gates, enterprise UI
hardening, story-driven seed data, lint/CI fixes, migration ceiling sync, and
CSRF/module-UI fixes — as the current immutable candidate.

The candidate commit is `20cc13a0f028e3b09573944bb9f7a1f943461253` (annotated
tag `v0.10.0-rc.8`, tag object `f54a238051a9ffc07a1d105189563bf5a50fad60`,
dated 2026-09-06; reconciled under the 2026-10-02 Option A ruling). Its line
diverges from the superseded rc.7 candidate at
merge-base `04ebd8a040ff3c5da6f90c6f8d0eab5f4a9ba336` and carries the rc.8
stabilization chain plus 33 further rc-line commits; it does not preserve the
`ec65cc0` application baseline.
It keeps the `v0.10-core` scope and the `000124_scoped_rbac_global_compatibility`
migration ceiling, excludes the v0.11-finance commit `1a8343e` and migration
`000125`, and has a green CI run ([34007099245](https://github.com/noah-isme/odyssey-erp/actions/runs/34007099245))
and a successful tag-push staging deployment run ([34007099231](https://github.com/noah-isme/odyssey-erp/actions/runs/34007099231))
on the candidate commit. Staging, provider, security, migration, and
operational certification remain pending in the [staging certification record](v0.10-core-staging-certification.md).

It was superseded by v0.10.0-rc.9 before any staging certification evidence was
collected, after the ISO-004 worker review found duplicate-effect and
forged-actor paths in in-profile worker handlers.

### v0.10.0-rc.9 — Worker idempotency and attribution fixes (superseded)

**Primary purpose:** ensure that every ISO-004 target worker handler reachable under
the `v0.10-core` profile rejects forged input and produces no duplicate effect on
redelivery, using only existing columns, row/advisory locks, and asynq TaskIDs.

The candidate is the superseded rc.8 tag commit plus the rc.9 fix commits; the exact tagged
commit is `07d2ba2bbb553a5594828771361e3ab546b35b70` (annotated tag
`v0.10.0-rc.9`, tag object `7c47a3ab22ad832bbd8a182ab1468889472bc41d`, dated
2026-10-04). It keeps the `v0.10-core`
scope and the `000124_scoped_rbac_global_compatibility` migration ceiling and
adds no migration. Fixes:

- **Payslip delivery** (`76b4afa`): `DeliverPayslipOnce` locks the undelivered
  payslip row (`FOR UPDATE ... SKIP LOCKED`) and writes `delivered_at` in the
  same transaction only after the SMTP send succeeds, so a concurrent duplicate
  skips and a failed send retries. Delivery is at-least-once: a crash after the
  SMTP server accepted the message and before COMMIT re-sends on retry. The mail
  client now has a 10s dial and 60s I/O deadline (a QUIT failure after the
  server accepted DATA is logged and ignored, so it cannot trigger a duplicate
  send; credentials are skipped for a HELO-only server exactly as
  `net/smtp.SendMail` does), and `payroll:payslip_email` tasks a 3-minute
  timeout. The delivery transaction uses a single pool
  connection and sets its own transaction-local
  `idle_in_transaction_session_timeout` (4 minutes, just above the task
  timeout), so no server-wide minimum is required (the staging preflight only
  records the server value).
- **AP invoice processing** (`04af2a6`): the actor is bound to
  `ap_invoices.created_by` and a contradicting payload actor is rejected and
  archived without retry; a non-blocking, namespaced advisory lock and
  per-(invoice, run, type) existence checks prevent duplicate matching runs and
  exceptions; the producer sets a stable TaskID. A concurrent duplicate that
  finds the lock held returns the retryable `ap.ErrInvoiceProcessingBusy`
  (never success), so a lock orphaned by a crashed worker cannot complete the
  redelivered task with the invoice unprocessed: the lock transaction sets
  `idle_in_transaction_session_timeout` to 6 minutes so PostgreSQL terminates an
  orphaned holder, `ap:invoice_process` uses `MaxRetry(5)` so its retries
  outlast that bound, and exhausted retries are archived (visible). In ISO-004
  `S09` a concurrent duplicate may therefore show `Retried >= 1` before it
  completes; the final effects (one run, one exception or one post) are
  unchanged. `posted_by` on the legitimate path is the invoice creator, as
  before. `ap_matching_runs.run_by` is not persisted by `RunMatch` (unchanged;
  it was not persisted before this candidate either).
- **Connector outbox** (`c404c3a`): each command is claimed (CAS that bumps
  `attempts` and leases it for 10 minutes) before the adapter runs; exhausted
  commands dead-letter. External execution remains at-least-once.
- **Bounded convergence** (`89abffe`): variance snapshot and board-pack tasks
  use `MaxRetry(5)` and TaskID dedupe; a not-found snapshot archives without
  retry.
- **Profile-gated worker handlers** (`003ac71`): `bankfeeds:sync`,
  `bankfeeds:event`, `cashforecast:refresh`, `finance:automation_dispatch`
  (handler and its every-minute schedule; no application code produces its
  commands, `FIND-008`), and `analytics:bi_export` are not registered under
  `v0.10-core`; injected tasks archive as "handler not found".
- **Worker DB pool** (`c307a35`): the worker pool defaults to 16 connections
  and is configured with the `PG_MAX_CONNS` environment variable (precedence:
  `PG_MAX_CONNS`, then `pool_max_conns` in the worker's `PG_DSN`, then 16). Do
  not put `pool_max_conns` into a `PG_DSN` shared with the deploy `migrate`
  step. The worker logs a startup warning when the pool is below 3 x its
  concurrency (5). Confirm PostgreSQL `max_connections` headroom before
  deploying.

Deferred findings (not fixed in rc.9; `FIND-001`–`FIND-007` in the [staging
certification record](v0.10-core-staging-certification.md#findings)): CRM
reminder select-to-mark race, overdue-invoice scan hardcoded recipient without
a sent marker, `mail:send`/`email:deliver` recipient trust, `analytics:bi_export`
company trust outside `v0.10-core`, connector outbox and payslip at-least-once
windows, and CMMS PM generator overlap without a duplicate guard. `FIND-008`
(`finance:automation_dispatch` registered and scheduled on every profile) is
resolved in rc.9 by the profile gating above.

rc.9 was tagged and deployed to staging, and its `certification/v0.10.0-rc.9`
dispatch branch was created, but staging certification never ran: the operator
lane was still waiting on the staging preflight inputs. It was superseded by
v0.10.0-rc.10 before certification, after the approval finalization defect
described in the rc.10 entry was found. rc.9 was never certified.

### v0.10.0-rc.10 — Approval finalization fix

**Primary purpose:** stop the approval engine from finalizing approved
documents as rejected or leaving them stuck, a defect present in every
candidate since rc.1.

The candidate is the superseded rc.9 tag commit `07d2ba2` plus the rc.10
commits; the exact tagged commit, recorded at tag time, is `0745957`. It
keeps the `v0.10-core` scope and the `000124_scoped_rbac_global_compatibility`
migration ceiling and adds no migration. It carries no HR UI work from `main`.

**Defect.** `approvals.Repository.Decide` returned the request as it was before
the decision; only a rejection refreshed it. On a final approval the approval
engine row was correct (`approval_requests.status = 'APPROVED'`), but
`Service.Decide` passed the stale status `PENDING` to the module finalizers and
the notifier. The defect entered with the configurable approval engine
(`6f50f41`) and is present in `v0.10.0-rc.1` through `v0.10.0-rc.9`.

**Impact.**

- `LEAVE`: approved leave requests were finalized as `REJECTED`, the pending
  balance was released without incrementing `used`, and a false
  `LEAVE_REJECTED` audit row was written under the approver.
- `PO`: approved purchase orders were `CANCELLED` with no approver recorded.
- `RFQ_AWARD`: approved awards were `REJECTED` and generated no POs.
- `PAYROLL`: payroll runs stayed in `APPROVAL` with no journal, pointing at an
  `APPROVED` request.
- Requesters received an "approved" notification that said the document "was
  PENDING", and approvers of step 2 and later on multi-step policies were never
  sent an assignment notification.
- The step quorum count counted approvals across every request rather than the
  request being decided, so a step with `required_approvals > 1` could complete
  early. The policy UI always creates `required_approvals = 1`, so only policies
  inserted directly in the database are affected.

**Fix** (`75289ba`, `fix(approvals): return post-decision request from
Decide`): `Decide` returns the request as it is after the decision, so
finalizers and notifications receive `APPROVED` and the next step's approvers
are notified on advance, and the quorum count is scoped to the request. pgxmock
tests in `internal/approvals/repository_test.go` cover final approval, advance,
an unsatisfied quorum, and rejection.

**Remediation.** Deploying rc.10 stops new damage but does not repair rows that
are already wrong. Every environment that ran any build from rc.1 through rc.9
must, after deploying rc.10, run the read-only **Find** queries in the
[approval finalization remediation runbook](v0.10-approval-finalization-remediation.md)
and repair each affected row only after a business owner confirms the intended
outcome; record every repaired row in that environment's release evidence.

## Follow-up work after v0.10.0-rc.10

This work is documented in the current [roadmap](../ROADMAP.md) and [module catalog](../reference/module-catalog.md); the candidate packages the current scope, while final promotion remains pending.

### Finance and operations

- v0.11-finance handoff: the worker now composes the durable
  `payment.result.import` boundary only; live provider execution and confirmed
  AP/GL/tax/FX/bank effects remain disabled until their adapters are certified.
- Accounts Payable: vendor invoices, payments, allocations, and aging.
- Banking: accounts, transactions, transfers, reconciliation, cash flow, and manual CSV/OFX imports.
- Inventory: stock takes, adjustments, lot/serial tracking, replenishment, and AVG/FIFO valuation.
- Fixed assets: register, categories, depreciation, disposal, and accounting integration.
- Transaction-level FX: realized/unrealized valuation and reversal model; staging/production gates remain.
- Tax compliance: immutable tax documents, PPN/PPh ledgers, GL reconciliation, period locks, and Coretax export; official portal validation remains.

### CRM and people operations

- CRM leads, contacts, opportunities, pipeline, activities, reminders, conversion, and win/loss analytics.
- HR employee directory, organization, leave, attendance, approvals, and Indonesian payroll.
- Notifications with in-app delivery, email preferences, SMTP worker delivery, and deduplication.

### Horizon foundation

- WMS bins, barcode aliases, pick waves, pick tasks, and scans.
- MRP BOMs and work orders.
- POS terminals, sessions, tickets, payments, refunds, and voids.
- Projects, tasks, members, timesheets, and FX snapshots.
- Public REST API, API keys, webhooks, customer/supplier/employee portals, isolation, and idempotency.

### Remaining release work

- Promote `v0.10.0` only after the rc.10 candidate passes staging, provider, security,
  migration, and operational certification gates.
- Complete staging/production acceptance for FX and Horizon features.
- Complete external Coretax validation.
- Decide scope for the next release: manufacturing depth, projects, POS, integrations,
  document management, CMMS/QMS, or enterprise security controls.

## Versioning rule going forward

The repository should update the release notes and version number when a coherent set
of work is packaged. Until final promotion, use:

- `README.md` for the current release candidate and high-level status.
- This report for differences between releases and post-release progress.
- [`docs/reference/feature-matrix.md`](../reference/feature-matrix.md) for release status.
- [`docs/reference/module-catalog.md`](../reference/module-catalog.md) for feature inventory.
- [`docs/ROADMAP.md`](../ROADMAP.md) for sequencing and release gates.
