# Odyssey ERP: Next Steps

**Updated:** 2026-09-02
**Current candidate:** `v0.10.0-rc.8` (`v0.10-core`, migration ceiling `000124`)
**Release state:** local repository gates pass; staging and production certification remain open

This is the active handoff for continuing the roadmap. The release boundary and
feature scope are authoritative in [`docs/ROADMAP.md`](docs/ROADMAP.md); this file
keeps the execution order short and operational.

## 1. Close v0.10-core

Do not add feature code or migrations to rc.8. Complete the staging evidence gate:

1. Provision staging-only certification identities, stable fixture IDs, host access,
   and the immutable S3-compatible evidence bucket with seven-year COMPLIANCE
   Object Lock. Keep values in the managed secret store.
2. Set `APP_ENV=staging`, `RELEASE_PROFILE=v0.10-core`, `APP_ADDR=127.0.0.1:8180`,
   `PG_DSN`, `REDIS_ADDR`, `SESSION_SECRET`, `CSRF_SECRET`, and
   `CONNECTORS_DEVELOPMENT_MODE=false` in the staging environment.
3. Create the `certification/v0.10.0-rc.8` dispatch branch (only the superseded
   rc.7 branch exists on the remote today), then rerun the fixed-candidate
   deployment and certification workflow from that ref. Verify the deployed
   revision, route profile, health endpoint, migrations through `000124`, and
   both automated and operator lanes.
4. Merge the 25 contract rows with
   [`scripts/staging-certification-closeout.sh`](scripts/staging-certification-closeout.sh).
   The final index must be write-once, every row `PASS`, and accompanied by its
   `SHA256SUMS` object. No `N/A`, local-only, mutable URI, missing artifact, or
   incomplete Object Lock metadata can produce `GO`.
5. Run the production gate from a clean, exact tagged checkout with the final
   evidence index URI and local file. Promote only after signed approval, backup/
   restore, rollback, security, provider, and observation evidence is complete.

Current external blocker: the configured staging identities, fixture variables,
and immutable evidence-store inputs are not present, so the latest certification
preflight cannot run to completion. The most recent fixed-candidate staging
deployment succeeded; deployment success alone is not certification.

## 2. Start v0.11-finance after v0.10 promotion

Branch from the released v0.10 baseline and use the cumulative
`RELEASE_PROFILE=v0.11-finance` profile. The first tranche is **Treasury + P2P**:

The isolated preparation candidate and its open gates are tracked in the
[v0.11-finance preparation handoff](docs/releases/v0.11-finance-prep-handoff.md).

- finish bank-feed ingestion, cash forecasting, treasury controls, and operational
  recovery paths;
- complete payment execution, settlement-result handling, reconciliation, retry,
  and durable audit effects; and
- close the existing purchase-to-pay loop from requisition/PO through receipt, AP,
  approval, payment, and ledger effects.

Keep live provider execution and tenant/company feature flags disabled by default.
Require deterministic unit/integration coverage, provider sandbox evidence, staging
journeys, scoped-access checks, migration rehearsal, and operational rollback before
any live enablement. Record the new candidate, migration ceiling, route manifest,
and evidence contract as a separate release line; do not reuse the rc.8 record.

## 3. Defer to v0.11.x

Asset locations, custody and transfers, warranty/maintenance extensions, and asset
capitalization operations are explicitly outside the first v0.11 release gate. Keep
them as a follow-on tranche rather than widening either the rc.8 candidate or the
initial Treasury + P2P certification scope.

## Local verification before handoff

```bash
make docs-check
RELEASE_PROFILE=v0.10-core make release-check
ODYSSEY_TEST_MODE=1 GOTENBERG_URL=http://127.0.0.1:0 go test ./... -count=1
go vet ./...
bash scripts/staging-certification-closeout_test.sh
```

The local checks establish repository consistency only. They do not replace the
staging deployment, external provider, backup/restore, security, observation, or
signed release evidence required by the production checklist.
