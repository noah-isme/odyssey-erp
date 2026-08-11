# Authoritative Feature Matrix

**Reviewed:** 2026-08-10

**Current release candidate:** `v0.10.0-rc.3`. This candidate is not
production-certified; the matrix below remains the authority for promotion evidence.

**Deployment target:** self-managed VPS using the [VPS production deployment
guide](../DEPLOYMENT.md). Hosting choice does not automatically change a
capability's production-certification status.

This is the single release-status authority for current product capabilities. The
[module catalog](module-catalog.md) remains the capability inventory and links to
guides; it must not be used as evidence of integration or production certification.
Archived plans and acceptance notes are historical unless this matrix links to them
as current evidence.

## Status definitions

Each capability is assessed independently across four dimensions:

- `code-complete` — the scoped behavior has an implementation and focused tests;
- `integration-complete` — the behavior is wired through its route, worker, persistence,
  and cross-module boundary for the supported lifecycle;
- `production-certified` — VPS staging/production evidence, operational controls,
  backups, rollback, and any required provider or build-tag checks are recorded;
- `documented` — active documentation describes the supported scope and its remaining
  gaps without implying a stronger status.

Values are `yes`, `partial`, or `no`. A `partial` value is intentionally not a release
claim. A route may exist for local evaluation while its row remains `partial` or `no`.
CI checks the source files listed in **Advertised production route source** only when
`integration-complete` is `yes`; it rejects explicit placeholder responses there.

| Capability | code-complete | integration-complete | production-certified | documented | Advertised production route source | Evidence / remaining gate |
|---|---|---|---|---|---|---|
| AR/AP invoice and payment lifecycle | yes | yes | no | yes | `/finance/ar` -> `internal/ar/handler.go`; `/finance/ap` -> `internal/ap/handler.go` | Core ledger and invoice workflows are implemented; production certification still requires environment-specific migration, reconciliation, and operational evidence. |
| Sales order and delivery lifecycle | yes | yes | no | yes | `/sales` -> `internal/sales/handler.go` | Core order and delivery paths are wired; broader returns, scheduling, and external fulfillment depth remain partial. |
| Inventory movement and stock-take foundation | yes | yes | no | yes | `/inventory` -> `internal/inventory/handler.go` | Movement, stock-take, adjustment, lot/serial, and AVG/FIFO foundations are available; breadth and production evidence remain open. |
| Document control foundation | yes | yes | no | yes | `/documents` -> `internal/documents/http/handler.go` | Versions, ACL, review, signatures, retention, and persistence boundaries are wired; provider-backed binary OCR and realtime fan-out are not certified. |
| CMMS maintenance foundation | yes | yes | no | yes | `/cmms` -> `internal/cmms/http/handler.go` | Work orders, assets, PM, meters, and spares are wired; mobile operations and production rollout evidence remain open. |
| Finance automation | partial | partial | no | yes | `/finance/bankfeeds` -> `internal/finance/bankfeeds/handler.go`; `/finance/forecasting` -> `internal/finance/forecasting/handler.go`; `/finance/treasury` -> `internal/finance/treasury/handler.go` | Tenant-safe bank-feed processing, rolling forecasts, treasury controls, outbox foundations, and a provider-neutral exact-money payment execution coordinator exist; durable treasury wiring, live providers, settlement effects, and full reconciliation remain. |
| Procurement, freight, and logistics execution | partial | partial | no | yes | `/procurement` -> `internal/procurement/handler.go`; `/logistics` -> `internal/logistics/handler.go`; `/freight` -> `internal/freight/handler.go` | Core records and workbench foundations exist; freight now has an injectable exact balanced journal boundary with deterministic source identity, but the purchase-order to freight, receipt, landed-cost, invoice, payment, and application-wiring loop is not complete. |
| Distribution lifecycle | partial | partial | no | yes | `/distribution` -> `internal/distribution/handler.go` | Planning, loads, shipment linkage, dispatch/delivery, routes, and transfer foundations are present; inventory/GL transfer posting, optimization, and operational workbenches remain. |
| Advanced document processing | partial | partial | no | yes | `/documents/library/{id}/versions/{versionID}/ocr` -> `internal/documents/http/handler.go`; `/documents/search` -> `internal/documents/http/handler.go` | Text extraction, indexing, collaboration persistence, and disposition foundations exist; scanned-file providers, websocket delivery, and richer disposition actions remain. |
| CMMS IoT and predictive features | partial | partial | no | yes | `/cmms/iot` -> `internal/cmms/http/handler.go`; `/cmms/predictive` -> `internal/cmms/http/handler.go` | Readings, model metadata, and heuristic alerts are persisted; calibrated inference, streaming telemetry, and configurable thresholds remain. |
| MRP planning and execution foundation | yes | partial | no | yes | `/mrp` -> `internal/mrp/handler.go` | BOM, planning, WIP, scheduling, quality, genealogy, and analytics foundations are implemented; downstream staging and regulated-policy validation remain. |
| MRP compliance controls | yes | partial | no | yes | `/mrp` -> `internal/mrp/handlers.go` | Canonical snapshots, server versions/hashes, policy checks, actor-bound challenges, reauthentication, and audit evidence are implemented; every decision path, retention/export, and regulator-specific validation remain. |
| Connector transport foundation | yes | partial | no | yes | `/settings/integrations` -> `internal/connectors/http_handlers.go`; `/webhooks/connectors` -> `internal/connectors/http_webhook.go` | Vault references, fail-closed production configuration, outbox/inbox, replay keys, scheduled reconciliation, dead-letter audit/replay, and provider transport foundations exist; provider sandbox and limited-production certification remain. |
| Payment, carrier, marketplace, messaging, and identity connectors | partial | partial | no | yes | `/webhooks/connectors` -> `internal/connectors/http_webhook.go` | Midtrans/Stripe and development-gated provider paths have transport coverage; channel-specific business workflows and provider certification remain. |
| Consolidation PDF exports | partial | no | no | yes | `/finance/consol/tb/pdf` -> `internal/consol/http/handlers_tb.go`; `/finance/consol/pl/pdf` -> `internal/consol/http/handlers_pl.go`; `/finance/consol/bs/pdf` -> `internal/consol/http/handlers_bs.go` | The default `!production && !pdf` build intentionally disables exporters and can return 503. Release CI must run `go test` and `go build` with `-tags "production pdf"`. |
| Phase 14 transaction FX and P7 foundation | partial | partial | no | yes | none | Local acceptance evidence is historical/local only. Staging migration, production worker, provider, and cross-feature evidence remain pending. |
| RBAC and scoped access governance | yes | partial | no | yes | `/permissions` -> `internal/rbac/permissions_handler.go` | Permission middleware and module policies exist; effective-dated company/branch assignments and company-scoped access-review persistence/service foundations now exist, while tenant middleware wiring, migration/seed rollout, and the newer-module role matrix remain integration work. |
| Portal self-service depth | partial | partial | no | yes | `/portal` -> `internal/portal/handler.go` | Authenticated read-only views are available; profile changes, RFQ negotiation, chat, analytics, and broader self-service remain partial. |
| Reporting and BI depth | partial | partial | no | yes | `/report` -> `report/sample.go` | Finance, consolidation, analytics, board-pack, audit, and scheduled-report foundations exist; governed builder/widgets, wider operational coverage, and managed BI delivery remain. |

## Release interpretation

Do not describe a capability as production-ready solely because `code-complete` is
`yes`. A production release requires all four columns to be `yes`, plus the route,
provider, migration, and operational evidence named in the final column. In particular,
the Phase 14/P7 evidence guide records local verification; it does not certify staging
or production. The VPS deployment target satisfies the infrastructure selection only;
it does not certify incomplete feature workflows.

Run the same checks locally and in CI:

```bash
make release-check
make pdf-release-check
```
