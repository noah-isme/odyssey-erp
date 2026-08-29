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

The workflow is manual-only. Dispatch it with an explicit commit, branch, or tag in
`candidate_ref`; there is no safe default because the v0.11 candidate must not be
silently replaced by the v0.10 `staging` ref. Before building, it requires both
`000129_ap_exception_resolution_events` migration files and verifies that the
candidate's newest migration is exactly `000129`. It builds one immutable Linux
artifact, generates `ROUTE_PROFILE.json` from that production-tagged binary,
records the profile and migration boundary, publishes checksums and an SPDX SBOM,
and then uploads that exact artifact to the sandbox host. The deployment step
rechecks the route manifest's profile and commit, every component checksum, and
the content-addressed bundle digest before it can switch `current`.

The recorded `ceba7a34102d43218700d59882435ca5ec214857` value is the clean preparation
baseline. The current working tree has additional uncommitted provider, worker,
precision, settlement, and AP-exception safeguards, so publish a new commit (or tag)
and regenerate the candidate identity before dispatching the workflow. The [v0.11-finance prep handoff](releases/v0.11-finance-prep-handoff.md)
tracks the candidate identity and remaining evidence gates.

Configure these secrets in the GitHub `finance-sandbox` environment:

| Secret | Purpose |
| --- | --- |
| `FINANCE_SANDBOX_HOST` | Dedicated sandbox VPS hostname or address |
| `FINANCE_SANDBOX_USER` | SSH deployment user |
| `FINANCE_SANDBOX_KNOWN_HOSTS` | Pinned SSH `known_hosts` entry for the dedicated host |
| `FINANCE_SANDBOX_SSH_KEY` | Private key for the deployment user |

Do not put production or v0.10 staging credentials in this environment.

The workflow refuses trust-on-first-use host discovery. Populate
`FINANCE_SANDBOX_KNOWN_HOSTS` from a reviewed host-key record and ensure it contains
the exact `FINANCE_SANDBOX_HOST` entry before dispatching a deployment.

## VPS configuration

Use a dedicated application root and database/Redis namespace:

```text
/opt/odyssey-finance-sandbox/
├── .env
├── current -> releases/<bundle-sha256>
└── releases/<bundle-sha256>/
    ├── odyssey
    ├── worker
    ├── bootstrap-admin
    ├── migrate
    ├── ROUTE_PROFILE.json
    ├── migrations/
    └── web/
```

Create `/opt/odyssey-finance-sandbox/.env` outside release directories. Keep
the database, Redis, board-pack storage, session secrets, and provider vault
references exclusive to this sandbox:

```bash
APP_ENV=finance-sandbox
RELEASE_PROFILE=v0.11-finance
APP_ADDR=127.0.0.1:8280
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

The application and worker wire the Midtrans Iris adapter in sandbox-only mode
when `APP_ENV=finance-sandbox`. Credentials marked `is_prod=true`, known
production Midtrans hosts, and unapproved custom hosts are rejected before any
provider request. Keep the connection secret on the documented sandbox
endpoint and do not reuse production payout credentials in this environment.

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
only passwordless restart/status access for these two units. The workflow transfers
and attests one release bundle named with the candidate SHA, stores it under its
content SHA-256 digest, verifies that digest before extraction, and checks the recorded commit/ref/revision
and migration ceiling against that candidate. It also verifies that every migration
has a paired up/down file. Bank-feed polling and signed statement transport serialize
work per connection through a PostgreSQL advisory lease; duplicate queued sync tasks
use a stable connection-scoped task ID. The generic bank-feed scheduler remains
disabled until an approved provider and recovery contract are provisioned. The
workflow then verifies that the worker unit is active and checks
`http://127.0.0.1:8280/healthz` after each deployment. Keep the application
bound to localhost behind a TLS-terminating reverse proxy, or otherwise
provide secure transport before exposing the sandbox beyond the VPS.

## Migration rehearsal and evidence collection

The deployment's `migrate up` is an apply check, not a rollback rehearsal. Run the
rehearsal against a disposable PostgreSQL clone whose name contains `rehearsal`,
`sandbox`, `test`, or `clone`; never point these commands at a shared or production
database. Use `pg_dump`/`pg_restore` clients from the same PostgreSQL major version as
the target server, record both client and server versions, and pass the database user
explicitly to restore commands. Keep the command output, applied version, schema
checksum, backup checksum, and restore output as one immutable evidence object
referenced by `DB-011` in the [v0.11-finance evidence index](releases/v0.11-finance-evidence-index.md):

```bash
export FINANCE_REHEARSAL_DSN='postgres://.../odyssey_finance_rehearsal?sslmode=require'
export FINANCE_RESTORE_DSN='postgres://.../odyssey_finance_rehearsal_restore?sslmode=require'
export MIGRATE_BIN=/opt/odyssey-finance-sandbox/current/migrate

pg_dump --version
pg_restore --version
"$MIGRATE_BIN" -path migrations -database "$FINANCE_REHEARSAL_DSN" up
"$MIGRATE_BIN" -path migrations -database "$FINANCE_REHEARSAL_DSN" version
"$MIGRATE_BIN" -path migrations -database "$FINANCE_REHEARSAL_DSN" down 1
"$MIGRATE_BIN" -path migrations -database "$FINANCE_REHEARSAL_DSN" version
"$MIGRATE_BIN" -path migrations -database "$FINANCE_REHEARSAL_DSN" up
"$MIGRATE_BIN" -path migrations -database "$FINANCE_REHEARSAL_DSN" version
pg_dump --dbname="$FINANCE_REHEARSAL_DSN" --format=custom \
  --file=finance-rehearsal.dump
sha256sum finance-rehearsal.dump
# Create FINANCE_RESTORE_DSN as an empty disposable database before restoring.
pg_restore --exit-on-error --no-owner --dbname="$FINANCE_RESTORE_DSN" \
  finance-rehearsal.dump
"$MIGRATE_BIN" -path migrations -database "$FINANCE_RESTORE_DSN" version
pg_dump --dbname="$FINANCE_RESTORE_DSN" --schema-only --no-owner --no-privileges \
  | sha256sum
```

The route/profile manifest is produced by `ODYSSEY_DUMP_ROUTES=1` with
`RELEASE_PROFILE=v0.11-finance` and must be retained with the bundle. The tenant and
security rows require recorded two-company/branch negative cases, not only local
unit tests. Provider and rollback rows remain `UNVERIFIED` until the approved
provider fixture, backup, and previous verified bundle have been exercised.

## Certification boundary

The sandbox profile exposes the v0.10 core routes plus the v0.11 finance
automation routes. It is an evidence environment, not a production claim:
keep `production-certified=no` until provider, accounting-effect, recovery,
security, and operational evidence is complete. Preserve the artifact digest,
migration status, provider test results, queue/worker logs, and rollback
evidence with the finance certification record.

Before enabling a live batch, provision a company-scoped active source bank
account with a GL account, an `AP / ap.payment.ap` mapping, an open accounting
period covering the settlement date, and a posted AP invoice on every live
batch item. Provider fees use `AP / ap.payment.fee`; existing installations may
temporarily use the seeded `AP / ap.payment.fx_loss` expense mapping.
