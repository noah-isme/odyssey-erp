# Security Checklist — Phase 6 Final Release

This checklist must be completed before tagging `v0.6.0-final`.

## Application Security

- [ ] Static analysis (golangci-lint) passes with no new high severity findings.
- [ ] Dependency audit (`go list -m all`) reviewed for CVEs; document exceptions.
- [ ] Finance HTTP handlers enforce RBAC per `internal/rbac` policies.
- [ ] Sensitive exports (CSV/PDF) validated for tenant isolation during QA.

## Infrastructure & Observability

- [ ] Prometheus endpoints served over HTTPS and require basic auth in production.
- [ ] Grafana admin password rotated and stored in Vault.
- [ ] Alertmanager routes verified for finance escalation path.
- [ ] `make alert-test` executed with evidence captured in `TESTING-PHASE6-S4.md`.

## Data Protection

- [ ] Redis cache contains no PII; verify serialization excludes sensitive fields.
- [ ] Database credentials rotated and applied to Kubernetes secrets.
- [ ] Backup and restore drill performed within the last 30 days.

## Change Management

- [ ] CHANGELOG updated with Phase 6 final notes.
- [ ] Runbook (`docs/runbook-ops-finance.md`) reviewed by operations.
- [ ] SLO document (`docs/slo-finance.md`) signed off by finance leadership.
- [ ] Release artifacts archived in the internal registry.

## Sign-off

| Role | Name | Date | Notes |
| --- | --- | --- | --- |
| Observability Lead | | | |
| QA Lead | | | |
| Security Lead | | | |
