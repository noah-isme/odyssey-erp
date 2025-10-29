# Observability Overview

This document captures the core observability assets for the finance and platform surfaces ahead of the Phase 6 release. It summarizes the dashboards, alert rules, and labeling conventions that are now part of the deployment playbook.

## Dashboards

### Finance Observability

* **UID:** `finance-dashboard`
* **Location:** `deploy/grafana/dashboards/finance.json`
* **Key panels:** HTTP request rate, 5xx percentage, finance latency p95 per route, cache hit ratio, analytics job success/failure, and anomaly detection trends.
* **Variables:**
  * `route` – Regex filter for finance endpoints (defaults to `/finance/.*`).
  * `company` – Multi-tenant selector for finance organizations.
  * `branch` – Narrow down to branches within a company.
  * `period` – Cache scope (defaults to `active`).

### Platform Health

* **UID:** `platform-dashboard`
* **Location:** `deploy/grafana/dashboards/platform.json`
* **Key panels:** CPU utilization, memory allocation, GC duration, goroutines, database pool usage, and Redis command throughput.

Both dashboards refresh every 30 seconds and assume a Prometheus datasource identified as `PROM_DS` in Grafana provisioning.

## Prometheus Alerts

The finance alert suite lives in `deploy/prometheus/alerts/finance.yml`. All rules apply consistent label keys (`route`, `code`, `job`, `severity`) and map to the finance runbook sections.

| Alert | Expr summary | For | Severity | Runbook |
| --- | --- | --- | --- | --- |
| `HighErrorRate` | Finance 5xx ratio > 2% over 5m | 5m | `critical` | `docs/runbook-ops-finance.md#high-error-rate` |
| `HighLatencyDashboard` | Finance p95 latency > 800ms | 10m | `warning` | `docs/runbook-ops-finance.md#high-latency` |
| `AnomalySpike` | High severity anomalies > 3 / hour | 15m | `warning` | `docs/runbook-ops-finance.md#anomaly-spike` |

## Labeling Conventions

* `route` – HTTP route template (e.g., `/finance/analytics`).
* `code` – HTTP status code family (`2xx`, `5xx`, etc.).
* `job` – Background job identifier (e.g., `analytics.insights_warmup`).
* `severity` – Alerting severity (`critical`, `warning`).

These keys are referenced in dashboards, alert routing, and forthcoming runbook steps to keep finance operations consistent.
