# Finance Operations Runbook (Phase 6 Final)

This runbook provides the operational response steps for finance observability alerts introduced in Phase 6.

## Contacts

* **Primary on-call:** Finance Platform Team (PagerDuty: `finance-platform` schedule).
* **Secondary:** Observability Engineer on rotation.
* **Escalation:** VP Engineering after 60 minutes unresolved critical alert.

## Dashboards

* Grafana finance dashboard (`finance-dashboard`) – route-level latency, error rate, anomaly trends.
* Grafana platform dashboard (`platform-dashboard`) – infrastructure level health.
* Prometheus alert status panel (`/alerts`) for acknowledgement.

## Alert Procedures

### HighErrorRate

* **Trigger:** `sum(rate(odyssey_http_requests_total{code=~"5.."}[5m])) / sum(rate(odyssey_http_requests_total[5m])) > 0.02` for 5 minutes.
* **Severity:** Critical.
* **Steps:**
  1. Check Grafana finance dashboard error panel for affected routes.
  2. Inspect Kubernetes pod logs for `/finance/**` services. Look for recent deploys.
  3. Roll back last deployment if error rate coincides with release.<br>Use `make monitor-demo` to replay traffic and confirm fix.
  4. If errors persist, fail over to standby instance via Terraform variable `finance_active_cluster`.
* **Resolution Validation:** Error rate < 2% for two consecutive evaluation intervals.
* **Post-incident:** File retro if more than 10% of error budget consumed.

### HighLatency

* **Trigger:** `histogram_quantile(0.95, sum(rate(odyssey_http_request_duration_seconds_bucket{route=~"/finance/.+"}[10m])) by (le, route)) > 0.8` for 10 minutes.
* **Severity:** Warning.
* **Steps:**
  1. Open Grafana finance latency panel and review cached vs cold split.
  2. Verify Redis health (`platform-dashboard` Redis Ops/sec panel). Look for drop in throughput.
  3. Run cache warm-up job: `make monitor-demo warm-cache=true`.
  4. If latency remains high, increase job concurrency by scaling `analytics-worker` deployment (HPA target 80%).
* **Resolution Validation:** Finance latency p95 < 800 ms for cached requests and < 2 s for cold paths.

### AnomalySpike

* **Trigger:** `sum(increase(odyssey_finance_anomalies_total{severity="HIGH"}[1h])) > 3` for 15 minutes.
* **Severity:** Warning.
* **Steps:**
  1. Inspect anomaly trend panel filtered by severity.
  2. Validate upstream job success ratio (Job Success vs Failure panel). Recover failing jobs first.
  3. Review recent data imports for the affected company/branch. Coordinate with data engineering to pause ingestion if needed.
  4. Notify finance analysts via Slack channel `#finance-ops` with anomaly context.
* **Resolution Validation:** High severity anomalies ≤ 3/hour for two hours.

## Operational Checklists

### Daily

* Confirm Grafana dashboards render without errors.
* Validate Prometheus scrape status (no failed targets).
* Ensure Redis latency < 5 ms (via `platform-dashboard`).

### Weekly

* Review SLO error budget burn-down in `docs/slo-finance.md`.
* Test alert routing via `make alert-test`.
* Refresh cache warm-up job schedule.

### Monthly

* Conduct synthetic export run for CSV/PDF to ensure RBAC rules.
* Update runbook if alert thresholds or procedures changed.

## References

* `docs/slo-finance.md` – canonical SLO/SLA definitions.
* `docs/observability-overview.md` – architecture overview.
* `security-checklist-phase6.md` – release security gates.
