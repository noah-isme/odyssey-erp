# Build & Test System Audit Analysis

**Project**: Odyssey ERP  
**Module**: UI Refactoring & System Stability Audit  
**Author**: Explorer 3 (Build & Test System Specialist)  
**Date**: 2026-07-29  

---

## 1. Executive Summary & Audit Scope

During the Midnight Ledger UI refactoring of Odyssey ERP, maintaining template validity, Go compilation integrity, and test suite execution is critical. This analysis details the exact compilation mechanics, template parsing architecture, test package inventory, and actionable verification rules for Workers and Reviewers.

Key Findings:
1. **Embedded Templates**: HTML templates in `web/templates/` and static assets in `web/static/` are statically compiled into Go binaries via `go:embed` in `web/embed.go`.
2. **Template Parsing Engine**: `view.NewEngine()` in `internal/view/templates.go` parses base layouts, partials, and clones each page template. Response buffering prevents truncated HTML delivery on runtime errors.
3. **Automated UI Contract Enforcers**: `internal/view/ui_contracts_test.go` mechanically enforces button styles (`.btn`), form controls (`.form-input`, `.form-select`), table wrappers, accessibility (`aria-label`, captions), and strictly forbids inline styles (`style="..."`) or inline scripts (`<script`) in migrated domain templates.
4. **Test Coverage Map**: The test suite spans 64 test files across 25+ Go packages, with targeted template execution tests in `internal/view` and route handler rendering tests in `internal/*/http`.

---

## 2. Build System & Go Compilation Mechanics (`make build`)

### 2.1 Entrypoints & Targets
- **Main HTTP Server**: `cmd/odyssey/main.go`
- **Asynq Worker**: `cmd/worker/main.go`
- **Bootstrap Admin Utility**: `cmd/bootstrap-admin/main.go`
- **CLI Commands**: `cmd/odyssey/cli/...`

### 2.2 Template & Asset Embedding (`web/embed.go`)
```go
package web

import "embed"

//go:embed templates/layouts/*.html templates/partials/*.html templates/partials/*/*.html templates/pages/*.html templates/pages/*/*.html templates/pages/*/*/*.html templates/reports/*.html templates/reports/finance/*.html
var Templates embed.FS

//go:embed static/**/*
var Static embed.FS
```
- **Build-Time Impact**: Any added or renamed template directory must match these glob patterns in `web/embed.go`. Otherwise, `view.NewEngine()` will fail at runtime or during test execution because `embed.FS` will not contain the files.

### 2.3 Build Flags & Conditional Compilation Tags
- `//go:build production || pdf` and `//go:build !production && !pdf` are used in `internal/consol/http` for PDF report generation handlers vs stub handlers.
- `//go:build !production` is used for testmode imports in consolidation.
- `ODYSSEY_TEST_MODE=1` environment variable configures test/stub behavior without requiring external Postgres or Gotenberg services during fast test runs.

### 2.4 Makefile Commands
- `make build`: Runs `go build ./...` across all workspace modules.
- `make test`: Runs `go test ./...`.
- `make vet`: Runs `go vet` against non-cmd packages.
- `make lint`: Runs `golangci-lint run ./...`.

---

## 3. Template Engine Architecture (`internal/view`)

### 3.1 Template Loading & Cloning (`internal/view/templates.go`)
`NewEngine()` constructs a thread-safe map of compiled `*html/template.Template` instances:
1. **Base parsing**: Parses base layout (`templates/layouts/*.html`) and all partials (`templates/partials/**/*.html`).
2. **Page cloning**: For every page file found under `templates/pages/**/*.html`, `base.Clone()` is called and the page HTML is parsed into the clone.
3. **Map keying**: Template names in the map are keyed by their relative path without `templates/` (e.g. `pages/sales/orders.html`, `pages/finance/trial_balance.html`).

### 3.2 Registered Template Functions (`funcMap`)
- **Formatting**: `formatDate`, `formatDecimal`, `formatCurrency`, `formatUUID`, `formatDateInput`, `formatDatePtr`.
- **String Helpers**: `lower`, `upper` (safely handles custom Go string types), `stringify`, `deref`, `default`, `isActive`.
- **Math Helpers**: `add`, `addf`, `sub`, `subf`, `mul`, `mulf`, `div`, `divf`, `isNegative`.
- **Logic Helpers**: `now`, `countByStatus`.

### 3.3 Safe Response Buffering (`RenderStatus`)
To prevent serving partial 200 OK responses with broken HTML:
```go
func (e *Engine) RenderStatus(w http.ResponseWriter, name string, data TemplateData, status int) error {
    ...
    var buf bytes.Buffer
    if err := tpl.ExecuteTemplate(&buf, name, data); err != nil {
        return err // Header not yet written! Caller can handle 500 error.
    }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.WriteHeader(status)
    _, _ = buf.WriteTo(w)
    return nil
}
```

---

## 4. Comprehensive Map of Test Packages

### 4.1 Web Rendering & UI Contract Test Package (`internal/view`)
Package `view` / `view_test` is the primary validator for templates:

| Test File | Test Name | Purpose & Coverage |
| :--- | :--- | :--- |
| `templates_test.go` | `TestNewEngine` | Verifies all embedded templates parse without syntax/parsing errors. |
| `templates_test.go` | `TestHandlerTemplateReferencesExist` | Scans `internal/**/*.go` source code for `pages/...html` strings and checks if every template referenced in code actually exists in `engine.templates`. |
| `templates_test.go` | `TestManagementRouteTemplatesExist` | Verifies 18 management route templates exist in `engine.templates`. |
| `ui_contracts_test.go` | `TestBusinessTemplatesUseCanonicalUIContracts` | Enforces button (`.btn`), non-role button, table (`.table`), table responsive container, form controls (`.form-input`, `.form-select`, `.form-textarea`, `.check__input`/`.checkbox-input`). |
| `ui_contracts_test.go` | `TestMigratedModulesRejectLegacyMarkup` | Enforces no `style="` inline styles, no `<script` inline scripts, no fixed grid utilities in migrated pages. |
| `ui_contracts_test.go` | `TestIconOnlyButtonsHaveAccessibleLabels` | Enforces `aria-label` on icon-only `<button>` elements. |
| `ui_contracts_test.go` | `TestFocusVisibleFallbackIsPreserved` | Enforces `:focus-visible` styling in `static/css/core/utilities.css`. |
| `ui_contracts_test.go` | `TestReportTablesHaveAccessibleNames` | Enforces `caption` or `aria-labelledby` on report tables. |
| `ar_templates_render_test.go` | `TestARInvoiceTemplatesExecute` | Renders `pages/ar/ar_invoice_list.html` and `pages/ar/ar_invoice_detail.html` with real domain structs. |
| `ar_templates_render_test.go` | `TestARPaymentListTemplateExecutes` | Renders `pages/ar/ar_payment_list.html` with real domain structs. |
| `finance_reports_render_test.go` | `TestFinanceReportTemplatesExecute` | Renders trial balance and balance sheet templates with domain data and empty states. |
| `finance_reports_render_test.go` | `TestRenderLeavesResponseUntouchedOnTemplateError` | Tests buffering safety on template execution error. |
| `finance_reports_render_test.go` | `TestRenderStatusAppliesStatusOnSuccess` | Tests status code header application. |
| `management_templates_test.go` | `TestManagementTemplatesRender` | Executes 26 masterdata/roles/users detail, form, and list templates with mock data structs. |

### 4.2 HTTP Route & Handler Rendering Test Packages
These packages instantiate `view.NewEngine()` and test HTTP routes and template rendering:

| Package Path | Test File | Description |
| :--- | :--- | :--- |
| `internal/analytics/http` | `handlers_test.go` | Dashboard template execution & SVG chart integration. |
| `internal/audit/http` | `handlers_test.go` | Audit log page rendering and filtering. |
| `internal/auth` | `handler_test.go` | Auth/Login template rendering and CSRF flow. |
| `internal/close/http` | `handler_test.go` | Financial close checklist and period lock templates. |
| `internal/consol/http` | `handlers_plbs_test.go`, `export_test.go` | Financial consolidation P&L and Balance Sheet rendering. |
| `internal/insights/http` | `handlers_test.go` | Executive insights reporting templates. |
| `internal/accounting` | `handler_test.go` | General Ledger and Journal entry rendering. |
| `internal/ap` | `handler_test.go` | Accounts Payable routes and templates. |
| `internal/app` | `routes_test.go`, `render_blueprint_test.go` | Chi router route walking and deployment specification. |

### 4.3 Full Inventory of Workspace Go Test Packages
- `cmd/bootstrap-admin`: `main_test.go`
- `cmd/odyssey/cli`: `fx_cli_test.go`
- `internal/accounting/banks`: `csv_test.go`
- `internal/accounting/journals`: `service_guard_test.go`
- `internal/accounting/reports`: `cf_test.go`, `excel_test.go`, `reports_test.go`
- `internal/analytics/export`: `export_test.go`
- `internal/analytics/svg`: `bar_test.go`, `line_test.go`
- `internal/ap`: `service_test.go`
- `internal/ar`: `service_test.go`
- `internal/audit`: `service_test.go`
- `internal/boardpack`: `builder_test.go`, `storage_test.go`
- `internal/consol/fx`: `convert_test.go`, `validate_test.go`
- `internal/consol/ic`: `engine_test.go`
- `internal/consol`: `service_bs_test.go`, `service_pl_test.go`
- `internal/delivery/export`: `pdf_test.go`
- `internal/e2e`: `alert_simulation_test.go`, `consol_ic_refresh_test.go`
- `internal/elimination`: `engine_test.go`
- `internal/finance/banking`: `import_test.go`, `service_test.go`
- `internal/fixedassets`: `service_test.go`
- `internal/insights`: `service_test.go`
- `internal/inventory`: `service_test.go`
- `internal/masterdata/products`: `service_test.go`
- `internal/observability`: `alerts_test.go`, `metrics_test.go`
- `internal/perf`: `bench_http_test.go`, `bench_jobs_test.go`
- `internal/platform/cache`: `redis_test.go`
- `internal/procurement`: `service_test.go`
- `internal/rbac`: `middleware_test.go`
- `internal/roles`: `service_test.go`
- `internal/sales/customers`: `service_test.go`

---

## 5. Actionable Guidelines for Workers and Reviewers

### 5.1 Step-by-Step Verification Protocol
Before marking any UI refactoring task complete, Workers and Reviewers must run:

1. **Template Validity & UI Contract Check**:
   ```bash
   go test -v ./internal/view/...
   ```
   *Verifies template syntax parsing, route handler template matching, and UI contract compliance (BEM classes, accessibility, zero inline styles).*

2. **Domain-Specific Handler Test Check**:
   ```bash
   # Run tests for the specific domain modified, e.g.:
   go test -v ./internal/auth/...
   go test -v ./internal/consol/http/...
   ```

3. **Full System Compilation**:
   ```bash
   make build
   # or: go build ./...
   ```
   *Verifies Go binary compilation and static file embedding integrity.*

4. **Complete Automated Test Suite**:
   ```bash
   ODYSSEY_TEST_MODE=1 go test ./...
   ```
   *Verifies zero regressions across all 64 test files in the codebase.*

### 5.2 UI Refactoring Rule & Pitfall Checklist
When editing any template file under `web/templates/`:

- [ ] **Buttons**: All `<button>` tags must include `class="btn ..."` (e.g. `btn btn--primary`). Do NOT use `role="button"`.
- [ ] **Icon Buttons**: Icon-only buttons without visible text MUST specify `aria-label="..."`.
- [ ] **Tables**: All `<table>` tags must have `class="table ..."` and be wrapped in a container with a class such as `table-responsive`, `table-wrap`, or `overflow-x-auto`.
- [ ] **Report Tables**: Tables in report templates must include a `<caption>` or `aria-labelledby="..."`.
- [ ] **Form Elements**: Use `.form-input`, `.form-select`, `.form-textarea`, `.check__input` / `.checkbox-input`.
- [ ] **No Inline Styling**: Do NOT add `style="..."` attributes to HTML tags in migrated pages (`pages/sales`, `pages/delivery`, `pages/inventory`, `pages/accounting`, `pages/finance`, `pages/masterdata`, `pages/ap`, `pages/ar`, `pages/close`, `pages/eliminations`, `pages/variance`). All styles belong in `web/static/css/`.
- [ ] **No Inline Scripts**: Do NOT embed `<script>` blocks inside domain page templates.
- [ ] **No Fixed Grid Utilities**: Do NOT use `grid-cols-2`, `grid-cols-3`, `grid-cols-4` fixed grid utility classes in templates.
- [ ] **Template Reference Names**: When referencing templates in Go handler code, use the exact path starting from `pages/` (e.g., `pages/sales/orders.html`).
- [ ] **HTTP Response Status**: In HTTP handlers, render templates via `templates.Render(w, name, data)` or `templates.RenderStatus(w, name, data, status)`. Never call `w.WriteHeader` prior to rendering.

---
