# Odyssey ERP Version and Progress Report

**Reviewed:** 2026-08-10

## How to read the version numbers

`v0.9.1` is the latest named production release in the repository. It is primarily
a UI/UX release dated 2026-05-28. `v0.10.0-rc.2` is the current release candidate
for the post-v0.9.1 platform work; it is not production-certified.

In other words:

- **Latest named production release:** v0.9.1.
- **Current release candidate:** v0.10.0-rc.2 (2026-08-10).
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
| v0.10.0-rc.2 | 2026-08-10 | Coretax and PPh 21 release-test completion | Fail-closed Coretax transport/validation, export-to-GL contract evidence, and annual last-tax-period PPh 21 reconciliation from a PMK 168/2023 fixture | Current release candidate; official tax, staging, provider, and operational certification remain open |

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

## Follow-up work after v0.10.0-rc.2

This work is documented in the current [roadmap](../ROADMAP.md) and [module catalog](../reference/module-catalog.md); the candidate packages the current scope, while final promotion remains pending.

### Finance and operations

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

- Promote `v0.10.0` only after the candidate passes staging, provider, security,
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
