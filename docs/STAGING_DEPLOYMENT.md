# ODYSSEY ERP: STAGING DEPLOYMENT GUIDE

**Branch:** `staging`
**Target:** Self-managed staging VPS
**Workflow:** `.github/workflows/deploy-native.yml`
**Status:** Staging runbook; production credentials and data are out of scope

The v0.10.0 staging certification profile is `v0.10-core`. Complete the
[v0.10-core staging certification record](releases/v0.10-core-staging-certification.md)
for the exact candidate before changing any feature-matrix row to
`production-certified=yes`.

The cumulative `v0.11-finance` profile is reserved for the next finance
automation certification cycle and is not production-certified. It must not be
used to promote the bounded v0.10.0 release.

The final release gate checks that this record names the exact candidate tag and
contains completed evidence. An untouched template, unchecked checklist item,
pending result, or unreplaced evidence placeholder cannot pass the gate.

This runbook defines the staging deployment contract. Staging is isolated from
production by GitHub environment, secrets, filesystem paths, systemd units,
application port, database, and Redis instance.

Certification has two explicit lanes. The automated lane deploys and tests the
exact candidate supplied at dispatch, validates the profile/route/migration boundary, and emits the
machine-readable `evidence-index.json`. The operator lane proves environment
identity, backup/restore, migration recovery, service/queue/alert behavior,
rollback, observation, and approvals. A green automated workflow is necessary
but is not a staging certification or a `GO` decision by itself. See the
[certification record](releases/v0.10-core-staging-certification.md) for the
evidence IDs, result policy, and decision rules.

## Deployment contract

The workflow deploys automatically only after a successful `CI` workflow for
the `staging` branch. For a v0.10-core certification, use the manual workflow
dispatch with the exact annotated candidate tag and its 40-character commit
SHA. The workflow verifies that the tag resolves to the checked-out commit and
that the same commit has a successful `CI` run before building. The current
staging branch contains work beyond the bounded profile (including v0.11
migrations), so it must never be used as the v0.10 candidate source.
It builds the Linux release artifacts once, creates `SHA256SUMS`, migration and
web manifests, an SPDX SBOM, and a provenance attestation, retains the bundle
as an immutable workflow artifact, then uploads that exact bundle to the VPS.
The VPS verifies the digests and profile/migration boundary before running
migrations, switching the release symlink atomically, restarting the staging
services, and verifying `http://127.0.0.1:8180/healthz`.

An automatic `workflow_run` deployment is refused when the checked-out commit
contains migration `000125`; use the manual candidate-tag dispatch for the
v0.10-core certification path instead of allowing the v0.11-finance line to
drift into staging.

The dispatch ref must contain the certification workflow and its evidence
collector. If `.github/workflows/certify-v010-staging.yml` is not present on the
selected GitHub ref, stop and publish the certification tooling before dispatch;
do not infer availability from an unpushed local file. The workflow accepts
`candidate_tag` and `expected_sha` inputs, so a corrected descendant candidate
(for example, `v0.10.0-rc.7`) can be certified without changing the workflow.

Configure a GitHub environment named `staging` with these secrets:

| Secret | Purpose |
| --- | --- |
| `STAGING_HOST` | Staging VPS hostname or IP address |
| `STAGING_USER` | SSH deployment user |
| `STAGING_SSH_KEY` | Private key for `STAGING_USER` |

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
artifact URL, bundle/SBOM/attestation digests, and deployment run output in the
certification evidence record.

## Certification preflight and required infrastructure

The repository deliberately does not contain staging credentials, fixture
values, host identifiers, backup data, or evidence-store access. The
certification workflow must run a preflight that reports every missing input and
exits before deployment or tests; a partial run is not evidence. Configure the
following in the GitHub `staging` environment, with secrets kept out of Git:

| Category | Required values | Provisioning requirement |
| --- | --- | --- |
| Deployment | `STAGING_HOST`, `STAGING_USER`, `STAGING_SSH_KEY` | Verify the existing isolated VPS and its systemd/sudo contract. Never reuse production credentials. |
| Test endpoint and identities | `STAGING_CERT_URL`, `STAGING_CERT_PG_DSN`, `STAGING_CERT_ADMIN_*`, `STAGING_CERT_BRANCH_*`, `STAGING_CERT_NO_ACCESS_*` | Staging-only identities and a read-only/least-privileged DSN for evidence queries. |
| Stable fixtures | `STAGING_CERT_COMPANY_ID`, `STAGING_CERT_BRANCH_ID`, `STAGING_CERT_OTHER_COMPANY_ID`, `STAGING_CERT_OTHER_BRANCH_ID`, `STAGING_CERT_CUSTOMER_ID`, `STAGING_CERT_SUPPLIER_ID`, `STAGING_CERT_PRODUCT_ID`, `STAGING_CERT_WAREHOUSE_ID`, `STAGING_CERT_GRN_ID`, `STAGING_CERT_DOCUMENT_CATEGORY_ID`, `STAGING_CERT_DOCUMENT_CLASSIFICATION_ID`, `STAGING_CERT_AMOUNT` | Non-production records covering both companies and branches; record their IDs in the evidence artifacts. |
| Immutable evidence store | `EVIDENCE_S3_ENDPOINT`, `EVIDENCE_S3_REGION`, `EVIDENCE_S3_BUCKET`, `EVIDENCE_S3_ACCESS_KEY_ID`, `EVIDENCE_S3_SECRET_ACCESS_KEY` | Pre-create an S3-compatible bucket with Object Lock enabled; verify seven-year `COMPLIANCE` retention before dispatch. |
| Operator controls | Sanitized backup, disposable migration clone, previous verified release, monitoring/alert access, host/database/Redis/storage identifiers, and named approvers | Must be available before the automated lane is considered complete. |

The exact names above are the configuration contract; do not silently rename a
missing value or substitute a test provider for a real connector required by a
journey. Missing infrastructure/configuration is a release finding and a
`NO-GO`, not an allowed `N/A`.

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
release gates. Accepted values are `v0.10-core`, `v0.11-finance`, and `full`;
staging uses `v0.10-core` so only the five bounded v0.10.0 capabilities are
exposed for certification. `v0.11-finance` selects those five capabilities plus
Finance automation, but remains not production-certified. Do not use an unset or
ad-hoc profile in a staging evidence run.

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

## Operator evidence lane

The following controls are not supplied by the HTTP harness and must be run by
an authorized operator against the isolated staging environment. Attach raw
output, timestamps, actor/host identifiers, and a SHA-256 digest to the
corresponding evidence ID; a screenshot or narrative without reproducible
output is insufficient.

| Control | Evidence IDs | Required operator evidence |
| --- | --- | --- |
| Environment and security | `ENV-001`, `ENV-003`, `OPS-001`, `OPS-002` | Host/database/Redis/storage identity, staging-only secrets/endpoints, service status/logs, health, secure cookie/CSRF/TLS checks, and no production leakage. |
| Database recovery | `DB-001`, `DB-003`, `DB-004` | Restore a sanitized production-like backup; exercise `000124` down/up only on a disposable clone; create and restore-verify the pre-deploy backup on a second clean database. |
| Queue, worker, and audit operations | `OPS-003`, `ISO-004` | Exercise delivery/retry/dead-letter convergence, forged/retried worker inputs containing another scope, and verify durable audit events and alert delivery. |
| Rollback and observation | `OPS-004`, `OPS-005` | Point the symlink to the previous verified bundle, prove health, restore the exact candidate and its digest, then observe for at least 60 minutes with request, latency, queue, database/Redis, resource, alert, incident, and handoff evidence. |
| Approval | `APR-001` | Obtain test, security, operations, and release-owner review of the complete index and operator artifacts, then sign the `GO`/`NO-GO` decision. |

Do not run an untested `000124` down migration against the live staging
database. A successful health check proves transport only; it does not close a
journey, recovery, isolation, or release-control row.

## Automated v0.10-core certification

After the manual candidate deployment succeeds, dispatch
`.github/workflows/certify-v010-staging.yml` from the certification tooling
branch with `candidate_tag` and `expected_sha` set to the exact annotated
candidate (the next bounded fix candidate is expected to be
`v0.10.0-rc.7`). The workflow verifies the successful deployment run,
`RELEASE_PROFILE=v0.10-core`, and the `000124` migration ceiling before
exercising the staging harness. It must first validate every required secret
and variable below and report all missing names before it starts a deployment
or test; it must not continue with a partial fixture set.

Configure these additional `staging` environment values without placing them
in the repository. Credential and DSN values are secrets; fixture IDs and the
positive test amount may be environment variables:

- Secrets: `STAGING_CERT_URL`, `STAGING_CERT_PG_DSN`,
  `STAGING_CERT_ADMIN_EMAIL`, `STAGING_CERT_ADMIN_PASSWORD`,
  `STAGING_CERT_BRANCH_EMAIL`, `STAGING_CERT_BRANCH_PASSWORD`,
  `STAGING_CERT_NO_ACCESS_EMAIL`, `STAGING_CERT_NO_ACCESS_PASSWORD`;
- Variables: `STAGING_CERT_COMPANY_ID`, `STAGING_CERT_BRANCH_ID`,
  `STAGING_CERT_OTHER_COMPANY_ID`, `STAGING_CERT_OTHER_BRANCH_ID`,
  `STAGING_CERT_CUSTOMER_ID`, `STAGING_CERT_SUPPLIER_ID`,
  `STAGING_CERT_PRODUCT_ID`, `STAGING_CERT_WAREHOUSE_ID`,
  `STAGING_CERT_GRN_ID`, `STAGING_CERT_DOCUMENT_CATEGORY_ID`,
  `STAGING_CERT_DOCUMENT_CLASSIFICATION_ID`, and `STAGING_CERT_AMOUNT`;
- Evidence secrets: `EVIDENCE_S3_ENDPOINT`, `EVIDENCE_S3_REGION`,
  `EVIDENCE_S3_BUCKET`, `EVIDENCE_S3_ACCESS_KEY_ID`, and
  `EVIDENCE_S3_SECRET_ACCESS_KEY` for the pre-created S3-compatible bucket
  with Object Lock enabled.

The harness must fail on missing configuration, skipped checks, unexpected
404s, accepted scope tampering, or incomplete persisted effects. It emits a
structured result for each expected evidence ID and a redacted evidence bundle
under
`<candidate-tag>/<candidate-short-sha>/<workflow-run-id>/<run-attempt>/`. Uploads use seven-year Object Lock
in `COMPLIANCE` mode and are verified with object metadata before the
certification record is updated.

The workflow must write exactly one `evidence-index.json` for the run. Each
entry contains `evidence_id`, `result`, `run_id`, `collected_utc`, `details`,
`artifact_uri`, and a 64-character lowercase `sha256`. The validator compares
the index with the registry in the certification record and rejects duplicate,
missing, or unexpected IDs; a failed check remains `FAIL` with its diagnostic
artifact. Early failures still upload candidate identity, failure logs, and the
validation result when the evidence store is reachable. Hash the index and
include it in the bundle's `SHA256SUMS`; never overwrite an existing key.

The canonical registry is
`scripts/staging-certification-contract.json` (25 evidence rows). The
collector validates the automated lane explicitly:

```bash
scripts/staging-certification-evidence.sh \
  --candidate staging-certification-candidate \
  --evidence staging-evidence \
  --contract scripts/staging-certification-contract.json \
  --lane automated
```

After the operator lane is collected, merge both lane indexes with the
write-once closeout validator. The command must use the exact candidate tag and
resolved SHA and must fail if any row is missing, duplicated, out of lane, or
still `PENDING`:

```bash
scripts/staging-certification-closeout.sh \
  --automated automated/evidence-index.json \
  --operator operator/evidence-index.json \
  --output closeout \
  --candidate-tag <candidate-tag> \
  --candidate-sha <40-hex-sha> \
  --prefix <immutable-closeout-prefix>
```

The closeout index and its `SHA256SUMS` are themselves immutable evidence; a
second invocation against the same output directory is rejected.

The automated lane covers candidate identity, profile/routes, migration ceiling,
the five core journeys, tenant isolation, and HTTP idempotency checks. It does
not replace the operator lane above: backup/restore, disposable migration,
rollback, forged worker-input checks, observation, alert review, and signed
approval must be collected and reviewed separately before the record can be
`GO`.

## Staging decision and closeout

Record `GO` only when the evidence index validates, every required automated and
operator row is `PASS` (or a justified, approved `N/A`), the exact candidate
identity and `000124` ceiling match, no critical/high finding remains, and test,
security, operations, and release-owner approvals are attached. Then update the
human-readable record from the immutable index and commit a single reviewed
closeout change to the five feature-matrix rows, Roadmap Milestone 1/2 status, release
checklist, and version history. Keep `production-certified=no` until the
separate production gate and final tag decision pass.

Record `NO-GO` for missing preflight configuration or infrastructure, any
identity/profile/migration mismatch, invalid or incomplete index, required
`FAIL`/`PENDING`, failed operator control, or open critical/high finding.
Preserve the immutable bundle, index, and diagnostic output; add an owned
finding with disposition and due date; leave the matrix unchanged; and create a
new candidate on a descendant commit if a fix is required. Never move, retag,
overwrite, or replace an existing candidate object in place.
