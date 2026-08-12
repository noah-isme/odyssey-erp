# ODYSSEY ERP: STAGING DEPLOYMENT GUIDE

**Branch:** `staging`
**Target:** Self-managed staging VPS
**Workflow:** `.github/workflows/deploy-native.yml`
**Status:** Staging runbook; production credentials and data are out of scope

The v0.10.0 staging certification profile is `v0.10-core`. Complete the
[v0.10-core staging certification record](releases/v0.10-core-staging-certification.md)
for the exact candidate before changing any feature-matrix row to
`production-certified=yes`.

The final release gate checks that this record names the exact candidate tag and
contains completed evidence. An untouched template, unchecked checklist item,
pending result, or unreplaced evidence placeholder cannot pass the gate.

This runbook defines the staging deployment contract. Staging is isolated from
production by GitHub environment, secrets, filesystem paths, systemd units,
application port, database, and Redis instance.

## Deployment contract

The workflow deploys automatically after a successful `CI` workflow for the
`staging` branch. Pushing the annotated `v0.10.0-rc.4` tag also starts the
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

An automatic `workflow_run` deployment is refused when the checked-out commit
exceeds migration `000124`; use the annotated rc.4 tag path for v0.10-core
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
APP_ADDR=:8180
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
