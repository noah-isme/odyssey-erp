# ODYSSEY ERP: STAGING DEPLOYMENT GUIDE

**Branch:** `staging`
**Target:** Self-managed staging VPS
**Workflow:** `.github/workflows/deploy-native.yml`
**Status:** Staging runbook; production credentials and data are out of scope

The v0.10.0 staging certification profile is `v0.10-core`. Complete the
[v0.10-core staging certification record](releases/v0.10-core-staging-certification.md)
for the exact candidate before changing any feature-matrix row to
`production-certified=yes`.

The current candidate is the immutable annotated tag `v0.10.0-rc.10` (commit
`<pending tag>`, recorded when the tag is cut). Its lineage is the
superseded rc.9 tag `07d2ba2` plus the rc.10 approval finalization fix; it adds no
migration and keeps the `000124` ceiling. Do not move, recreate, or replace that tag while
collecting evidence.

Staging ran earlier candidates (most recently the superseded rc.9 tag), so
after rc.10 is deployed run the read-only **Find** queries in the
[approval finalization remediation runbook](releases/v0.10-approval-finalization-remediation.md)
against the staging database and record every repair a business owner
confirms. Deploying rc.10 stops new damage but does not repair rows that the
approval engine already finalized wrongly.

The final release gate checks that this record names the exact candidate tag and
contains completed evidence. An untouched template, unchecked checklist item,
pending result, or unreplaced evidence placeholder cannot pass the gate.

This runbook defines the staging deployment contract. Staging is isolated from
production by GitHub environment, secrets, filesystem paths, systemd units,
application port, database, and Redis instance.

## Deployment contract

The workflow deploys automatically after a successful `CI` workflow for the
`staging` branch. Pushing the annotated `v0.10.0-rc.10` tag also starts the
release-candidate deployment, so the candidate can run even before this
workflow reaches the repository's default branch. A manual dispatch using the
same tag remains available once the workflow is on the default branch. Every
path verifies that the tag resolves to the checked-out commit and that the same
commit has a successful `CI` run before building.
It builds the Linux release artifacts once, creates `SHA256SUMS`, migration and
web manifests, and an SPDX SBOM, then creates a GitHub build-provenance
attestation whose subject is the content-addressed release bundle. It retains
the bundle and recorded digest as a workflow artifact, then uploads that exact
bundle to the VPS.
The VPS verifies the digests and profile/migration boundary before running
migrations, switching the release symlink atomically, restarting the staging
services, and verifying `http://127.0.0.1:8180/healthz`.

The certification lane must run `gh attestation verify` against the downloaded
bundle, constrain the signer to this repository's `deploy-native.yml` workflow,
and retain the JSON verification result beside the evidence log. A textual
provenance claim without that verification output is not sufficient for
`REL-003`.

An automatic `workflow_run` deployment is refused when the checked-out commit
exceeds migration `000124`; use the annotated candidate tag path for v0.10-core
certification instead of allowing the v0.11-finance line to drift into staging.

Configure a GitHub environment named `staging` with these secrets:

| Secret | Purpose |
| --- | --- |
| `STAGING_HOST` | Staging VPS hostname or IP address |
| `STAGING_USER` | SSH deployment user |
| `STAGING_SSH_KEY` | Private key for `STAGING_USER` |
| `STAGING_KNOWN_HOSTS` | Pre-approved host-key entry for `STAGING_HOST`; verify its fingerprint out of band |

Do not place `PRODUCTION_HOST`, `PRODUCTION_USER`, or
`PRODUCTION_SSH_KEY` in the staging environment. The workflow must never
target the production environment.

The `production` Go build tag in the workflow selects the deployable release
build, including production PDF behavior. It is a compile-time build choice;
the deployment target remains staging and runtime configuration uses
`APP_ENV=staging` and `RELEASE_PROFILE=v0.10-core`.

For the v0.10-core candidate, the uploaded bundle must contain migrations only
through `000124_scoped_rbac_global_compatibility`; migration `000125` and
v0.11-finance-only routes are outside this deployment profile. Keep the workflow
artifact URL, component and SBOM digests, provenance-attestation URL and
verification output, and deployment run output in the certification evidence
record.

## VPS layout

Provision the staging host with an application user, a service user, and an
isolated root. The SSH deployment user must own the release directory and the
root directory so it can upload releases and update `current`; the service
user only needs to read the release files.

```bash
sudo useradd --system --create-home --home-dir /var/lib/odyssey \
  --shell /usr/sbin/nologin odyssey
sudo useradd --create-home --groups odyssey odyssey-deploy

sudo install -d -o odyssey-deploy -g odyssey -m 0755 /opt/odyssey-staging
sudo install -d -o odyssey-deploy -g odyssey -m 0755 \
  /opt/odyssey-staging/releases
sudo install -o odyssey -g odyssey -m 0640 /dev/null \
  /opt/odyssey-staging/.env
```

The deployment layout is:

```text
/opt/odyssey-staging/
├── .env
├── current -> releases/<short-sha>
└── releases/<short-sha>/
    ├── odyssey
    ├── worker
    ├── bootstrap-admin
    ├── migrate
    ├── migrations/
    └── web/
```

Create `/opt/odyssey-staging/.env` outside release directories:

```bash
APP_ENV=staging
RELEASE_PROFILE=v0.10-core
APP_ADDR=127.0.0.1:8180
LOG_FORMAT=json

PG_DSN=postgres://odyssey_staging:password@db-host:5432/odyssey_staging?sslmode=require
REDIS_ADDR=redis-staging-host:6379

SESSION_SECRET=replace-with-a-staging-only-random-secret
SESSION_TTL=720h
CSRF_SECRET=replace-with-a-different-staging-only-secret

CONNECTORS_DEVELOPMENT_MODE=false
GOTENBERG_URL=http://127.0.0.1:3000
```

Use a staging-only database and Redis namespace. Never point staging at the
production `PG_DSN`, `REDIS_ADDR`, session secrets, or connector credentials.

`RELEASE_PROFILE` is required explicitly by the application configuration and
release gates. Accepted values are `v0.10-core` and `full`; staging uses
`v0.10-core` so only the five bounded v0.10.0 capabilities are exposed for
certification. Do not use an unset or ad-hoc profile in a staging evidence run.

### Worker database pool and PostgreSQL settings

Since rc.9 the worker opens its PostgreSQL pool with 16 connections by default.
The worker runs 5 concurrent tasks and logs a startup warning when the
effective pool is below 3 x that concurrency (15): AP invoice processing holds
one connection for its advisory lock while nested transactions borrow more. The
web process ignores the setting below and keeps pgx's default pool size
(`max(4, NumCPU)`).

To change the worker pool size, set `PG_MAX_CONNS` in
`/opt/odyssey-staging/.env` (a positive integer, for example
`PG_MAX_CONNS=24`; omit the variable rather than leaving it empty). The
worker resolves its pool size in this order:

1. `PG_MAX_CONNS`, when set (the supported setting);
2. pgx's `pool_max_conns` parameter in the worker's `PG_DSN`, if present;
3. the built-in default of 16.

Do not put `pool_max_conns` into the shared `/opt/odyssey-staging/.env`
`PG_DSN`: the deployment's `migrate` step connects with the same `PG_DSN`
through `lib/pq`, which forwards unknown parameters to the server as run-time
settings, and PostgreSQL rejects `pool_max_conns`. `PG_MAX_CONNS` is a separate
variable, so it is safe in the shared file and does not reach `migrate`.

Before deploying the candidate, record in the preflight:

- `SHOW max_connections;` and the current connection count
  (`SELECT count(*) FROM pg_stat_activity;`): the worker can now hold up to 16
  connections (up to 12 more than before rc.9) in addition to the web pool,
  `migrate`, and operator sessions; confirm the headroom.
- `SHOW idle_in_transaction_session_timeout;`: record the raw output; no
  minimum is required. Payslip delivery holds the payslip row lock in an open
  transaction while it renders and sends the email (bounded by a 10s SMTP dial
  deadline, a 60s SMTP I/O deadline, and the 3-minute task timeout), and AP
  invoice processing holds an idle transaction for its advisory lock. Both
  set a transaction-local `idle_in_transaction_session_timeout` themselves
  (4 minutes for payslip delivery, 6 minutes for the AP lock; just above their
  task timeouts of 3 and 5 minutes), which overrides the server, database, and
  role setting for those transactions only, so a lower or unset server value
  neither kills a healthy send nor leaves an orphaned lock indefinitely. The
  database role must be allowed to set the parameter (the default for ordinary
  roles).

Payslip email delivery is at-least-once: `delivered_at` is committed only after
the SMTP server accepts the message, so a crash or lost database connection
between that acceptance and the commit re-sends the payslip on retry (recorded
as `FIND-006` in the certification record). A concurrent duplicate task skips
the locked row instead of sending.

## Systemd services

Create `/etc/systemd/system/odyssey-staging.service`:

```ini
[Unit]
Description=Odyssey ERP Staging Application
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=odyssey
Group=odyssey
WorkingDirectory=/opt/odyssey-staging/current
EnvironmentFile=/opt/odyssey-staging/.env
ExecStart=/opt/odyssey-staging/current/odyssey
Restart=on-failure
RestartSec=10
StartLimitIntervalSec=60
StartLimitBurst=3
KillMode=process
KillSignal=SIGTERM
TimeoutStopSec=30
StandardOutput=journal
StandardError=journal
SyslogIdentifier=odyssey-staging
PrivateTmp=yes
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

Create `/etc/systemd/system/odyssey-staging-worker.service`:

```ini
[Unit]
Description=Odyssey ERP Staging Worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=odyssey
Group=odyssey
WorkingDirectory=/opt/odyssey-staging/current
EnvironmentFile=/opt/odyssey-staging/.env
ExecStart=/opt/odyssey-staging/current/worker
Restart=on-failure
RestartSec=10
StartLimitIntervalSec=60
StartLimitBurst=3
KillMode=process
KillSignal=SIGTERM
TimeoutStopSec=30
StandardOutput=journal
StandardError=journal
SyslogIdentifier=odyssey-staging-worker
PrivateTmp=yes
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

Enable the services and grant the deployment user only the required
passwordless systemd operations:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now odyssey-staging.service odyssey-staging-worker.service
```

For example, create `/etc/sudoers.d/odyssey-staging-deploy` with `visudo`:

```text
odyssey-deploy ALL=(root) NOPASSWD: /usr/bin/systemctl restart odyssey-staging.service, /usr/bin/systemctl restart odyssey-staging-worker.service, /usr/bin/systemctl --no-pager --full status odyssey-staging.service, /usr/bin/systemctl --no-pager --full status odyssey-staging-worker.service
```

Validate the rule before enabling the workflow:

```bash
sudo visudo -cf /etc/sudoers.d/odyssey-staging-deploy
sudo -u odyssey-deploy sudo -n systemctl status odyssey-staging.service
```

The deployment workflow refuses to start a second supervised process if the
systemd restart path is unavailable and port `8180` is already occupied. Keep
`ss` installed on hosts that use this fallback and resolve the owning process
through the staging service account before retrying.

## Verification and rollback

After a deployment, verify the local health endpoint and service logs:

```bash
curl -fsS http://127.0.0.1:8180/healthz
sudo systemctl --no-pager --full status odyssey-staging.service odyssey-staging-worker.service
sudo journalctl -u odyssey-staging.service -u odyssey-staging-worker.service -n 100
```

Record the deployment commit, artifact digest, migration/schema checksum,
backup/restore result, core capability journeys, tenant-isolation tests, and
the 60-minute observation window in the [staging certification record](releases/v0.10-core-staging-certification.md).
The deployment health check is only a transport check; it is not feature or
production certification.

The workflow runs migrations before changing the `current` symlink. To roll
back application code, point the symlink at a previously verified staging
release and restart only the staging services:

```bash
sudo ln -sfn /opt/odyssey-staging/releases/<previous-short-sha> \
  /opt/odyssey-staging/current
sudo systemctl restart odyssey-staging.service odyssey-staging-worker.service
curl -fsS http://127.0.0.1:8180/healthz
```

Database rollback requires a tested staging backup and a migration-specific
recovery procedure. Do not run production rollback commands against staging or
vice versa.
