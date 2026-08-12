# Production Release Checklist

**State:** `v0.10.0-rc.5` uses application baseline `ec65cc0`; the candidate
packaging commit, production promotion, and final tag approval are still pending.

**Reviewed:** 2026-08-12

This is the final-release runbook for Odyssey ERP. It does not turn local tests
into production certification. The [authoritative feature matrix](../reference/feature-matrix.md)
is the release-status authority; a capability is not a production claim until
its `production-certified` evidence is recorded there.

## 1. Release identity and scope

- [x] Freeze the application baseline at
      `ec65cc08639c184030c63e3407791987eee92804` after reviewing the exact
      application diff; record the later packaging commit through the annotated
      `v0.10.0-rc.5` tag.
- [x] Set the candidate migration ceiling to
      `000124_scoped_rbac_global_compatibility`; migration
      `000125_payment_settlement_results`, v0.11-finance routes, and commit
      `1a8343e4499420467ba3dda04a2683782c6c79d7` are outside this release.
- [x] Define the bounded `v0.10-core` scope in the feature matrix. The profile
      includes AR/AP, sales/delivery, inventory/stock-take, document control,
      and CMMS maintenance foundations only.
- [ ] Confirm the deployment environment sets `RELEASE_PROFILE=v0.10-core`.
      `full` is allowed only when every matrix row is certified; an unset or
      unknown profile is a release-gate failure.
- [ ] Confirm the working tree is clean, excluding generated `graphify-out/`
      state, and review every migration, configuration, and documentation change.
- [ ] Create a signed final release tag only after all required gates below pass.
- [ ] Record the commit, image digest, migration range, and rollback target in
      the release notes.

The `v0.10.0-rc.5` candidate is a packaging checkpoint, not a production approval.
The [v0.10-core staging certification record](v0.10-core-staging-certification.md)
is the evidence hook for the bounded profile. The final gate remains intentionally
blocked until its scope and evidence are certified by the release owner. The
candidate tag and exact commit recorded in that evidence must refer to rc.5.

## 2. Repeatable repository gates

Run with repository-local Go scratch space so test compilation does not depend on
the availability or writability of a system temporary directory:

```bash
mkdir -p .go-tmp .go-cache .golangci-cache
GOTMPDIR="$PWD/.go-tmp" \
GOCACHE="$PWD/.go-cache" \
GOLANGCI_LINT_CACHE="$PWD/.golangci-cache" \
make lint

GOTMPDIR="$PWD/.go-tmp" \
GOCACHE="$PWD/.go-cache" \
make test-migrate

GOTMPDIR="$PWD/.go-tmp" \
GOCACHE="$PWD/.go-cache" \
GOLANGCI_LINT_CACHE="$PWD/.golangci-cache" \
ODYSSEY_TEST_MODE=1 \
GOTENBERG_URL=http://127.0.0.1:0 \
make production-build-check
```

The production build check covers the full unit suite, `go vet`, the Linux
production build, and the tagged PDF build/test. `make release-check` also
validates documentation links, the release scope and four feature-matrix statuses,
and advertised route placeholder responses.

Before release, run the final gate from a clean, tagged checkout:

```bash
RELEASE_PROFILE=v0.10-core make production-release-check
```

It intentionally fails when the candidate is not tagged, release-gated tests are
blocked, the selected profile's matrix certification is missing, the VPS runbook
or profile contract is incomplete, or the candidate has uncommitted non-Graphify
changes. It also requires the staging certification record to identify the exact
tagged candidate and contain completed evidence rather than the template,
unchecked-box, pending-result, or placeholder state. For `v0.10-core`, rows marked
`v0.10.0 scope=no` are not certification requirements; they remain outside the
production route claim.

The seeded local HTTP regression sweep was reverified on 2026-08-12: 143 page
routes, 65 parameterised route patterns, 316 guarded mutation routes, and the
bank-feed webhook boundary contract passed. This clears the local route
regressions documented in the [E2E regression guide](../guides/e2e-regression.md);
it does not replace the staging, provider, security, or operational evidence
listed below.

## 3. Database and migration evidence

- [ ] Take and verify a production backup before applying migrations.
- [ ] Apply all migrations to an isolated staging clone and run the complete
      migration-backed test suite.
- [ ] Exercise the newest up and down path where rollback is supported; document
      irreversible migrations and the rollback procedure.
- [ ] Restore a backup into a clean database and run post-restore integrity checks.
- [ ] Confirm PostgreSQL roles, TLS, connection limits, extensions, and tenant
      isolation settings match the deployment environment.
- [ ] Record the final migration version and schema checksum.
- [ ] Confirm the staging migration history ends at
      `000124_scoped_rbac_global_compatibility`; do not apply
      `000125_payment_settlement_results` to the v0.10-core candidate database.

## 4. External and regulated certification

- [x] Run the deterministic Coretax export/validator/GL reconciliation contract
      test and retain its export hash and totals in the build evidence.
- [x] Run the annual PPh 21 release fixture from PMK 168/2023, including the
      December correction and the signed over-withholding case.
- [ ] Supply the official Coretax XSD/converter and approved representative-month
      staging/import evidence; the local contract test is not authority acceptance.
- [ ] Review the effective annual PPh 21 rule version and company-specific
      December payroll evidence with payroll/legal owners.
- [ ] Run provider sandbox/limited-production contract tests for every connector
      included in the release, including signatures, retries, idempotency, and
      replay handling.
- [x] Run `make midtrans-sandbox-certify` and retain its deterministic evidence for
      duplicate/out-of-order callbacks, expiry, refunds, payout reconciliation, and
      timeout recovery; merchant-account and bank-confirmed refund evidence remain
      external gates.
- [ ] Verify production connector secrets resolve from the managed secret store;
      `CONNECTORS_DEVELOPMENT_MODE` must remain `false`.
- [ ] Verify Gotenberg availability and the `production pdf` build artifact if PDF
      routes are in scope.

For rc.5, Coretax authority acceptance, payroll/legal review, and connector checks
that belong only to the v0.11-finance profile may be recorded as profile-scoped
`N/A` only when the route manifest and runtime configuration prove that they are
neither exposed nor required by the five v0.10-core journeys. Record the evidence
for each `N/A`; it is not a certification result. Any dependency exercised by a
core journey, including Gotenberg when required by the document workflow, remains
in scope.

## 5. Runtime and security controls

- [ ] Use managed PostgreSQL with backups and restore monitoring, persistent Redis,
      managed object storage, and a production Gotenberg endpoint.
- [ ] Set strong rotated `SESSION_SECRET`, `CSRF_SECRET`, and application/master
      keys from the secret manager; do not use `.env.example` values.
- [ ] Terminate HTTPS at the owned edge, enforce secure cookies, and document TLS
      renewal and reverse-proxy health checks.
- [ ] Configure structured logs, metrics, alerts, audit-log access, error-rate
      thresholds, worker queue monitoring, payment recovery metrics from
      `WORKER_METRICS_ADDR`, and database/Redis capacity alerts.
- [ ] Run the backup/restore and disaster-recovery drill, recording measured RPO/RTO.
- [ ] Confirm rate limits, admin bootstrap controls, RBAC assignments, session
      revocation, and incident-response ownership.

## 6. Deployment and rollback

- [ ] Build and scan the immutable image; record its digest and SBOM.
- [ ] Deploy the web and worker artifacts from the same commit/image family.
- [ ] Run migrations as a controlled pre-deploy step with an observable lock and
      a tested failure path; do not rely on a free-tier demo command for production.
- [ ] Run `/healthz`, authentication, session, one representative financial
      transaction, one worker task, and any scoped connector/PDF smoke tests.
- [ ] Monitor error rate, latency, queue depth, database health, and audit events
      through the observation window.
- [ ] Keep the previous image, schema rollback decision, and operator contacts
      ready before promoting traffic.
- [ ] Record the go/no-go decision and post-deploy evidence in the release notes.

## Current blockers

The current repository deliberately does not claim production release readiness:

- `v0.10.0-rc.5` is a release candidate, not a production-certified final release;
- the application baseline is `ec65cc0`, the candidate is resolved by the rc.5
  tag, and its migration set must stop at `000124`; v0.11-finance commit
  `1a8343e` and migration `000125` are excluded;
- the feature matrix records `production-certified=no` for the current capability
  rows until staging/provider/operational evidence is supplied;
- local Coretax and annual PPh 21 release tests pass, but official Coretax
  staging/import and payroll/legal production evidence remain outstanding;
- the self-managed VPS deployment still requires completed backup/restore,
  TLS, monitoring, smoke, and rollback evidence;
- final staging, provider, security, migration, and operational evidence is still
  required before promoting the candidate to `v0.10.0`.

These are release controls, not suggestions to bypass. Complete the evidence,
then update the matrix and release notes together.
