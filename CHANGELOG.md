# Changelog

## Phase 7 Sprint 3.4.4

### Added

- Production-ready consolidation PDF exporter backed by Gotenberg with 10s timeout, two retries, and minimum-size validation.
- Streaming CSV exporter with buffered writes, metadata comment headers, and regression tests for P&L/Balance Sheet.
- `docs/runbook-consol-plbs.md` plus Makefile helpers (`export-demo`, `fx-tools`) for day-two export and FX operations.

### Changed

- Consolidation warnings now persist in cached view-models and render consistently across SSR banners, CSV metadata, and PDF warning lists.

### Testing

- Updated `TESTING-PHASE7-S3.md` to cover prod-tag PDF checks, CSV streaming, and warning parity.

## Phase 6 Final (v0.6.0-final)

### Added

- Grafana dashboards for finance and platform with latency, error, anomaly, and infrastructure panels.
- Prometheus finance alert rules covering high error rate, latency, and anomaly spikes with runbook annotations.
- Performance regression tests for HTTP latency, job throughput, and alert simulations.
- Finance SLO/SLA documentation, operations runbook, and release security checklist.
- Makefile targets for monitoring demo and alert simulations, plus Phase 6 release automation.

### Changed

- Prefixed HTTP metrics with `odyssey_` to align dashboards and alerts.
- Updated observability overview to include final dashboard variables and alert naming.

### Testing

- Documented automated, performance, and alert simulation coverage in `TESTING-PHASE6-S4.md`.
