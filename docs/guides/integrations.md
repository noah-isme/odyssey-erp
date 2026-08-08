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
| **Payment gateways — Stripe** | **Implemented (partial)** | Checkout intent and `payment.captured` event translation implemented; AR auto-allocation wired via shared outbox handler. Full webhook verification uses HMAC-SHA256 `Stripe-Signature` header. |
| **Payment gateways — MockPay** | **Implemented** | Test-only adapter for local and CI environments |
| **E-commerce — Shopify** | **Implemented (partial)** | `orders/create` and `orders/updated` webhook translation to `ecommerce.order.*` canonical events routed to sales module; inventory push not yet implemented |
| **Shipping — DHL** | **Implemented (stub)** | Adapter scaffolded; shipment booking and tracking translation not yet implemented |
| **AI gateway — OpenAI** | **Implemented (stub)** | Adapter scaffolded; governed AI use-cases not yet implemented |
| **Object storage — AWS S3** | **Implemented** | Board-pack and document storage driver backed by S3-compatible endpoints |
| **OIDC / identity providers** | **Implemented (stub)** | Adapter scaffolded; SSO federation not yet implemented |
| **WhatsApp** | **Implemented (stub)** | Adapter scaffolded; transactional message delivery not yet implemented |
| SMS or push providers | Planned | SMS fallback and client-dependent push delivery are specified; no provider is implemented |
| External BI tools | Planned | Versioned, company-scoped export contracts are specified; no managed BI connector is implemented |

Integrations must preserve company scope, avoid logging secrets, use idempotency for retries, and record failures without silently changing source-document state. See [`horizon-mvp.md`](horizon-mvp.md) and [`security.md`](security.md).

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
2. Use `conn.GetCredentials(a.vault)` for all credential access — never store keys in struct fields.
3. Register the adapter in both `cmd/odyssey/main.go` and `cmd/worker/main.go`.
4. Add a test file `adapter_test.go` covering: signature verification (valid, invalid, wrong key, malformed), webhook translation (all status paths), command dispatch (supported and unsupported), and lifecycle methods.
5. Update this document and `ROADMAP.md`.
