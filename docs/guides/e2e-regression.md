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

## Known blockers recorded 2026-08-10

The static, database, unit-test, build, and production/PDF gates passed locally on
the final working tree. The full `TestRegressionFlow` sweep remains red and must be
treated as a release blocker. The failures are application gaps, not reasons to
weaken the sweep or mark the affected feature rows complete.

| Area | Observed failure | Required follow-up |
|---|---|---|
| Accounting banks | Bank-statements rendering expects `.StatementDate.Time`, while the loaded value is `time.Time`. | Align the view model and template type, then add a rendering regression test. |
| CMMS | PM schedules and selected dashboard forms fail on missing template fields or empty CSRF values. | Populate the complete view model and handle CSRF generation failures explicitly. |
| Distribution | Loads, planning horizons, routes, and transfers return short placeholder responses instead of the application shell. | Complete the advertised UI handlers or keep the capabilities partial and remove them from the advertised route sweep. |
| Documents | `/documents/search` returns 400 and workspace rendering can emit an empty CSRF token. | Define the search request contract and verify the workspace session/CSRF path with seeded data. |
| Finance banking/forecasting | Banking detail has a template data-shape mismatch; the latest forecasting page returns 400. | Replace `interface{}` view data with typed page models and add route-level smoke tests. |
| MRP | BOM pages return 400 and several advanced pages return placeholder responses or incomplete view data. | Complete the page contracts and retain the feature-matrix `partial` status until the workflows are integrated. |
| Procurement | Contract and variance pages return 403 for the seeded CI account. | Add the required permissions to the CI seed or document the intentional role boundary with an authorized test account. |
| QMS | New audit, CAPA, NCR, and supplier-quality forms reference missing template fields. | Add complete option lists to the handlers and template rendering tests. |
| Bank-feed webhook | A missing provider/signature request returns 400 instead of the sweep's expected 403. | Decide and test the security contract: reject unauthorized webhook requests consistently while preserving provider-signature validation. |

Do not push or certify a release while this list is unresolved. After each fix,
rerun the focused route test, then the complete `TestRegressionFlow` sweep and the
standard CI gates above. Update this section with the verification date and remove a
row only when its acceptance test passes.
