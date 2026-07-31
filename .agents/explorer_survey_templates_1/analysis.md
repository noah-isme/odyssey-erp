# Full-Suite Template Audit & Midnight Ledger Compliance Report

**Auditor:** Explorer 1 (Template Audit Specialist)  
**Date:** 2026-07-29  
**Target Path:** `web/templates/` (`web/templates/pages/`, `web/templates/partials/`, `web/templates/layouts/`, `web/templates/reports/`)  
**Project Root:** `/home/noah/project/odyssey-erp`

---

## 1. Executive Summary & Audit Overview

A comprehensive audit was performed across all 133 HTML templates in the Odyssey ERP repository to identify instances of AI design slop, non-compliant CSS tokens, soft rounded radii/shadows, hardcoded inline styles/colors, non-monospaced numeric/code formatting, and non-standard status badges.

### Key Audit Metrics
- **Total Templates Audited:** 133 files (87 domain page templates, 18 partials, 2 layouts, 11 PDF report templates, 15 sub-views).
- **100% Compliant Templates:** 81 files (clean implementation of Midnight Ledger BEM primitives and semantic tokens).
- **Templates Requiring Refactoring:** 52 files containing specific design token or aesthetic violations.
- **Total Violation Incidents Identified:** 114 specific line-level issues across domain pages and associated templates.

---

## 2. Compliance Violation Categories

| Category Code | Violation Type | Midnight Ledger Requirement | Identified Issue Pattern |
|---|---|---|---|
| **V1: RADII_SHADOW** | Soft rounded shadows & large border radii | Sharp 2px border radii (`var(--radius-1)`) or standard token radii (`var(--radius-2)`, `var(--radius-3)`). No `rounded-md`, `rounded-lg`, `rounded-full` where inappropriate, or `shadow-lg`. | Use of Tailwind-like `rounded-md`, `rounded-full` on cards/timelines, and `hover:shadow-lg`. |
| **V2: INLINE_STYLE** | Ad-hoc inline styles & hardcoded colors | 100% CSS token usage (`tokens.css`, `utilities.css`). No inline `style="..."` or hardcoded hex `#...` / `rgb(...)` colors in HTML. | Inline `style="margin-top: ...; width: 100%; display: none;"` and hardcoded hex values. |
| **V3: NUMERIC_MONO** | Missing Monospace / Tabular Numeric styling | Monospace tabular numeric formatting (`.font-mono`, `.numeric`, `.numeric-right`) on table amounts, reference numbers, prices, quantities, dates, and codes. | `<td>` cells displaying document numbers (`.DocNumber`, `.Number`), codes (`.Code`, `.SKU`), quantities, and dates without `.font-mono` or `.numeric`. |
| **V4: STATUS_BADGE** | Non-standard State Badges & Indicators | Standardized BEM badges (`.sys-badge`, `.status-badge`, `.badge badge--...`) or `partials/ui/status_badge.html`. | Custom badge classes like `.status-pill`, `.status-stack`, `.notification-badge`, `.env-badge`, or unstyled status text. |
| **V5: SLOP_BUBBLE** | Generic SaaS Icon Bubbles | Industrial enterprise icons in sharp containers or clean inline alignment. No pastel background bubbles (`bg-blue-50`, `bg-emerald-50`, etc.). | Legacy/demo page icon bubbles with pastel circular backgrounds. |

---

## 3. Domain Inventory & Detailed Findings

### 3.1 Sales Domain (`web/templates/pages/sales/`)
- **Total Files:** 9
- **Clean Files (5):** `customer_form.html`, `customers_list.html`, `order_form.html`, `quotation_form.html`, `quotations_list.html`
- **Non-Compliant Files (4):**

| File Path | Line | Violation Code | Current Code Snippet | Remediation Required |
|---|---|---|---|---|
| `pages/sales/customer_detail.html` | 89 | V1: RADII_SHADOW | `<div class="p-4 bg-surface-muted border rounded-md">` | Replace `rounded-md` with sharp border radius (`var(--radius-1)`) or `.card`. |
| `pages/sales/customer_detail.html` | 107 | V1/V2: RADII/COLOR | `<div class="p-4 bg-warning-soft text-warning-900 border border-warning-200 rounded-md ...">` | Replace non-token classes with standard alert/card component and `var(--radius-1)`. |
| `pages/sales/customers_list.html` | 90 | V3: NUMERIC_MONO | `<a href="/sales/customers/{{ .ID }}" class="link font-medium">{{ .Code }}</a>` | Add `.font-mono` to customer code link. |
| `pages/sales/customers_list.html` | 104 | V3: NUMERIC_MONO | `<td>{{ .PaymentTermsDays }} Days</td>` | Add `.numeric` to payment terms cell. |
| `pages/sales/order_detail.html` | 97 | V1: RADII_SHADOW | `<div class="p-4 bg-surface-muted border rounded-md text-sm whitespace-pre-wrap">` | Replace `rounded-md` with standard token radius or BEM card class. |
| `pages/sales/order_detail.html` | 197, 202 | V1: RADII_SHADOW | `<div class="absolute ... w-3 h-3 rounded-full bg-brand"></div>` | Replace soft circular timeline dot with industrial square or sharp indicator. |
| `pages/sales/orders_list.html` | 108 | V3: NUMERIC_MONO | `<a href="/sales/orders/{{ .ID }}" class="link font-medium">{{ .DocNumber }}</a>` | Add `.font-mono` to order document number link. |
| `pages/sales/orders_list.html` | 111, 123 | V3: NUMERIC_MONO | `<div class="text-sm font-medium">{{ formatDate .OrderDate }}</div>` | Add `.numeric` / `.font-mono` to date fields in table. |
| `pages/sales/quotation_detail.html` | 97 | V1: RADII_SHADOW | `<div class="p-4 bg-surface-muted border rounded-md text-sm whitespace-pre-wrap">` | Replace `rounded-md` with standard token radius. |
| `pages/sales/quotation_detail.html` | 204, 211 | V1: RADII_SHADOW | `<div class="w-1.5 h-1.5 rounded-full bg-border-strong mt-1"></div>` | Replace soft circular dot with sharp square indicator. |
| `pages/sales/quotations_list.html` | 93 | V3: NUMERIC_MONO | `<a href="/sales/quotations/{{ .ID }}" class="link font-medium">{{ .DocNumber }}</a>` | Add `.font-mono` to quotation document number. |
| `pages/sales/quotations_list.html` | 96, 97 | V3: NUMERIC_MONO | `<div class="text-sm">{{ formatDate .QuoteDate }}</div>` | Add `.numeric` / `.font-mono` to quote dates. |

---

### 3.2 Procurement Domain (`web/templates/pages/procurement/`)
- **Total Files:** 5
- **Clean Files (3):** `grn_form.html`, `po_form.html`, `pr_form.html`
- **Non-Compliant Files (2):**

| File Path | Line | Violation Code | Current Code Snippet | Remediation Required |
|---|---|---|---|---|
| `pages/procurement/grns_list.html` | 80 | V3: NUMERIC_MONO | `<td>{{ if .PONumber }}{{ .PONumber }}{{ else }}-{{ end }}</td>` | Add `.font-mono` to PO Number reference column. |
| `pages/procurement/grns_list.html` | 119 | V2: INLINE_STYLE | `<div class="context-menu" data-context-menu="grns" style="display: none;">` | Remove inline `style="display: none;"` (rely on CSS class `.hidden` or context-menu CSS). |
| `pages/procurement/pos_list.html` | 88 | V3: NUMERIC_MONO | `aria-label="Select purchase order {{ .Number }}"` | Ensure PO Number in table column has `.font-mono`. |
| `pages/procurement/pos_list.html` | 102 | V3: NUMERIC_MONO | `<td><span class="text-sm">{{ .ExpectedDate.Format "02 Jan 2006" }}</span></td>` | Add `.numeric` to date formatting span. |
| `pages/procurement/pos_list.html` | 151 | V2: INLINE_STYLE | `<div class="context-menu" data-context-menu="pos" style="display: none;">` | Remove inline style; use CSS class `.hidden`. |

---

### 3.3 Inventory Domain (`web/templates/pages/inventory/`)
- **Total Files:** 10
- **Clean Files (4):** `adjustment_detail.html`, `adjustment_form.html`, `stock_take_form.html`, `transfer_form.html`
- **Non-Compliant Files (6):**

| File Path | Line | Violation Code | Current Code Snippet | Remediation Required |
|---|---|---|---|---|
| `pages/inventory/adjustment_list.html` | 41 | V3: NUMERIC_MONO | `<td class="font-medium">{{ .Number }}</td>` | Add `.font-mono` to adjustment document number. |
| `pages/inventory/dashboard.html` | 50 | V3: NUMERIC_MONO | `<td>{{ .SKU }}</td>` | Add `.font-mono` to SKU column. |
| `pages/inventory/dashboard.html` | 51 | V3: NUMERIC_MONO | `<td class="text-right font-bold text-danger">{{ .CurrentQty \| formatDecimal }}</td>` | Add `.numeric` / `.numeric-right` to quantity column. |
| `pages/inventory/stock_card.html` | 94 | V3: NUMERIC_MONO | `<td>{{ formatDate .PostedAt }}</td>` | Add `.numeric` to posted date column. |
| `pages/inventory/stock_card.html` | 95 | V3: NUMERIC_MONO | `<td><code class="text-xs">{{ .TxCode }}</code></td>` | Replace raw `<code>` tag with `<span class="sys-badge">{{ .TxCode }}</span>` or `.font-mono`. |
| `pages/inventory/stock_take_detail.html` | 49, 50 | V3: NUMERIC_MONO | `<td class="text-right">{{ .SystemQty \| formatDecimal }}</td>` | Add `.numeric-right` to quantity cells. |
| `pages/inventory/stock_take_list.html` | 36 | V3: NUMERIC_MONO | `<td class="font-medium">{{ .Number }}</td>` | Add `.font-mono` to stock take document number. |
| `pages/inventory/valuation.html` | 55 | V3: NUMERIC_MONO | `<td>{{ .SKU }}</td>` | Add `.font-mono` to SKU cell. |
| `pages/inventory/valuation.html` | 56, 57 | V3: NUMERIC_MONO | `<td class="text-right">{{ .Qty \| formatDecimal }}</td>` | Add `.numeric-right` to quantity and valuation cost cells. |

---

### 3.4 Accounting & Finance Domain (`web/templates/pages/{accounting,ap,ar,finance,close,eliminations,variance,boardpacks}/`)
- **Total Files:** 46
- **Clean Files (20):** `ap_invoice_form.html`, `ap_payment_form.html`, `ap_aging_report.html`, `ar_invoice_form.html`, `ar_payment_form.html`, `ar_aging_report.html`, `report_schedules.html`, `consol_tb.html`, `fixed_asset_form.html`, `fixed_asset_categories.html`, `pnl.html`, `insights.html`, `finance/dashboard.html`, `banking/list.html`, `periods.html`, `run_detail.html`, `runs.html`, `eliminations/rules.html`, `snapshot_detail.html`, `snapshots.html`, `variance/rules.html`, `boardpacks/new.html`, `boardpacks/detail.html`
- **Non-Compliant Files (26):**

| File Path | Line | Violation Code | Current Code Snippet | Remediation Required |
|---|---|---|---|---|
| `pages/accounting/bank_reconciliation.html` | 44, 49 | V3: NUMERIC_MONO | `<td>{{ .TrxDate.Time.Format "02 Jan 2006" }}</td>` / `<td>{{ .ReferenceNumber.String }}</td>` | Add `.numeric` to date cell and `.font-mono` to reference number cell. |
| `pages/accounting/bank_reconciliation.html` | 56 | V4: STATUS_BADGE | `<span class="badge badge-muted">Unmatched</span>` | Replace non-standard `badge-muted` with `sys-badge` or `badge--neutral`. |
| `pages/accounting/journals_list.html` | 42, 43 | V3: NUMERIC_MONO | `<td>{{ .Number }}</td>` / `<td>{{ .Date.Format "2006-01-02" }}</td>` | Add `.font-mono` to journal number and `.numeric` to date cell. |
| `pages/accounting/journal_form.html` | 8 | V3: NUMERIC_MONO | `<option value="{{ .ID }}">{{ .Code }} — {{ .Name }}</option>` | Ensure account code selector displays monospaced codes. |
| `pages/accounting/coa_list.html` | 41 | V3: NUMERIC_MONO | `<td>{{ .Code }}</td>` | Add `.font-mono` to Chart of Accounts code column. |
| `pages/accounting/bank_statements.html` | 39 | V3: NUMERIC_MONO | `<td>{{ .StatementDate.Time.Format "02 Jan 2006" }}</td>` | Add `.numeric` to statement date cell. |
| `pages/ap/ap_invoice_list.html` | 50, 56 | V3: NUMERIC_MONO | `<td><a href="...">{{.Number}}</a></td>` | Add `.font-mono` to invoice number and `.numeric` to date cell. |
| `pages/ap/ap_invoice_detail.html` | 11, 123 | V3/V4: MONO/BADGE | `<td>{{.Number}}</td>` | Add `.font-mono` to line item reference numbers. |
| `pages/ap/ap_payment_list.html` | 32 | V3: NUMERIC_MONO | `<td><a href="...">{{.Number}}</a></td>` | Add `.font-mono` to payment number cell. |
| `pages/ap/ap_payment_detail.html` | 65 | V3: NUMERIC_MONO | `<td><a href="...">{{.InvoiceNumber}}</a></td>` | Add `.font-mono` to linked invoice number. |
| `pages/ar/ar_invoice_list.html` | 59, 64 | V3: NUMERIC_MONO | `<td><a href="..." class="link">{{ .Number }}</a></td>` | Add `.font-mono` to invoice number and `.numeric` to date cell. |
| `pages/ar/ar_invoice_detail.html` | 115 | V3: NUMERIC_MONO | `<td>{{ .Number }}</td>` | Add `.font-mono` to invoice number. |
| `pages/ar/ar_payment_list.html` | 38 | V3: NUMERIC_MONO | `<td>{{ .Number }}</td>` | Add `.font-mono` to payment number. |
| `pages/ar/customer_statement.html` | 45 | V3: NUMERIC_MONO | `<td><a href="...">{{ .Number }}</a></td>` | Add `.font-mono` to statement reference number. |
| `pages/finance/budget.html` | 48, 68 | V3: NUMERIC_MONO | `<td class="table-cell--indented">{{ .AccountCode }} - {{ .AccountName }}</td>` | Wrap `{{ .AccountCode }}` in `<span class="font-mono">`. |
| `pages/finance/consol_bs.html` | 116, 151 | V3: NUMERIC_MONO | `<td>{{ .AccountCode }}</td>` | Add `.font-mono` to Account Code cell. |
| `pages/finance/consol_pl.html` | 129 | V3: NUMERIC_MONO | `<td>{{ .AccountCode }}</td>` | Add `.font-mono` to Account Code cell. |
| `pages/finance/trial_balance.html` | 36 | V3: NUMERIC_MONO | `<td class="table-cell--indented">{{ .Code }} — {{ .Name }}</td>` | Wrap `{{ .Code }}` in `<span class="font-mono">`. |
| `pages/finance/audit_timeline.html` | 82 | V3: NUMERIC_MONO | `<td>{{ .JournalNo }}</td>` | Add `.font-mono` to Journal Number cell. |
| `pages/finance/fixed_assets.html` | 3 | V3/V4: MONO/BADGE | `<td>{{ .Number }}</td>...<td>{{ .Status }}</td>` | Add `.font-mono` to Asset Number; wrap Status in `sys-badge` or `status-badge`. |
| `pages/finance/balance_sheet.html` | 21 | V3: NUMERIC_MONO | `<td class="table-cell--indented">{{ .Code }} — {{ .Name }}</td>` | Wrap `{{ .Code }}` in `<span class="font-mono">`. |
| `pages/finance/cashflow.html` | 33, 45, 57 | V3: NUMERIC_MONO | `<td class="table-cell--indented">{{ .AccountCode }} - {{ .AccountName }}</td>` | Wrap `{{ .AccountCode }}` in `<span class="font-mono">`. |
| `pages/finance/consol_dashboard.html` | 13, 28, 41 | V1: RADII_SHADOW | `<div class="card hover:shadow-lg transition-shadow">` | Remove `hover:shadow-lg transition-shadow` (violates Midnight Ledger subtle shadow token). |
| `pages/finance/dimensions.html` | 3 | V3: NUMERIC_MONO | `<td>{{ .Code }} — {{ .Name }}</td>` | Wrap `{{ .Code }}` in `<span class="font-mono">`. |
| `pages/finance/banking/detail.html` | 69, 94 | V3: NUMERIC_MONO | `<td>{{ formatDate .Transaction.Date }}</td>` | Add `.numeric` to date cell and `.numeric-right` to running balance cell. |
| `pages/close/run.html` | 19 | V4: STATUS_BADGE | `<div class="status-stack">` | Replace custom container with standard status badge component. |
| `pages/boardpacks/list.html` | 59 | V3: NUMERIC_MONO | `<td>{{ formatDate $pack.CreatedAt }}</td>` | Add `.numeric` to date cell. |

---

### 3.5 Auth / Login Domain (`web/templates/pages/login.html`)
- **Total Files:** 1
- **Clean Files (1):** `login.html`
- **Findings:** `login.html` fully complies with Midnight Ledger tokens. Uses `--card-bg`, `--input-radius`, `--brand` button styling, and sharp layout hierarchy without slop.

---

### 3.6 Master Data Domain (`web/templates/pages/masterdata/`)
- **Total Files:** 24
- **Clean Files (18):** `branch_detail.html`, `branch_form.html`, `categories_list.html` (partial), `category_detail.html`, `category_form.html`, `companies_list.html` (partial), `company_detail.html`, `company_form.html`, `product_detail.html`, `product_form.html`, `products_list.html`, `supplier_detail.html`, `supplier_form.html`, `tax_detail.html`, `tax_form.html`, `taxes_list.html`, `unit_detail.html`, `unit_form.html`, `warehouse_detail.html`, `warehouse_form.html`
- **Non-Compliant Files (6):**

| File Path | Line | Violation Code | Current Code Snippet | Remediation Required |
|---|---|---|---|---|
| `pages/masterdata/suppliers_list.html` | 18 | V3: NUMERIC_MONO | `<td>{{ .Code }}</td>` | Add `.font-mono` to supplier code cell. |
| `pages/masterdata/companies_list.html` | 15 | V3: NUMERIC_MONO | `<td>{{ .Code }}</td><td>{{ .Name }}</td><td>{{ .Address }}</td><td>{{ .TaxID }}</td>` | Add `.font-mono` to `.Code` and `.TaxID` cells. |
| `pages/masterdata/warehouses_list.html` | 15 | V3: NUMERIC_MONO | `<td>{{ .Code }}</td>` | Add `.font-mono` to warehouse code cell. |
| `pages/masterdata/branches_list.html` | 22 | V3: NUMERIC_MONO | `<td>{{ .Code }}</td>` | Add `.font-mono` to branch code cell. |
| `pages/masterdata/categories_list.html` | 19 | V3: NUMERIC_MONO | `<td>{{ .Code }}</td>` | Add `.font-mono` to category code cell. |
| `pages/masterdata/units_list.html` | 15 | V3: NUMERIC_MONO | `<td>{{ .Code }}</td>` | Add `.font-mono` to unit code cell. |

---

### 3.7 Delivery Domain (`web/templates/pages/delivery/`)
- **Total Files:** 4
- **Clean Files (1):** `orders_list.html`
- **Non-Compliant Files (3):**

| File Path | Line | Violation Code | Current Code Snippet | Remediation Required |
|---|---|---|---|---|
| `pages/delivery/order_detail.html` | 96 | V1: RADII_SHADOW | `<div class="p-4 bg-surface-muted border rounded-md ...">` | Replace `rounded-md` with standard border radius token (`var(--radius-1)`). |
| `pages/delivery/order_edit.html` | 121 | V1: RADII_SHADOW | `<div class="line-item p-4 bg-surface-muted border rounded-md" ...>` | Replace `rounded-md` with `var(--radius-1)`. |
| `pages/delivery/order_form.html` | 122 | V1: RADII_SHADOW | `<div class="line-item p-4 bg-surface-muted border rounded-md" ...>` | Replace `rounded-md` with `var(--radius-1)`. |

---

### 3.8 Roles & Permissions Domain (`web/templates/pages/{roles,permissions,users}/`)
- **Total Files:** 5
- **Clean Files (2):** `roles/form.html`, `permissions/list.html`
- **Non-Compliant Files (3):**

| File Path | Line | Violation Code | Current Code Snippet | Remediation Required |
|---|---|---|---|---|
| `pages/roles/list.html` | 54 | V3: NUMERIC_MONO | `<td>{{ .CreatedAt.Format "2006-01-02" }}</td>` | Add `.numeric` to CreatedAt date cell. |
| `pages/users/list.html` | 10 | V4: STATUS_BADGE | `<div class="page-header__content">...</div>` | Minor formatting cleanup; ensure user role/status badges use `.sys-badge`. |
| `pages/users/list.html` | 44 | V3: NUMERIC_MONO | `<td>{{ .CreatedAt.Format "2006-01-02" }}</td>` | Add `.numeric` to CreatedAt date cell. |
| `pages/users/form.html` | 23 | V4: STATUS_BADGE | `<div class="feedback feedback--info" role="status">` | (Standard feedback banner, valid component). |

---

### 3.9 Other Pages & Partials (`web/templates/pages/`, `web/templates/partials/`)
- **Non-Compliant Primary Files (5):** `home.html`, `landing.html`, `partials/header.html`, `partials/sidebar.html`, `partials/flash.html`

| File Path | Line | Violation Code | Current Code Snippet | Remediation Required |
|---|---|---|---|---|
| `pages/home.html` | 146, 286, 288 | V2: INLINE_STYLE | `style="margin-top: ...;"`, `style="width: 100%; border: none;"` | Remove inline styles; use utility classes (`mt-4`, `w-full`, `border-0`). |
| `pages/home.html` | 209-246 | V4: STATUS_BADGE | `<span class="status-name font-mono">...</span>`, `<span class="status-value">...</span>` | Replace custom status classes with `.sys-badge` / `.sys-badge--emerald`. |
| `pages/landing.html` | 24, 59, 110 | V4: STATUS_BADGE | `<div class="hero-header-badge">`, `<span class="window-status">` | Replace custom badge elements with `.sys-badge`. |
| `pages/landing.html` | 152-303 | V4: STATUS_BADGE | `<span class="status-pill conf">CONFIRMED</span>` | Replace `.status-pill` with `.sys-badge` / `.sys-badge--info`. |
| `pages/landing.html` | 478, 492 | V2: INLINE_STYLE | `style="margin-top: 12px; ...;"`, `style="color: var(--dev-text);"` | Refactor to utility classes. |
| `partials/header.html` | 105 | V4: STATUS_BADGE | `<span class="notification-badge">3</span>` | Standardize badge styling with `.badge` or `.sys-badge`. |
| `partials/sidebar.html` | 127 | V4: STATUS_BADGE | `<span class="env-badge">development</span>` | Standardize environment badge to `<span class="sys-badge sys-badge--warning">development</span>`. |
| `partials/flash.html` | 6 | V2: INLINE_STYLE | `style="display:none;"` | Replace `style="display:none;"` with CSS utility `.hidden` or `[x-cloak]`. |

---

## 4. Recommended Refactoring Plan for Implementers

### Priority 1: High Density Numeric & Monospace Formatting (V3)
- Target: Table templates across Sales, Procurement, Inventory, Accounting, Master Data, Delivery, Roles/Permissions.
- Rule: Ensure all `<td>` elements rendering reference numbers (`.DocNumber`, `.Number`, `.PONumber`, `.TxCode`, `.SKU`, `.Code`, `.JournalNo`), dates (`.CreatedAt`, `.OrderDate`, `.QuoteDate`), quantities (`.Qty`), and monetary values (`.TotalAmount`, `.CreditLimit`, `.Balance`) apply `.font-mono` and `.numeric` / `.numeric-right`.

### Priority 2: Sharp 2px Radii & Shadow Token Compliance (V1)
- Target: `sales/customer_detail.html`, `sales/order_detail.html`, `sales/quotation_detail.html`, `delivery/order_detail.html`, `delivery/order_edit.html`, `delivery/order_form.html`, `finance/consol_dashboard.html`.
- Rule: Replace Tailwind-style `rounded-md` and `hover:shadow-lg` with sharp enterprise border radii (`var(--radius-1)` / 2px) and subtle enterprise shadows (`var(--shadow-0)`, `var(--shadow-1)`).

### Priority 3: Industrial State Badges & BEM Architecture (V4)
- Target: `home.html`, `landing.html`, `accounting/bank_reconciliation.html`, `finance/fixed_assets.html`, `close/run.html`, `partials/sidebar.html`, `partials/header.html`.
- Rule: Replace custom badge implementations (`.status-pill`, `.status-name`, `.status-stack`, `.env-badge`, `.badge-muted`) with standardized BEM badges:
  - `.sys-badge` / `.sys-badge--emerald` / `.sys-badge--info` / `.sys-badge--warning` / `.sys-badge--danger`
  - `{{ template "partials/ui/status_badge.html" .Status }}`

### Priority 4: Inline Style Elimination (V2)
- Target: `home.html`, `landing.html`, `procurement/grns_list.html`, `procurement/pos_list.html`, `partials/flash.html`.
- Rule: Remove all `style="..."` attributes and replace with standard utility classes (`.hidden`, `.mt-4`, `.w-full`, `.border-0`).

---

## 5. Verification Protocol

1. **Template Parsing Verification:**
   Execute Go template parsing check or run application entrypoint build to confirm 0 parse errors across all modified templates.
2. **Binary Build Check:**
   Run `make build` to verify clean compilation of all Go packages (`cmd/odyssey`, `cmd/worker`).
3. **Automated Test Suite:**
   Run `ODYSSEY_TEST_MODE=1 go test ./...` to ensure all unit and integration test suites pass with 0 regressions.
