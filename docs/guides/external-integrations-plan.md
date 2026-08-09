# External Integrations Plan

**Priority:** High

**Status:** In Progress — connector foundation, Midtrans, and provider transport hardening implemented; end-to-end channel workflows and operational rollout remain

**Scope:** Payment gateways, shipping/carrier services, marketplace synchronization,
WhatsApp/SMS/push delivery, external BI, identity providers, and AI connectors.

## Outcome

Deliver a provider-neutral integration platform that lets each company connect
external services without weakening Odyssey's accounting controls, tenant isolation,
or operational traceability. A provider outage, duplicate callback, expired token, or
partial synchronization must become a visible and recoverable integration state; it
must not silently corrupt an order, payment, shipment, user, or journal.

The target boundary is:

```text
Domain module -> transactional outbox -> connector command -> provider API
       ^                                                   |
       |                                                   v
       +-- canonical event <- inbox/dedupe <- signed callback or polling

Company connection -> encrypted secret reference + policy + object mappings
                   -> sync runs + attempts + health + audit + dead-letter replay
```

Provider adapters translate only between a provider contract and a versioned Odyssey
contract. Sales, AR, POS, delivery, inventory, notifications, auth, and analytics
services remain the owners of business state and invariants.

## Current provider hardening baseline

Stripe, S3 export, WhatsApp Cloud API, DHL, Shopify, and OIDC adapters now resolve
credentials through the vault and fail closed when production configuration is
missing. Provider calls use injected HTTP clients, bounded retries, and durable
command identifiers for idempotency where the provider contract permits it. Signed
callbacks are verified before translation, and inbox replay keys use provider event
IDs or deterministic payload hashes instead of random fallbacks. Local simulations
are available only with `CONNECTORS_DEVELOPMENT_MODE=true`; this flag must remain
unset/false in production.

The adapter layer is not the same as a complete business-channel rollout: WhatsApp
notification preferences/templates, Shopify object mapping/reconciliation, DHL
rate/label lifecycle, OIDC PKCE/linking policy, and BI export scheduling still need
their respective acceptance gates below.

## Existing baseline

Build on the current implementation rather than introducing a second job, webhook,
or notification framework:

- `internal/api/` supplies company-scoped API keys, encrypted signed-webhook secrets,
  deduplicated webhook deliveries, retries, and replay controls.
- `jobs/` and Asynq supply scheduled and asynchronous execution.
- `internal/notifications/` supplies persisted in-app notifications, per-type channel
  preferences, email dispatch, and delivery deduplication.
- `internal/sales/`, `internal/pos/`, `internal/ar/`, and `internal/ap/` own commercial
  documents and recorded payments. POS payment idempotency already exists.
- `internal/delivery/`, `internal/inventory/`, and `internal/wms/` own fulfillment,
  stock movement, picking, and delivery state.
- `internal/analytics/`, `internal/insights/`, and report exports provide curated data
  that can form the BI boundary.
- `internal/auth/`, `internal/users/`, `internal/rbac/`, and Redis-backed sessions own
  login, identity, permissions, and sessions.
- `internal/audit/`, shared approvals, durable tax/payroll outboxes, and correlation
  IDs provide reusable control patterns.

`internal/integration/` currently contains in-process accounting adapters. Record an
ADR before implementation and keep external provider adapters in a new, distinctly
named package (the proposed location is internal/connectors) so the two meanings are
not mixed.

## Scope boundaries

The first production increment includes one certified provider per connector type,
provider-neutral contracts, connection administration, and an operator recovery
surface. It does not promise feature parity across every provider.

The following remain outside the initial scope:

- Acting as a payment processor, carrier, identity provider, messaging network, BI
  warehouse, or foundation-model host.
- Allowing connectors to write directly to domain tables or post journals.
- Cross-company credentials, catalog mappings, identity policies, or data exports.
- Unreviewed autonomous AI actions, arbitrary SQL generation, or unrestricted tools.
- Marketing automation, warehouse CDC, SCIM provisioning, and multi-carrier route
  optimization unless promoted by a later, separately accepted increment.

## Delivery principles

- Every connection, command, event, mapping, run, and attempt is scoped by
  `company_id`, both in schema constraints and repository queries.
- Domain transactions write an outbox record atomically. Provider calls happen only
  after commit; inbound callbacks first enter a durable inbox before domain handling.
- Idempotency uses the company, provider, connection, operation, and provider object or
  event ID. Retry never means blindly resubmitting a financial or fulfillment action.
- Credentials are stored in a managed secret store or encrypted envelope referenced by
  the database. Plaintext credentials, authorization codes, tokens, and sensitive
  payload fields never enter logs, audit metadata, or job payloads.
- Canonical contracts are versioned. Provider payloads do not leak into domain APIs,
  and adapters do not bypass lifecycle, approval, period, stock, or accounting checks.
- Polling and callbacks converge on the same inbox handler and deduplication rule.
- A connector reports `healthy`, `degraded`, `action_required`, or `disabled`, with
  last success, last failure, token/consent expiry, lag, and queued/dead-letter counts.
- Feature flags and per-company allowlists gate every connector through sandbox,
  internal pilot, limited production, and general availability.

## Phase 0 — Shared connector foundation

**Estimate:** 3–4 weeks

**Blocks:** Every provider implementation

### Architecture and contracts

1. Record ADRs for package ownership, credential storage, transactional outbox/inbox,
   canonical contract versioning, and retention of provider payloads.
2. Define shared interfaces for connection validation, OAuth/token refresh, commands,
   callback verification, polling, health checks, and reconciliation. Keep each
   provider SDK behind an adapter interface and an owned HTTP client with explicit
   timeouts.
3. Add common records for connections, encrypted secret references, external object
   mappings, sync cursors/runs, inbox events, outbox commands, delivery attempts, and
   dead letters. Use narrow connector-specific extension tables for domain details.
4. Standardize state transitions, correlation IDs, error categories, retryability,
   exponential backoff with jitter, provider rate-limit handling, circuit breaking,
   and manual replay/cancel rules.
5. Define canonical event envelopes containing contract version, event ID, company,
   aggregate type/ID, occurrence time, correlation/causation IDs, and a typed payload.

### Administration and controls

- Add `/settings/integrations` for connection setup, scopes, status, last/next sync,
  token expiry, recent runs, failure details, disable/reconnect, test, and replay.
- Add least-privilege permissions for viewing integrations, managing connections,
  rotating credentials, replaying failures, and viewing sensitive operational data.
- Audit connection creation, scope changes, credential rotation, enable/disable,
  mapping changes, replay, and destructive unlink actions without auditing secrets.
- Add per-provider request, error, latency, throttling, token-refresh, queue-age,
  callback-signature, sync-lag, and dead-letter metrics with correlation-aware logs.
- Define retention and redaction for request/response metadata and raw provider
  payloads. Store raw bodies only when needed for replay or compliance.

### Foundation acceptance gate

- Database-backed tests prove company isolation, atomic outbox creation, inbox
  deduplication, concurrent worker claiming, retry scheduling, dead-lettering, replay,
  credential redaction, and payload retention.
- Contract tests run the same adapter suite against a fake provider and one provider
  sandbox, including timeout, `429`, `5xx`, malformed callback, rotated secret, token
  expiry, duplicate event, and out-of-order event cases.
- Killing a worker between provider success and local acknowledgement recovers by
  querying provider state and produces no duplicate external or domain action.

## Track A — Commerce integrations

### Phase A1 — Payment gateways

**Estimate:** 4–6 weeks

**Depends on:** Phase 0

Treat a gateway payment as customer collection for AR, sales orders, or POS. Do not
reuse `ap_payments`, which represent supplier payments, and do not mark an invoice or
ticket paid merely because checkout was created.

- Define payment intent, checkout/session, authorization, capture, settlement,
  expiration, cancellation, refund, dispute, fee, and payout reconciliation models.
- Use an explicit lifecycle such as `CREATED -> PENDING -> AUTHORIZED -> CAPTURED ->
  SETTLED`, with `EXPIRED`, `FAILED`, `CANCELLED`, `REFUNDED/PARTIALLY_REFUNDED`, and
  `DISPUTED` branches. Persist every provider status transition without allowing an
  older callback to regress terminal state.
- Create checkout links/QR/virtual-account/card-token requests through the outbox.
  Store only provider tokens and references; never store PAN, CVV, or equivalent
  sensitive payment credentials.
- Verify callback signatures and timestamps, ingest callbacks before acknowledging,
  and reconcile ambiguous requests through provider lookup before retry.
- Create and allocate an Odyssey AR/POS payment only at the configured authoritative
  event (normally capture or settlement). Refunds and disputes use existing controlled
  reversal/refund services and accounting dates; adapters never post journals.
- Reconcile gross amount, gateway fee, tax, net settlement, payout, currency, and FX.
  Surface unmatched and partially matched settlements in an operator queue.
- Start with one selected Indonesian-capable gateway after an ADR compares supported
  methods, settlement reporting, sandbox quality, webhook semantics, refunds, fees,
  tokenization scope, availability, and commercial/legal requirements.

**Acceptance gate:** A sandbox scenario completes checkout, duplicate/out-of-order
callbacks, expiration, full/partial refund, payout reconciliation, and recovery from
an ambiguous timeout with exactly one Odyssey payment and balanced accounting output.

### Phase A2 — Shipping and carrier services

**Estimate:** 3–5 weeks

**Depends on:** Phase 0; payment integration is not required

- Define provider-neutral address validation, service/quote, shipment, parcel, label,
  pickup, manifest, tracking event, proof-of-delivery, cancellation, return, and charge
  models. Store measurements and money as exact units/decimals.
- Map shipments to existing delivery orders and WMS picks. Booking requires a released,
  sufficiently picked delivery; a tracking callback may advance delivery state only
  through the delivery service's validated lifecycle.
- Support quote expiry, selected service, dimensional weight, multi-parcel shipments,
  label/document storage, pickup cut-offs, tracking history, failed delivery, return to
  sender, and manual-carrier fallback.
- Ingest signed callbacks when available and poll active shipments as a fallback.
  Deduplicate by provider event ID and protect against out-of-order scans.
- Reconcile quoted versus billed freight and expose exceptions without modifying the
  sales order, invoice, inventory, or GL directly.
- Certify one aggregator or direct carrier first; add adapters only when the canonical
  contract covers their required semantics.

**Acceptance gate:** Sandbox or provider certification proves quote, booking, label,
pickup, in-transit, delivered, failed/returned, cancellation, duplicate tracking, and
provider outage flows; stock moves once and the full tracking history remains auditable.

### Phase A3 — Marketplace synchronization

**Estimate:** 6–8 weeks

**Status:** Partial — adapter transport exists; end-to-end marketplace synchronization,
reconciliation, and production certification remain.

**Depends on:** Phase 0 and the stable payment/shipping domain contracts from A1/A2

- Add channel/store connections and mappings for warehouses, products/SKUs, variants,
  units, taxes, prices, promotions, customers, shipping services, payment methods, and
  marketplace status codes.
- Implement inbound order, cancellation, return/refund, and fulfillment events plus
  outbound catalog, price, available-to-promise inventory, shipment, and tracking
  updates. Treat marketplace orders as external source documents with immutable
  provider identifiers and snapshots.
- Define Odyssey as the authority for physical stock and fulfillment after order
  acceptance. Publish available-to-promise stock, including reservations and a safety
  buffer, rather than raw on-hand quantity.
- Use cursor-based incremental synchronization plus scheduled reconciliation. Webhooks
  accelerate ingestion but never replace reconciliation. Detect gaps, duplicates,
  missing mappings, stale cursors, and changed provider objects.
- Define conflict policy per field: reject/quarantine unmapped orders, never silently
  create products, preserve marketplace prices/taxes as order facts, and require review
  for unsupported discounts or negative totals.
- Make cancellation/refund race conditions explicit. A cancellation after pick or ship
  becomes an exception/return workflow rather than deleting a sale or stock movement.
- Pilot one marketplace/storefront and one warehouse. Add the next channel only after
  the adapter conformance suite and operational runbook pass unchanged.

**Acceptance gate:** End-to-end tests cover new/updated/duplicate orders, unmapped SKU,
stock race, partial fulfillment, cancellation before/after pick, return/refund,
pagination restart, webhook gap, and reconciliation repair with no oversell or duplicate
order/payment/stock movement.

## Track B — Communications

### Phase B1 — WhatsApp, SMS, and push

**Estimate:** 3–4 weeks

**Depends on:** Phase 0

- Generalize the notification dispatcher to `in_app`, `email`, `whatsapp`, `sms`, and
  `push` channel ports while retaining per-notification-type preferences and dedupe.
- Add company channel connections, approved/versioned templates, locale, variables,
  sender identities, recipient consent/opt-out, quiet hours, fallback order, and
  transactional-versus-marketing classification.
- Store a notification dispatch plus per-channel attempts and provider message IDs.
  Ingest sent/delivered/read/failed receipts idempotently and show final channel state.
- Reject missing template variables and invalid destinations before enqueue. Redact
  phone numbers and device tokens from routine logs and restrict their operational UI.
- Implement transactional WhatsApp first, then SMS fallback, then web/mobile push when
  Odyssey has a registered client capable of managing device tokens. Do not claim push
  support based only on a provider adapter.
- Keep OTP/identity verification separate from business notifications so its rate
  limits, templates, data retention, and abuse controls can be stricter.

**Acceptance gate:** Provider sandbox tests cover opt-out, preference changes, template
rejection, duplicate enqueue/receipt, invalid token/number, provider throttle/outage,
fallback, and secret rotation without duplicate end-user messages.

## Track C — Enterprise integrations

### Phase C1 — External BI

**Estimate:** 2–4 weeks

**Depends on:** Phase 0

- Publish a versioned, read-only analytics contract from curated reporting models.
  Start with scheduled Parquet/CSV exports to configured object storage or scoped APIs;
  do not expose the transactional database directly as the default integration.
- Define datasets, dimensions, measures, currencies, accounting status, time zone,
  schema version, watermark, correction/deletion semantics, and data-freshness SLA.
- Add company-scoped export connections, schedules, incremental checkpoints, run
  manifests, row counts, hashes, failure/retry state, and backfill controls.
- Use dedicated read-only credentials and least-privilege object paths. Encrypt in
  transit/at rest, define retention, and audit export configuration and download/access
  where the target permits it.
- Certify one consumption path (for example, object storage plus a documented Power BI
  or equivalent import model) before adding warehouse-native writers or CDC.

**Acceptance gate:** A full load, incremental load, correction, schema-version change,
retry, and backfill reconcile source/run row counts and hashes; one company's export
contains no identifiers or rows belonging to another company.

### Phase C2 — Identity providers

**Estimate:** 4–6 weeks

**Depends on:** Phase 0 and completion of the production security baseline in
`security.md`

- Implement OIDC authorization-code flow with PKCE first. Add SAML only when a customer
  requirement justifies its metadata, signature, and operational complexity.
- Add company identity connections with issuer/metadata, client secret reference,
  allowed domains, claim mappings, group-to-role mappings, login policy, and certificate
  or key rotation state.
- Bind external subject plus issuer to one Odyssey identity. Email alone is not a stable
  identity key. Require a controlled linking flow for existing users and prevent an
  external identity from linking across companies accidentally.
- Support explicit invitation first; allow just-in-time provisioning only as an optional,
  policy-controlled. Keep role assignment deny-by-default and cap mapped roles so an
  IdP group cannot grant more privilege than the connection policy allows.
- Rotate the local session after federation, validate issuer/audience/nonce/state/time,
  record authentication assurance, and implement IdP-initiated logout/session revocation
  only where the provider contract supports it safely.
- Preserve tested break-glass local administrators before enforcing SSO. SCIM user/group
  lifecycle and automated deprovisioning are a separate follow-on phase.

**Acceptance gate:** Tests cover discovery, login, invitation/linking, domain mismatch,
duplicate email, removed/changed group, disabled user, expired/rotated key, replayed
assertion, logout, IdP outage, and break-glass access without privilege escalation.

## Track D — Governed AI connectors

### Phase D1 — AI gateway and first use cases

**Estimate:** 4–6 weeks

**Depends on:** Phase 0 plus approved data-classification, privacy, retention, and AI-use
policies

- Add one internal AI gateway with provider/model adapters, structured requests,
  timeouts, retry rules, budgets, rate limits, model allowlists, and per-company feature
  flags. Domain packages never call external model SDKs directly.
- Classify and minimize data before sending it. Redact secrets, credentials, payment
  tokens, unnecessary personal data, and cross-company context; configure provider data
  retention/training controls contractually and technically.
- Persist prompt/template version, model, parameters, input data references or hashes,
  output, token/cost/latency, user, company, correlation ID, review outcome, and policy
  decision. Apply a retention policy rather than logging raw prompts indiscriminately.
- Start with read-only, human-reviewed tasks such as narrative summaries of existing
  reports, draft customer/supplier communications, and extraction into a review queue.
  Keep accounting entries, payments, inventory movements, approvals, identity changes,
  and outbound messages behind normal validation and explicit human confirmation.
- Treat retrieved documents and external content as untrusted. Use allowlisted tools,
  schema-constrained output, bounded context, authorization-filtered retrieval, prompt-
  injection tests, and no arbitrary SQL, URL fetch, code execution, or connector replay.
- Build an offline evaluation set for accuracy, grounding, refusal, sensitive-data
  leakage, prompt injection, multilingual behavior, latency, and cost. Version quality
  gates by use case rather than assuming one model is safe for every workflow.

**Acceptance gate:** A provider outage or model change degrades gracefully; evaluation
thresholds and budget caps pass; adversarial tests cannot access another company, leak a
secret, call a disallowed action, or turn an unreviewed output into a domain mutation.

## Recommended program sequence

After Phase 0, the tracks can proceed in parallel with separate owners:

| Release | Scope | Exit signal |
|---|---|---|
| R1 — Foundation | Shared runtime, admin, permissions, audit, observability | Fake-provider and one sandbox conformance suite pass |
| R2 — Initial value | Transactional WhatsApp, BI export, OIDC pilot | Three limited companies operate with runbooks and no cross-tenant findings |
| R3 — Commerce core | One gateway and one carrier | Collection-to-reconciliation and pick-to-delivery scenarios pass |
| R4 — Channel commerce | One marketplace/store and one warehouse | Thirty days of reconciliation without duplicate orders or stock drift |
| R5 — Governed AI | AI gateway and one read-only use case | Security evaluation, human-review, quality, and cost gates pass |

The commerce critical path is Phase 0 -> A1/A2 -> A3. Identity, BI, and communications
can start independently after Phase 0. AI should start only after the shared controls and
data-governance decisions are operational. Estimates exclude provider contracting,
production-account approval, template approval, marketplace review, and legal/privacy
review; those external lead times should begin during Phase 0.

With one shared squad, the listed work is roughly 29–43 engineering weeks and should
be delivered connector by connector rather than held for one large release. With three
small stream teams (commerce, communications/enterprise, and platform/security), the
target is a 3–4 week foundation, first limited-production connectors in 8–12 weeks, and
the full initial program in roughly 18–24 weeks. This assumes two backend engineers,
one frontend engineer, and shared QA, product, security/SRE, and finance/operations
review across the active streams; external provider approvals can extend calendar time.

## Provider selection scorecard

Before committing an adapter, record an ADR scoring:

- Required Indonesian methods, carriers, marketplace features, languages, regions, and
  currencies.
- Sandbox fidelity, API/versioning quality, webhook signatures, idempotency support,
  reconciliation/reporting, rate limits, pagination, and historical replay.
- OAuth/token rotation, IP restrictions, data location/retention, sub-processors,
  incident notification, audit/compliance evidence, and deletion/export support.
- Availability/SLA, support escalation, status visibility, backward-compatibility
  policy, SDK maintenance, pricing, settlement/contract terms, and exit/data-portability
  plan.
- Fit to the canonical contract. A provider requiring unsafe domain shortcuts is not a
  suitable first adapter even when its commercial coverage is attractive.

## Definition of done for every connector

A connector remains **Planned** or **Pilot** until all applicable conditions pass:

1. Canonical contract and lifecycle are documented and provider-neutral.
2. Schema, repository, handler, and worker paths enforce company scope.
3. Secrets are encrypted/referenced, redacted, rotatable, and absent from job payloads.
4. Outbox/inbox, idempotency, ordering, retry, rate-limit, dead-letter, and replay cases
   have database-backed tests.
5. Domain mutations go through the owning service and preserve approval, stock,
   accounting, FX, tax, and audit invariants.
6. Sandbox contract tests and an end-to-end certification scenario pass.
7. Admin health, metrics, structured logs, alerts, support diagnostics, and operator
   runbooks cover outage, reconnect, reconciliation, replay, and disable/unlink.
8. Security, privacy, data-retention, backup/restore, and provider-exit reviews are
   recorded, including deletion of credentials and retained payloads.
9. A limited production pilot meets its lag, success-rate, reconciliation, duplicate,
   and incident thresholds before general availability.
10. Documentation and the module catalog distinguish implemented providers and exact
    supported operations from planned or unsupported capabilities.

## Program success measures

- Zero cross-company integration reads, writes, callbacks, exports, or AI context.
- Zero duplicate financial, order, stock, shipment, or notification effects during
  retry/replay testing and production pilots.
- At least 99% of valid callbacks enter the durable inbox successfully; provider/API
  delivery and synchronization SLOs are defined per connector rather than hidden in one
  aggregate percentage.
- All failed operations are traceable by company, connection, correlation ID, provider
  reference, attempt, and final recovery action.
- Marketplace inventory/order drift, payment settlement exceptions, shipment tracking
  lag, message delivery failures, BI freshness, identity login failures, and AI
  cost/quality each have an owned reconciliation metric and alert threshold.
