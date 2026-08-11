# Architecture Decision Records

This directory contains foundational architecture decisions for Odyssey ERP. Domain-specific ADRs are located in [`docs/decisions/`](../decisions/).

## Foundational ADRs

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [001-modular-monolith.md](001-modular-monolith.md) | Modular Monolith over Microservices | Accepted | 2024-01-10 |
| [002-server-side-rendering.md](002-server-side-rendering.md) | Server-Side Rendering over SPA | Accepted | 2024-01-15 |
| [003-sqlc-over-orm.md](003-sqlc-over-orm.md) | sqlc over ORM | Accepted | 2024-01-20 |
| [004-asynq-background-jobs.md](004-asynq-background-jobs.md) | Asynq for Background Jobs | Accepted | 2024-02-05 |
| [005-gotenberg-pdf-reports.md](005-gotenberg-pdf-reports.md) | Gotenberg for PDF Reports | Accepted | 2024-02-12 |
| [006-manual-dependency-injection.md](006-manual-dependency-injection.md) | Manual Dependency Injection | Accepted | 2024-02-20 |
| [007-uuid-primary-keys.md](007-uuid-primary-keys.md) | UUID Primary Keys | Accepted | 2024-03-01 |
| [008-decimal-type.md](008-decimal-type.md) | Decimal Type for Financial Data | Accepted | 2024-03-10 |
| [009-dark-theme-default.md](009-dark-theme-default.md) | Dark Theme as Default | Accepted | 2024-03-22 |
| [010-valkey-over-redis.md](010-valkey-over-redis.md) | Valkey over Redis | Accepted | 2024-04-05 |
| [011-postgresql-sole-database.md](011-postgresql-sole-database.md) | PostgreSQL as Sole Database | Accepted | 2024-04-18 |
| [012-css-custom-properties.md](012-css-custom-properties.md) | CSS Custom Properties over Utility Framework | Accepted | 2024-04-25 |

## Domain-Specific ADRs

| ADR | Title | Status |
|-----|-------|--------|
| [ADR-0001-tools.md](../decisions/ADR-0001-tools.md) | Pemilihan Tooling Inti | Partially superseded |
| [ADR-0002-rbac.md](../decisions/ADR-0002-rbac.md) | Role-Based Access Control | Accepted |
| [ADR-0003-inventory-costing.md](../decisions/ADR-0003-inventory-costing.md) | Inventory Costing Strategy | Accepted – Phase 3 |
| [ADR-0004-accounting-model.md](../decisions/ADR-0004-accounting-model.md) | Accounting Model & Ledger Architecture | Accepted – Phase 4.2 |
| [ADR-0005-transaction-fx.md](../decisions/ADR-0005-transaction-fx.md) | Transaction-level FX | Accepted |
| [ADR-0006-bank-ownership-and-feed-ingestion.md](../decisions/ADR-0006-bank-ownership-and-feed-ingestion.md) | Bank ownership and feed ingestion | Approved |
| [ADR-0007-payment-execution-and-settlement.md](../decisions/ADR-0007-payment-execution-and-settlement.md) | Payment execution and settlement | Approved |
| [ADR-0008-p2p-matching-and-exceptions.md](../decisions/ADR-0008-p2p-matching-and-exceptions.md) | Purchase-to-pay matching and exceptions | Approved |
| [ADR-0009-asset-capitalization-and-operations.md](../decisions/ADR-0009-asset-capitalization-and-operations.md) | Asset capitalization and operations | Approved |
| [ADR-0010-external-integrations-foundation.md](../decisions/ADR-0010-external-integrations-foundation.md) | External Integrations Foundation | Accepted |
| [ADR-0011-governed-reporting.md](../decisions/ADR-0011-governed-reporting.md) | Governed Reporting and Dashboards | Accepted |
| [ADR-0012-scoped-rbac.md](../decisions/ADR-0012-scoped-rbac.md) | Scoped Role-Based Access Control | Accepted |
| [ADR-0013-fiscal-calendar-and-timezone.md](../decisions/ADR-0013-fiscal-calendar-and-timezone.md) | Fiscal Calendars and Timezone Policy | Accepted |
| [ADR-0014-repository-and-http-boundaries.md](../decisions/ADR-0014-repository-and-http-boundaries.md) | Repository-Owned Persistence and HTTP Error Boundaries | Accepted |

## Creating a New ADR

To create a new architecture decision record, copy the format of existing ADRs. Name the file sequentially based on the next available number.

### Naming Conventions

- **Foundational ADRs:** `NNN-short-kebab-case-title.md` (e.g., `013-new-feature.md`)
- **Domain-Specific ADRs:** `ADR-NNNN-short-kebab-case-title.md` (e.g., `ADR-0015-new-domain.md`)

### Template

```markdown
# ADR-[NNN]: [Title]

## Status
[Proposed | Accepted | Rejected | Deprecated | Superseded]

## Date
[YYYY-MM-DD]

## Context
[What is the issue that we're seeing that is motivating this decision or change?]

## Decision
[What is the change that we're proposing and/or doing?]

## Alternatives
[What were the alternatives? Why did we choose this one?]

## Consequences
### Positive
- [Positive consequence 1]
### Negative
- [Negative consequence 1]
```
