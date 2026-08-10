# Changelog

All notable released changes are recorded here at a summary level. Detailed
release notes are immutable records under `docs/releases/`; completed phase logs
and older changelog material live in `docs/archive/`. Release readiness is tracked
only in the [authoritative feature matrix](reference/feature-matrix.md).

## Unreleased

- Hardened the Midtrans connector with structured vaulted credentials, explicit
  sandbox/live endpoint selection, injected transport retries, provider status health
  checks, strict webhook validation, monotonic payment transitions, refund/status
  commands, timeout recovery, payout reconciliation, and executable sandbox
  certification coverage.
- Operationalized payment recovery with scheduled provider reconciliation, durable
  unmatched-payment issues and administrator alerts, transactional refund request
  state persistence, connector dead-letter audit/replay records, and Prometheus
  recovery metrics.
- CI now supplies a non-production encryption key for route-dump/server smoke tests,
  aligns reporting and project migrations with the existing BIGINT company/user
  schema, and regenerates the reporting SQLC boundary accordingly.
- The remaining full HTTP E2E blockers and their acceptance criteria are recorded in
  the [HTTP E2E regression guide](guides/e2e-regression.md). The release candidate is
  not certified while that sweep remains red.

## v0.10.0-rc.3 — 2026-08-10

Self-managed VPS deployment target and release-gate cleanup on top of the
`v0.10.0-rc.2` candidate. See the [release notes](releases/v0.10.0-rc.3.md).
Production certification remains pending.

### Changed

- Removed the obsolete Render configuration, deployment guide, and blueprint test.
- Updated the production release gate to verify the self-managed VPS runbook.
- Documented the VPS deployment target and kept feature certification explicitly
  evidence-based in the authoritative feature matrix.

## v0.10.0-rc.2 — 2026-08-10

Coretax and Indonesian PPh 21 release-test completion on top of the
`v0.10.0-rc.1` candidate. See the [release notes](releases/v0.10.0-rc.2.md).
Production certification remains pending.

### Added

- **Coretax release contract:** Explicit endpoint configuration, fail-closed
  submission/validation, accepted-response handling, and export/validator/GL
  reconciliation coverage.
- **Annual PPh 21 reconciliation:** Versioned progressive annual bands,
  integer-rupiah taxable-income rounding, last-tax-period corrections, and a
  PMK 168/2023 worked-example fixture.

## v0.10.0-rc.1 — 2026-08-10

Release candidate for the post-v0.9.1 platform work. See the
[release notes](releases/v0.10.0-rc.1.md). Production certification remains pending.

### Added
- **Production release gates**: Added a repeatable production build check, a final
  tagged/certified release gate, and a release checklist covering migrations,
  external certification, runtime controls, deployment, and rollback. The candidate
  remains non-production-certified until its evidence gates are complete.
- **MRP compliance hardening**: Added server-generated canonical snapshots with immutable JSONB/hash/version evidence, actor-bound one-time challenges, active policy-role and separation-of-duties checks, password/TOTP reauthentication, retention metadata, and immutable decision/signature/audit guards. Migration `000118` adds the evidence boundary and challenge bindings; downstream staging flows and regulator-specific validation remain planned.
- **Document and CMMS advanced foundations**: Added a durable Asynq OCR task with
  text extraction and search-index updates, persisted collaboration changes,
  tenant-scoped full-text search, retention-expiry to disposition processing, and
  company-checked CMMS IoT readings, predictive-model records, and idempotent anomaly
  alerts. Migration `000117` adds collaboration-change/search constraints and fixes
  the CMMS advanced foreign keys to the existing `assets` table. Scanned-document OCR
  and real ML inference still require injected providers.
- **Finance automation**: Added tenant-scoped treasury batch controls with SQL-backed
  active-item totals, verified bank-feed event inbox processing, database-backed rolling
  forecasts with FX snapshots, and an idempotent finance outbox dispatcher. Live bank-feed
  provider adapters and payment operation handlers remain deployment work.
- **Distribution**: Added authenticated `/distribution` routes for planning horizons,
  load/shipment lifecycle, dispatch, delivery inventory posting, manual routes, and
  transfer orders. Added a database-backed opt-in lifecycle test.
- **Capability status hygiene**: Reclassified advanced documents, CMMS telemetry,
  WMS, portal, POS, marketplace, and project depth as partial or planned where their
  end-to-end workflows are not complete. See the [feature matrix](reference/feature-matrix.md)
  for the current release dimensions.
- **Connector hardening**: Provider transport now has vault-resolved configuration,
  fail-closed production behavior, retry/idempotency controls, deterministic webhook
  replay keys, and contract-test seams. This does not certify every provider or
  channel workflow; development fakes require `CONNECTORS_DEVELOPMENT_MODE=true`.

### Changed

- Consolidated current setup, testing, architecture, and contributor guidance.
- Archived obsolete task artifacts, phase test plans, and duplicate setup guides.
- Added `make docs-check` to validate active documentation links and Make targets.
- Isolated generated SQLC/database types behind repository adapters and local service contracts.
- Standardized safe HTTP error classification and response helpers across affected handlers.
- Documented the repository and HTTP boundary policy in ADR-0014.

## v0.9.1 — 2026-05-28

Enterprise UI/UX overhaul across core operations. See the
[release notes](releases/v0.9.1.md).

## v0.9.0 — 2026-01-11

Sales and Accounts Receivable baseline. See the
[release notes](releases/v0.9.0.md).

## v0.8.0

Board Pack Generator release. See the [release notes](releases/v0.8.0.md).

## v0.7.0

Finance export reliability release. See the [release notes](releases/v0.7.0.md).
