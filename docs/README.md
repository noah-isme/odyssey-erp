# Odyssey ERP Documentation

Dokumentasi lengkap untuk Odyssey ERP - Modern ERP system built with Go.

## 🚀 Getting Started

| Document | Description |
|----------|-------------|
| [Quick Start (Docker)](getting-started/GETTING_STARTED.md) | Setup dalam 3 langkah |
| [Native Setup](getting-started/native-setup.md) | Setup tanpa Docker |
| [Docker Setup](getting-started/docker-setup.md) | PostgreSQL via Docker |
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
| [Integration Tests](guides/integration-tests.md) | End-to-end testing |
| [PDF Generation](guides/pdf-generation.md) | Generate PDF reports |

### Operations
| Document | Description |
|----------|-------------|
| [Accounting Runbook](guides/accounting.md) | Operasi accounting |
| [Procurement SOP](guides/procurement.md) | Prosedur procurement |
| [Board Pack](guides/howto-boardpack.md) | Generate board pack |
| [Profil & Pengaturan](guides/user-profile-settings.md) | Panduan pengguna untuk profil, tema, bahasa, notifikasi, dan password |
| [Runbook Preferensi Pengguna](guides/runbook-user-preferences.md) | Deployment dan troubleshooting pengaturan UI |
| [Notifications & Email](guides/notifications.md) | Arsitektur, API, SMTP, deployment, dan troubleshooting notifikasi |
| [Approvals & HR Core](guides/approvals-hr-core.md) | Policy routing, delegation, leave workflow, dan attendance CSV |
| [Payroll Engine](guides/payroll.md) | Versioned TER/PTKP/BPJS rules, payroll approval, posting, payment export, dan payslips |
| [Tax Compliance](guides/tax-compliance.md) | Immutable faktur, PPN/PPh ledgers, GL reconciliation, period lock, dan Coretax export |
| [Coretax Validation Sign-off](guides/tax-staff-coretax-validation.md) | Checklist staf pajak untuk artefak DJP, portal testing, rekonsiliasi, dan persetujuan rilis |
| [CRM](guides/crm.md) | Leads, pipeline, activities, reminders, ownership, conversion, dan win/loss |

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

## 📝 Architecture Decision Records

| ADR | Title |
|-----|-------|
| [ADR-0001](decisions/ADR-0001-tools.md) | Tooling Stack |
| [ADR-0002](decisions/ADR-0002-rbac.md) | RBAC Implementation |
| [ADR-0003](decisions/ADR-0003-inventory-costing.md) | Inventory Costing |
| [ADR-0004](decisions/ADR-0004-accounting-model.md) | Accounting Model |

## 📦 Releases

| Version | Notes |
|---------|-------|
| [v0.9.1](releases/v0.9.1.md) | **Current** — Enterprise UI/UX overhaul (2026-05-28) |
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
