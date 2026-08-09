# Changelog

All notable released changes are recorded here at a summary level. Detailed
release notes are immutable records under `docs/releases/`; completed phase logs
and older changelog material live in `docs/archive/`.

## Unreleased

### Added
- **Finance automation**: Added tenant-scoped treasury batch controls with SQL-backed
  active-item totals, verified bank-feed event inbox processing, database-backed rolling
  forecasts with FX snapshots, and an idempotent finance outbox dispatcher. Live bank-feed
  provider adapters and payment operation handlers remain deployment work.
- **Distribution**: Added authenticated `/distribution` routes for planning horizons,
  load/shipment lifecycle, dispatch, delivery inventory posting, manual routes, and
  transfer orders. Added a database-backed opt-in lifecycle test.
- **Documents**: Advanced QMS (SPC, ATE, LIMS), OCR, real-time collaboration, and full-text content search.
- **HR**: Benefits administration and performance review processing.
- **CMMS**: Predictive maintenance, IoT sensor integration, and anomaly alert heuristic engine.
- **WMS**: Wave planning, put-away tasks, cross-docking plans, and material handling equipment (MHE) management.
- **Portal**: Profile updates, RFQ negotiation, direct chat messaging, and analytics tracking.
- **POS**: Scanner/printer hardware configuration, loyalty members tier tracking, and gift card balances (split-tender ready).
- **Connectors**: Marketplace synchronization (Shopify) including inbound sales orders, outbox event bridging, and outbound inventory available-to-promise sync.
- **Connector hardening**: Replaced simulated Stripe, S3, WhatsApp, DHL, Shopify, and OIDC success paths with vault-resolved provider calls, fail-closed signature verification, retry/idempotency handling, deterministic webhook replay keys, and provider contract tests. Development fakes now require `CONNECTORS_DEVELOPMENT_MODE=true`.
- **Projects**: Milestone tracking, employee resource allocation, and expense submission/tracking.

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
