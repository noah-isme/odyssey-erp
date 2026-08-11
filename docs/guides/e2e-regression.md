# HTTP E2E regression

The regression suite uses Go's standard `net/http` client—no browser or test
framework dependency is required. It validates login/session/CSRF, core report
pages, reporting administration pages, and native Excel downloads against a
running application.

Run against a local or staging instance:

```bash
ODYSSEY_E2E_URL=http://127.0.0.1:8080 \
ODYSSEY_E2E_EMAIL=admin@odyssey.local \
ODYSSEY_E2E_PASSWORD=admin123 \
go test ./tests/e2e -run TestRegressionFlow -v
```

Without `ODYSSEY_E2E_URL`, the suite skips intentionally. Configure a dedicated
seeded test account in CI; do not use production credentials.

## CI-equivalent verification

The authoritative CI sequence is defined in
[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml). For local Compose,
the repository maps PostgreSQL to `localhost:5434` and Redis to `localhost:6382`,
so use those host ports when reproducing the CI database and server checks. The
route-dump and server steps also require a non-production `APP_MASTER_KEY`; without
it, startup logs can precede `routes.json`, causing the route dump to be invalid
JSON.

At minimum, a release candidate must pass the following gates on a clean database:

```bash
make release-check
make pdf-release-check
make vet
sqlc generate
go test ./...
go build ./...
```

Run the HTTP sweep only after applying migrations and loading `make seed-phase4`.
Keep the server process alive for the health check and the E2E command in the same
shell; a background process started by a short-lived shell may be terminated before
the sweep begins.

## Release-blocking regressions resolved 2026-08-12

The seeded local release sweep now passes. `TestRegressionFlow` completed with
143 HTML page routes, 65 parameterised route patterns, 316 CSRF-guarded mutation
routes, and the explicit bank-feed webhook contract. The route classifier keeps
JSON/API endpoints out of app-shell assertions while retaining their handler and
mutation coverage.

| Area | Resolution and acceptance evidence |
|---|---|
| Accounting banks | Corrected bank statement/reconciliation template types and verified the statement detail page plus bank mutations. |
| CMMS | Corrected PM schedule data access and supplied CSRF tokens to the CMMS shells; dashboard, schedule, and work-order routes pass. |
| Distribution | Kept the distribution load/planning/route/transfer endpoints as JSON contracts and covered their guarded mutations; they are not advertised as HTML pages by the sweep. |
| Documents | Blank browser searches render the document library, blank JSON searches return an empty result set, and the workspace has a CSRF token. |
| Finance banking/forecasting | Typed the banking account detail model and corrected its template fields; forecasting latest-run remains a JSON endpoint and its guarded run route passes. |
| MRP | Bare BOM pages select a seeded product when needed, page shells receive CSRF tokens, and advanced JSON endpoints are classified separately from HTML pages. |
| Procurement | Wired contract and scorecard services into the application, changed list access to the view permission, and seeded the Phase 3 administrator permissions. Contract, scorecard, and variance mutations pass. |
| QMS | Corrected the audit, CAPA, NCR, and supplier-quality form data paths; all listed QMS pages and mutations pass. |
| Bank-feed webhook | Provider callbacks intentionally bypass browser CSRF and use provider validation at the boundary; the regression contract verifies the expected validation response for an incomplete callback. |

The sweep still reports parameterised patterns for which no seeded page link was
available (for example, some distribution, CMMS, QMS, and procurement detail
routes). Those are coverage follow-ups rather than failing release-blocking
assertions. Distribution, MRP, and forecasting also retain their documented
feature-scope limitations; a passing HTTP contract does not promote them to
production certification.
