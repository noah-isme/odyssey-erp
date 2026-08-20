# Odyssey ERP: Finance Sandbox Deployment

**Workflow:** [Deploy Finance Sandbox](../.github/workflows/deploy-finance-sandbox.yml)
**GitHub environment:** `finance-sandbox`
**Status:** v0.11 finance-automation certification only; not a production release

This is a deliberately separate deployment target for the cumulative
`RELEASE_PROFILE=v0.11-finance` route set. It must not reuse the v0.10 staging
host, database, Redis instance, storage directory, secrets, systemd units, or
port. The existing
[v0.10 staging workflow](../.github/workflows/deploy-native.yml) remains pinned
to `RELEASE_PROFILE=v0.10-core` and migration ceiling `000124`.

## Workflow contract

The workflow is manual-only. Dispatch it with a commit, branch, or tag in
`candidate_ref`; the default is `staging`. Before building, it requires both
`000128_finance_payment_operations_permission` migration files and verifies that
the candidate's newest migration is `000128` or newer. It runs the deterministic
Midtrans Iris, payment recovery, settlement, metrics, and worker-handler contract
tests before building one immutable Linux artifact, records the profile and
migration boundary, publishes checksums and an SPDX SBOM, and then uploads that
exact artifact to the sandbox host.

Configure these secrets in the GitHub `finance-sandbox` environment:

| Secret | Purpose |
| --- | --- |
| `FINANCE_SANDBOX_HOST` | Dedicated sandbox VPS hostname or address |
| `FINANCE_SANDBOX_USER` | SSH deployment user |
| `FINANCE_SANDBOX_SSH_KEY` | Private key for the deployment user |

Do not put production or v0.10 staging credentials in this environment.

## VPS configuration

Use a dedicated application root and database/Redis namespace:

```text
/opt/odyssey-finance-sandbox/
├── .env
├── current -> releases/<short-sha>
└── releases/<short-sha>/
    ├── odyssey
    ├── worker
    ├── bootstrap-admin
    ├── migrate
    ├── migrations/
    ├── web/
    └── certification/
```

Create `/opt/odyssey-finance-sandbox/.env` outside release directories. Keep
the database, Redis, board-pack storage, session secrets, and provider vault
references exclusive to this sandbox:

```bash
APP_ENV=finance-sandbox
RELEASE_PROFILE=v0.11-finance
APP_ADDR=:8280
WORKER_METRICS_ADDR=127.0.0.1:9191
LOG_FORMAT=json

PG_DSN=postgres://finance_sandbox:<password>@db-host:5432/odyssey_finance_sandbox?sslmode=require
REDIS_ADDR=redis-finance-sandbox-host:6379
BOARD_PACK_STORAGE=/var/lib/odyssey-finance-sandbox/boardpacks

SESSION_SECRET=<sandbox-only-random-secret>
SESSION_TTL=720h
CSRF_SECRET=<different-sandbox-only-random-secret>
APP_MASTER_KEY=<sandbox-only-vault-key>
CONNECTORS_DEVELOPMENT_MODE=false
GOTENBERG_URL=http://127.0.0.1:3000
```

`APP_ENV=finance-sandbox` is validated by `internal/app/config.go`: the
release profile must be explicit and must be exactly `v0.11-finance`. The
worker loads the same environment file as the HTTP process, so its queue and
database connections remain in the finance sandbox namespace.

Create systemd units named exactly:

```text
odyssey-finance-sandbox.service
odyssey-finance-sandbox-worker.service
```

Both units should run as the non-root `odyssey` user with
`WorkingDirectory=/opt/odyssey-finance-sandbox/current` and
`EnvironmentFile=/opt/odyssey-finance-sandbox/.env`. Grant the deployment user
only passwordless restart/status access for these two units. The workflow
verifies that the worker unit is active and checks
`http://127.0.0.1:8280/healthz` after each deployment. Keep the application
bound to localhost behind a TLS-terminating reverse proxy, or otherwise
provide secure transport before exposing the sandbox beyond the VPS.

## Certification boundary

The sandbox profile exposes the v0.10 core routes plus the v0.11 finance
automation routes. It is an evidence environment, not a production claim:
keep `production-certified=no` until provider, accounting-effect, recovery,
security, and operational evidence is complete. Preserve the artifact digest,
migration status, provider test results, queue/worker logs, finance recovery
metrics, and rollback evidence with the [v0.11 finance sandbox certification
record](releases/v0.11-finance-sandbox-certification.md).

The finance worker registers `finance:payment_recovery_scan` only when both
`APP_ENV=finance-sandbox` and `RELEASE_PROFILE=v0.11-finance`. It scans
`payment_executions`, `payment_settlement_results`, settlement effect links, and
the finance automation outbox every five minutes. The scan is read-only with
respect to payment state: it emits stable, company-scoped notifications for
ambiguous, partial, failed, unapplied-effect, unmatched, stalled, and
dead-letter cases, and never performs an automatic provider lookup or blind
resubmission. An operator must use the guarded operations workbench to resolve
the case.

Before marking the deployment healthy, verify the worker metrics listener is
reachable only on `127.0.0.1:9191` and retain output containing the finance
payment execution, recovery-attempt, and provider-lookup metric families. The
rollback drill must record the previous release target, restore that exact
artifact, verify web/worker health, then restore the candidate and repeat the
health and metric checks. Do not run an untested down migration against the
sandbox database.

Before enabling a live batch, provision a company-scoped active source bank
account with a GL account, an `AP / ap.payment.ap` mapping, an open accounting
period covering the settlement date, and a posted AP invoice on every live
batch item. Provider fees use `AP / ap.payment.fee`; existing installations may
temporarily use the seeded `AP / ap.payment.fx_loss` expense mapping.
