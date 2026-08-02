# Reporting and Administration Depth Execution Plan

**Status:** Planned. Odyssey currently provides finance analytics, accounting and
consolidation reports, board packs, scheduled P&L/budget delivery, operational module
views, RBAC, Indonesian/English user preferences, and company-specific accounting
period overlays. It does not yet provide a governed custom report builder, configurable
dashboard widgets, complete operational/HR reporting, a current cross-module role
matrix, or explicit language/timezone/fiscal-calendar policy.

## Summary

Deliver four coordinated capabilities:

- A governed reporting catalog and semantic layer for custom reports.
- Permission-aware dashboard widgets and scheduled/exported report runs.
- Certified operational and HR report coverage with reconciliation evidence.
- Administration policy for scoped roles, language, timezone, and fiscal calendars.

The implementation must reuse authoritative module services and reporting datasets.
Users never receive arbitrary SQL execution, unrestricted joins, or permission bypass
through reports, widgets, caches, schedules, or exports.

## 1. Shared governance foundations

Apply these controls across reporting and administration:

- Company-scoped definitions, runs, caches, assignments, settings, periods, jobs, and
  exports.
- Exact PostgreSQL `NUMERIC` values and Odyssey's exact money/FX types from query
  through export. Charts may derive display coordinates, but monetary API and service
  boundaries must not add new `float64` values.
- Optimistic versions for report definitions, dashboards, role matrices, and company
  policies.
- Idempotency keys for report runs, schedules, deliveries, fiscal-period generation,
  policy activation, and bulk role assignment.
- The shared approval engine for publishing sensitive reports, external distribution,
  high-risk role changes, and fiscal-calendar activation/change.
- The existing audit system for definition changes, query/run facts, exports,
  recipient decisions, role diffs, settings, and fiscal-period transitions.
- Permissions seeded only to administrator roles. Non-administrator assignments and
  role-template adoption remain explicit.
- Per-company feature flags and staged migration gates.
- Immutable published definitions and activated policies; changes create new versions.
- Transactional outbox delivery for schedules and cross-module reporting facts.

Every run captures definition version, dataset version, actor, company and row scope,
filters, locale, timezone, currency mode, fiscal calendar/version, permission
fingerprint, start/end timestamps, row count, and outcome.

## 2. Governed reporting semantic layer

### 2.1 Dataset ownership

Introduce a reporting catalog whose datasets are registered by code and migrations,
not entered as SQL by users. Each dataset declares:

- Stable key, version, business owner, technical owner, description, and grain.
- Authoritative tables/views/service adapter and supported freshness mode.
- Allow-listed dimensions, measures, aggregations, joins, filters, and sort fields.
- Data types, units, exact scale, currencies, and date/time semantics.
- Mandatory company/branch/owner/manager scope predicates.
- Permission and sensitive-data classification for every field.
- Reconciliation source, known exclusions, freshness SLO, and certification status.
- Maximum lookback, row limit, execution timeout, and export policy.

Dataset lifecycle:

`DRAFT → VALIDATED → CERTIFIED → PUBLISHED → DEPRECATED → RETIRED`

Only published dataset versions may back shared reports, dashboards, schedules, or
exports. Existing finance analytics, accounting reports, MRP analytics views, board
packs, and the reporting catalog become initial registered datasets rather than being
rewritten into one generic fact table.

### 2.2 Report definitions

Add company-scoped records for:

- Report folders and catalog entries.
- Report definitions and immutable versions.
- Selected fields, filters, grouping, sorting, totals, and presentation settings.
- Validated calculated fields using an allow-listed expression language.
- Sharing grants for roles/users and sensitive-field access rules.
- Report runs, result manifests, failures, cancellations, and lineage.
- Schedules, recipients, delivery attempts, and delivery evidence.
- Export objects and retention metadata.

Report lifecycle:

`DRAFT → VALIDATION → REVIEW → PUBLISHED → RETIRED`

With `REJECTED` and `DISABLED`.

- Draft edits use optimistic concurrency.
- Validation compiles the definition against one published dataset version.
- Publishing freezes the dataset version, field contracts, filters, display metadata,
  owner, sharing policy, and approval evidence.
- A published change creates a successor version; existing scheduled/run evidence
  remains linked to the prior version.
- Dataset retirement blocks new runs after a declared deadline and identifies every
  dependent report/widget/schedule.

### 2.3 Safe query compiler

The builder stores a structured definition and never raw SQL. The server compiler:

1. Loads the published dataset and report version.
2. Intersects actor permissions, role/company/branch scope, manager hierarchy, and
   field classifications.
3. Validates selected fields, joins, filters, functions, grouping, and sort keys.
4. Injects mandatory tenant and row-scope predicates that users cannot remove.
5. Uses parameterized values and allow-listed SQL fragments only.
6. Estimates cost and rejects unsafe cardinality or lookback before execution.
7. Runs on a read-only reporting connection with statement timeout, row limit, and
   cancellation.
8. Records the normalized plan hash, dataset version, scope, and result metadata.

Do not support arbitrary SQL, stored procedure calls, arbitrary JSON paths, arbitrary
joins, user-supplied HTML/JavaScript, or calculated-field functions capable of data
access.

### 2.4 Builder functionality

The first release supports:

- Table and pivot-style output.
- Dimension/measure selection, grouping, sorting, subtotals, and grand totals.
- Date, period, enum, boolean, text, numeric range, entity, company, and branch filters.
- Safe arithmetic, conditional labels, date bucketing, percentage, and variance
  calculations.
- Saved personal drafts and governed shared reports.
- Preview with sampled/bounded results and visible freshness timestamp.
- CSV and XLSX export; PDF for layouts that pass page-size validation.
- Asynchronous export for results above the interactive threshold.

Cross-dataset ad hoc joins, nested subreports, write-back, and arbitrary pixel-perfect
layout are deferred.

### 2.5 Scheduling and distribution

Extend existing report scheduling from fixed report types to published report-version
references while preserving current P&L and budget schedules.

- Resolve schedules using company timezone and fiscal period, not server-local time.
- Freeze parameters, locale, currency mode, recipients, and report version per run.
- Check the schedule owner and each internal recipient's current access before every
  delivery.
- External recipients require an allow-list, explicit distribution permission, and
  optional approval for sensitive classifications.
- Produce one idempotent delivery per schedule/run/recipient/format.
- Record queued, delivered, failed, suppressed, and expired outcomes.
- Store generated files through the planned document-management/object-storage
  boundary and apply report-class retention.
- Disable schedules whose owner, dataset, report, or recipient policy is no longer
  valid, and notify the owner.

## 3. Configurable dashboard widgets

### 3.1 Dashboard model

Add:

- Company dashboard templates and user dashboards.
- Versioned responsive layouts.
- Widgets referencing published report versions or certified KPI definitions.
- Widget type, title, visualization, filters, refresh policy, and drill-down target.
- Role defaults, user overrides, visibility, and sharing grants.
- Cache manifests and refresh/run history.

Supported widget types are KPI card, table, bar, line, area, and aging/status summary.
Reuse existing server-rendered analytics/SVG patterns where practical. Widgets cannot
embed arbitrary HTML, scripts, iframes, URLs, or SQL.

Dashboard lifecycle:

`DRAFT → PUBLISHED → SUPERSEDED → RETIRED`

Users may personalize layout and allowed filters without changing the published report
or company template.

### 3.2 Widget security and cache policy

- Authorize every widget and drill-down at request time.
- Intersect report sharing with the user's effective company/branch/manager scope.
- Include company, user/row scope, permission fingerprint, report/dataset version,
  locale, timezone, currency, fiscal period, and filters in the cache key.
- Never reuse a broader-scope cache result for a narrower user.
- Apply short TTLs to operational data and explicit refresh timestamps.
- Invalidate or version-bust caches after permission, dataset, role, policy, or
  published-definition changes.
- Return a permission-safe unavailable state rather than leaking widget titles, counts,
  or filter values.

## 4. Operational report coverage

Certify reports in waves. Each report needs a route, dataset version, owner, grain,
filters, permissions, exports, reconciliation rule, performance budget, and catalog
entry before being advertised.

### Wave 1: current transactional coverage

| Module | Reports |
|---|---|
| Sales/AR | Sales by customer/product, monthly revenue, order backlog, fulfillment status, returns, margin lineage |
| Procurement/AP | PR-to-PO lead time, PO commitments, receipt/invoice variance, spend by supplier/product, overdue receipts |
| Inventory/WMS | Stock valuation, inventory aging, dead/fast-moving stock, turnover, stockout, replenishment, lot/serial expiry, pick performance |
| Delivery | On-time shipment/delivery, partial fulfillment, backlog, return rate, carrier/reference completeness |
| MRP | Schedule adherence, WIP value, work-center utilization, yield/scrap, inspection/hold/NCR status |
| Projects | Task/timesheet status and labor usage from current foundations |
| POS | Sales by terminal/cashier/product, session variance, payment/refund/void activity |

### Wave 2: planned-module coverage

Publish these reports only after their source modules are implemented:

- Project milestone, Gantt, Kanban, budget, commitment, forecast, and margin reports.
- POS loyalty, gift-card liability, and hardware health reports.
- Logistics carrier, fleet, route, freight, dispatch, and distribution reports.
- CMMS availability, downtime, MTBF, MTTR, maintenance cost, and compliance reports.
- QMS inspection, hold, NCR, CAPA, audit, supplier quality, and training evidence.
- CRM segmentation, consent, campaign delivery, conversion, and attribution.

### Reconciliation requirements

- Financial measures reconcile to posted GL or a documented operational subledger.
- Inventory quantity/value reconciles to authoritative movements and costing method.
- Counts expose included/excluded lifecycle states.
- Currency reports preserve original values/rates and state translation currency/rate.
- Period comparisons use fiscal sequence rather than calendar-month string arithmetic.
- Snapshots show data-as-of and refresh timestamps.
- Report totals and exported totals use the same exact calculation path.

## 5. HR report coverage and privacy

### 5.1 Initial HR reports

- Headcount by effective company, department, position, manager, and status.
- New hires, inactive employees, and tenure bands.
- Attendance detail/summary, absence, lateness where captured, and import-quality
  exceptions.
- Leave request status, usage, balance, pending reservation, and expiry where policy
  supports it.
- Payroll run summary, variance, exception, liability, payment, and payslip-delivery
  status.
- Organization hierarchy and manager span.

### 5.2 Reports enabled by planned HR depth

- Recruitment funnel, requisition aging, time-to-fill, source effectiveness, interview
  score completion, and offer acceptance.
- Performance-cycle completion, goal progress, rating distribution, calibration
  movement, and acknowledgement.
- Required-training compliance, attendance, completion, failure/waiver, certificate
  expiry, and renewal.

### 5.3 HR row and field security

- `view_own` sees the employee's own records.
- Manager scope follows an effective-dated employee hierarchy and only approved report
  fields.
- HR operational scope sees company HR data excluding restricted payroll/medical or
  confidential-review fields unless separately granted.
- Payroll scope remains separate from general HR reporting.
- Recruitment candidate PII, performance peer identity, compensation, and protected
  characteristics use distinct classifications and permissions.
- Small-group suppression prevents dashboards/exports from revealing sensitive
  aggregates below a configurable threshold.
- Export, scheduled distribution, and access to sensitive HR fields are audited.

## 6. Reporting permissions

Introduce:

- `report.catalog.view`
- `report.run`
- `report.definition.create`, `report.definition.edit`, and
  `report.definition.publish`
- `report.share`
- `report.export`
- `report.schedule.manage`
- `report.distribute.external`
- `report.dataset.view` and `report.dataset.admin`
- `dashboard.view`, `dashboard.personalize`, and `dashboard.publish`
- `report.sensitive.hr`, `report.sensitive.payroll`, and other classification-specific
  permissions where needed

Existing permissions such as finance analytics/export and board-pack access remain
valid. Dataset permissions are additive; a generic report permission never grants
underlying finance, payroll, HR, quality, document, or customer data.

## 7. Fuller role matrix for newer modules

### 7.1 Scoped role model

Retain the global permission catalog, but scope operational role assignments
explicitly to company and optionally branch. Add:

- Versioned system role templates.
- Company roles derived from a template or created explicitly.
- Effective-dated user-role assignments with company and optional branch scope.
- Role-permission snapshots, change reason, actor, and approval reference.
- High-risk permission classification and separation-of-duties policies.
- Preview of added/removed effective permissions before activation.
- Permission-coverage inventory mapping routes/actions to required permissions.

Migrate current user-role links into explicit assignments without removing existing
access. Compatibility reads remain until every middleware lookup includes active
company scope. Do not auto-assign new non-administrator roles during migration.

Role lifecycle:

`DRAFT → ACTIVE → RETIRED`

Published system templates are suggestions, not implicit grants. Company role changes
are versioned; high-risk changes may require approval.

### 7.2 Recommended role templates

| Role template | Primary permission boundary | Critical exclusion |
|---|---|---|
| Company Administrator | Company configuration, users, roles | Cannot bypass runtime separation-of-duties policy |
| Read-only Auditor | Reports, audit, approved records | No create, transition, adjustment, or export of restricted data without grant |
| Finance Controller | GL, close, reporting, accounting policy | No payment proposal/execution combination |
| Treasury Operator | Bank feeds, cash forecast, payment execution | Cannot approve own proposal |
| AP/AR Specialist | Payables or receivables operations | No period lock or unrelated treasury execution |
| Procurement Buyer | PR/RFQ/bid/PO preparation | Cannot approve own award or contract exception |
| Procurement Approver | Award, contract, purchasing exceptions | No bid entry alteration after submission |
| Warehouse Operator | Pick, receipt, movement execution | No valuation/accounting policy administration |
| Inventory Manager | Inventory controls, stocktake approval, reporting | No unrelated finance administration |
| Logistics Planner | Carrier/fleet/route/freight planning | No dispatch confirmation or own variance approval |
| Dispatcher | Dispatch and delivery events | No freight policy/rate administration |
| Manufacturing Planner | MRP, capacity, work-order planning | No quality disposition approval |
| Manufacturing Operator | Production execution | No BOM or controlled-decision approval |
| Manufacturing Quality Approver | Inspection/hold/NCR disposition | No production execution override |
| Project Manager | Project plan, board, budget preparation, member scope | Cannot approve own budget/variance when policy requires separation |
| Project Member | Assigned tasks and own timesheets | No project budget or member administration |
| POS Manager | Terminals, sessions, controlled POS adjustments | No unrestricted gift-card/loyalty value change without approval |
| POS Cashier | Assigned terminal/session, tickets, permitted tender | No program, hardware, or manual value administration |
| HR Administrator | Employee, recruitment, training operations | No payroll or confidential performance access unless separately granted |
| People Manager | Direct/indirect report workflows | No unrelated employee/candidate records |
| Payroll Operator/Approver | Payroll preparation or approval | Operator and final approver separated |
| Performance Administrator | Cycle/template/calibration administration | No payroll change from rating |
| CRM Sales Representative | Owned leads/opportunities/activities | No segment export or campaign launch |
| CRM/Marketing Manager | Team CRM, consent, segments, campaigns | No transactional notification-policy override |
| Report Author | Personal/shared report drafting | No publish, sensitive field, or external distribution by default |
| Report Publisher | Dataset validation and report/dashboard publication | Cannot grant underlying data access |
| Integration Administrator | API keys, webhooks, connector configuration | No implicit access to business records |
| Document/QMS/CMMS Administrators | Future module policy/configuration | Activated only when the module is implemented |

Document every template as permission families and explicit grants in the RBAC
reference. Avoid wildcard permissions and role-name checks in business services.

### 7.3 RBAC administration controls

- Route/action inventory detects unprotected mutations and unknown permissions in CI.
- Role matrix shows inherited/effective permissions, scope, risk, and conflicts.
- Bulk assignment validates active employment, company membership, and branch scope.
- Temporary assignments require start/end timestamps and expire automatically.
- Removing the last company administrator requires a guarded replacement workflow.
- Self-escalation and approval of one's own high-risk grant are prohibited.
- Permission/role changes invalidate sessions or effective-permission caches promptly.
- Quarterly access review exports assignments, usage, conflicts, dormant accounts, and
  attestation status.

## 8. Language and locale policy

### 8.1 Resolution order

Use BCP 47 locale identifiers. Initially support `id-ID` and `en-US`, with compatibility
mapping from existing `id` and `en` values.

Resolve display locale in this order:

1. Active user's supported locale preference.
2. Active company's default locale.
3. System fallback `id-ID`.

The company policy defines supported locales and fallback. A user cannot select an
unsupported arbitrary locale.

### 8.2 Locale behavior

- Translation catalogs use stable keys and are shared consistently by server-rendered
  pages, client scripts, emails, reports, and workers.
- Templates render one active language, not bilingual copy.
- Store codes, identifiers, money, quantities, dates, timestamps, and audit facts in
  canonical forms; locale affects presentation only.
- Format dates, numbers, percentages, and currency using the resolved locale while
  preserving ISO currency code where ambiguity is possible.
- Generated reports capture locale and translation-catalog version.
- Master-data descriptions may add explicit translated values later; do not machine-
  translate authoritative names during reads.
- Missing translation keys fall back to company fallback then `id-ID`, and emit a
  bounded metric rather than exposing a raw key silently.

## 9. Timezone policy

Store IANA timezone identifiers on the company and optional user preference. Validate
identifiers against the runtime timezone database.

Use these semantics:

| Value | Timezone rule |
|---|---|
| Event/audit timestamp | Store as UTC `TIMESTAMPTZ`; display in user timezone, then company fallback |
| Company operational deadline | Interpret and schedule in company timezone |
| User reminder | Schedule in user timezone when explicitly user-owned |
| Accounting/document date | Store as `DATE`; never shift it during display |
| Fiscal-period boundary | Company accounting date/calendar, not viewer timezone |
| Provider timestamp | Preserve provider timestamp/offset and normalized UTC instant |

Resolution for event display is user timezone, company timezone, then UTC. Accounting
and operational cutoff logic always uses the company timezone even if the viewer uses
another timezone.

- Jobs compute the next local occurrence with explicit daylight-saving behavior.
- Ambiguous/nonexistent local times use a documented earlier/later/skip policy and
  record the resolved instant.
- Report runs capture the timezone used for date buckets and labels.
- Changing company timezone is versioned, audited, and affects future scheduling only;
  historical instants and report evidence are not rewritten.

## 10. Fiscal-year and period policy

### 10.1 Calendar model

Add company-scoped fiscal calendar versions, fiscal years, periods, and optional
adjustment periods. Support:

- Calendar-month years.
- Month-based years starting in a configured month.
- 4-4-5, 4-5-4, and 5-4-4 week patterns.
- Explicit custom periods.
- 12, 13, 52/53-week, and approved adjustment-period structures.

Each period has company, fiscal calendar/version, fiscal year, sequence, stable code,
start/end date, type, status, and close lineage. Enforce no overlap per company/calendar
version and chronological sequence.

Fiscal calendar versions use:

`DRAFT → VALIDATED → APPROVED → ACTIVE → RETIRED`

Fiscal years use `PLANNED → OPEN → SOFT_CLOSED → HARD_CLOSED`. Periods retain
the documented open/soft-close/hard-close controls.

### 10.2 Policy rules

- Only one active fiscal calendar version applies to a company/date.
- Activating a version generates periods idempotently and requires approval.
- A period containing posted journals cannot be deleted, resized, reordered, or moved
  to another fiscal year.
- Future unused periods may be regenerated only through an audited approved change.
- Comparative reports resolve prior periods by fiscal sequence, not calendar month.
- Tax/payroll/project calendars may reference accounting periods but keep their own
  legally required lifecycle where already modeled.
- Company fiscal policy cannot be overridden by a user preference.

### 10.3 Legacy-period migration

Current code uses both legacy global `periods` rows and company-aware
`accounting_periods`. Migrate safely:

1. Inventory every period resolver and direct period query.
2. Create a fiscal calendar/version and fiscal-year mapping for each company.
3. Map company-specific accounting periods first; resolve company-neutral rows using
   an explicit migration decision and reconciliation report.
4. Add fiscal year/sequence/type references without changing existing journal IDs.
5. Introduce one company-scoped Period Resolver service for posting and reporting.
6. Move callers from global period lookup to the resolver behind a feature flag.
7. Compare posting eligibility, close status, report totals, tax, payroll, depreciation,
   revaluation, consolidation, and board-pack behavior.
8. Block new legacy global lookups in CI after all companies pass reconciliation.
9. Retain compatibility tables/views until rollback and audit retention gates expire.

Do not perform a one-step replacement of legacy period IDs because journals and
multiple subledgers already reference them.

## 11. Administration permissions and interfaces

Introduce:

- `admin.company_policy.view` and `admin.company_policy.manage`
- `admin.locale.manage`, `admin.timezone.manage`, and `admin.fiscal_calendar.manage`
- `admin.role.view`, `admin.role.manage`, and `admin.role.approve`
- `admin.access_review.view` and `admin.access_review.attest`

Add administration workspaces under:

- `/reports/catalog`, `/reports/builder`, `/reports/runs`, and `/reports/schedules`.
- `/dashboards` and `/admin/dashboard-templates`.
- `/admin/roles`, `/admin/role-matrix`, and `/admin/access-reviews`.
- `/admin/company-policy`, `/admin/locales`, `/admin/timezone`, and
  `/admin/fiscal-calendars`.

All mutation routes use CSRF protection, service-layer authorization, company scope,
optimistic versions, safe errors, and audit events.

## 12. Delivery sequence

| Phase | Deliverable | Indicative duration |
|---|---|---:|
| 0 | Reporting/RBAC/fiscal ADRs, inventories, classifications, migration fixtures | 2–3 weeks |
| 1 | Dataset catalog, safe compiler, run metadata, quotas, and first certified datasets | 5–6 weeks |
| 2 | Report builder, versioning, sharing, export, scheduling, and delivery evidence | 5–6 weeks |
| 3 | Dashboard templates, widgets, scoped cache, drill-down, and personalization | 4–5 weeks |
| 4 | Wave 1 operational reports and source reconciliations | 5–7 weeks |
| 5 | Initial HR reports, privacy classifications, manager scope, and suppression | 4–5 weeks |
| 6 | Scoped role assignments, newer-module templates, SoD, and access reviews | 5–6 weeks |
| 7 | Locale catalog, company/user resolution, and timezone-safe jobs/reports | 4–5 weeks |
| 8 | Fiscal calendar/version model and legacy-period caller migration | 6–8 weeks |
| 9 | Hardening, performance, recovery, staging certification, and rollout | 3–4 weeks |

Reporting datasets, role templates, locale/timezone, and fiscal migration can proceed
as separate streams after Phase 0. Fiscal caller migration requires a dedicated
release gate because it affects posting correctness.

## 13. Rollout

1. Inventory reports, fields, permissions, roles, routes, time calculations, jobs, and
   all legacy period callers.
2. Add schemas and services without enabling builder/admin navigation.
3. Register current reports as datasets and reconcile their existing output.
4. Pilot the builder with read-only finance/operational authors and bounded datasets.
5. Pilot widgets with one company and verify cache/permission isolation.
6. Publish operational and HR reports only after business-owner reconciliation.
7. Generate role-matrix diffs without changing assignments; remediate conflicts
   explicitly.
8. Activate locale/timezone policies before moving schedules to company-local time.
9. Migrate fiscal resolution per company and dual-verify all posting/reporting paths.
10. Obtain reporting owner, Finance, HR/privacy, security, operations, and company
    administrator sign-off before each applicable capability is enabled.

## 14. Mandatory validation

### Reporting and widgets

- Dataset certification, version dependency, retirement, and owner tests.
- Expression/field/join allow-list and raw-SQL/script rejection tests.
- Tenant, branch, ownership, manager, sensitive-field, and small-group suppression
  tests.
- Query cost, row, lookback, timeout, cancellation, and concurrent-run tests.
- Exact numeric total, CSV/XLSX/PDF round-trip, locale, timezone, and fiscal-period
  tests.
- Schedule owner/recipient reauthorization, external distribution approval,
  idempotency, retry, and retention tests.
- Widget cache-key, permission downgrade, scope isolation, invalidation, unavailable,
  and drill-down tests.
- Source-to-report and screen-to-export reconciliation tests.

### RBAC

- Route/action permission coverage and unknown/orphan permission tests.
- Company/branch scope and compatibility assignment migration tests.
- Self-escalation, last-admin, temporary expiry, high-risk approval, and SoD tests.
- Permission-cache/session invalidation and access-review evidence tests.
- Matrix fixtures for every newer implemented/planned module permission family.

### Locale and timezone

- Locale resolution/fallback and missing-key tests across SSR, email, worker, report,
  and export output.
- Canonical value storage and language-switch tests.
- IANA validation, user/company fallback, DST gap/overlap, cutoff, reminder, and
  schedule tests.
- Accounting `DATE` invariance across viewer timezones.

### Fiscal calendar

- Calendar-month, shifted-year, 4-4-5, 53-week, custom, and adjustment-period fixtures.
- Overlap, gap, sequence, activation, regeneration, and immutable-posted-period tests.
- Company-specific posting and close concurrency tests.
- Tax, payroll, fixed assets, FX, consolidation, project, scheduled report, and board-
  pack regression tests.
- Legacy/new resolver reconciliation and rollback rehearsal.

Run full verification with `go test ./...`, `go vet ./...`, `make lint`, and
`make docs-check`.

## 15. Exit criteria

- Custom reports compile only from published governed datasets and cannot bypass row
  or field permissions.
- Shared reports, exports, schedules, and widgets are versioned, auditable, bounded,
  and permission-safe.
- Wave 1 operational and initial HR reports have owners, reconciliation evidence,
  filters, exports, and performance budgets in the reporting catalog.
- Newer-module roles have documented templates, scopes, SoD conflicts, and explicit
  assignments without unintended privilege expansion.
- UI, email, worker, report, and export surfaces use one locale resolution policy.
- Operational scheduling and event display follow documented IANA timezone rules;
  accounting dates do not shift by viewer timezone.
- Every company has an approved fiscal calendar/version and company-scoped resolver.
- No production posting path depends on an unscoped global period lookup.
- Each capability can be enabled per company after staging sign-off.

## 16. Deferred scope

- Arbitrary SQL, unrestricted joins, write-back reports, or executable scripts.
- Full data lake/warehouse, OLAP cube, and external BI replacement.
- Pixel-perfect report designer and arbitrary dashboard plugins.
- Natural-language-to-SQL report generation.
- Predictive/AI analytics and automated business decisions.
- Machine translation of authoritative business records.
- Jurisdiction-specific HR analytics certification.

## Assumptions

- PostgreSQL remains the initial reporting source; a read replica may be introduced
  without changing dataset contracts.
- `id-ID` and `en-US` are the first supported locales.
- Company timezone governs operational cutoffs and schedules; user timezone governs
  event display and explicitly personal reminders.
- Accounting fiscal policy is company-owned and cannot be overridden per user.
- Existing reports remain available while they are registered and reconciled into the
  governed catalog.
