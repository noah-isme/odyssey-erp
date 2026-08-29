# Odyssey ERP: Next Steps and Release Decision Guide

**Reviewed:** 2026-08-29

**v0.10-core candidate:** `v0.10.0-rc.7` (immutable; migration ceiling `000124`)

**Current v0.11-finance prep baseline:** `release/v0.11-finance-prep` at
`ceba7a34102d43218700d59882435ca5ec214857` (working tree has uncommitted prep changes)

**Migration ceiling:** `000129_ap_exception_resolution_events`

**Status:** isolated preparation candidate; not a release tag and not
production-certified

This guide replaces the superseded phase-1-to-5 launch estimate. It does not claim
that the system is production-ready or that all RBAC work is complete. The immutable
rc.7 candidate remains the v0.10-core release line; the v0.11-finance branch above is
an isolated preparation line. Use the [authoritative feature matrix](docs/reference/feature-matrix.md)
and the [v0.11-finance prep handoff](docs/releases/v0.11-finance-prep-handoff.md) for
capability and evidence status.

## Current position

The repository contains a working modular-monolith foundation with several integrated
business lifecycles and many partially implemented advanced workflows. Base permission
middleware and module policies are present, while scoped assignments, access reviews,
and the newer-module role matrix remain integration work. Production certification is
still open for the capabilities marked `no` or `partial` in the matrix.

The v0.11 preparation slice adds a versioned PostgreSQL payment-execution snapshot
store, durable bank-file result/effect idempotency boundaries, exact decimal treasury
amounts, verified statement-transport ingestion, additional forecast source readers,
fail-closed provider connection validation, same-company settlement GL guards,
company-scoped PO/GRN workbench reads and transitions, settlement-result consistency
checks, a bounded AP exception workbench, and profile-gated scheduled bank-feed and
forecast scans. These are bounded foundations; provider adapters, token/rate-limit
operations, atomic treasury settlement lifecycle, immutable artifact custody,
cross-module accounting effects, full P2P closeout, and production evidence remain
open. Treasury proposal/export controls now include active-allocation checks, stale
approval invalidation, optimistic revision/status CAS, and route-specific scoped RBAC.

The v0.10.0 production claim is limited to AR/AP invoice and payment lifecycle,
sales order and delivery, inventory movement and stock-take, document control
foundation, and CMMS maintenance foundation. The [authoritative feature
matrix](docs/reference/feature-matrix.md) records this with its `v0.10.0 scope`
column. Use `RELEASE_PROFILE=v0.10-core` for staging and promotion; `full` requires
certification of every matrix row.

## v0.11-finance prep candidate

The candidate is based on the immutable v0.10.0-rc.7 line and includes migrations
`000125` through `000129`. Keep it separate from the rc.7 promotion record. The finance
sandbox workflow requires an explicit `candidate_ref`; supply the published branch,
tag, or commit that resolves to the intended candidate rather than relying on a staging
default. The complete preparation checklist is in the [v0.11-finance prep handoff](docs/releases/v0.11-finance-prep-handoff.md).
Its structured evidence registry is [v0.11-finance-evidence-index.md](docs/releases/v0.11-finance-evidence-index.md);
run `RELEASE_PROFILE=v0.11-finance make finance-sandbox-check` before dispatch.

## Immediate verification

Run the same release-hygiene checks used by CI before describing a capability as
available to users:

```bash
export ODYSSEY_TEST_MODE=1
export GOTENBERG_URL='http://127.0.0.1:0'
RELEASE_PROFILE=v0.11-finance make release-check
make pdf-release-check
```

`RELEASE_PROFILE=v0.11-finance make release-check` validates the feature matrix, active documentation links, and
advertised integrated route sources. `make pdf-release-check` explicitly compiles and
tests the production PDF implementation with `production pdf` build tags. The default
non-production PDF build is intentionally disabled and may return HTTP 503.

The final tagged gate requires an explicit profile:

```bash
RELEASE_PROFILE=v0.10-core make production-release-check
```

The current CI-equivalent verification also records unresolved application-level E2E
blockers in the [HTTP E2E regression guide](docs/guides/e2e-regression.md). Static,
database, unit-test, build, and PDF gates may be green while the full route sweep is
still red; that state is not release-ready and must not be hidden by weakening the
test or reclassifying partial features as integrated.

For v0.10 staging and final promotion, record identity, migration, restore, journey,
security, rollback, and observation evidence in the [v0.10-core staging
certification record](docs/releases/v0.10-core-staging-certification.md). For v0.11
finance sandbox preparation, use the [prep handoff](docs/releases/v0.11-finance-prep-handoff.md).

## Recommended development sequence

1. Keep the matrix current as each capability moves from code to integration.
2. Complete one end-to-end lifecycle at a time, including persistence, jobs, retries,
   idempotency, and cross-module accounting boundaries.
3. Add staging evidence before changing `production-certified` to `yes`.
4. Update the relevant guide and changelog entry in the same change; use `docs/archive`
   for superseded plans or historical acceptance records.
5. Dispatch the finance sandbox only with an explicit candidate ref and preserve the
   candidate SHA, migration ceiling, artifact digest, and evidence links together.

## v0.11-finance continuation order

- Finish bank-feed ingestion: implement and wire supported provider ports, token and
  consent refresh/rate-limit behavior, connection operations, company isolation, and
  sandbox contract evidence; the profile-gated scan/recovery scaffold is complete.
- Complete payment execution and settlement: compose worker handlers, certify the provider
  sandbox path, and prove idempotent settlement-to-AP/GL/tax/FX/reconciliation effects.
- Close the existing P2P loop from requisition/PO through receipt, AP, approval, payment,
  and ledger effects, including exact-money matching and database-backed scenarios.
- Keep the P2P HTTP workbench's authenticated-company lookup and transition guards on
  PR/PO/GRN detail, submit, approve, and post paths; context-free worker reads remain
  an explicit internal contract while the end-to-end P2P evidence is still open.
- Record migration rehearsal, route manifest, scoped-access cases, rollback, operator
  runbooks, metrics/alerts, artifact digests, and the evidence index in the prep handoff.
- Keep live provider execution and company feature flags disabled until the sandbox and
  staging gates are complete; broader procurement/logistics and asset operations remain
  outside this first finance tranche.

## Release rules

Do not use the superseded launch-time estimate. A `v0.10-core` release is ready only
when every matrix row with `v0.10.0 scope=yes` is `yes` for `code-complete`,
`integration-complete`, `production-certified`, and `documented`, the staging
certification record is complete, and the CI build/tag and route-placeholder checks
pass. A `full` release applies the same rule to every matrix row.

The v0.11-finance preparation candidate is not a release. Do not set
`production-certified=yes` or enable live execution from this branch. Its next gate is
an explicit sandbox dispatch and a reproducible evidence record covering provider,
worker, accounting-effect, P2P, security, migration, rollback, and operational checks.
