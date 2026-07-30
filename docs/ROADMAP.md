# Odyssey ERP — Future Roadmap & Recommendations

**Prepared:** 2026-01-11
**Revised:** 2026-07-20 (synced to current implementation)
**Revised:** 2026-07-29 (added the gap-analysis phased plan below)
**Current Version:** v0.9.1

## Executive Summary

Odyssey ERP has moved well beyond Phase 9. Since this roadmap was first drafted,
**Phase 10 (Accounts Payable)** and **Phase 11 (Bank & Cash Management)** have been
built and are user-facing, and **Phase 12 (Inventory Enhancements)** is largely
complete. This document has been revised to reflect what is actually implemented and
to re-prioritise the genuinely remaining work.

---

## Implementation Status (verified against code, 2026-07-14)

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
| Phase 14 — Transaction-level multi-currency | ❌ Not started | FX exists only for consolidation (`internal/consol/fx/`), not realized/unrealized gain on AR/AP |
| Phase 15 — Reporting enhancements | 🟡 Partial | P&L and Budget vs Actual support department/cost-center filters, native `.xlsx`, and scheduled email; report builder/widgets remain |

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

**Status:** Bank accounts, transactions, transfers, reconciliation, and cash flow report
are implemented (`internal/finance/banking/`, `internal/accounting/banks/`). Only the
**auto bank feed (CSV/OFX import)** remains outstanding.
**Priority:** ~~🔴 High~~
**Estimated Effort:** ~~2-3 weeks~~

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Bank Accounts | Multiple bank account management | High |
| Bank Transactions | Record deposits, withdrawals, transfers | High |
| Bank Reconciliation | Match transactions with bank statement | High |
| Cash Flow Report | Actual cash flow from transactions | Medium |
| Auto Bank Feed | Import bank statements (CSV/OFX) | Low |

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
**Estimated Effort:** 2 weeks

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Auto FX Rate | Fetch daily rates from API | Medium |
| Realized Gain/Loss | Calculate on payment | High |
| Unrealized Gain/Loss | Revalue outstanding invoices | High |
| Currency Revaluation | Month-end revaluation job | High |

### Technical Notes
- Integrate with free FX API (exchangerate-api.com)
- Add `original_currency_amount` to AR/AP
- Create revaluation journal entries

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

## Phase 16: Audit & Compliance

**Priority:** 🟢 Low  
**Estimated Effort:** 2 weeks

### Features
| Feature | Description | Priority |
|---------|-------------|----------|
| Audit Log Viewer | UI for viewing audit logs | High |
| Data Export | Export all data for audit | Medium |
| Document Attachment | Attach files to transactions | Medium |
| E-Signature | Digital approval signatures | Low |
| Change History | View change history per record | Medium |

---

## Phase 17: Integration

**Priority:** 🟢 Low  
**Estimated Effort:** Varies

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
- [ ] Refactor handler error responses to be consistent

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
| P4 | **Payroll Indonesia** — PPh 21 (TER), BPJS, slip gaji, GL posting | XL | P3 |
| P5 | **Tax compliance** — faktur pajak numbering, PPN/PPh reports, e-Faktur export (expands Phase 17 row) | L | P1 |
| P6 | **CRM** — leads, pipeline, activities (feeds existing quotations) | L | P2 |
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

### P4 — Payroll Indonesia
- Salary components (gaji pokok, tunjangan, potongan, overtime, THR); monthly runs
  draft → approve → post; slip gaji PDF via Gotenberg
- PPh 21 with TER monthly rates (PMK 168/2023) + PTKP statuses; BPJS Kesehatan &
  Ketenagakerjaan; expense journals per cost-center dimension
- **Done when:** PPh 21 computes correctly for all PTKP statuses (DJP test cases),
  slips are emailed, and the journal balances.

### P5 — Tax Compliance
- Regulated faktur pajak sequence on AR invoices; PPN keluaran/masukan + SPT recap;
  e-Faktur/Coretax CSV/XML export; PPh 23/4(2) withholding on AP
- Retur integration from P1
- **Done when:** a month exports into an e-Faktur-importable file reconciling to GL
  PPN accounts to the rupiah.

### P6 — CRM
- Leads, opportunities/pipeline stages, win/loss, activities with reminders;
  convert lead → customer → quotation without re-keying
- **Done when:** win/loss analytics appear on the sales dashboard.

### P7 — Multi-Currency + Horizon
- Exchange-rate table, rate lock per document, realized gain/loss on payment,
  unrealized revaluation at period close (see Phase 14 technical notes)
- Horizon packs by vertical: WMS (bins, picking, barcode), Manufacturing/MRP (BOM,
  work orders), POS, Projects/timesheets, public REST API + webhooks, portals

**Sequencing note:** P2 is early because it is small, visible, and P3/P4/P6 emit events
through it. P4 and P6 may swap if a customer commits. P7 is the only phase worth
breaking the order for (export/import demand).

---

## Recommended Next Steps

> For product-feature sequencing, follow the gap-analysis phased plan above; the
> module-choice framing below reflects the 2026-07-20 revision.

1. **Immediate (highest value, low effort)**
   - Add integration coverage for the Budget vs Actual query and form a release dataset that includes revenue and expense budgets.
   - Monitor the login-specific rate limiter on `POST /auth/login` (5 attempts/IP/minute) and tune its threshold based on production traffic.
   - Extend the new Users/Roles/RBAC unit coverage with handler and database integration scenarios.

2. **Short-term (1 month)**
   - Automated DB backup via Docker volume snapshots (`pg_dump`)
   - Finish Phase 12: decide on LIFO costing / `cost_method`; stock reorder automation
   - Bank auto-feed (CSV/OFX import) to close out Phase 11

3. **Medium-term (3 months)**
   - Choose the next major module: **Fixed Assets (Phase 13)** or **transaction-level
     multi-currency with realized/unrealized gain (Phase 14)**, driven by business need
   - Department reporting and native Excel export (Phase 15)

---

## Conclusion

The core finance cycle (GL, AR, **AP**, and **Banking with reconciliation**) is complete
and user-facing, and Inventory is largely enhanced. The highest-leverage work now is
**finishing what is half-built** — wiring up Budget vs Actual, hardening login rate
limiting, and covering the RBAC/users/roles surface with tests — before starting the next
major module (**Fixed Assets** or **transaction-level multi-currency**).
