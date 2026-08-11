# Product Requirements Document (PRD)
## Odyssey ERP

### 1. Vision & Purpose
Odyssey ERP is a finance-led operations system for growing businesses. It centralizes finance and daily operations — purchasing, sales, inventory, delivery, accounting, reporting, approvals, and audit — into one self-hosted platform. Built as an open-source Go modular monolith with server-side rendering, Odyssey provides data sovereignty and operational efficiency without the overhead of enterprise ERPs or the fragmentation of multiple SaaS tools.

### 2. Problem Statement
Small and medium-sized businesses (SMBs) struggle with:
- **Fragmented Operations:** Relying on spreadsheets, disconnected SaaS subscriptions, or expensive, complex enterprise ERPs.
- **Lack of Data Sovereignty:** Cloud-only solutions dictate data control, limiting privacy and ownership.
- **Reconciliation Overhead:** Disconnected systems create isolated data silos, leading to extensive manual reconciliation.
- **Cost vs. Value:** Enterprise ERP solutions are excessively expensive and complex for SMB scale.

### 3. Target Users & Personas
- **Target Market:** Small and medium-sized businesses (10-500 employees).
- **Core Personas:**
  - **Operations Managers:** Need to oversee inventory, supply chain, and daily fulfillment.
  - **Finance Teams:** Require accurate general ledgers, accounts payable/receivable, and fast financial reporting.
  - **Procurement Officers:** Manage supplier relationships, purchase orders, and goods receipts.
  - **Business Owners/Founders:** Need a single source of truth for business health, KPIs, and centralized oversight.

### 4. Product Scope (Core Modules)
The core modules that constitute the MVP and current functionality include:

| Module | Description | Key Features |
| :--- | :--- | :--- |
| **Authentication & User Management** | Secure access control and user administration. | Session-based auth, user CRUD, basic roles. |
| **Dashboard** | High-level business overview. | KPIs, charts, at-a-glance metrics. |
| **Sales** | Order-to-cash workflow management. | Customer management, invoices (with line items), payments, credit notes. |
| **Procurement** | Procure-to-pay workflow management. | Supplier management, purchase orders (with approval workflow), goods receipts. |
| **Inventory** | Stock and warehouse management. | Products, warehouses, stock movements, stock adjustments. |
| **Finance** | Core accounting and general ledger. | Chart of accounts, journal entries, general ledger, trial balance, balance sheet, income statement. |
| **Governance (GRC)** | Compliance, risk, and control management. | Policies, risks, controls, audits, incidents, reviews, frameworks, compliance matrix. |
| **Reports** | Document generation. | PDF report generation (via Gotenberg). |
| **Admin** | System administration. | User administration and system settings. |

### 5. Extended Roadmap Modules
Future features planned for development:
- Accounts Payable / Accounts Receivable
- Banking & Treasury
- CRM, HR, Payroll
- Manufacturing (MRP)
- Logistics & Freight
- Quality Management (QMS)
- Fixed Assets
- Multi-entity Consolidation
- Connectors (Stripe, Shopify, DHL, etc.)
- POS & Portal

### 6. Non-Functional Requirements
- **Architecture:** Open-source Go modular monolith. Single binary + worker process.
- **Deployment:** Self-hosted via one-command deployment using Docker Compose.
- **Data Stack:** PostgreSQL for transactional data, Redis/Valkey for sessions and background queues.
- **Performance:** Sub-second page loads for server-side rendered (SSR) pages.
- **Accessibility:** WCAG 2.2 AA compliance target.
- **User Interface:** Mobile-responsive, dark-themed UI.

### 7. Success Metrics & Criteria
- [x] Complete CRUD functionality for all 9 core modules.
- [x] Functional PDF report generation system.
- [x] Reliable background job processing.
- [x] Comprehensive E2E test coverage for core workflows.
- [x] One-command deployment via Docker Compose is functional and documented.

### 8. Constraints & Assumptions
- **Assumptions:** Users have basic technical knowledge to deploy via Docker Compose or use a provided hosted environment.
- **Constraints:** As a self-hosted solution, system uptime and hardware maintenance depend on the user's infrastructure unless a managed hosting tier is introduced later.

### 9. Glossary
- **ERP:** Enterprise Resource Planning.
- **GRC:** Governance, Risk, and Compliance.
- **SSR:** Server-Side Rendering.
- **MRP:** Material Requirements Planning.
- **QMS:** Quality Management System.
- **WCAG:** Web Content Accessibility Guidelines.
