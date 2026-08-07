# ADR-0010: External Integrations Foundation

## Status
Accepted

## Context
Odyssey ERP requires a scalable and secure way to integrate with external providers (payment gateways, shipping carriers, marketplaces, BI tools, IdPs, and AI gateways). Currently, `internal/integration/` is used for in-process accounting adapters, which risks blurring the lines between internal domain coupling and external service boundaries. A robust foundation is required before building specific external connectors to ensure security, idempotency, traceability, and tenant isolation.

## Decision

1. **Package Ownership**: All external provider adapters and their shared foundational logic will be located in `internal/connectors/`. The existing `internal/integration/` package will remain dedicated strictly to internal in-process adapters (e.g., between Procurement and AP).

2. **Credential Storage**: Provider credentials (API keys, OAuth tokens) will never be stored in plaintext in the database or logged. They must be stored in an encrypted envelope referenced by the database (or a managed secret store).

3. **Transactional Outbox/Inbox**:
   - **Outbound**: Domain modules (e.g., Sales, WMS) will publish canonical events to a durable outbox within the same transaction. The `connectors` module will process this outbox asynchronously.
   - **Inbound**: Webhooks and polling responses will be written to a durable inbox first, ensuring deduplication and signature verification before being processed into canonical events for the domain modules.

4. **Canonical Contract Versioning**:
   - The `connectors` module will translate provider-specific payloads into a versioned Odyssey canonical contract.
   - Domain modules will never depend on provider-specific models.

5. **Retention of Provider Payloads**:
   - Raw provider request/response payloads will be retained only for a limited configurable period to facilitate debugging and replay.
   - Payloads will be scrubbed of sensitive fields (PANs, tokens, PII) before persistence unless encrypted.

## Consequences
- **Positive**: Complete separation of external dependencies from core ERP domains.
- **Positive**: High reliability through inbox/outbox patterns, preventing data loss during provider outages.
- **Negative**: Increased upfront engineering effort to establish the shared foundation before delivering the first specific integration (e.g., payment gateway).

## References
- [External Integrations Plan](../guides/external-integrations-plan.md)
