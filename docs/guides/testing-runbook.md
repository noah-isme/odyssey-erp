# Testing Runbook

## Standard local checks

Set test mode to prevent external side effects, then run the same checks used by
the repository workflow:

```bash
export ODYSSEY_TEST_MODE=1
export GOTENBERG_URL='http://127.0.0.1:0'
make docs-check
make vet
go test ./...
go build ./...
```

Use `go test ./internal/<module>/...` to focus on a module. Tests that require a
database or an external acceptance environment document their own prerequisites
beside the relevant module or release guide.

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
claiming staging or production certification.
