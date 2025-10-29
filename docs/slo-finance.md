# Finance Observability SLO/SLA (Phase 6 Final)

This document captures the final service objectives for the Odyssey ERP finance surface as shipped in Phase 6. All metrics are collected from Prometheus via the consolidated `/metrics` endpoint and surfaced in Grafana dashboards located in `deploy/grafana/dashboards/finance.json`.

## Service Level Objectives

| Category | Metric | Target | Measurement Window | Alert | Error Budget |
| --- | --- | --- | --- | --- | --- |
| Availability | Successful HTTP responses (`odyssey_http_requests_total{code=~"2.."}`) | ≥ 99.2% | 30 days | `HighErrorRate` critical alert | 576 minutes / month |
| Latency (cached) | `/finance/**` cached request p95 | ≤ 500 ms | Rolling 7 days | `HighLatency` warning alert | 3.5 hours / week |
| Latency (cold) | `/finance/**` cold request p95 | ≤ 2 s | Rolling 7 days | `HighLatency` warning alert | 14 hours / week |
| Anomaly detection | `odyssey_finance_anomalies_total{severity="HIGH"}` increases | ≤ 3 / hour | Rolling 24 hours | `AnomalySpike` warning alert | 4 spikes / day |
| Job reliability | Analytics job success ratio (`odyssey_jobs_total`) | ≥ 90% | Rolling 24 hours | Pager escalation on failure trend | 10% of daily runs |

### Latency Calculation

Latency budgets separate cached versus cold requests. Cached requests are served from Redis warmers and observed via the `analytics.cached_insights` job histogram. Cold requests represent cache-miss workloads and are instrumented by `analytics.cold_refresh`.

```
latency_p95(route, scope) = histogram_quantile(0.95, sum(rate(odyssey_http_request_duration_seconds_bucket{route=~route,company=~scope.company,branch=~scope.branch}[5m])) by (le, route))
```

### Error Budget Policy

* **Availability:** An incident consuming >10% of the monthly error budget triggers a post-incident review.
* **Latency:** Two consecutive windows over budget require immediate cache warm-up validation.
* **Anomalies:** Any warning alert that persists longer than 30 minutes escalates to engineering leadership.

## Service Level Agreements

| Stakeholder | Commitment | Notes |
| --- | --- | --- |
| Finance analysts | Dashboards and exports available during business hours (08:00–20:00 WIB) with <2% 5xx errors. | Supported by redundancy across finance pods and the `HighErrorRate` alert.
| Audit and compliance | High anomaly detections triaged within 1 business day. | Workflows described in the finance runbook.
| Platform operations | Prometheus and Grafana availability ≥ 99%. | Ensured by managed Kubernetes add-ons and daily health checks.

## Dependencies

* Redis cache cluster (`odyssey_redis_commands_total`) for analytics warm ups.
* PostgreSQL connection pool metrics (`odyssey_db_pool_in_use`, `odyssey_db_pool_wait_count`).
* Background job instrumentation from `internal/jobs` for throughput and anomaly accounting.

## Review Cadence

* Weekly SLO review meeting with finance + platform teams.
* Monthly retro to adjust thresholds and evaluate alert fatigue.
* Quarterly audit of runbook accuracy and PagerDuty escalation paths.

## Change Log

* **Phase 6 Final:** Introduced dedicated severity filter for anomaly trend, split cached vs cold latency benchmarks, and aligned alerting thresholds with the final SLA targets.
