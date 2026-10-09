# Odyssey ERP: Next Steps

**Updated:** 2026-10-09
**Current candidate:** `v0.10.0-rc.11` (tag pending; `v0.10-core`, migration ceiling `000124`)
**Release state:** local repository gates pass; the local clean-schema certification run passes; staging and production certification remain open

This is the active handoff for continuing the roadmap. The release boundary and
feature scope are authoritative in [`docs/ROADMAP.md`](docs/ROADMAP.md); this file
keeps the execution order short and operational.

## 1. Close v0.10-core

`release/v0.10.0-rc.11` is the superseded rc.10 tag `0745957` plus the rc.11 fix
commits, with no migration. Do not add feature code or migrations to rc.11.
Tagging `v0.10.0-rc.11`, creating `certification/v0.10.0-rc.11` and the identity
record commit wait for the user's approval; a tag push auto-deploys staging, so
the staging rebuild (step 2) must be finished first.

1. Prove the bundle is reproducible before the tag push: dispatch
   `deploy-native.yml` twice from the release branch with the same fixed
   `identity_tag` (the second run compares its digest with the first):

   ```bash
   gh workflow run deploy-native.yml --ref release/v0.10.0-rc.11 \
     -f candidate_tag=v0.10.0-rc.11 -f build_only=true -f identity_tag=v0.10.0-rc.11-repro
   # after the first run succeeds:
   gh workflow run deploy-native.yml --ref release/v0.10.0-rc.11 \
     -f candidate_tag=v0.10.0-rc.11 -f build_only=true -f identity_tag=v0.10.0-rc.11-repro
   ```

2. Rebuild staging from an empty database after written user approval
   (`FIND-020`; section 0 of the
   [staging preflight checklist](docs/releases/v0.10-staging-preflight-checklist.md)):
   forensic dump to `v0.10.0-rc.10/forensics/<UTC timestamp>/` with its key and
   SHA-256 recorded in `FIND-010`, drop/recreate, `migrate up` to `000124`,
   read-only certification role, schema checksum against a PG17 reference,
   password rotation, `go run ./scripts/seed/staging`, the storage directory
   created as root, and `DOCUMENT_STORAGE_DIR`, `RBAC_GLOBAL_ROLES=` (empty) and
   `CMMS_SYSTEM_USER_ID` in `/opt/odyssey-staging/.env`.
3. Provision the remaining staging-only certification inputs (identities, the
   fixture IDs the seeder prints, host access, and the immutable S3-compatible
   evidence bucket with seven-year COMPLIANCE Object Lock). Keep values in the
   managed secret store.
4. After approval, cut the annotated `v0.10.0-rc.11` tag, create the
   `certification/v0.10.0-rc.11` dispatch branch from it, record the identity,
   then run the certification workflow from that ref. Before **every** dispatch,
   re-run the seeder for a fresh un-invoiced GRN, update `STAGING_CERT_GRN_ID`,
   and do not dispatch on the last day of a month (`FIND-031`).
5. Verify the deployed revision, route profile, health endpoint, migrations
   through `000124`, the schema checksum gate, and both automated and operator
   lanes. Run the tenancy scope **find** queries after deploy (ENV-001 evidence).
6. Merge the 25 contract rows with
   [`scripts/staging-certification-closeout.sh`](scripts/staging-certification-closeout.sh).
   The final index must be write-once, every row `PASS`, and accompanied by its
   `SHA256SUMS` object. No `N/A`, local-only, mutable URI, missing artifact, or
   incomplete Object Lock metadata can produce `GO`.
7. Run the production gate from a clean, exact tagged checkout with the final
   evidence index URI and local file. Promote only after signed approval, backup/
   restore, rollback, security, provider, and observation evidence is complete,
   with `FIND-028` to `FIND-033` dispositioned and `FIND-019`, `FIND-022` and
   `FIND-024` signed off by the release owner.

Environments other than staging that keep their database: follow the
[tenancy scope rollout runbook](docs/releases/v0.10-tenancy-scope-rollout.md)
before upgrading, run the read-only **Find** queries in the
[approval finalization remediation runbook](docs/releases/v0.10-approval-finalization-remediation.md)
if they ran a build from rc.1 through rc.9, and run the role 1 review query in
the preflight checklist (`FIND-022`).

Current external blocker: the configured staging identities, fixture variables,
and immutable evidence-store inputs are not present, and staging has not been
rebuilt, so the certification preflight cannot run to completion. The
superseded rc.10 tag was deployed to staging; its only certification run was
invalidated and it was never certified.

## Follow-ups for main and v0.11

- Move the dev compose database to `postgres:17` (staging and the schema
  checksum gate use PG17).
- Trusted proxy ranges for `RealIP`, so document access-event IPs are not
  header-spoofable (`FIND-024`).
- A migration keyed on the role name replacing the `role_id = 1` grant of
  `000083` (`FIND-022`).
- Scoped permission evaluation (`FIND-019`, the v0.11 design:
  `ScopedPermissionReader.EffectivePermissionsInScope` / `ScopedRoute`).
- The open rc.11 findings `FIND-028` to `FIND-033` (tax configuration after
  posting, delivery document numbering migration, delivery confirm atomicity
  and journal gaps, the last-day-of-month period guard, sales/freight actor
  fallbacks and unscoped GET-by-id checks, AP invoice handlers without
  matching).

## Merge-back plan after rc.11 is tagged

Start only after `v0.10.0-rc.11` is tagged. rc.11 adds no migration, so
`migrations/` is untouched by both merges. Each merge is its own PR with the
full test matrix (`go test ./...`, `make integration-test`, lint, docs checks)
and a line in `docs/releases/VERSION_HISTORY.md`.

1. **`release/v0.10.0-rc.11` into `main`** (`main` carries post-tag HR work;
   merge-base: the superseded rc.10 tag commit `0745957`):
   - `internal/app/router.go`: both win. Keep rc.11's
     `loadWorkspaceCompanies`, `requireActiveCompany` and `releaseProfile`
     and main's HR routes.
   - `cmd/odyssey/main.go`: union of the wiring (rc.11 audit loggers, document
     storage and AP orchestrator, plus main's HR wiring).
   - `scripts/seed/*`: rc.11 wins.
   - `internal/hr/employees/service.go`: main wins (rc.11 does not change it
     after the middleware-guard decision).
   - `tests/e2e/*`: both.
   - Verify with `go test ./...`, `make integration-test`, and main's
     `tests/staging/hr_core_test.go` suite.
2. **`release/v0.10.0-rc.11` into `release/v0.11-finance-prep`** (merge-base
   with v0.11 is `e582cd3`; v0.11's diff against the superseded rc.10 tag:
   `internal/ap/orchestrator.go` 182 lines, `internal/ap/matching.go` 49,
   `internal/ap/handler.go` 205, `internal/rbac/*` about 1500 added,
   `internal/app/router.go` 442, `cmd/worker/main.go` 556,
   `cmd/odyssey/main.go` 107, `internal/cmms/service.go` 158; v0.11 has no
   `scripts/seed/staging/`):
   - `internal/ap/orchestrator.go`, `matching.go`, `handler.go`: **v0.11
     semantics win**. Re-apply rc.11's three behaviours on top as a follow-up
     commit with the same tests: inline `ProcessInvoice` before the enqueue,
     `ErrGRNHasNoPO`, and PO-line resolution by product.
   - `internal/rbac/*`: v0.11 wins untouched (rc.11 does not modify it).
   - `internal/app/router.go`: v0.11's router wins; re-apply rc.11's company
     scoping, `requireActiveCompany` and `releaseProfile` on top, and verify
     that v0.11's `ScopedRoute` reads the same session company.
   - `cmd/worker/main.go`: v0.11 wins; re-apply the worker AP wiring
     (`newWorkerAPStack`, ledger hooks and audit) and the CMMS audit logger and
     `CMMS_SYSTEM_USER_ID`.
   - `internal/cmms/*`: both (rc.11 routes and transitions plus v0.11's
     predictive additions).
   - `scripts/seed/staging/`: added wholesale from rc.11.
   - `sql/queries` and `internal/sqlc`: merge the queries, then `make sqlc-gen`.
   - `migrations/`: untouched (rc.11 adds none).

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
and evidence contract as a separate release line; do not reuse the v0.10-core record.
Carry the approval finalization fix and the rc.11 fixes to the v0.11 line first
(see the merge-back plan above); the `release/v0.11-finance-prep` branch has
neither.

## 3. Defer to v0.11.x

Asset locations, custody and transfers, warranty/maintenance extensions, and asset
capitalization operations are explicitly outside the first v0.11 release gate. Keep
them as a follow-on tranche rather than widening either the rc.11 candidate or the
initial Treasury + P2P certification scope.

## Local verification before handoff

```bash
make docs-check
RELEASE_PROFILE=v0.10-core make release-check
ODYSSEY_TEST_MODE=1 GOTENBERG_URL=http://127.0.0.1:0 go test ./... -count=1
go vet ./...
bash scripts/staging-certification-closeout_test.sh
bash scripts/check-stale-candidate-refs.sh
```

The local checks establish repository consistency only. They do not replace the
staging deployment, external provider, backup/restore, security, observation, or
signed release evidence required by the production checklist.
