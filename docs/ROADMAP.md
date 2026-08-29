# Odyssey ERP Project Roadmap

**Reviewed:** 2026-08-29
**v0.10-core candidate:** `v0.10.0-rc.7` (immutable; migration ceiling `000124`)
**Current v0.11-finance prep baseline:** `release/v0.11-finance-prep` at
`ceba7a34102d43218700d59882435ca5ec214857` (working tree has uncommitted prep changes)
**Release profile:** `v0.11-finance` (sandbox preparation only)

Odyssey ERP is a Go modular monolith for finance, sales, procurement, inventory,
governance, and operational workflows. The [authoritative feature
matrix](reference/feature-matrix.md) is the status authority; this roadmap tracks
the order in which the release is hardened and the next capabilities are integrated.
The immutable v0.10-core candidate and the v0.11-finance preparation candidate are
separate release lines. Neither is production-certified; this branch is a preparation
candidate, not a release tag.

The v0.10.0-rc.7 application baseline remains limited to migrations through
`000124_scoped_rbac_global_compatibility`. The v0.11-finance preparation baseline includes
`000129_ap_exception_resolution_events`. Its migration ceiling is `000129`.
Keep this candidate in the isolated finance sandbox line until the evidence in the
[v0.11-finance prep handoff](releases/v0.11-finance-prep-handoff.md) is complete.

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
- [x] Retain the immutable rc.7 application baseline and keep the `v0.10-core`
      migration ceiling at `000124`; v0.11-finance routes and migrations remain
      outside that profile. Record the v0.11 preparation candidate separately from
      the final rc.7 packaging and certification record.
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

- [ ] Deploy the exact `v0.10.0-rc.7` commit to an isolated staging VPS with
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

### v0.11-finance preparation handoff

The next bounded workstream is represented by `RELEASE_PROFILE=v0.11-finance`.
The clean preparation baseline is branch
`release/v0.11-finance-prep` at
`ceba7a34102d43218700d59882435ca5ec214857`, with migrations through `000129` in the
current uncommitted preparation tree.
The current working tree additionally contains uncommitted provider-router, worker,
exact-quantity, settlement-consistency, company-scoped PO/GRN workbench and
transition guards, AP-exception safeguards, profile-gated scheduled bank-feed and
forecast scans, and route-specific treasury duty RBAC. It includes the
cumulative profile/route boundary,
statement-transport and forecast foundations, exact decimal treasury amounts, and
durable payment-result/effect idempotency contracts. Treasury proposal/export controls
now include active-allocation checks, stale-approval invalidation, and an export
revision/status compare-and-set. It is intentionally not a production claim: provider
adapters, token/rate-limit operations, confirmed AP/GL/tax/FX/reconciliation effects,
immutable artifact custody, atomic treasury settlement lifecycle, full P2P closeout,
and staging evidence remain open.

Use the [v0.11-finance prep handoff](releases/v0.11-finance-prep-handoff.md) and its
[evidence index](releases/v0.11-finance-evidence-index.md) for the candidate identity,
explicit sandbox dispatch, reproducible evidence checklist, and release rule. Run
`RELEASE_PROFILE=v0.11-finance make finance-sandbox-check` before publishing a
preparation candidate; the strict completion gate remains external-evidence-bound.
See [NEXT_STEPS.md](../NEXT_STEPS.md) for the execution order and
[docs/releases/VERSION_HISTORY.md](releases/VERSION_HISTORY.md) for historical
candidate notes. Superseded phase notes belong under `docs/archive/` and are not
release status evidence.
