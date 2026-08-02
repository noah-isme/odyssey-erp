# Product Workflow Depth Execution Plan

**Status:** Planned. Odyssey currently provides project/task/timesheet foundations,
POS terminals/sessions/tickets/payments, HR employees/leave/attendance/payroll, and CRM
leads/contacts/opportunities/activities. The workflow depth defined here is not yet
implemented.

## Summary

Deliver four coordinated product streams:

- Projects: milestones, dependency-aware Gantt scheduling, task-backed Kanban boards,
  budget versions, actual cost, forecasting, and variance control.
- POS: loyalty programs, an immutable points ledger, gift-card liability and balance
  control, and governed scanner/printer/cash-drawer integration.
- HR: recruitment, structured performance cycles, and training/certification records.
- CRM: governed segmentation and consent-aware campaign execution.

Existing modules remain authoritative for their current records. New workflows extend
projects, tasks, tickets, employees, contacts, customers, and opportunities rather
than copying them into parallel masters.

## 1. Shared foundations

Apply these controls to every stream:

- Company-scoped tables, queries, jobs, exports, caches, and uniqueness constraints.
- Exact PostgreSQL `NUMERIC` values and Odyssey's exact money/FX types for budgets,
  costs, gift-card balances, campaign spend, and other monetary data. Do not add new
  `float64` monetary boundaries.
- Optimistic versions on editable plans and idempotency keys on finalizing actions,
  worker dispatch, imports, points posting, gift-card posting, and message delivery.
- The shared approval engine for project-budget activation/variance, POS manual value
  adjustments, job requisitions/offers, performance calibration, and campaign launch.
- The existing audit system for lifecycle changes, baselines, board moves, overrides,
  score evidence, points/value movements, hardware activity, and campaign events.
- Existing Asynq, SMTP, notification, and outbox patterns for retryable background
  work. Marketing messages use a dedicated campaign outbox even when they reuse the
  SMTP transport.
- Permissions seeded only to administrator roles. Assignments to project managers,
  cashiers, HR, managers, trainers, and marketers remain explicit.
- Feature flags and staged activation per company and, where relevant, per terminal or
  project.
- Immutable approved/finalized snapshots; corrections use a new version, reversal,
  or explicit adjustment event.

Use the planned general document service for resumes, offer letters, training
materials, certificates, campaign assets, and project evidence when it becomes
available. Do not introduce another module-specific binary store.

## 2. Projects: planning, boards, and budgeting

### 2.1 Ownership and data model

Keep the existing project and task records authoritative. Add:

- Project calendars, working days, holidays, and scheduling timezone.
- Milestones with owner, planned/actual dates, acceptance criteria, and status.
- Task hierarchy, planned dates, duration, progress, estimate, priority, assignee, and
  milestone link.
- Typed task dependencies: finish-to-start, start-to-start, finish-to-finish, and
  start-to-finish, with optional lag.
- Schedule baseline versions and immutable before/after snapshots.
- Project boards, ordered columns, WIP limits, and per-column task-state mappings.
- Project budget versions, cost categories, period lines, resource plans, exact rates,
  contingency, forecast, commitments, actuals, and variance explanations.
- Cost-source links to approved timesheets, procurement commitments, AP actuals,
  inventory issues, expenses, and journals.

A Kanban card is a view of a project task, not a duplicate work item. Gantt and Kanban
therefore share assignee, status, progress, dates, dependencies, and audit history.

### 2.2 Lifecycles and scheduling rules

Retain the project lifecycle:

`DRAFT → OPEN → ON_HOLD → COMPLETED`

With `CANCELLED`. Reopening a completed project requires permission and an audited
reason.

Milestones use:

`PLANNED → IN_PROGRESS → AT_RISK → COMPLETED`

With `CANCELLED`.

Tasks retain `OPEN → IN_PROGRESS → DONE`, with `CANCELLED`. Board column moves
invoke the task transition service and use an optimistic version; handlers must not
update task state directly.

Scheduling must:

- Reject self-dependencies and dependency cycles.
- Calculate dates using the project calendar and explicit lag.
- Preserve manually fixed dates and identify the constraint causing a conflict.
- Show critical-path and float calculations from a versioned schedule snapshot.
- Recalculate asynchronously for large projects and discard stale results.
- Require a reason and optional approval for changes to an active baseline.
- Block project completion while required milestones or tasks remain open.

### 2.3 Kanban behavior

- Configure one or more boards per project from the same task population.
- Map columns to valid task states and define allowed forward/backward moves.
- Enforce WIP limits transactionally; overrides require permission and reason.
- Support rank-based ordering without renumbering every task on each move.
- Filter by milestone, assignee, priority, due state, and label.
- Record actor, source/target column, before/after rank, version, and override evidence.
- Update Gantt progress immediately after a successful task move.

### 2.4 Project budgeting

Budget versions use:

`DRAFT → SUBMITTED → APPROVED → ACTIVE → SUPERSEDED/CLOSED`

With `REJECTED`.

- Only one active budget version is allowed per project.
- Submission freezes lines, rates, FX snapshots, and approval amount.
- Use company base currency for reporting while preserving source currency and dated
  FX snapshots.
- Separate original budget, approved changes, commitments, actuals, estimate to
  complete, forecast at completion, and variance.
- Approved/locked timesheets feed labor actuals exactly once.
- Approved POs feed commitments; GRN/AP/accounting events replace commitments with
  actual cost according to a documented source hierarchy.
- Variance thresholds may require approval before new commitments or project closure.
- Never infer project accounting from free-text descriptions; every cost source uses a
  validated project/task/cost-category reference.

### 2.5 Project permissions and interfaces

Introduce:

- `projects.view`, `projects.manage`, and `projects.member.manage`
- `projects.milestone.manage`
- `projects.schedule.view`, `projects.schedule.manage`, and
  `projects.schedule.override`
- `projects.board.manage` and `projects.task.move`
- `projects.budget.view`, `projects.budget.edit`, `projects.budget.submit`, and
  `projects.budget.approve`
- `projects.cost.view` and `projects.cost.close`

Add workspaces under `/projects/{id}/milestones`, `/projects/{id}/gantt`,
`/projects/{id}/board`, `/projects/{id}/budget`, and `/projects/{id}/costs`.

### 2.6 Project reporting

Provide milestone health, overdue/blocked work, critical path, schedule variance,
resource load, WIP breaches, budget-versus-actual, committed cost, estimate to
complete, forecast at completion, and project margin where revenue is linked.

## 3. POS: loyalty, gift cards, and hardware

### 3.1 Loyalty model

Add:

- Versioned loyalty programs, earning/redemption rules, tiers, effective dates, and
  expiry policies.
- Customer loyalty accounts and tier history.
- Immutable points ledger entries with earn, redeem, expire, reverse, adjust, reserve,
  and release types.
- Source links to ticket, line, payment, refund, campaign, and manual adjustment.
- Point reservations that prevent concurrent redemption overspend.

Rules:

- A customer identity is required to earn or redeem points; anonymous tickets do not
  create shadow customers.
- A completed eligible ticket posts points exactly once.
- Redemption reserves points while the ticket is open and posts on completion.
- Cancellation releases reservations; refund reverses the original eligible earn and
  restores original redeemed points according to the configured policy.
- Expiry runs through a retry-safe worker and posts ledger entries rather than
  rewriting balances.
- Manual adjustments require reason, permission, and optionally approval above a
  configured threshold.
- Program changes create a new effective version and never recalculate posted history.

### 3.2 Gift cards

Add gift-card programs, cards, secure token hashes, activation state, currency,
expiry, immutable value ledger, transaction attempts, and accounting links.

Gift cards use:

`CREATED → ACTIVATED → PARTIALLY_REDEEMED → REDEEMED`

With `SUSPENDED`, `EXPIRED`, and `CANCELLED`.

- Generate high-entropy card identifiers and store only protected lookup material;
  never treat a printed sequential number as the authorization secret.
- Activation, load, redeem, refund, transfer correction, expiry, and void are
  idempotent ledger movements.
- Lock the card or use an optimistic balance version during redemption.
- Prohibit negative balances and cross-company use.
- A card is denominated in one currency; v1 does not perform gift-card FX conversion.
- Gift-card sale/load creates a liability, redemption relieves that liability, and
  breakage recognition requires an explicit accounting policy and journal lineage.
- Refunds return value to the original gift card where policy permits; cash conversion
  requires a separately approved exception.

### 3.3 Hardware boundary

Add terminal hardware profiles, registered devices, capability assignments, health
state, configuration versions, print jobs, device events, and operator diagnostics.

Support the following v1 boundary:

- Keyboard-wedge barcode scanners, with product/barcode validation and optional GS1
  parsing.
- Receipt printers through an authenticated local print agent using allow-listed
  ESC/POS profiles.
- Cash drawers triggered through the assigned receipt printer after an eligible
  payment event.
- Reprint jobs that reference an immutable completed ticket and carry a reprint mark.
- Device heartbeats, retry-safe print jobs, bounded retries, and operator-visible
  failures.

The browser never accepts arbitrary host commands or printer addresses. The local
agent authenticates to Odyssey, is bound to a company/terminal, fetches signed jobs,
and acknowledges a job idempotently. Raw customer/payment secrets must not be written
to device logs.

Certified payment-terminal integrations remain in the external payment-integration
stream; this plan does not process card PAN or PIN data.

### 3.4 POS permissions

Introduce:

- `pos.view`, `pos.manage`, `pos.session.open`, and `pos.session.close`
- `pos.loyalty.view`, `pos.loyalty.manage`, `pos.loyalty.redeem`, and
  `pos.loyalty.adjust`
- `pos.gift_card.view`, `pos.gift_card.issue`, `pos.gift_card.redeem`, and
  `pos.gift_card.adjust`
- `pos.hardware.view`, `pos.hardware.manage`, and `pos.receipt.reprint`

Keep ticket payment, refund, and void permissions separate from value-adjustment and
hardware-administration permissions.

### 3.5 POS reporting

Provide loyalty liability and expiry, earn/redemption/reversal activity, tier movement,
gift-card outstanding liability and breakage candidates, suspicious adjustment/use
patterns, print success/failure, device health, and terminal-level hardware incidents.

## 4. HR: recruitment, performance, and training

### 4.1 Recruitment

Keep HR employees, departments, positions, and manager relationships authoritative.
Add:

- Job requisitions, openings, headcount/budget references, approval, and publication.
- Candidates, consent, source, contact identity, retention date, and duplicate-review
  controls.
- Applications with configurable ordered stages.
- Interview panels, schedules, structured scorecards, decisions, and conflict flags.
- Offer versions, approval, acceptance/decline evidence, and expiry.
- Onboarding checklists and idempotent candidate-to-employee conversion.

Requisitions use:

`DRAFT → APPROVAL → APPROVED → OPEN → ON_HOLD → FILLED/CLOSED`

With `REJECTED` and `CANCELLED`.

Applications use:

`APPLIED → SCREENING → INTERVIEW → ASSESSMENT → OFFER → HIRED`

With `WITHDRAWN` and `REJECTED`.

Offers use `DRAFT → APPROVAL → SENT → ACCEPTED/DECLINED/EXPIRED`.

- Freeze interview scorecards on submission and retain scoring evidence.
- Require declared reasons for rejection and offer changes.
- Restrict candidate PII and compensation to recruitment permissions.
- Apply configurable candidate retention/anonymization while preserving aggregate
  hiring metrics and required audit evidence.
- Conversion creates or links one employee and records application/offer lineage;
  retries return the same employee.

### 4.2 Performance management

Add versioned review templates, competencies, rating scales, cycles, participants,
goals, check-ins, self reviews, manager reviews, peer inputs, calibration decisions,
acknowledgements, and development actions.

Performance cycles use:

`DRAFT → CONFIGURED → ACTIVE → CALIBRATION → FINALIZED → CLOSED`

Reviews use:

`NOT_STARTED → SELF_REVIEW → MANAGER_REVIEW → CALIBRATION → FINAL → ACKNOWLEDGED`

- Snapshot employee, position, department, manager, template, and goals at cycle
  launch.
- Validate total section weights and use exact decimal ratings.
- Freeze submitted inputs; corrections require reopening with reason and audit.
- Separate review author, calibration approver, and finalizer where configured.
- Preserve confidential peer attribution according to policy.
- Do not automatically change payroll or compensation from a rating. Any later merit
  workflow consumes an approved result through an explicit integration.

### 4.3 Training and certification

Add:

- Course catalog, versions, providers, delivery mode, prerequisites, and validity.
- Sessions, capacity, instructors, attendance, and results.
- Employee assignments from role, position, manager, QMS requirement, or manual need.
- Enrollment, waitlist, completion, failure, waiver, acknowledgement, and expiry.
- Certificates, evidence links, renewal requirements, and reminder/escalation jobs.
- Training matrices by employee, role, course, and compliance requirement.

Assignments use:

`ASSIGNED → ENROLLED → IN_PROGRESS → COMPLETED`

With `FAILED`, `WAIVED`, `CANCELLED`, and `EXPIRED`.

Training content and certificates link to the planned document service. QMS owns the
controlled-procedure requirement; HR owns assignment, attendance, completion, and
employee training history.

### 4.4 HR permissions

Introduce:

- `hr.recruitment.view`, `hr.recruitment.manage`,
  `hr.recruitment.interview`, and `hr.recruitment.offer.approve`
- `hr.performance.view_own`, `hr.performance.review`,
  `hr.performance.manage`, `hr.performance.calibrate`, and
  `hr.performance.finalize`
- `hr.training.view_own`, `hr.training.manage`, `hr.training.instruct`, and
  `hr.training.verify`

Manager access is derived from the effective employee hierarchy and never implies HR
administrator access to unrelated candidates or confidential cycles.

### 4.5 HR reporting

Provide time-to-fill, stage conversion, source effectiveness, offer acceptance,
requisition aging, performance-cycle completion, rating distribution/calibration,
goal progress, required-training compliance, upcoming expiry, overdue training, and
certification coverage.

## 5. CRM: campaigns and segmentation

### 5.1 Identity, consent, and segmentation

Keep CRM leads/contacts and Sales customers authoritative. Add:

- Marketing identities that link existing lead, contact, and customer records without
  copying those masters.
- Channel-specific consent, lawful basis, source, capture evidence, expiry, and
  withdrawal history.
- Company suppression lists for unsubscribe, hard bounce, complaint, and manual block.
- Segment definitions using a validated expression tree over allow-listed fields.
- Live preview counts and immutable audience snapshots.
- Membership inclusion/exclusion evidence and deduplication by marketing identity.

Do not accept raw SQL segment definitions. Query compilation remains server-side,
parameterized, company-scoped, complexity-limited, and covered by an explain/cost
guard before activation.

Segments use `DRAFT → VALIDATED → ACTIVE → RETIRED`. A campaign launch freezes
an audience snapshot so later profile changes do not silently change the send list.
Consent and suppression are rechecked immediately before every dispatch even when the
audience is frozen.

### 5.2 Campaign execution

Add:

- Campaigns, objectives, owners, channels, exact budgets, schedules, and approval.
- Versioned templates, sender identities, test sends, and optional A/B variants.
- Frozen audience snapshots and exclusions.
- Recipient/message records, delivery attempts, provider references, and idempotency.
- Delivery, bounce, complaint, unsubscribe, open, click, conversion, and revenue
  attribution events with confidence/source metadata.
- Pause, cancel, retry, and failure-recovery controls.

Campaigns use:

`DRAFT → REVIEW → APPROVED → SCHEDULED → RUNNING → COMPLETED`

With `PAUSED`, `REJECTED`, and `CANCELLED`.

Email is the first execution channel and reuses the configured SMTP transport through
a dedicated, rate-limited campaign outbox. SMS, WhatsApp, advertising networks, and
other channels remain disabled until their provider adapters exist.

- Require a test send and approval before scheduling a production audience.
- Resolve template variables from an allow-list and fail a recipient safely when a
  required value is unavailable.
- Check consent/suppression at dispatch time and append the reason for every skip.
- Make each recipient/channel/campaign dispatch idempotent.
- Process unsubscribe, hard bounce, and complaint events before future sends.
- Keep transactional notification preferences separate from marketing consent.
- Attribute conversions to CRM opportunities, quotations, sales orders, or completed
  POS tickets without rewriting source revenue.

### 5.3 CRM permissions

Introduce:

- `crm.segment.view`, `crm.segment.manage`, and `crm.segment.export`
- `crm.campaign.view`, `crm.campaign.create`, `crm.campaign.approve`,
  `crm.campaign.launch`, and `crm.campaign.pause`
- `crm.consent.view` and `crm.consent.manage`
- `crm.marketing.admin`

Owning a lead or opportunity does not grant access to export a segment or launch a
campaign.

### 5.4 CRM reporting

Provide audience size and exclusions, deliverability, bounce/complaint/unsubscribe
rates, engagement, conversion funnel, attributed pipeline/revenue, spend versus
budget, segment performance, consent coverage, and suppression growth.

## 6. Cross-module integration rules

| Producer | Consumer | Contract |
|---|---|---|
| Projects | Accounting/Procurement/HR | Project/task/cost-category references and exact commitment/actual facts |
| HR timesheets | Projects | Approved/locked labor quantity, rate snapshot, currency, and idempotent source ID |
| POS | Loyalty/Gift Card/Accounting | Completed/refunded ticket and immutable points/value/liability movements |
| CRM campaigns | POS/Sales/CRM | Attribution links only; source revenue and ticket/order state remain authoritative |
| HR Training | QMS | Assignment/completion evidence for QMS-defined controlled requirements |
| Document Management | All four streams | Governed files and evidence links; no module-specific binary duplication |

Outbox consumers must reject company mismatches, tolerate duplicates, and record the
source event/version they processed.

## 7. Delivery sequence

| Phase | Deliverable | Indicative duration |
|---|---|---:|
| 0 | Ownership ADRs, permission matrix, event contracts, migration fixtures | 2–3 weeks |
| 1 | Project milestones, task hierarchy/dependencies, Gantt, and baselines | 4–5 weeks |
| 2 | Project Kanban, budgets, commitments, actuals, and variance reporting | 5–6 weeks |
| 3 | POS loyalty ledger, rules, expiry, redemption, and refund integration | 4–5 weeks |
| 4 | Gift cards, liability accounting, hardware registry, scanner, and print agent | 5–6 weeks |
| 5 | HR recruitment, offers, conversion, privacy, and reporting | 5–6 weeks |
| 6 | HR performance cycles, calibration, training, and certification | 6–8 weeks |
| 7 | CRM consent, segmentation, email campaigns, suppression, and attribution | 6–8 weeks |
| 8 | Cross-module hardening, staging certification, runbooks, and rollout | 3–4 weeks |

After Phase 0, separately staffed product streams can proceed in parallel. Each phase
must deliver a disabled-by-default vertical slice with migration, rollback, tests,
metrics, and operator documentation.

## 8. Rollout

1. Add schemas, permissions, services, and events without exposing navigation.
2. Seed new permissions only to administrator roles.
3. Enable one bounded staging company and load production-like fixtures.
4. Pilot Projects with one active project and compare schedule/budget actuals.
5. Pilot POS value programs on one terminal with low-value test instruments.
6. Pilot HR workflows with synthetic candidate/review/training data before real PII.
7. Pilot CRM with internal recipients, enforced consent, and a strict send cap.
8. Obtain product owner, finance, HR/privacy, sales/marketing, security, and operations
   sign-off for the applicable stream.
9. Enable per company, project, program, cycle, campaign, or terminal; do not use one
   global cutover.

## 9. Mandatory validation

### Shared

- Fresh-schema, upgrade, and rollback migration rehearsals.
- Company isolation, RBAC, CSRF, audit, safe-error, and export tests.
- Exact money/FX serialization and round-trip tests.
- Optimistic-conflict, duplicate request, worker retry, and outbox replay tests.
- Approval, delegation, separation-of-duties, and notification tests.

### Projects

- Dependency-cycle rejection and all four dependency types.
- Calendar, timezone, lag, fixed-date, critical-path, and stale-recalculation tests.
- Concurrent Kanban move, ordering, WIP limit, and override tests.
- Budget version, FX snapshot, timesheet, commitment/actual replacement, variance, and
  close tests with no duplicate cost.

### POS

- Concurrent points/card redemption and insufficient-balance rejection.
- Ticket completion/refund/retry producing one correct ledger outcome.
- Point expiry, rule-version, gift-card activation, refund, expiry, and liability tests.
- Scanner mapping, signed print job, agent authentication, reprint marking, retry,
  offline device, and secret-redaction tests.

### HR

- Requisition/offer approval, stage transition, duplicate candidate, scorecard freeze,
  and idempotent employee conversion tests.
- Candidate PII scope, retention, anonymization, and audit-evidence tests.
- Review snapshot, weighting, confidentiality, reopen, calibration, finalization, and
  acknowledgement tests.
- Training prerequisite, capacity, attendance, completion, waiver, expiry, renewal,
  and QMS evidence tests.

### CRM

- Segment expression validation, tenant injection, complexity guard, deduplication,
  snapshot repeatability, and membership evidence tests.
- Consent withdrawal, suppression, test-send, approval, scheduling, pause, cancel,
  rate-limit, retry, bounce, complaint, and unsubscribe tests.
- Recipient dispatch idempotency and attribution without source revenue mutation.

Run full verification with `go test ./...`, `go vet ./...`, `make lint`, and
`make docs-check`.

## 10. Exit criteria

- Project Gantt and Kanban show the same task state and preserve an approved baseline.
- Project budget, commitment, actual, forecast, and variance reconcile to source
  transactions without duplicates.
- Loyalty and gift-card balances are derived from immutable ledgers and remain correct
  under concurrency, retry, void, and refund.
- Supported POS hardware is terminal-bound, authenticated, observable, and unable to
  execute arbitrary host commands.
- Recruitment converts one accepted application into one employee with complete
  approval and privacy evidence.
- Performance cycles are versioned, role-controlled, calibrated, finalized, and
  auditable.
- Required training and certificate expiry are visible and can satisfy linked QMS
  requirements.
- Campaigns cannot dispatch without approval, valid consent, and a suppression check.
- Segment membership and campaign delivery/conversion evidence can be reproduced.
- Every stream can be enabled independently per company after staging sign-off.

## 11. Deferred scope

- Automatic resource leveling and portfolio optimization.
- Offline-first POS ticket/payment processing.
- Card-PAN processing and certified payment-terminal kernels.
- Native drivers for every scanner/printer model and arbitrary browser device access.
- Public applicant portal, external job-board publishing, and AI candidate ranking.
- Compensation planning driven directly from performance ratings.
- SCORM/xAPI content authoring, video hosting, and virtual-classroom delivery.
- Visual marketing journey builder, ad-network buying, and unsupported messaging
  channels.

## Assumptions

- Project budgeting is operational subledger analysis; the General Ledger remains the
  accounting authority.
- Loyalty points are non-monetary. Gift cards are monetary liabilities and follow
  Finance-approved accounting policy.
- The first campaign execution channel is email through the existing SMTP transport.
- Candidate and employee privacy controls are configurable but do not themselves
  certify compliance with a specific jurisdiction.
