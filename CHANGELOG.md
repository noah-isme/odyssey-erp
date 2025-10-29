# Changelog

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
