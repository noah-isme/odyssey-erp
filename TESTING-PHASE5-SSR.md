# Testing Plan – Phase 5 SSR Dashboard

## Automated Tests
- `go test ./...` covers:
  - `internal/analytics/http`: handler authorization, filter validation, CSV rendering.
  - `internal/analytics/svg`: snapshot sanity for line/bar renderers.
  - Existing analytics service tests ensure cache behaviour.

## Manual / Integration Checks
1. Start application: `make run`.
2. Open `http://localhost:8080/finance/analytics?period=2025-01&company_id=1` with a user that has `finance.view_analytics`.
3. Verify KPI cards, trend charts, and aging tables render with responsive layout.
4. Trigger exports:
   - `make analytics-dashboard-pdf` → check PDF > 1 KB and opens correctly.
   - `make analytics-dashboard-csv` → verify CSV sections (KPI, trend, cashflow, AR, AP).
5. Ensure rate limiting: perform >10 rapid PDF requests → expect HTTP 429.
6. Validate RBAC: remove permissions and confirm endpoints return HTTP 403.
7. Measure response timings (cold vs cached) using `curl -w` to confirm <2s cold, <500ms cached.
