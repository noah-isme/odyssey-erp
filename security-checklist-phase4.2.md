# Security Checklist – Phase 4.2

- [ ] Finance RBAC permissions (`finance.gl.view`, `finance.gl.edit`, `finance.period.close`, `finance.override.lock`) provisioned for relevant roles.
- [ ] Audit logging configured for journal post/void/reverse and period transitions.
- [ ] Source link uniqueness enforced to prevent duplicate postings.
- [ ] Period lock override gated by `finance.override.lock` and approval workflow.
- [ ] Materialized view refresh limited to finance operators.
