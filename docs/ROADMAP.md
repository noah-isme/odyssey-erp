# Odyssey ERP — Future Roadmap & Recommendations

**Prepared:** 2026-01-11
**Revised:** 2026-07-20 (synced to current implementation)
**Revised:** 2026-07-29 (added the gap-analysis phased plan below)
**Revised:** 2026-07-30 (Phase 5 tax compliance implementation and release gate)
**Revised:** 2026-08-01 (Phase 14 transaction-level FX implementation)
**Revised:** 2026-08-01 (Phase 14 and P7 local acceptance gates)
**Revised:** 2026-08-02 (manufacturing/MRP execution, planning, quality, analytics, and compliance foundations)
**Revised:** 2026-08-02 (external integrations implementation plan)
**Revised:** 2026-08-02 (linked execution plans for remaining module depth and administration)
**Revised:** 2026-08-09 (repository persistence boundaries and shared HTTP error policy)
**Revised:** 2026-08-09 (document processing and CMMS telemetry foundations)
**Revised:** 2026-08-09 (MRP canonical compliance snapshots and reauthentication hardening)
**Revised:** 2026-08-10 (v0.10.0-rc.3 release candidate packaging)
**Current Version:** v0.10.0-rc.3

> For current release status, use the [Authoritative Feature Matrix](reference/feature-matrix.md).
> The [Module Catalog](reference/module-catalog.md) is the capability inventory. This
> roadmap tracks sequencing and release gates; it is not a second feature-status
> authority.

## Executive Summary

Odyssey ERP has moved well beyond Phase 9. Since this roadmap was first drafted,
**Phase 10 (Accounts Payable)** and **Phase 11 (Bank & Cash Management)** have been
built and are user-facing, and **Phase 12 (Inventory Enhancements)** is largely
complete. This document has been revised to reflect what is actually implemented and
to re-prioritise the genuinely remaining work.

## Linked execution plans

The following guides are the implementation plans for the remaining depth and control
work. They contain ownership boundaries, lifecycles, permissions, data migrations,
integration contracts, rollout gates, and acceptance criteria. The
[`Feature Matrix`](reference/feature-matrix.md) remains authoritative for current
release status, while the [`Module Catalog`](reference/module-catalog.md) provides
capability navigation.

| Plan | Scope |
|---|---|
| [`Core Finance Automation Plan`](guides/core-finance-automation-plan.md) | Bank feeds, cash forecasting, payment execution, purchase-to-pay automation, and fixed-asset operations |
| [`External Integrations Plan`](guides/external-integrations-plan.md) | Shared connector foundation, payments, carriers, marketplaces, messaging, BI, identity, and governed AI integrations |
| [`Procurement and Logistics Depth Plan`](guides/procurement-logistics-depth-plan.md) | RFQ/bids/awards, supplier contracts/ratings/price history, carriers, fleet, routes, freight, and distribution planning |
| [`Manufacturing Governance Plan`](guides/manufacturing-governance-plan.md) | Mandatory controlled-record enforcement, manufacturing quality boundaries, and staging certification |
| [`CMMS, QMS, and Document Management Plan`](guides/cmms.md) | Operational maintenance, standalone QMS migration, managed storage, versions, signatures, retention, and document permissions |
| [`Product Workflow Depth Plan`](guides/product-workflow-depth-plan.md) | Project milestones/Gantt/Kanban/budgets, POS loyalty/gift cards/hardware, HR talent workflows, and CRM campaigns/segmentation |
| [`Reporting and Administration Depth Plan`](guides/reporting-administration-depth-plan.md) | Governed report builder/widgets, operational and HR coverage, role matrix, locale, timezone, and fiscal-calendar policy |

---

## Implementation Status (verified against code, 2026-08-01)

| Area | Status | Evidence |
|------|--------|----------|
| Phase 10 — Accounts Payable | ✅ Implemented | `internal/ap/`, migrations 000023/000025, route `/finance/ap` |
| Phase 11 — Bank accounts & transactions | ✅ Implemented | `internal/finance/banking/`, migration 000026, route `/finance/banking` |
| Phase 11 — Bank reconciliation | ✅ Implemented | `internal/accounting/banks/`, migration 000030, route `/accounting/banks` |
| Phase 11 — Cash flow report | ✅ Implemented | `internal/accounting/reports/cf.go` |
| Phase 12 — Stock take & adjustment | ✅ Implemented | migrations 000027/000028, `/inventory/.../stock-takes`, `/adjustments` |
| Phase 12 — Stock valuation | 🟡 Partial | Per-product AVG/FIFO cost method, lot/serial receiving, and reorder PR are implemented; LIFO is intentionally excluded |
| Phase 15 — Budget vs Actual | ✅ Implemented | `/accounting/budget` loads `accounting_budgets` and posted journal actuals for the selected month |
| Phase 13 — Fixed Assets | ✅ Implemented | Register, straight-line depreciation worker, disposal accounting, and category setup |
| Phase 14 — Transaction-level multi-currency | 🟡 Locally certified; staging/production verification pending | `internal/fx/`, AR/AP valuation, realized FX, revaluation/reversal, migrations `000053/000054`; local acceptance evidence in `docs/guides/phase14-p7-acceptance-evidence.md` |
| P7 — Multi-Currency and Horizon MVP foundation | 🟡 Locally certified; staging/production verification pending | WMS, MRP, POS, projects/timesheets, API/webhooks, portals; migrations `000055/000056/000060`; local acceptance evidence in `docs/guides/phase14-p7-acceptance-evidence.md` |
| Manufacturing / MRP expansion | 🟡 Locally verified; downstream staging and regulated-policy validation pending | Approved BOM revisions, planning/firming, WIP cost transfer, finite-capacity scheduling, exceptions, quality/genealogy, analytics, canonical immutable compliance snapshots, role checks, real reauthentication, and audit evidence; migrations `000062`–`000075`, `000118`; [`manufacturing-mrp.md`](guides/manufacturing-mrp.md) |
| Phase 15 — Reporting enhancements | 🟡 Partial | P&L and Budget vs Actual support department/cost-center filters, native `.xlsx`, and scheduled email; report builder/widgets remain |
| Connector foundation (Phase 0) | ✅ Implemented | `internal/connectors/` — `ProviderAdapter` interface, vault-encrypted `SecretRef`, transactional outbox/inbox, deduplication, canonical event routing, `/settings/integrations` UI; migrations `000076`+ |
| Payment gateway — Midtrans (Phase A1) | ✅ Implemented | Snap checkout, SHA-512 webhook signature verification, `payment.captured/authorized/failed` canonical events, automatic AR invoice allocation; `internal/connectors/providers/midtrans/`; 17-test suite |
| Payment gateway — Stripe (Phase A1) | ✅ Implemented | Vault-resolved live API calls, webhook verification, stable charge idempotency keys, and full AR webhook allocation; `internal/connectors/providers/stripe/` |
| Freight charge workbench | 🟡 Partial | Rate cards, surcharges, freight charge calculation, landed costs, cost centers, GL posting; `internal/freight/`; 5-test suite with mock repository |
| Logistics UI (fleet/trip/dispatch) | 🟡 Partial | Fleet, vehicle, driver, trip, and cargo management screens implemented; `internal/logistics/`; rate cards and freight charges UI linked from sidebar |
| Distribution planning and load execution | 🟡 Partial | `/distribution` planning horizons, loads, shipment linkage, dispatch/delivery, manual routes, and transfer orders; transfer inventory accounting, route optimization, freight execution, and workbenches remain |

The phase descriptions below are retained for reference. **Completed phases (10, 11, and
most of 12) are kept for historical context; focus new work on the "Remaining Priorities"
section near the end.**

---

## Phase 10: Accounts Payable (AP) — ✅ DONE

**Status:** Implemented (`internal/ap/`, route `/finance/ap`).
**Priority:** ~~🔴 High~~
**Estimated Effort:** ~~3-4 weeks~~

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| AP Invoice | Create invoices from GRN/PO | High |
| AP Invoice Lines | Line items with product/service details | High |
| AP Payment | Record vendor payments | High |
| Payment Allocation | Allocate payments across invoices | Medium |
| AP Aging Report | Outstanding payables by vendor | High |
| Vendor Statement | Statement reconciliation | Medium |

### Technical Notes
- Mirror AR structure (`ap_invoices`, `ap_invoice_lines`, `ap_payment_allocations`)
- Link to `suppliers` and `purchase_orders`
- Add `finance.ap.*` permissions

---

## Phase 11: Bank & Cash Management — ✅ DONE (auto bank feed pending)

**Status:** Bank accounts, transactions, transfers, reconciliation, cash flow reporting,
manual CSV/OFX statement import, and the provider-neutral bank-feed connection/event
consumer are implemented (`internal/finance/banking/`, `internal/accounting/banks/`,
`internal/finance/bankfeeds/`). Provider adapters and sandbox certification remain
outstanding.
**Priority:** ~~🔴 High~~
**Estimated Effort:** ~~2-3 weeks~~

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Bank Accounts | Multiple bank account management | High |
| Bank Transactions | Record deposits, withdrawals, transfers | High |
| Bank Reconciliation | Match transactions with bank statement | High |
| Cash Flow Report | Actual cash flow from transactions | Medium |
| Scheduled Bank Feed | Provider-backed incremental sync and verified event ingestion | Medium |

### Technical Notes
- New entity `bank_accounts` with `company_id`
- `bank_transactions` with type (deposit/withdrawal/transfer)
- Reconciliation status tracking

---

## Phase 12: Inventory Enhancements — 🟡 MOSTLY DONE

**Status:** Stock take, stock adjustment (with audit trail), and valuation (Average + FIFO)
are implemented. Per-product costing, stock reorder
automation, batch/lot tracking, and serial numbers.
**Priority:** 🟡 Medium
**Estimated Effort:** remaining items ~1-2 weeks

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Stock Valuation | FIFO/LIFO/Average costing methods | High |
| Stock Reorder | Auto-generate PO when below min | Medium |
| Batch/Lot Tracking | Track items by batch number | Medium |
| Serial Number | Track individual items | Low |
| Stock Take | Physical inventory count | High |
| Stock Adjustment | Adjust stock with audit trail | High |

### Technical Notes
- Add `costing_method` to products
- `inventory_lots` for batch tracking
- `stock_takes` and `stock_take_lines`

---

## Phase 13: Fixed Assets — ✅ DONE

**Status:** Register, category account configuration, monthly straight-line
depreciation, disposal accounting, and worker scheduling are implemented.

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Asset Register | List of company assets | High |
| Depreciation | Auto-calculate depreciation | High |
| Asset Categories | Group assets by type | Medium |
| Asset Disposal | Record sale/disposal | Medium |
| Asset Transfer | Transfer between branches | Low |

### Technical Notes
- `fixed_assets` with depreciation method, useful life
- Monthly depreciation job
- Journal entries for depreciation expense

---

## Phase 14: Multi-Currency Enhancement

**Priority:** 🟡 Medium  
**Status:** 🟡 Locally certified; staging and production deployment verification pending
**Migration:** `000053_transaction_fx`

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Auto FX Rate | Fetch daily rates from configurable provider | ✅ Implemented |
| Realized Gain/Loss | Calculate on AR/AP payment allocations | ✅ Implemented |
| Unrealized Gain/Loss | Revalue outstanding invoices | ✅ Implemented |
| Currency Revaluation | Month-end job with reversal model | ✅ Implemented |

### Technical Notes
- Consolidation continues using monthly `fx_rates`; transaction FX uses `fx_daily_rates`.
- AR/AP retain original values and locked invoice/payment rates as PostgreSQL `NUMERIC`.
- `odyssey fx fetch` and `odyssey fx status` support operations; the worker fetches in `Asia/Jakarta`.
- Focused unit and package validation is complete:
  `go test ./internal/ar ./internal/ap ./internal/fx ./internal/accounting/journals ./internal/integration`,
  `go test ./cmd/odyssey ./cmd/worker ./cmd/odyssey/cli`,
  and `go test ./jobs -run 'TestFXDailyRates'`.

### Phase 14 release gate

The FX implementation is complete, but Phase 14 is not production-certified until the
following acceptance work is recorded:

- [x] Add and pass a database-backed USD AR end-to-end test.
- [x] Add and pass a database-backed USD AP end-to-end test.
- [x] Run the complete local integration suite with the clean schema through migration `000061`.
- [ ] Execute migration `000053_transaction_fx` on staging and verify the four FX account mappings.
- [ ] Execute and verify the production migration and account mappings.
- [ ] Confirm the staging and production workers are deployed and the daily FX job is running.
- [ ] Run post-migration smoke tests for invoice posting, partial payment, revaluation, and reversal.

Local gate evidence, including FX audit/error behavior and CLI secret redaction, is recorded
in `docs/guides/phase14-p7-acceptance-evidence.md`. A provider fetch without a configured
ExchangeRate API key is expected to fail safely and record `fx_fetch_runs`; this does not
constitute a staging provider smoke test.

After deployment, verify rate fetch audit rows, journal idempotency, and the AR/AP
valuation fields before closing this gate. This is a deployment and database-backed
acceptance track; it does not represent unfinished core FX functionality.

---

## Phase 15: Reporting & Analytics — 🟡 PARTIAL

**Priority:** 🟡 Medium
**Estimated Effort:** 3-4 weeks

> **Status note:** Analytics and Insights modules exist. Budget vs Actual reads
> `accounting_budgets` and posted journal actuals for the selected month. P&L and
> Budget vs Actual support department/cost-center filters, native Excel exports,
> and scheduled email delivery. Report builder and dashboard widgets are not started.

### Features
| Feature | Description | Priority | Status |
|---------|-------------|----------|--------|
| Custom Reports | Build reports with drag-drop | Low | Not started |
| Dashboard Widgets | Customizable dashboard | Medium | Not started |
| Budget vs Actual | Compare to budget | High | Implemented with posted journal actuals |
| Department Reporting | P&L by department/cost center | Medium | P&L and Budget vs Actual filters implemented |
| Export to Excel | Native Excel export | High | P&L and Budget vs Actual `.xlsx` implemented |
| Scheduled Reports | Email reports on schedule | Medium | Hourly worker scan and email queue implemented for P&L and Budget vs Actual |

### Technical Notes
- Consider Go templating or external BI tool
- `accounting_budgets` table already exists (migration 000029) — wire the handler to it
- Add `department_id` to transactions

---

## Phase 16: Audit & Compliance — 🟡 PARTIAL

**Priority:** 🟢 Low  
**Estimated Effort:** 2 weeks

> The audit timeline, protected exports, immutable tax/audit records, and RBAC
> controls already exist. The remaining items below are hardening and general-purpose
> document-management work; they are not a claim that audit logging is absent.

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Audit Log Viewer | UI for viewing audit logs | High |
| Data Export | Export all data for audit | Medium |
| Document Attachment | Attach files to transactions | Medium |
| E-Signature | Digital approval signatures | Low |
| Change History | View change history per record | Medium |

---

## Phase 17: Integration — 🟡 PARTIAL

**Priority:** 🟢 Low  
**Estimated Effort:** Varies

> Public API, webhooks, SMTP, PDF/Gotenberg, CSV/OFX statement import, portals, and
> Coretax export are implemented or locally certified. The candidates below are
> additional external connectors, not the complete integration surface.

### Potential Integrations
| Integration | Description | Effort |
|-------------|-------------|--------|
| E-Faktur | Indonesian tax invoice | 2 weeks |
| Payment Gateway | Online payment (Midtrans, etc) | 2 weeks |
| E-Commerce | Tokopedia, Shopee integration | 3 weeks |
| Shipping | JNE, J&T, SiCepat API | 2 weeks |
| WhatsApp | Invoice delivery via WA | 1 week |

---

## Quick Wins (Low Effort, High Impact)

These can be implemented in 1-2 days each:

1. **PDF Invoice Email** — Send invoice PDF via email
2. **Duplicate Invoice Check** — Prevent duplicate PO numbers
3. **Dashboard KPIs** — AR/AP totals on dashboard
4. **Keyboard Shortcuts** — Quick navigation
5. **Bulk Actions** — Multi-select for status updates
6. **Search Everywhere** — Global search bar
7. **Recent Activity** — User activity feed
8. **Export to CSV** — All list views exportable

---

## Technical Debt & Improvements

> Checklist reviewed against code on 2026-07-14. Items marked ✅ are verified resolved.

### Code Quality
- [x] ✅ Fix template embedding issue — templates now embedded via `go:embed` (`web/embed.go`, `internal/view/templates.go`)
- [x] ✅ Add comprehensive unit tests for AR module — `internal/ar/service_test.go` exists
- [ ] Add integration tests for AR workflows — `internal/integration/` dir exists but is **empty**
- [x] ✅ Refactor handler error responses to use shared status classification and safe response helpers (`internal/shared/http.go`)

### Performance
- [ ] Add database indexes for AR queries
- [ ] Cache frequently accessed data (company settings)
- [ ] Optimize aging report query

### Security
- [ ] Add rate limiting on login — a global limiter exists (`httprate`), but `POST /login` is **not** specifically throttled
- [ ] Implement password complexity rules
- [ ] Add 2FA support
- [ ] Session timeout configuration

### DevOps
- [x] ✅ Add CI/CD pipeline — `.github/workflows/ci.yml` (build + Postgres/Redis services)
- [ ] Add staging environment
- [ ] Implement blue-green deployment
- [ ] Add automated database backups via Docker volume snapshots

---

## Product Gap Analysis → Phased Plan (2026-07-29)

Full-ERP benchmark (ERPNext/Odoo class) against `main` @ `deb23bb`. Verdict:
**finance-complete, operationally thin** — consolidation/eliminations is ahead of most
SMB ERPs, but buyers will ask first for returns, notifications, payroll, and tax
compliance. Phases below are sequenced business-value first, dependencies second;
each ships independently usable software. Where a phase expands an older Phase 14-17
item, the detail stays in those sections — this table supersedes only the ordering.

| # | Theme | Size | Depends on |
|---|-------|------|-----------|
| P1 | **Returns & credit/debit notes** ✅; document attachments remain | L | none (journals, inventory, AP links ✓) |
| P2 | **Notification center + transactional email** ✅ | M | `shared/mail.go` ✓, asynq ✓, user prefs ✓ |
| P3 | **Configurable approval engine + HR core** ✅ (employees, org, leave, attendance) | L | P2 |
| P4 | **Payroll Indonesia** ✅ — versioned PPh 21 (TER/PTKP), BPJS, payslips, approval, payment export, GL posting | XL | P3 |
| P5 | **Tax compliance** 🟡 — immutable faktur/PPN/PPh ledgers, GL recap, locks, and versioned Coretax export implemented; official portal acceptance pending | L | P1 |
| P6 | **CRM** ✅ — leads, contacts, pipeline, activities/reminders, explicit customer/quotation conversion, win/loss | L | P2 |
| P7 | **Multi-currency** (detail in Phase 14) + horizon packs: WMS, MRP, POS, public API/webhooks, portals | XL | stable AR/AP from P1 |

Sizing legend: S <2w · M 2–4w · L 1–2m · XL 2m+ (rough, single team).

### P1 — Returns & Credit/Debit Notes — ✅ DONE
- Sales returns and AR credit notes support stock restocking, invoice linkage, allocation,
  posting/voiding, GL journals, SSR views, and PDF output.
- GRN-based supplier returns and AP debit notes support inventory reversal, invoice linkage,
  allocation, posting/voiding, GL journals, SSR views, and PDF output.
- Document attachments on invoices, POs, and GRNs remain deferred to the Phase 16 attachment work.

### P2 — Communication Backbone
- ✅ Notification table, recent/unread APIs, live bell badge, and mark-one/all-read.
- ✅ Per-user, per-event in-app and email channel preferences, kept separate from
  the global workspace bell toggle.
- ✅ Configured SMTP client injected into Asynq; the `mail:send` placeholder now
  calls `shared.MailClient.SendEmail`.
- ✅ Initial events: posted AR invoice, submitted PO approval request, delivered
  board pack, and authenticated password change.
- Follow-up: route approval requests to resolved approvers when the configurable
  approval engine lands in P3, and reuse `password_reset` for a future
  forgot-password flow.

### P3 — Approvals + HR Core
- ✅ Shared multi-step policies with module/company/amount resolution,
  role/user/manager assignment, delegation, decisions, and escalation alerts.
- ✅ My Approvals and policy-management UI with RBAC.
- ✅ Employee directory, departments, positions, manager relationships, leave
  balances/requests, and validated attendance CSV imports.
- ✅ Large-PO threshold routing and manager-routed leave share one engine;
  approval finalizes the document, balance, audit record, and notification.
- Payroll remains exclusively in P4.

### P4 — Payroll Indonesia — ✅ DONE
- Effective-dated, source-referenced and reviewed versions cover PTKP/TER, BPJS
  rates/caps, and company overtime/rounding policies. Reviewed regulatory ranges
  and company-policy ranges cannot overlap.
- Compensation assignments, recurring components, one-off adjustments, overtime,
  THR, attendance/leave inputs, and integer-rupiah calculations produce a complete
  employee breakdown tied to the exact rule versions used.
- Runs follow `DRAFT → APPROVAL → POSTED` through the shared P3 engine. Regular-run
  source IDs are deterministic by company/period, posting is idempotent, posted
  data is immutable, and rejection actors/notes are audited.
- Balanced journals are grouped by department/cost center; posted runs generate
  bank-transfer CSV instructions and restricted Gotenberg payslips.
- Payslip records form a durable outbox. Initial enqueue failures do not undo
  posting, and the worker retries undelivered records every five minutes before
  sending through the shared SMTP jobs.
- Calculator, service, HTTP, worker, and migration coverage proves TER categories,
  PTKP evidence, BPJS caps, overtime, THR, negative adjustments, rounding, approval,
  journal balance, repeated-post idempotency, and concealed payslip access.
- Release boundary: `CalculateAnnualPPh21` now covers the December/last-tax-period
  rule with explicit progressive bands, prior-withholding reconciliation, and an
  official PMK 168/2023 worked-example fixture. Production activation still
  requires payroll/legal review of the selected effective rule version.

### P5 — Tax Compliance — 🟡 RELEASE VALIDATION PENDING
- Reviewed effective-dated PPN/PPh rules, tax codes, NPWP/NITKU identities,
  controlled faktur ranges, and category-to-GL mappings are implemented without
  silently seeding volatile regulatory values.
- Posted AR/AP invoices and P1 credit/debit notes create immutable source-hashed
  tax documents and signed ledger rows. PPh 23/PPh 4(2) supports invoice or
  prorated partial-payment recognition.
- Cancellation and replacement append audit/correction events and reversal rows;
  database controls prevent duplicate faktur/source numbers, edits/deletes, and
  activity in locked periods.
- `/tax` provides posted-source rebuild, monthly PPN/PPh-to-GL recap, rupiah-exact
  lock control, and permissioned versioned XML export with persisted totals and
  content hashes.
- Source posting also writes a durable capture outbox in the same transaction;
  workers retry tax capture without duplicating immutable documents. Export is
  POST-only and its XML declaration/optional fields belong to the reviewed
  schema version.
- Remaining release gate: the local Coretax validator contract and zero-difference
  GL reconciliation are covered by the release suite, but tax staff must still
  validate each version against the current official DJP XSD/converter and prove
  a representative month imports in Coretax. Until that external acceptance is
  recorded, Phase 5 is implemented but not certified for production filing.

### P6 — CRM
- ✅ Company/owner-scoped leads, distinct contacts, ordered opportunities,
  activity timeline, reassignment, reminders, and HR-manager escalation.
- ✅ Won opportunities explicitly link/create customer master data and create
  draft quotations through the existing pricing and lifecycle service. A unique
  opportunity reference makes quotation retries idempotent, and CRM links are
  finalized atomically.
- ✅ Pipeline board/list and win/loss analytics are available under `/crm`.
- ✅ Opportunity values retain `NUMERIC(18,2)` precision end to end.

### P7 — Multi-Currency + Horizon
- ✅ Local MVP foundation gates pass for company isolation, idempotency, lifecycle rules,
  WMS, MRP, POS, projects/timesheets, public REST API, webhooks, and portals.
- ✅ Local clean migrations through v61, build, lint, unit tests, full tests, and tagged
  integration tests pass. Detailed evidence is in `docs/guides/phase14-p7-acceptance-evidence.md`.
- Horizon packs by vertical: WMS (bins, picking, barcode), Manufacturing/MRP, POS,
  Projects/timesheets, public REST API + webhooks, portals.
- ✅ Manufacturing/MRP now includes controlled BOM revisions, warehouse-aware demand and
  supply planning, recommendation firming, routing operations, WIP execution and
  finished-goods cost transfer, finite-capacity scheduling, exceptions, quality and
  genealogy records, live analytics, and compliance-control foundations. See
  [`manufacturing-mrp.md`](guides/manufacturing-mrp.md) for the supported boundary.
- Production certification remains pending staging smoke tests, including the Asia/Jakarta
  FX worker, webhook delivery inspection, and the USD cross-feature scenario.

**Sequencing note:** P2 is early because it is small, visible, and P3/P4/P6 emit events
through it. P4 and P6 may swap if a customer commits. P7 is the only phase worth
breaking the order for (export/import demand).

---

## Recommended Next Steps

> For product-feature sequencing, follow the gap-analysis phased plan above; the
> module-choice framing below reflects the 2026-07-20 revision.

The high-priority finance automation work is specified in the
[`Core Finance Automation Plan`](guides/core-finance-automation-plan.md). It sequences
automated bank feeds, rolling cash forecasts, scheduled payments, purchase-to-pay
orchestration, and asset operations on the current banking, AP, procurement,
approval, and fixed-asset modules with ticket-ready work packages, dependencies,
release gates, and a two-stream delivery schedule.

The high-priority external connector program is specified in the
[`External Integrations Plan`](guides/external-integrations-plan.md). It establishes a
shared outbox/inbox, connection, secret, mapping, retry, and observability foundation,
then sequences payment gateways, carriers, marketplaces, messaging, BI, identity, and
governed AI connectors.

1. **Immediate (highest value, low effort)**
   - Add integration coverage for the Budget vs Actual query and form a release dataset that includes revenue and expense budgets.
   - Monitor the login-specific rate limiter on `POST /auth/login` (5 attempts/IP/minute) and tune its threshold based on production traffic.
   - Extend the new Users/Roles/RBAC unit coverage with handler and database integration scenarios.

2. **Short-term (1 month)**
   - Define and test automated DB backup, restore, retention, and RPO/RTO controls
   - Expand reporting catalog coverage for operational and HR reports
   - Document the supported lifecycle and integration contracts for current modules

3. **Medium-term (3 months)**
   - 📝 **IN PROGRESS**: Projects, POS, CMMS, and WMS depth remains partial (milestones/budgets,
     POS hardware/loyalty/gift cards, CMMS calibrated predictive AI/streaming/mobile, and WMS
     put-away/cross-dock/MHE remain). CMMS IoT readings and deterministic anomaly alerts now have
     persisted service paths. HR benefits are done; advanced QMS (SPC/ATE/calibration/LIMS), Portal
     depth (profiles/RFQ/chat/analytics), and provider connectors are not.
   - Enforce manufacturing controlled-record policies at approval and release decision
     points; add document OCR providers and realtime collaboration transport on top of the
     document processing/search backend, plus 2FA/SSO and enterprise compliance controls

---

## Conclusion

The core finance cycle (GL, AR, **AP**, banking with reconciliation, tax capture, and
transaction-level FX) is user-facing, subject to the release gates stated above.
Inventory and manufacturing execution are substantially expanded, while several wider
ERP surfaces remain partial. The next work should follow the authoritative
[`module catalog`](reference/module-catalog.md) and close staging, documentation, and
control-enforcement gaps before advertising regulated enterprise capabilities.
