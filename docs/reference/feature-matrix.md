# Authoritative Feature Matrix

**Reviewed:** 2026-08-27

**Current release candidate:** `v0.10.0-rc.7`. This candidate is not
production-certified; the matrix below remains the authority for promotion evidence.

**v0.10.0-rc.7 boundary:** the application baseline is
`ec65cc08639c184030c63e3407791987eee92804`; the annotated rc.7 tag identifies
the candidate commit `5ed11da8aea342708be67284ea7a71224f90ccdc` containing the
release-hygiene correction. The candidate includes migrations through
`000124_scoped_rbac_global_compatibility`.
Commit `1a8343e4499420467ba3dda04a2683782c6c79d7`, migration
`000125_payment_settlement_results`, and v0.11-only routes are excluded.

**Release profiles:** `v0.10-core` is the bounded v0.10.0 production profile and
claims only rows marked `yes` in the **v0.10.0 scope** column. `full` is reserved
for a release that certifies every row in this matrix. The final production gate
requires `RELEASE_PROFILE` to be set explicitly; staging and v0.10.0 promotion
use `RELEASE_PROFILE=v0.10-core`.

**Deployment target:** self-managed VPS using the [VPS production deployment
guide](../DEPLOYMENT.md). Hosting choice does not automatically change a
capability's production-certification status.

This is the single release-status authority for current product capabilities. The
[module catalog](module-catalog.md) remains the capability inventory and links to
guides; it must not be used as evidence of integration or production certification.
Archived plans and acceptance notes are historical unless this matrix links to them
as current evidence.

## Status definitions

Each capability is assessed independently across the release scope and four
status dimensions:

- `v0.10.0 scope` — `yes` means the capability is part of the bounded
  `v0.10-core` release claim; `no` means it remains outside that profile. A
  v0.11-only capability or migration is never included by this column. The
  `full` profile evaluates every row regardless of this column;
- `code-complete` — the scoped behavior has an implementation and focused tests;
- `integration-complete` — the behavior is wired through its route, worker, persistence,
  and cross-module boundary for the supported lifecycle;
- `production-certified` — VPS staging/production evidence, operational controls,
  backups, rollback, and any required provider or build-tag checks are recorded;
- `documented` — active documentation describes the supported scope and its remaining
  gaps without implying a stronger status.

Scope values are `yes` or `no`. Status values are `yes`, `partial`, or `no`. A
`partial` value is intentionally not a release claim. A route may exist for local
evaluation while its row remains `partial` or `no`.
CI checks the source files listed in **Advertised production route source** only when
`integration-complete` is `yes`; it rejects explicit placeholder responses there.

| Capability | v0.10.0 scope | code-complete | integration-complete | production-certified | documented | Advertised production route source | Evidence / remaining gate |
|---|---|---|---|---|---|---|
| AR/AP invoice and payment lifecycle | yes | yes | yes | no | yes | `/finance/ar` -> `internal/ar/handler.go`; `/finance/ap` -> `internal/ap/handler.go` | Core ledger and invoice workflows are implemented; production certification still requires environment-specific migration, reconciliation, and operational evidence. |
| Sales order and delivery lifecycle | yes | yes | yes | no | yes | `/sales` -> `internal/sales/handler.go` | Core order and delivery paths are wired; broader returns, scheduling, and external fulfillment depth remain partial. |
| Inventory movement and stock-take foundation | yes | yes | yes | no | yes | `/inventory` -> `internal/inventory/handler.go` | Movement, stock-take, adjustment, lot/serial, and AVG/FIFO foundations are available; breadth and production evidence remain open. |
| Document control foundation | yes | yes | yes | no | yes | `/documents` -> `internal/documents/http/handler.go` | Versions, ACL, review, signatures, retention, and persistence boundaries are wired; provider-backed binary OCR and realtime fan-out are not certified. |
| CMMS maintenance foundation | yes | yes | yes | no | yes | `/cmms` -> `internal/cmms/http/handler.go` | Work orders, assets, PM, meters, and spares are wired; mobile operations and production rollout evidence remain open. |
| Finance automation | no | partial | partial | no | yes | `/finance/bankfeeds` -> `internal/finance/bankfeeds/handler.go`; `/finance/forecasting` -> `internal/finance/forecasting/handler.go`; `/finance/treasury` -> `internal/finance/treasury/handler.go` | Tenant-safe bank-feed processing, rolling forecasts, treasury controls, outbox foundations, a provider-neutral exact-money payment execution coordinator, and versioned PostgreSQL execution snapshots exist; outbox delivery wiring, live providers, settlement effects, and full reconciliation remain. |
| Procurement, freight, and logistics execution | no | partial | partial | no | yes | `/procurement` -> `internal/procurement/handler.go`; `/logistics` -> `internal/logistics/handler.go`; `/freight` -> `internal/freight/handler.go` | Core records and workbench foundations exist; freight now has an injectable exact balanced journal boundary with deterministic source identity, but the purchase-order to freight, receipt, landed-cost, invoice, payment, and application-wiring loop is not complete. |
| Distribution lifecycle | no | partial | partial | no | yes | `/distribution` -> `internal/distribution/handler.go` | Planning, loads, shipment linkage, dispatch/delivery, transfer foundations, and deterministic v1 route ordering/metrics are present; inventory/GL transfer posting, provider-backed routing, and operational workbenches remain. |
| Advanced document processing | no | partial | partial | no | yes | `/documents/library/{id}/versions/{versionID}/ocr` -> `internal/documents/http/handler.go`; `/documents/search` -> `internal/documents/http/handler.go` | Text extraction, indexing, collaboration persistence, and disposition foundations exist; scanned-file providers, websocket delivery, and richer disposition actions remain. |
| CMMS IoT and predictive features | no | partial | partial | no | yes | `/cmms/iot` -> `internal/cmms/http/handler.go`; `/cmms/predictive` -> `internal/cmms/http/handler.go` | Readings, model metadata, heuristic alerts, and injectable finite threshold rules are persisted or supported; calibrated inference, streaming telemetry, and persisted per-sensor threshold policy remain. |
| MRP planning and execution foundation | no | yes | partial | no | yes | `/mrp` -> `internal/mrp/handler.go` | BOM, planning, WIP, scheduling, quality, genealogy, and analytics foundations are implemented; downstream staging and regulated-policy validation remain. |
| MRP compliance controls | no | yes | partial | no | yes | `/mrp` -> `internal/mrp/handlers.go` | Canonical snapshots, server versions/hashes, policy checks, actor-bound challenges, reauthentication, and audit evidence are implemented; every decision path, retention/export, and regulator-specific validation remain. |
| Connector transport foundation | no | yes | partial | no | yes | `/settings/integrations` -> `internal/connectors/http_handlers.go`; `/webhooks/connectors` -> `internal/connectors/http_webhook.go` | Vault references, fail-closed production configuration, outbox/inbox, replay keys, scheduled reconciliation, dead-letter audit/replay, and provider transport foundations exist; provider sandbox and limited-production certification remain. |
| Payment, carrier, marketplace, messaging, and identity connectors | no | partial | partial | no | yes | `/webhooks/connectors` -> `internal/connectors/http_webhook.go` | Midtrans/Stripe and development-gated provider paths have transport coverage; channel-specific business workflows and provider certification remain. |
| Consolidation PDF exports | no | partial | no | no | yes | `/finance/consol/tb/pdf` -> `internal/consol/http/handlers_tb.go`; `/finance/consol/pl/pdf` -> `internal/consol/http/handlers_pl.go`; `/finance/consol/bs/pdf` -> `internal/consol/http/handlers_bs.go` | The default `!production && !pdf` build intentionally disables exporters and can return 503. Release CI must run `go test` and `go build` with `-tags "production pdf"`. |
| Phase 14 transaction FX and P7 foundation | no | partial | partial | no | yes | none | Local acceptance evidence is historical/local only. Staging migration, production worker, provider, and cross-feature evidence remain pending. |
| RBAC and scoped access governance | no | yes | partial | no | yes | `/permissions` -> `internal/rbac/permissions_handler.go` | Permission middleware and module policies exist; effective-dated company/branch assignments, compatibility migration/seed rollout, company-scoped access-review APIs, and exact scoped checks across the v0.10-core route groups are implemented. Remaining work is staging evidence, broader route adoption, and the newer-module role matrix. |
| Portal self-service depth | no | partial | partial | no | yes | `/portal` -> `internal/portal/handler.go` | Authenticated read-only views are available; profile changes, RFQ negotiation, chat, analytics, and broader self-service remain partial. |
| Reporting and BI depth | no | partial | partial | no | yes | `/report` -> `report/sample.go` | Finance, consolidation, analytics, board-pack, audit, and scheduled-report foundations exist; governed builder/widgets, wider operational coverage, and managed BI delivery remain. |

## Release interpretation

Do not describe a capability as production-ready solely because `code-complete` is
`yes`. For `RELEASE_PROFILE=v0.10-core`, every row marked `v0.10.0 scope=yes` must
have all four status columns set to `yes`, plus the route, provider, migration, and
operational evidence named in the final column. `RELEASE_PROFILE=full` applies the
same rule to every row. Rows outside the selected profile are not release claims and
must remain unavailable to that profile's production route set. In particular, the
Phase 14/P7 evidence guide records local verification; it does not certify staging or
production. The VPS deployment target satisfies the infrastructure selection only; it
does not certify incomplete feature workflows. For rc.7, selecting
`RELEASE_PROFILE=v0.10-core` must not expose v0.11-only routes or apply migration
`000125`.

Run the same checks locally and in CI:

```bash
make release-check
make pdf-release-check
# The final gate always requires an explicit profile:
RELEASE_PROFILE=v0.10-core make production-release-check
```
