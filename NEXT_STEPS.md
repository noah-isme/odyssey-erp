# Odyssey ERP: Next Steps and Release Decision Guide

**Reviewed:** 2026-08-12

**Release profile:** `v0.10-core` (the bounded v0.10.0 scope)

This guide replaces the superseded phase-1-to-5 launch estimate. It does not claim
that the system is production-ready or that all RBAC work is complete. Use the
[authoritative feature matrix](docs/reference/feature-matrix.md) for capability and
release status.

## Current position

The repository contains a working modular-monolith foundation with several integrated
business lifecycles and many partially implemented advanced workflows. Base permission
middleware and module policies are present, while scoped assignments, access reviews,
and the newer-module role matrix remain integration work. Production certification is
still open for the capabilities marked `no` or `partial` in the matrix.

The latest implementation slice adds a versioned PostgreSQL payment-execution snapshot
store, durable bank-file result/effect idempotency boundaries, exact decimal treasury
amounts, verified statement-transport ingestion, additional forecast source readers,
deterministic v1 distribution route ordering and metrics, injectable CMMS predictive
thresholds, and company/branch-aware RBAC middleware helpers. These are bounded
foundations; live-provider wiring, cross-module accounting effects, and production
evidence below remain open.

The v0.10.0 production claim is limited to AR/AP invoice and payment lifecycle,
sales order and delivery, inventory movement and stock-take, document control
foundation, and CMMS maintenance foundation. The [authoritative feature
matrix](docs/reference/feature-matrix.md) records this with its `v0.10.0 scope`
column. Use `RELEASE_PROFILE=v0.10-core` for staging and promotion; `full` requires
certification of every matrix row.

## Immediate verification

Run the same release-hygiene checks used by CI before describing a capability as
available to users:

```bash
export ODYSSEY_TEST_MODE=1
export GOTENBERG_URL='http://127.0.0.1:0'
make release-check
make pdf-release-check
```

`make release-check` validates the feature matrix, active documentation links, and
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

For staging and final promotion, record identity, migration, restore, journey,
security, rollback, and observation evidence in the [v0.10-core staging
certification record](docs/releases/v0.10-core-staging-certification.md).

## Recommended development sequence

1. Keep the matrix current as each capability moves from code to integration.
2. Complete one end-to-end lifecycle at a time, including persistence, jobs, retries,
   idempotency, and cross-module accounting boundaries.
3. Add staging evidence before changing `production-certified` to `yes`.
4. Update the relevant guide and changelog entry in the same change; use `docs/archive`
   for superseded plans or historical acceptance records.

## Highest-value remaining work

- Complete v0.10-core staging certification: deploy the explicit profile, exercise
  the five journeys, and attach tenant-isolation, migration, restore, rollback, and
  access-review evidence. Profile enforcement, compatibility migration, access-review
  APIs, and exact scoped checks are implemented; remaining route adoption and newer
  module role coverage stay on the post-release backlog.
- Integrate the provider-neutral payment execution coordinator and durable result inbox
  with application worker composition, live provider adapters, settlement-to-AP/GL/tax/FX
  effects, and reconciliation. The provider-neutral outbox commands and ambiguous-outcome
  dead-letter behavior are now covered; they are not provider or accounting certification.
- Close the remaining procurement, landed-cost, logistics, and distribution lifecycle
  gaps. Distribution now has deterministic v1 route ordering and metrics; freight still
  has an injectable exact journal-posting boundary, but application wiring and the full
  PO-to-GL orchestration remain open.
- Complete live provider workflows and external merchant-account certification for
  connectors. Midtrans has deterministic sandbox certification plus scheduled
  reconciliation, unmatched-payment alerts, refund-state persistence, dead-letter
  audit/replay, and recovery metrics; live payout and bank-confirmed-refund evidence
  remains deployment work.
- Add provider-backed OCR, realtime collaboration delivery, richer disposition
  operations, CMMS telemetry/predictive operations, and the remaining MRP compliance
  decision paths, retention jobs, and regulated-policy validation.

## Release rule

Do not use the superseded launch-time estimate. A `v0.10-core` release is ready only
when every matrix row with `v0.10.0 scope=yes` is `yes` for `code-complete`,
`integration-complete`, `production-certified`, and `documented`, the staging
certification record is complete, and the CI build/tag and route-placeholder checks
pass. A `full` release applies the same rule to every matrix row.
