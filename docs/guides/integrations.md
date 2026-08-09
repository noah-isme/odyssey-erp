# Integration Boundaries

The [module catalog](../reference/module-catalog.md) is the authoritative capability-status source. In this guide, **Implemented** is available in the current code/documentation; **Planned** appears as a committed roadmap item but is not a production connector; **Unsupported** has no current connector or committed interface. The cross-connector architecture, sequencing, and acceptance gates are defined in the [External Integrations Plan](external-integrations-plan.md).

| Integration | Status | Boundary |
|---|---|---|
| Public REST API | Implemented | `/api/v1`; API keys, scopes, authenticated `/me`, and idempotent project creation |
| Webhooks | Implemented | Signed secrets, retries, deduplication, and company-scoped subscriptions |
| SMTP/email | Implemented | Worker-owned SMTP delivery for transactional email; credentials remain environment configuration |
| PDF/Gotenberg | Implemented | Server-side document rendering; external Gotenberg endpoint required in deployment |
| Indonesian tax/Coretax export | Implemented, release validation pending | Versioned XML export and hashes; official portal/XSD acceptance remains an external gate |
| Banking statement import | Implemented | CSV/OFX statement import; no live bank API connection |
| **Connector foundation** | **Implemented** | Provider-neutral `ProviderAdapter` interface, vault-encrypted `SecretRef`, transactional outbox/inbox, deduplication, canonical event routing to domain modules, and `/settings/integrations` administration UI |
| **Payment gateways — Midtrans** | **Implemented** | Snap checkout intent, SHA-512 webhook signature verification, `payment.captured/authorized/failed` canonical events, and automatic AR invoice allocation via worker outbox. Sandbox mode default; production requires `isProd=true`. 17-test suite covers signature, translation, and lifecycle. |
| **Payment gateways — Stripe** | **Implemented** | Vault-resolved API/webhook secrets, live balance and charge calls, Stripe webhook verification, stable outbox idempotency keys, and canonical payment events. |
| **Payment gateways — MockPay** | **Implemented** | Test-only adapter for local and CI environments |
| **E-commerce — Shopify** | **Implemented** | Vault-resolved shop credentials, signed webhook verification, live order/inventory API calls, stable command keys, and `ecommerce.order.*` canonical events. Provider object mappings are required for inventory commands. |
| **Shipping — DHL** | **Implemented** | Vault-resolved API credentials, HTTPS production endpoint enforcement, authenticated shipment booking, HMAC callback verification, retry policy, and tracking-status translation. |
| **AI gateway — OpenAI** | **Implemented (stub)** | Adapter scaffolded; governed AI use-cases not yet implemented |
| **Object storage — AWS S3** | **Implemented** | Board-pack/document storage plus live connector exports through S3-compatible `PutObject`; vault credentials and deterministic object keys prevent simulated success in production. |
| **OIDC / identity providers** | **Implemented** | Vault-resolved issuer/client configuration, live discovery/JWKS verification for back-channel logout, and SCIM provisioning/deprovisioning calls. |
| **WhatsApp** | **Implemented** | Vault-resolved Cloud API credentials, HMAC callback verification, live message delivery, retry policy, and stable command keys. |
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
| Stripe | `api_key`, `webhook_secret` | — |
| AWS S3 | `access_key_id`, `secret_access_key`, `region`, `bucket` | `session_token`, `endpoint`, `use_path_style` |
| WhatsApp | `access_token`, `phone_number_id`, `app_secret` | — |
| DHL | `api_key`, `api_secret` | `account_number`, `base_url`, `webhook_secret`, `health_path` |
| Shopify | `shop_url`, `access_token` | `app_secret`, `webhook_secret` |
| OIDC | `issuer`, `client_id` | `client_secret`, `scim_endpoint`, `scim_token` |

Provider adapters decrypt these values only while handling a request. They must not log credential payloads or retain decrypted secrets in long-lived adapter state. `AllowPlaintextCredentials` exists only for in-process provider contract tests and must not be enabled by application wiring.

## AR Payment Auto-Allocation via Connector Events

When a payment gateway (e.g. Midtrans, Stripe) confirms a successful payment, the
following pipeline runs automatically in the background worker:

```
Provider webhook
  → InboxProcessor (signature verification, deduplication)
  → TranslateWebhook → payment.captured CanonicalEvent
  → Outbox dispatcher → ar.RegisterOutboxHandlers
  → RegisterARPayment (creates ARPayment record + allocation against invoice)
```

The `order_id` / `CorrelationID` encodes the invoice reference using the format
`inv-{invoiceID}-{unixTimestamp}` so the worker can resolve and fully allocate the
invoice balance without a separate lookup table.

## Adding a New Provider

1. Create `internal/connectors/providers/<name>/adapter.go` implementing `connectors.ProviderAdapter`.
2. Resolve credentials through `ProviderOptions.ResolveSecret`/the application vault — never store keys in struct fields or accept plaintext production secrets.
3. Register the adapter in both `cmd/odyssey/main.go` and `cmd/worker/main.go`.
4. Add a test file `adapter_test.go` covering: signature verification (valid, invalid, wrong key, malformed), webhook translation (all status paths), command dispatch (supported and unsupported), retry/idempotency behavior, and a provider sandbox contract using an injected HTTP transport.
5. Update this document and `ROADMAP.md`.
