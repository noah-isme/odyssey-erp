# Odyssey ERP Documentation

Dokumentasi lengkap untuk Odyssey ERP - Modern ERP system built with Go.

## 🚀 Getting Started

| Document | Description |
|----------|-------------|
| [Quick Start (Docker)](getting-started/quick-start.md) | Supported Compose setup |
| [Native Setup](getting-started/native-setup.md) | Setup tanpa Docker |
| [Test Accounts](getting-started/test-accounts.md) | Kredensial testing |
| [Troubleshooting](getting-started/troubleshooting.md) | Common issues & fixes |

## 🏗️ Architecture

| Document | Description |
|----------|-------------|
| [Overview](architecture/arsitektur.md) | Arsitektur modular monolith |
| [Directory Structure](architecture/directory-structure.txt) | Struktur folder project |

## 📖 Guides

### Development
| Document | Description |
|----------|-------------|
| [Handler Guidelines](guides/handlers.md) | Pola handler HTTP |
| [Testing Guide](guides/testing-runbook.md) | Menjalankan tests |
| [Test Coverage Hardening Audit (2026-08-09)](archive/audits/test-coverage-hardening-2026-08-09.md) | Coverage baseline dan tindak lanjut modul berisiko tinggi |
| [E2E Browser Testing](guides/e2e-browser-testing.md) | Setup Playwright, test data, dan CI integration |

### Operations
| Document | Description |
|----------|-------------|
| [Accounting Runbook](guides/accounting.md) | Operasi accounting |
| [Core Finance Automation Plan](guides/core-finance-automation-plan.md) | Execution-ready plan untuk bank feeds, forecasting, payment, P2P, dan operasi aset |
| [External Integrations Plan](guides/external-integrations-plan.md) | Payment, carrier, marketplace, messaging, BI, identity, dan AI connectors |
| [Payment Connector Recovery](guides/payment-recovery.md) | Scheduled reconciliation, unmatched-payment alerts, refund recovery, dead letters, and metrics |
| [Procurement SOP](guides/procurement.md) | Prosedur procurement |
| [Procurement and Logistics Depth Plan](guides/procurement-logistics-depth-plan.md) | RFQ, supplier intelligence, fleet, distribution planning, dan freight-cost execution plan |
| [Distribution planning](guides/distribution.md) | Planning horizons, load/shipment lifecycle, dispatch, delivery, inventory posting, and remaining gaps |
| [Board Pack](guides/howto-boardpack.md) | Generate board pack |
| [Profil & Pengaturan](guides/user-profile-settings.md) | Panduan pengguna untuk profil, tema, bahasa, notifikasi, dan password |
| [Runbook Preferensi Pengguna](guides/runbook-user-preferences.md) | Deployment dan troubleshooting pengaturan UI |
| [Notifications & Email](guides/notifications.md) | Arsitektur, API, SMTP, deployment, dan troubleshooting notifikasi |
| [Approvals & HR Core](guides/approvals-hr-core.md) | Policy routing, delegation, leave workflow, dan attendance CSV |
| [Payroll Engine](guides/payroll.md) | Versioned TER/PTKP/BPJS rules, payroll approval, posting, payment export, dan payslips |
| [Tax Compliance](guides/tax-compliance.md) | Immutable faktur, PPN/PPh ledgers, GL reconciliation, period lock, dan Coretax export |
| [Transaction-level FX](guides/transaction-fx.md) | Daily rates, AR/AP valuation, realized FX, revaluation, reversal, and operations |
| [Phase 14/P7 Acceptance Evidence](guides/phase14-p7-acceptance-evidence.md) | Local gate results, evidence checklist, and remaining release gates |
| [Horizon MVP Foundation](guides/horizon-mvp.md) | WMS, MRP, POS, projects/timesheets, API/webhooks, portals, and isolation rules |
| [Coretax Validation Sign-off](guides/tax-staff-coretax-validation.md) | Checklist staf pajak untuk artefak DJP, portal testing, rekonsiliasi, dan persetujuan rilis |
| [CRM](guides/crm.md) | Leads, pipeline, activities, reminders, ownership, conversion, dan win/loss |
| [Manufacturing / MRP](guides/manufacturing-mrp.md) | BOM revisions, planning, WIP, scheduling, quality, analytics, and documented compliance boundaries |
| [Manufacturing Governance Plan](guides/manufacturing-governance-plan.md) | Mandatory controlled decisions, manufacturing quality boundaries, and staging certification |
| [CMMS](guides/cmms.md) | Standalone maintenance, assets, and work orders |
| [QMS](guides/qms.md) | Enterprise quality management, inspections, NCRs, and CAPAs |
| [Document Management](guides/documents.md) | Managed storage, versioning, e-signature, retention, and permissions |
| [WMS](guides/wms.md) | Warehouse management, bins, picking, and barcode aliases |
| [Portal](guides/portal.md) | Customer, supplier, and employee portals |
| [Product Workflow Depth Plan](guides/product-workflow-depth-plan.md) | Project planning/budgeting, POS value programs/hardware, HR talent workflows, and CRM campaigns/segmentation |
| [Reporting and Administration Depth Plan](guides/reporting-administration-depth-plan.md) | Governed report builder/widgets, operational and HR coverage, role matrix, locale, timezone, and fiscal policy |
| [Projects](guides/projects.md) | Projects, tasks, members, timesheets, and scope boundaries |
| [POS](guides/pos.md) | Terminals, sessions, tickets, payments, refunds, and gaps |
| [Supply Chain](guides/supply-chain.md) | WMS, warehouse, fulfillment, supplier, and logistics boundaries |
| [Integration Boundaries](guides/integrations.md) | Implemented, planned, and unsupported integrations |
| [Lifecycle Reference](architecture/lifecycles.md) | Supported document states and cross-module flows |

## 📚 Reference

| Document | Description |
|----------|-------------|
| [RBAC System](reference/rbac.md) | Role-Based Access Control |
| [RBAC SQL Examples](reference/RBAC_EXAMPLES.sql) | SQL scripts untuk RBAC |
| [Inventory Integration](reference/inventory.md) | Integrasi inventory |
| [Inventory traceability & replenishment](guides/inventory-replenishment.md) | Lot, serial, costing, dan reorder PR |
| [HTTP E2E regression](guides/e2e-regression.md) | Smoke/regression suite tanpa browser |
| [Fixed Assets operations](guides/fixed-assets.md) | Kategori, register, depresiasi, dan disposal aset |
| [Account Mapping](reference/account-mapping.md) | Default account setup |
| [Period Policy](reference/period-policy.md) | Kebijakan periode accounting |
| [Observability](reference/observability.md) | Monitoring & metrics |
| [SLO Finance](reference/slo-finance.md) | Service Level Objectives |
| [Authoritative Feature Matrix](reference/feature-matrix.md) | Four-dimensional release status: code, integration, production certification, and documentation |
| [Module Catalog](reference/module-catalog.md) | Capability inventory and guide navigation |
| [Reporting Catalog](reference/reporting-catalog.md) | Reports, KPIs, routes, data sources, filters, and exports |

## 📝 Architecture Decision Records

| ADR | Title |
|-----|-------|
| [ADR-0001](decisions/ADR-0001-tools.md) | Tooling Stack |
| [ADR-0002](decisions/ADR-0002-rbac.md) | RBAC Implementation |
| [ADR-0003](decisions/ADR-0003-inventory-costing.md) | Inventory Costing |
| [ADR-0004](decisions/ADR-0004-accounting-model.md) | Accounting Model |
| [ADR-0005](decisions/ADR-0005-transaction-fx.md) | Transaction-level FX |
| [ADR-0006](decisions/ADR-0006-bank-ownership-and-feed-ingestion.md) | Bank Ownership and Feed Ingestion |
| [ADR-0007](decisions/ADR-0007-payment-execution-and-settlement.md) | Payment Execution and Settlement |
| [ADR-0008](decisions/ADR-0008-p2p-matching-and-exceptions.md) | P2P Matching and Exceptions |
| [ADR-0009](decisions/ADR-0009-asset-capitalization-and-operations.md) | Asset Capitalization and Operations |
| [ADR-0010](decisions/ADR-0010-external-integrations-foundation.md) | External Integrations Foundation |
| [ADR-0011](decisions/ADR-0011-governed-reporting.md) | Governed Reporting and Dashboards |
| [ADR-0012](decisions/ADR-0012-scoped-rbac.md) | Scoped Role-Based Access Control |
| [ADR-0013](decisions/ADR-0013-fiscal-calendar-and-timezone.md) | Fiscal Calendars and Timezone Policy |
| [ADR-0014](decisions/ADR-0014-repository-and-http-boundaries.md) | Repository-Owned Persistence and HTTP Error Boundaries |

## 📦 Releases

| Version | Notes |
|---------|-------|
| [v0.10.0-rc.3](releases/v0.10.0-rc.3.md) | **Current release candidate** — VPS deployment target and hosted-blueprint removal on top of Coretax/PPh 21 release-test completion (2026-08-10); not production-certified |
| [v0.9.1](releases/v0.9.1.md) | **Latest named release** — Enterprise UI/UX overhaul (2026-05-28) |
| [Version and Progress Report](releases/VERSION_HISTORY.md) | Differences between releases and post-v0.9.1 implementation progress |
| [Production Release Checklist](releases/production-release-checklist.md) | Code, certification, operations, deployment, and rollback gates |
| [VPS Production Deployment](DEPLOYMENT.md) | Self-managed production VPS runbook for systemd, Nginx, backups, health checks, and rollback |
| [VPS Staging Deployment](STAGING_DEPLOYMENT.md) | Isolated `staging` branch deployment contract, systemd services, health checks, and rollback |
| [v0.9.0](releases/v0.9.0.md) | Phase 9 — Sales & AR complete |
| [v0.8.0](releases/v0.8.0.md) | Phase 8 — Board Pack |
| [v0.7.0](releases/v0.7.0.md) | Phase 7 |

## 📁 Archive

Historical documentation dari development phases tersedia di [archive/](archive/).

---

## Quick Links

- 📖 [Root README](../README.md) - Project overview
- 🔧 [Quick Reference](../QUICK_REFERENCE.md) - Command cheatsheet
- 📜 [Changelog](CHANGELOG.md) - Version history
- 📐 [Documentation policy](DOCUMENTATION_POLICY.md) - Ownership, status, and archive rules
