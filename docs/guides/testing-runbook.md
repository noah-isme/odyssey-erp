# Testing Runbook

## Standard local checks

Set test mode to prevent external side effects, then run the same checks used by
the repository workflow:

```bash
export ODYSSEY_TEST_MODE=1
export GOTENBERG_URL='http://127.0.0.1:0'
make release-check
make pdf-release-check
make vet
go test ./...
go build ./...
```

Use `go test ./internal/<module>/...` to focus on a module. Tests that require a
database or an external acceptance environment document their own prerequisites
beside the relevant module or release guide.

## Coverage maintenance

Use focused coverage runs when changing a high-risk module. The following
scope covers the modules included in the current coverage audit:

```bash
GOCACHE=/tmp/odyssey-go-cache \
ODYSSEY_TEST_MODE=1 \
GOTENBERG_URL='http://127.0.0.1:0' \
go test -count=1 -p 1 -cover \
  ./internal/cmms/... \
  ./internal/documents/... \
  ./internal/qms/... \
  ./internal/dashboard/... \
  ./internal/distribution/... \
  ./internal/logistics/... \
  ./internal/portal/... \
  ./internal/reporting/... \
  ./internal/search/... \
  ./internal/outbox/... \
  ./internal/masterdata/... \
  ./internal/accounting/... \
  ./internal/sales/... \
  ./internal/delivery/... \
  ./internal/finance/... \
  ./internal/hr/... \
  ./internal/connectors/...
```

Package coverage is a signal for prioritization, not a release claim by
itself. A package with no tests may still compile and be reachable through a
parent handler, while a high percentage can miss database and external-system
behavior. Pair new service, handler, repository, and provider work with tests
for validation, error paths, lifecycle transitions, and integration boundaries.

When a full parallel run exhausts local temporary storage, rerun it with
`-p 1` and a task-specific `GOCACHE` as shown above. The dated
[coverage-hardening audit](../archive/audits/test-coverage-hardening-2026-08-09.md)
records the current package baselines and remaining follow-up areas.

## Database-backed checks

Start PostgreSQL and Redis through Compose, set `PG_DSN` and `REDIS_ADDR`, then
apply migrations and seed the data before running the target package:

```bash
docker compose up -d postgres redis
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
export REDIS_ADDR='localhost:6380'
make migrate-up
make seed-phase4
```

The HTTP regression flow is run in CI from `tests/e2e`. It requires a running
application, an authenticated seeded user, and a route dump; use the workflow
definition as the authoritative invocation.

## Release gates

Some release checks deliberately require external evidence. See the
[Coretax validation sign-off guide](tax-staff-coretax-validation.md) and
[Phase 14/P7 acceptance evidence](phase14-p7-acceptance-evidence.md) before
claiming staging or production certification. The [authoritative feature matrix](../reference/feature-matrix.md)
is the release-status source; the Phase 14/P7 document is local evidence only.
