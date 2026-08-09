# Production Release Checklist

**State:** `v0.10.0-rc.2` candidate prepared; production promotion and final tag
approval are still pending.

**Reviewed:** 2026-08-10

This is the final-release runbook for Odyssey ERP. It does not turn local tests
into production certification. The [authoritative feature matrix](../reference/feature-matrix.md)
is the release-status authority; a capability is not a production claim until
its `production-certified` evidence is recorded there.

## 1. Release identity and scope

- [x] Choose the release candidate number and date after reviewing the exact commit.
- [ ] Define the capability scope. Update the feature matrix only with evidence
      for that scope; do not promote partial capabilities by implication.
- [ ] Confirm the working tree is clean, excluding generated `graphify-out/`
      state, and review every migration, configuration, and documentation change.
- [ ] Create a signed final release tag only after all required gates below pass.
- [ ] Record the commit, image digest, migration range, and rollback target in
      the release notes.

The `v0.10.0-rc.2` candidate is a packaging checkpoint, not a production approval.
The final gate remains intentionally blocked until its scope and evidence are
certified by the release owner.

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
GOLANGCI_LINT_CACHE="$PWD/.golangci-cache" \
ODYSSEY_TEST_MODE=1 \
GOTENBERG_URL=http://127.0.0.1:0 \
make production-build-check
```

The production build check covers the full unit suite, `go vet`, the Linux
production build, and the tagged PDF build/test. `make release-check` also
validates documentation links, the four feature-matrix statuses, and advertised
route placeholder responses.

Before release, run the final gate from a clean, tagged checkout:

```bash
make production-release-check
```

It intentionally fails when the candidate is not tagged, release-gated tests are
blocked, matrix certification is missing, the Render Free blueprint is present,
or the candidate has uncommitted non-Graphify changes.

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
- [ ] Verify production connector secrets resolve from the managed secret store;
      `CONNECTORS_DEVELOPMENT_MODE` must remain `false`.
- [ ] Verify Gotenberg availability and the `production pdf` build artifact if PDF
      routes are in scope.

## 5. Runtime and security controls

- [ ] Use managed PostgreSQL with backups and restore monitoring, persistent Redis,
      managed object storage, and a production Gotenberg endpoint.
- [ ] Set strong rotated `SESSION_SECRET`, `CSRF_SECRET`, and application/master
      keys from the secret manager; do not use `.env.example` values.
- [ ] Terminate HTTPS at the owned edge, enforce secure cookies, and document TLS
      renewal and reverse-proxy health checks.
- [ ] Configure structured logs, metrics, alerts, audit-log access, error-rate
      thresholds, worker queue monitoring, and database/Redis capacity alerts.
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

- `v0.10.0-rc.2` is a release candidate, not a production-certified final release;
- the feature matrix records `production-certified=no` for the current capability
  rows until staging/provider/operational evidence is supplied;
- local Coretax and annual PPh 21 release tests pass, but official Coretax
  staging/import and payroll/legal production evidence remain outstanding;
- `render.yaml` is a Free demo/staging blueprint without a worker, persistent
  Redis, managed Gotenberg, or production database backup guarantees;
- final staging, provider, security, migration, and operational evidence is still
  required before promoting the candidate to `v0.10.0`.

These are release controls, not suggestions to bypass. Complete the evidence,
then update the matrix and release notes together.
