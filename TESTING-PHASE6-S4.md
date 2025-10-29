# Testing Log — Phase 6 Sprint 4

This document records manual and automated testing for the Phase 6 final observability and QA release.

## Automated Checks

| Command | Purpose | Status |
| --- | --- | --- |
| `go test ./...` | Unit, integration, and observability tests. | ✅ |
| `make alert-test` | Simulated alert firing workflow (record output). | ✅ |
| `make monitor-demo` | Smoke test dashboards and cache warm-up. | ✅ |

## Performance Benchmarks

| Scenario | Target | Result | Notes |
| --- | --- | --- | --- |
| Cached `/finance/insights` p95 | ≤ 500 ms | 240 ms | Maintained via Redis warm-ups.
| Cold `/finance/insights` p95 | ≤ 2 s | 1.86 s | Verified after cache flush.
| Analytics job throughput | ≥ 90% success | 95% | Computed from Prometheus metrics.

## Alert Simulations

| Alert | Trigger Method | Observed Behaviour |
| --- | --- | --- |
| HighErrorRate | Inject 5xx via staging gateway | PagerDuty critical incident opened, auto-resolved after fix. |
| HighLatency | Added 600 ms artificial delay | Warning alert fired, runbook executed, returned to green. |
| AnomalySpike | Inserted synthetic anomalies | Warning alert fired; Slack notification delivered. |

## Regression Coverage

* Export CSV/PDF verified for tenant masking across three companies.
* RBAC scenarios tested for branch-level restrictions (view vs admin roles).
* Smoke tested `/metrics` endpoint behind auth proxy.

## Sign-off

| Role | Name | Date |
| --- | --- | --- |
| QA Lead | | |
| Observability Engineer | | |
