# Odyssey ERP: Next Steps and Release Decision Guide

**Reviewed:** 2026-08-09

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

The current CI-equivalent verification also records unresolved application-level E2E
blockers in the [HTTP E2E regression guide](docs/guides/e2e-regression.md). Static,
database, unit-test, build, and PDF gates may be green while the full route sweep is
still red; that state is not release-ready and must not be hidden by weakening the
test or reclassifying partial features as integrated.

## Recommended development sequence

1. Keep the matrix current as each capability moves from code to integration.
2. Complete one end-to-end lifecycle at a time, including persistence, jobs, retries,
   idempotency, and cross-module accounting boundaries.
3. Add staging evidence before changing `production-certified` to `yes`.
4. Update the relevant guide and changelog entry in the same change; use `docs/archive`
   for superseded plans or historical acceptance records.

## Highest-value remaining work

- Complete live provider workflows and external merchant-account certification for
  connectors. Midtrans now has deterministic sandbox certification plus scheduled
  reconciliation, unmatched-payment alerts, refund-state persistence, dead-letter
  audit/replay, and recovery metrics; live payout and bank-confirmed-refund evidence
  remains deployment work.
- Finish finance automation provider adapters, reconciliation, payment execution, and
  outbox operations.
- Close the procurement, freight, logistics, and distribution lifecycle gaps.
- Add provider-backed OCR, realtime collaboration delivery, and richer disposition
  operations.
- Finish CMMS telemetry/predictive operations and the remaining MRP compliance decision
  paths, retention jobs, and regulated-policy validation.
- Complete scoped RBAC assignments and access-review operations.

## Release rule

Do not use the superseded launch-time estimate. A release is ready only when
the relevant matrix row is `yes` for `code-complete`, `integration-complete`,
`production-certified`, and `documented`, and the CI build/tag and route-placeholder
checks pass.
