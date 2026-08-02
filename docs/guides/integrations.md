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
| Payment gateways | Planned | Provider-neutral collection, callback, settlement, refund, and reconciliation work is specified; no gateway is implemented |
| E-commerce stores | Planned | Marketplace order/catalog/inventory/fulfillment sync is specified; no store connector is implemented |
| Shipping providers | Planned | Quote, booking, label, tracking, and reconciliation work is specified; no carrier connector is implemented |
| WhatsApp | Planned | Transactional delivery through the shared notification dispatcher is specified; no connector is implemented |
| SMS or push providers | Planned | SMS fallback and client-dependent push delivery are specified; no provider is implemented |
| SSO / identity providers | Planned | OIDC-first federation and controlled identity linking are specified; no provider is implemented |
| External BI tools | Planned | Versioned, company-scoped export contracts are specified; no managed BI connector is implemented |
| AI assistants | Planned | A governed, human-reviewed AI gateway is specified; no model provider is implemented |

Integrations must preserve company scope, avoid logging secrets, use idempotency for retries, and record failures without silently changing source-document state. See [`horizon-mvp.md`](horizon-mvp.md) and [`security.md`](security.md).
