# Odyssey ERP Documentation

Welcome to the Odyssey ERP documentation. Use this index to navigate the full documentation set.

## Core Documents

| Document | Purpose | Path |
|----------|---------|------|
| PRD | Why we're building this | [docs/PRD.md](./PRD.md) |
| Architecture | System design & structure | [docs/ARCHITECTURE.md](./ARCHITECTURE.md) |
| ADRs | Key technical decisions | [docs/ADR/](./ADR) |
| Design System | UI behavior & look | [docs/DESIGN.md](./DESIGN.md) |
| Agent Guidelines | How AI agents should work | [docs/AGENTS.md](./AGENTS.md) |
| Developer Guide | How to run & develop | [docs/README.md](./README.md) |
| Roadmap | What work remains | [docs/ROADMAP.md](./ROADMAP.md) |

## Getting Started
New to Odyssey ERP? Start here:
* [Quick Start](./getting-started/quick-start.md)
* [Native Setup](./getting-started/native-setup.md)
* [Test Accounts](./getting-started/test-accounts.md)
* [Troubleshooting](./getting-started/troubleshooting.md)

## Module Guides
Detailed guides for various business domains and technical workflows.

### Finance & Accounting
* [Accounting](./guides/accounting.md)
* [Fixed Assets](./guides/fixed-assets.md)
* [Payment Recovery](./guides/payment-recovery.md)
* [Tax Compliance](./guides/tax-compliance.md)
* [Tax Staff CoreTax Validation](./guides/tax-staff-coretax-validation.md)
* [Transaction FX](./guides/transaction-fx.md)
* [Core Finance Automation Plan](./guides/core-finance-automation-plan.md)

### Supply Chain & Manufacturing
* [Supply Chain](./guides/supply-chain.md)
* [Inventory Replenishment](./guides/inventory-replenishment.md)
* [Procurement](./guides/procurement.md)
* [Procurement Logistics Depth Plan](./guides/procurement-logistics-depth-plan.md)
* [Distribution](./guides/distribution.md)
* [Manufacturing MRP](./guides/manufacturing-mrp.md)
* [Manufacturing Governance Index](./guides/INDEX-MANUFACTURING-GOVERNANCE.md)
* [Manufacturing Governance Plan](./guides/manufacturing-governance-plan.md)
* [Manufacturing Governance Execution Map](./guides/MANUFACTURING-GOVERNANCE-EXECUTION-MAP.md)
* [Manufacturing Governance Execution Plan](./guides/MANUFACTURING-GOVERNANCE-EXECUTION-PLAN.md)
* [Manufacturing Governance Quick Start](./guides/MFG-GOVERNANCE-QUICK-START.md)
* [WMS (Warehouse Management)](./guides/wms.md)

### HR, Governance & Operations
* [Approvals & HR Core](./guides/approvals-hr-core.md)
* [Payroll](./guides/payroll.md)
* [CMMS (Maintenance)](./guides/cmms.md)
* [QMS (Quality Management)](./guides/qms.md)
* [Documents](./guides/documents.md)
* [Projects](./guides/projects.md)

### Customer & Sales
* [CRM](./guides/crm.md)
* [POS](./guides/pos.md)
* [Portal](./guides/portal.md)

### Architecture & Engineering
* [External Integrations Plan](./guides/external-integrations-plan.md)
* [Integrations](./guides/integrations.md)
* [Product Workflow Depth Plan](./guides/product-workflow-depth-plan.md)
* [Reporting Administration Depth Plan](./guides/reporting-administration-depth-plan.md)
* [Security](./guides/security.md)
* [Notifications](./guides/notifications.md)
* [Handlers](./guides/handlers.md)
* [Horizon MVP](./guides/horizon-mvp.md)
* [Phase 14 P7 Acceptance Evidence](./guides/phase14-p7-acceptance-evidence.md)

### Testing & Operations
* [E2E Browser Testing](./guides/e2e-browser-testing.md)
* [E2E Regression](./guides/e2e-regression.md)
* [Testing Auth Flow](./guides/testing_auth_flow.md)
* [Testing Runbook](./guides/testing-runbook.md)
* [User Profile Settings Runbook](./guides/runbook-user-preferences.md)
* [Boardpack Runbook](./guides/runbook-boardpack.md)
* [How-to Boardpack](./guides/howto-boardpack.md)

## Reference
* [Account Mapping](./reference/account-mapping.md)
* [Feature Matrix](./reference/feature-matrix.md)
* [Insights Usage](./reference/insights-usage.md)
* [Inventory](./reference/inventory.md)
* [Module Catalog](./reference/module-catalog.md)
* [Observability](./reference/observability.md)
* [Period Policy](./reference/period-policy.md)
* [RBAC (Role-Based Access Control)](./reference/rbac.md)
* [RBAC Examples (SQL)](./reference/RBAC_EXAMPLES.sql)
* [Reporting Catalog](./reference/reporting-catalog.md)
* [SLO Finance](./reference/slo-finance.md)

## Architecture Decisions
Key technical choices are documented as Architecture Decision Records (ADRs).

**Technical ADRs (`docs/ADR/`)**:
* [001 Modular Monolith](./ADR/001-modular-monolith.md)
* [002 Server-Side Rendering](./ADR/002-server-side-rendering.md)
* [003 SQLC over ORM](./ADR/003-sqlc-over-orm.md)
* [004 Asynq Background Jobs](./ADR/004-asynq-background-jobs.md)
* [005 Gotenberg PDF Reports](./ADR/005-gotenberg-pdf-reports.md)
* [006 Manual Dependency Injection](./ADR/006-manual-dependency-injection.md)
* [007 UUID Primary Keys](./ADR/007-uuid-primary-keys.md)
* [008 Decimal Type](./ADR/008-decimal-type.md)
* [009 Dark Theme Default](./ADR/009-dark-theme-default.md)
* [010 Valkey over Redis](./ADR/010-valkey-over-redis.md)

**Domain Decisions (`docs/decisions/`)**:
* [0001 Tools](./decisions/ADR-0001-tools.md)
* [0002 RBAC](./decisions/ADR-0002-rbac.md)
* [0003 Inventory Costing](./decisions/ADR-0003-inventory-costing.md)
* [0004 Accounting Model](./decisions/ADR-0004-accounting-model.md)
* [0005 Transaction FX](./decisions/ADR-0005-transaction-fx.md)
* [0006 Bank Ownership & Feed Ingestion](./decisions/ADR-0006-bank-ownership-and-feed-ingestion.md)
* [0007 Payment Execution & Settlement](./decisions/ADR-0007-payment-execution-and-settlement.md)
* [0008 P2P Matching & Exceptions](./decisions/ADR-0008-p2p-matching-and-exceptions.md)
* [0009 Asset Capitalization & Operations](./decisions/ADR-0009-asset-capitalization-and-operations.md)
* [0010 External Integrations Foundation](./decisions/ADR-0010-external-integrations-foundation.md)
* [0011 Governed Reporting](./decisions/ADR-0011-governed-reporting.md)
* [0012 Scoped RBAC](./decisions/ADR-0012-scoped-rbac.md)
* [0013 Fiscal Calendar & Timezone](./decisions/ADR-0013-fiscal-calendar-and-timezone.md)
* [0014 Repository & HTTP Boundaries](./decisions/ADR-0014-repository-and-http-boundaries.md)

## Releases
* [Version History](./releases/VERSION_HISTORY.md)
* [Production Release Checklist](./releases/production-release-checklist.md)
* [v0.10-core Staging Certification Record](./releases/v0.10-core-staging-certification.md)
* [v0.10.0-rc.6 candidate](./releases/v0.10-core-staging-certification.md)
* [v0.11-finance Sandbox Certification Record](./releases/v0.11-finance-sandbox-certification.md)
* [v0.10.0-rc.4 (superseded)](./releases/v0.10.0-rc.4.md)
* [v0.10.0-rc.3](./releases/v0.10.0-rc.3.md)
* [v0.10.0-rc.2](./releases/v0.10.0-rc.2.md)
* [v0.10.0-rc.1](./releases/v0.10.0-rc.1.md)
* [v0.9.1](./releases/v0.9.1.md)
* [v0.9.0](./releases/v0.9.0.md)
* [v0.8.0](./releases/v0.8.0.md)
* [v0.7.0](./releases/v0.7.0.md)

## Deployment
Instructions and details for deploying Odyssey ERP across different environments:
* [Local Deployment](./DEPLOYMENT.md)
* [Staging Deployment](./STAGING_DEPLOYMENT.md)
* [Finance Sandbox Deployment](./FINANCE_SANDBOX_DEPLOYMENT.md)
* [Production Deployment](./PRODUCTION_DEPLOYMENT.md)

## Archive
The `docs/archive/` directory contains older documents, historical feature checklists, previous release notes, completed phase summaries, and other deprecated documentation that is no longer actively maintained but kept for historical context.
