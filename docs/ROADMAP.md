# Odyssey ERP Project Roadmap

**Reviewed:** 2026-08-12
**Current candidate:** `v0.10.0-rc.4`
**Release profile:** `v0.10-core`

Odyssey ERP is a Go modular monolith for finance, sales, procurement, inventory,
governance, and operational workflows. The [authoritative feature
matrix](reference/feature-matrix.md) is the status authority; this roadmap tracks
the order in which the release is hardened and the next capabilities are integrated.
The current candidate is not production-certified.

The `v0.10.0-rc.4` application baseline is
`ec65cc08639c184030c63e3407791987eee92804`. The eventual annotated rc.4 tag
identifies the candidate commit after release packaging and documentation are
committed; that tag is the authority for the exact candidate SHA. The candidate
is limited to migrations through `000124_scoped_rbac_global_compatibility`.
Commit `1a8343e4499420467ba3dda04a2683782c6c79d7`, migration
`000125_payment_settlement_results`, and v0.11-finance routes are outside the
v0.10 line.

## v0.10.0 — bounded core release

The v0.10.0 production claim is intentionally limited to five integrated
capabilities: AR/AP invoice and payment lifecycle, sales order and delivery,
inventory movement and stock-take, document control foundation, and CMMS
maintenance foundation. Required authentication, company selection, master data,
approvals, notifications, administration, health, and accounting support are
release prerequisites but are not separate capability claims in the matrix.

### Milestone 1 — Freeze and enforce the core scope

- [x] Keep `v0.10.0 scope=yes` only for the five bounded core rows in the feature
      matrix; all advanced and partial capabilities remain outside the profile.
- [x] Enforce an explicit `RELEASE_PROFILE` of `v0.10-core` or `full` in runtime
      configuration and release gates; staging and v0.10.0 promotion use
      `v0.10-core`.
- [x] Expose only the selected capability set in navigation and production route
      policy; preview and out-of-scope routes return 404 and are not production
      claims.
- [x] Implement scoped access assignment migration/compatibility, core-route
      adoption, company/branch selection enforcement, and access-review APIs.
- [x] Freeze the rc.4 application baseline at `ec65cc0` and keep the candidate
      migration ceiling at `000124`; v0.11-finance routes and migration `000125`
      remain outside the `v0.10-core` profile. Record the final packaging commit
      through the annotated candidate tag.
- [ ] Complete staging evidence for the scoped controls and certify the five core
      journeys before promotion.
- [ ] Keep the [production release checklist](releases/production-release-checklist.md)
      and [staging certification record](releases/v0.10-core-staging-certification.md)
      aligned with the exact candidate commit.

**Exit:** release and documentation gates pass; the selected profile is explicit;
out-of-scope capabilities are not advertised; tenant/company access is fail-closed
for direct URLs, forms, workers, and company changes; and staging evidence is
attached to the certification record.

### Milestone 2 — Certify the core profile in staging

- [ ] Deploy the exact `v0.10.0-rc.4` commit to an isolated staging VPS with
      `RELEASE_PROFILE=v0.10-core`, production build tags, separate database,
      Redis, secrets, storage, and connector configuration. Verify that the
      migration history stops at `000124` and does not include `000125`.
- [ ] Rehearse migrations on a production-like clone, exercise the newest
      reversible path, document irreversible recovery, and prove backup restore.
- [ ] Run lint, release hygiene, production/PDF builds, unit tests, vet, SQL
      generation checks, and the HTTP regression suite from a clean checkout.
- [ ] Execute the five core journeys, including persistence, worker retries,
      idempotency, audit evidence, and accounting effects where applicable.
- [ ] Exercise two-company and branch-scoped negative cases, including direct URL
      access, form tampering, company changes, expired assignments, and revocation.
- [ ] Record commit/artifact identity, migration and schema evidence, test outputs,
      backup/restore results, route manifest, findings, and approver sign-off in the
      [staging certification record](releases/v0.10-core-staging-certification.md).

**Exit:** staging journeys and security cases pass, restore is proven, no critical
or high findings remain, and the evidence record is reproducible.

### Milestone 3 — Promote, observe, and close v0.10.0

- [ ] Build and scan immutable web/worker artifacts from the certified commit;
      retain component digests, the SPDX SBOM, and the GitHub provenance
      attestation for the immutable release bundle.
- [ ] Verify production secrets, TLS, PostgreSQL, Redis, storage, monitoring,
      alerts, backup schedule, rollback target, and operator ownership.
- [ ] Take and verify the pre-deploy backup, apply migrations as a controlled
      step, atomically switch the release, and smoke-test health, authentication,
      each core capability, one worker task, and company isolation.
- [ ] Observe production for 60 minutes and roll back on failed health checks,
      sustained 5xx/latency or queue thresholds, migration integrity failure, or
      any authorization-scope breach.
- [ ] Attach production evidence, set `production-certified=yes` only for rows
      with complete evidence, run `RELEASE_PROFILE=v0.10-core make
      production-release-check`, and create the signed `v0.10.0` tag after go/no-go
      approval.

**Exit:** the five core rows are certified, the release evidence is attached, the
observation window is clean, and the final tag and rollback record are complete.

## Post-v0.10 backlog

Work stays ordered by end-to-end business value and operational risk. Each item
must complete persistence, jobs, retries, idempotency, cross-module accounting,
security, documentation, and staging evidence before it becomes a production claim.

1. **Payment execution and settlement.** Integrate the provider-neutral payment
   execution coordinator with finance outbox delivery, live provider adapters,
   settlement-to-AP/GL effects, and reconciliation.
2. **Procurement and logistics depth.** Complete the purchase-order to freight,
   receipt, landed-cost, AP, payment, and GL orchestration, then close distribution
   inventory/GL transfer posting and operational workbenches.
3. **Scoped access governance.** Extend the exact scoped route pattern to remaining
   modules, complete newer-module role coverage, and collect production-like access
   review evidence.
4. **External integrations and advanced operations.** Certify connector providers,
   document OCR/realtime delivery, CMMS telemetry/predictive operations, and the
   remaining MRP compliance decisions.
5. **Broader enterprise modules.** Expand governed reporting/BI, portals, CRM,
   HR/payroll, QMS, POS, MRP, manufacturing, fixed assets, and other partial
   modules only after their workflows and deployment evidence are complete.

See [NEXT_STEPS.md](../NEXT_STEPS.md) for the current implementation handoff and
[docs/releases/VERSION_HISTORY.md](releases/VERSION_HISTORY.md) for candidate
history. Superseded phase notes belong under `docs/archive/` and are not release
status evidence.
