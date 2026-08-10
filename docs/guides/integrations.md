# Integration Boundaries

The [authoritative feature matrix](../reference/feature-matrix.md) is the release
status source. The [module catalog](../reference/module-catalog.md) is the capability
inventory. In this guide, **Implemented** means a scoped code path exists; it does not
mean integration-complete or production-certified. The cross-connector architecture,
sequencing, and acceptance gates are defined in the [External Integrations Plan](external-integrations-plan.md).

| Integration | Status | Boundary |
|---|---|---|
| Public REST API | Implemented | `/api/v1`; API keys, scopes, authenticated `/me`, and idempotent project creation |
| Webhooks | Implemented | Signed secrets, retries, deduplication, and company-scoped subscriptions |
| SMTP/email | Implemented | Worker-owned SMTP delivery for transactional email; credentials remain environment configuration |
| PDF/Gotenberg | Partial; release tag required | Server-side document rendering; the default non-production build disables the exporter and may return 503. CI verifies `production pdf` tags. |
| Indonesian tax/Coretax export | Code-complete; release validation pending | Versioned XML export and hashes; official portal/XSD acceptance remains an external gate |
| Banking statement import | Code-complete | CSV/OFX statement import; no live bank API connection |
| **Connector foundation** | **Code-complete; integration partial** | Provider-neutral `ProviderAdapter` interface, vault-encrypted `SecretRef`, transactional outbox/inbox, deduplication, canonical event routing to domain modules, and `/settings/integrations` administration UI |
| **Payment gateways — Midtrans** | **Code-complete; certification pending** | Snap checkout intent, SHA-512 webhook signature verification, monotonic payment lifecycle transitions, expiry/refund commands, status recovery, payout equation checks, and automatic AR invoice allocation via worker outbox. Sandbox mode default; production requires structured credentials with `is_prod=true`. |
| **Payment gateways — Stripe** | **Code-complete; certification pending** | Vault-resolved API/webhook secrets, live balance and charge calls, Stripe webhook verification, stable outbox idempotency keys, and canonical payment events. |
| **Payment gateways — MockPay** | **Development-only** | Test-only adapter for local and CI environments |
| **E-commerce — Shopify** | **Code-complete; integration partial** | Vault-resolved shop credentials, signed webhook verification, live order/inventory API calls, stable command keys, and `ecommerce.order.*` canonical events. Provider object mappings and reconciliation remain. |
| **Shipping — DHL** | **Code-complete; certification pending** | Vault-resolved API credentials, HTTPS production endpoint enforcement, authenticated shipment booking, HMAC callback verification, retry policy, and tracking-status translation. |
| **AI gateway — OpenAI** | **Planned / stub** | Adapter scaffolded; governed AI use-cases are not implemented |
| **Object storage — AWS S3** | **Code-complete; integration partial** | Board-pack/document storage plus live connector exports through S3-compatible `PutObject`; provider rollout and operational evidence remain. |
| **OIDC / identity providers** | **Code-complete; certification pending** | Vault-resolved issuer/client configuration, live discovery/JWKS verification for back-channel logout, and SCIM provisioning/deprovisioning calls. |
| **WhatsApp** | **Code-complete; certification pending** | Vault-resolved Cloud API credentials, HMAC callback verification, live message delivery, retry policy, and stable command keys. |
| SMS or push providers | Planned | SMS fallback and client-dependent push delivery are specified; no provider is implemented |
| External BI tools | Planned | Versioned, company-scoped export contracts are specified; no managed BI connector is implemented |

Integrations must preserve company scope, avoid logging secrets, use idempotency for retries, reject unverifiable callbacks, and record failures without silently changing source-document state. See [`horizon-mvp.md`](horizon-mvp.md) and [`security.md`](security.md).

## Runtime safety

Provider adapters resolve `Connection.SecretRef` through the application vault. Production requires `APP_MASTER_KEY`; a missing vault is a startup/configuration failure and never falls back to simulated credentials. Set `CONNECTORS_DEVELOPMENT_MODE=true` only in an isolated development or test process when explicit local fakes are required. The default is `false`.

Webhook inbox keys use provider event IDs where available and a SHA-256 payload key otherwise. This prevents identical callbacks from being accepted repeatedly even when a provider omits an event ID. Outbound commands retain their durable outbox correlation and command IDs as provider idempotency keys where the provider supports request headers; S3 exports use deterministic object keys.

## Provider credential payloads

The integration administration flow encrypts the provider credential JSON before storing it in `Connection.SecretRef`. The following fields are the provider contract; they are a schema reference for the administration/API layer, not a reason to insert plaintext secrets directly into the database:

| Provider | Required fields | Optional fields |
|---|---|---|
| Midtrans | `server_key` | `is_prod` (defaults to `false`), `base_url` (provider-compatible HTTPS endpoint) |
| Stripe | `api_key`, `webhook_secret` | — |
| AWS S3 | `access_key_id`, `secret_access_key`, `region`, `bucket` | `session_token`, `endpoint`, `use_path_style` |
| WhatsApp | `access_token`, `phone_number_id`, `app_secret` | — |
| DHL | `api_key`, `api_secret` | `account_number`, `base_url`, `webhook_secret`, `health_path` |
| Shopify | `shop_url`, `access_token` | `app_secret`, `webhook_secret` |
| OIDC | `issuer`, `client_id` | `client_secret`, `scim_endpoint`, `scim_token` |

Provider adapters decrypt these values only while handling a request. They must not log credential payloads or retain decrypted secrets in long-lived adapter state. `AllowPlaintextCredentials` exists only for in-process provider contract tests and must not be enabled by application wiring.

## AR Payment Auto-Allocation via Connector Events

When a payment gateway (e.g. Midtrans, Stripe) confirms a successful payment, the
following pipeline runs automatically in the background worker. `capture` and
`settlement` are the authoritative confirmation events; expiry, cancellation, and
refund events advance the payment intent but never create another AR payment:

```
Provider webhook
  → InboxProcessor (signature verification, deduplication)
  → TranslateWebhook → payment.captured/payment.settled CanonicalEvent
  → Outbox dispatcher → ar.RegisterOutboxHandlers
  → RegisterARPayment (creates ARPayment record + allocation against invoice)
```

The `order_id` / `CorrelationID` encodes the invoice reference using the format
`inv-{invoiceID}-{unixTimestamp}` so the worker can resolve and fully allocate the
invoice balance without a separate lookup table.

## Midtrans sandbox certification

Run the deterministic provider-compatible sandbox contract with:

```bash
make midtrans-sandbox-certify
```

The gate covers Snap checkout, duplicate and out-of-order callbacks, expiry,
partial and full refunds, gross/fee/tax/net/payout reconciliation, and recovery
after a provider-accepted checkout whose response times out. It asserts that a
callback replay cannot regress a terminal intent or create a second confirmation.
The test uses an injected sandbox transport and never requires credentials.

External Midtrans certification still requires a merchant sandbox account and
operator evidence for customer completion, provider expiry timing, refund bank
confirmation, and payout reporting. Keep those credentials outside the test
fixtures; the live connector defaults to the Midtrans sandbox unless
`is_prod=true` is explicitly configured.

## Payment reconciliation operations

See the [Payment Connector Recovery Runbook](payment-recovery.md) for operator
queries, refund triage, dead-letter replay controls, and metric alerts.

The worker runs `payments:reconcile` and `connectors:dead_letter_audit` every five
minutes. Reconciliation checks stale `CREATED`, `PENDING`, `AUTHORIZED`, `CAPTURED`,
`SETTLED`, and `PARTIALLY_REFUNDED` intents through adapters that implement status
lookup, then applies the same monotonic reducer used by callbacks. Each pass stores
run counts and duration evidence in `payment_reconciliation_runs`.

Lookup failures, unmapped provider statuses, missing local intents, and invalid state
transitions are persisted as open `payment_reconciliation_issues`. The worker sends
deduplicated in-app/email notifications to company administrators and resolves the
issue when a later provider lookup succeeds.

Refund requests are persisted before their `payment.refund` command is queued. They
move through `PENDING` → `PROCESSING` → provider-confirmed `PARTIALLY_REFUNDED` or
`REFUNDED`; a command that exhausts retries becomes `FAILED`. Connector dead letters
are retained in `connector_dead_letter_events`, alerted once per hour, and require an
explicit replay decision rather than automatic resubmission.

Recovery counters are exposed on the worker's `WORKER_METRICS_ADDR` endpoint
(default `:9091`) as Prometheus metrics, including
`odyssey_payment_recovery_transitions_total`,
`odyssey_payment_reconciliation_issues_total`,
`odyssey_payment_refund_status_total`, and
`odyssey_connector_dead_letters_total`.

## Adding a New Provider

1. Create `internal/connectors/providers/<name>/adapter.go` implementing `connectors.ProviderAdapter`.
2. Resolve credentials through `ProviderOptions.ResolveSecret`/the application vault — never store keys in struct fields or accept plaintext production secrets.
3. Register the adapter in both `cmd/odyssey/main.go` and `cmd/worker/main.go`.
4. Add a test file `adapter_test.go` covering: signature verification (valid, invalid, wrong key, malformed), webhook translation (all status paths), command dispatch (supported and unsupported), retry/idempotency behavior, and a provider sandbox contract using an injected HTTP transport.
5. Update this document and `ROADMAP.md`.
