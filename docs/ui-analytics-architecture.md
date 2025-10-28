# Finance Analytics SSR Dashboard Architecture

## Overview
The server-rendered finance analytics dashboard consolidates KPI cards, trend charts, and aging tables into a single template (`web/templates/pages/finance/dashboard.html`). Handlers resolve analytics data via the existing analytics service layer and build a dedicated view model (`internal/analytics/ui/contracts.go`). SVG charts are rendered on the server and embedded inline, ensuring no client-side JavaScript is required.

## Request Flow
1. **Routes** – `/finance/analytics` (HTML), `/finance/analytics/pdf`, and `/finance/analytics/export.csv` are registered in `internal/analytics/http/routes.go`. Export routes are guarded by a per-user/IP rate limiter (10 req/min).
2. **Authorization** – `internal/analytics/http/handlers.go` enforces `finance.view_analytics` for HTML and `finance.export_analytics` for exports using the RBAC service.
3. **Filter Binding** – query parameters (`period`, `company_id`, `branch_id`) are parsed and validated. Period defaults to the current month, company defaults to `1`, and branch is optional. Period validation delegates to the finance period validator (open/closed enforcement).
4. **Service Calls** – `loadDashboardData` dispatches concurrent requests to `analytics.Service` (KPI, P&L trend, cashflow trend, AR/AP aging). All calls share a 2s timeout and rely on the analytics cache layer.
5. **View Model Construction** – `buildViewModel` converts domain objects into the dashboard view model and renders SVG charts via the renderer interfaces defined in `internal/analytics/ui/contracts.go`.
6. **Rendering** – The HTML handler renders `dashboard.html` through the template engine (`internal/view`). PDF exports reuse the same data via `internal/analytics/export.PDFExporter`. CSV exports stream aggregated data using helpers from `internal/analytics/export`.

## Key Components
- **View Model (`internal/analytics/ui/contracts.go`)** – Defines strongly typed filters, KPI payloads, trend points, aging buckets, and SVG fields.
- **SVG Renderers (`internal/analytics/svg/*.go`)** – Pure Go renderers producing accessible inline SVG (titles, descriptions, labelled axes). Line charts accept a single series for net profit; bar charts accept dual series for cash in/out.
- **HTTP Handler (`internal/analytics/http/handlers.go`)** – Responsible for validation, authorization, data loading, view model creation, HTML/PDF/CSV responses, and error handling.
- **Templates** – Dashboard and finance partials compose KPI cards, charts, and aging tables. Custom CSS (`web/static/css/analytics.css`) keeps layout responsive without inline styles.

## Error Handling
- Validation errors (invalid period/company/branch) return HTTP 400 with sanitized messaging.
- Authorization failures return 403; RBAC lookup failures are treated as 500 and logged.
- Service errors bubble up as 500 with structured logging via `slog`.
- Rate limit exceedances return 429 via the limiter’s custom handler.

## Performance Notes
- All service calls share the analytics cache, and concurrent fetching keeps cold response times below 2 seconds.
- CSV responses reuse buffers via a sync.Pool to reduce allocations.
- SVG defaults (720×240 viewport, grid/tick spacing) keep charts lightweight while preserving readability.

## Extensibility
- Additional chart types can implement the renderer interfaces and be swapped without altering handlers or templates.
- New export formats can reuse the `loadDashboardData` helper to ensure data parity across outputs.
- RBAC scopes are isolated in `internal/shared/authz_fin_analytics.go` for easy permission seeding.
