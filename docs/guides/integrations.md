# Integration Boundaries

Statuses in this document are authoritative: **Implemented** is available in the current code/documentation; **Planned** appears as a committed roadmap item but is not a production connector; **Unsupported** has no current connector or committed interface.

| Integration | Status | Boundary |
|---|---|---|
| Public REST API | Implemented | `/api/v1`; API keys, scopes, authenticated `/me`, and idempotent project creation |
| Webhooks | Implemented | Signed secrets, retries, deduplication, and company-scoped subscriptions |
| SMTP/email | Implemented | Worker-owned SMTP delivery for transactional email; credentials remain environment configuration |
| PDF/Gotenberg | Implemented | Server-side document rendering; external Gotenberg endpoint required in deployment |
| Indonesian tax/Coretax export | Implemented, release validation pending | Versioned XML export and hashes; official portal/XSD acceptance remains an external gate |
| Banking statement import | Implemented | CSV/OFX statement import; no live bank API connection |
| Payment gateways | Planned | Roadmap candidate; no gateway connector is documented |
| E-commerce stores | Planned | Marketplace/store sync is a roadmap item; no Tokopedia/Shopee connector is documented |
| Shipping providers | Planned | JNE/J&T/SiCepat are roadmap candidates; no carrier API is documented |
| WhatsApp | Planned | Invoice/notification delivery candidate; no connector is documented |
| SMS or push providers | Unsupported | No provider, credentials, delivery contract, or retry policy is documented |
| SSO / identity providers | Unsupported | No SAML/OIDC connector is documented |
| External BI tools | Unsupported | Exports exist, but no managed BI connector is documented |
| AI assistants | Unsupported | No supported AI integration boundary is documented |

Integrations must preserve company scope, avoid logging secrets, use idempotency for retries, and record failures without silently changing source-document state. See [`horizon-mvp.md`](horizon-mvp.md) and [`security.md`](security.md).
