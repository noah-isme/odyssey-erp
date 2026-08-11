# Odyssey ERP Project Roadmap

## Vision Statement
Odyssey ERP aims to be a comprehensive, open-source Enterprise Resource Planning system that empowers businesses of all sizes to manage their operations efficiently. We strive to provide a modern, scalable, and intuitive platform that breaks down silos between departments, from finance and inventory to governance and risk compliance (GRC).

## Phase 1 — Core ERP (Completed ✓)
The foundational phase of Odyssey ERP is complete, establishing a robust architecture with all essential core modules fully functional.

- [x] **Auth & User Management**: Secure login, session handling.
- [x] **Dashboard**: Centralized overview of key metrics.
- [x] **Sales**: Customers, Invoices, Payments, Credit Notes.
- [x] **Procurement**: Suppliers, Purchase Orders, Goods Receipts.
- [x] **Inventory**: Products, Warehouses, Stock Movements, Adjustments.
- [x] **Finance**: Chart of Accounts, Journal Entries, Ledger, Trial Balance, Balance Sheet, Income Statement.
- [x] **Governance, Risk, and Compliance (GRC)**: Policies, Risks, Controls, Audits, Incidents, Reviews, Frameworks, Compliance Matrix.
- [x] **Reports**: Automated PDF generation via Gotenberg.
- [x] **Admin**: User CRUD operations.
- [x] **Infrastructure**: 29 database migrations, E2E test coverage, Docker Compose deployment.

## Phase 2 — Enhanced Core (Planned)
The next phase focuses on hardening the core application, adding essential operational features, and improving overall user experience and security.

### High Priority

| Feature | Priority | Estimated Effort | Dependencies | Description |
| :--- | :---: | :---: | :--- | :--- |
| **Role-Based Access Control (RBAC)** | High | Large | Auth Module | Granular permissions beyond simple admin/user roles. |
| **Multi-currency Support** | High | Large | Finance, Sales, Procurement | Exchange rates, multi-currency transactions and accounting. |
| **Audit Trail / Activity Log** | High | Medium | All Modules | Track all entity changes across the system for accountability. |
| **Email Notifications** | High | Medium | Infrastructure | Invoice reminders, PO approvals, system alerts. |
| **File Attachments** | High | Medium | Storage / DB | Document management for invoices, POs, policies, etc. |
| **REST API** | High | Large | Core Services | API endpoints for external integrations and future clients. |
| **Global Search** | High | Medium | Database Indexing | Cross-module search functionality for quick access. |

### Medium Priority

| Feature | Priority | Estimated Effort | Dependencies | Description |
| :--- | :---: | :---: | :--- | :--- |
| **Import/Export** | Medium | Medium | CSV Parsing | Bulk data import/export via CSV/Excel formats. |
| **Recurring Invoices** | Medium | Small | Sales, Cron Jobs | Automated invoice generation on a defined schedule. |
| **Tax Management** | Medium | Medium | Finance, Sales | Tax rates, tax reports, and compliance tracking. |
| **CSRF Protection Hardening** | Medium | Small | Security | Ensure all forms have proper CSRF tokens. |
| **Accessibility Improvements** | Medium | Medium | UI/UX | Skip-to-content, ARIA roles, better focus management. |

## Phase 3 — Extended Modules (Future)
Phase 3 will expand Odyssey ERP's capabilities into new business domains, making it a true end-to-end enterprise solution.

- **Accounts Payable / Accounts Receivable (AP/AR)**: Dedicated modules for advanced tracking.
- **Banking & Treasury**: Bank reconciliation, cash flow forecasting.
- **CRM**: Leads, Opportunities, and pipeline management.
- **HR & Payroll**: Employee records, attendance, payroll processing.
- **Manufacturing (MRP)**: Bill of Materials (BOMs), Production Orders, routing.
- **Logistics & Freight**: Shipping integration, vehicle tracking.
- **Quality Management (QMS)**: Quality checks, non-conformance tracking.
- **Fixed Assets Management**: Asset lifecycle, depreciation scheduling.
- **Multi-entity Consolidation**: Support for subsidiary companies and consolidated reporting.
- **Point of Sale (POS)**: Retail front-end integration.
- **External Portal**: Self-service portals for customers and suppliers.

## Phase 4 — Enterprise Features (Vision)
The long-term vision includes enterprise-grade features for scalability, integration, and advanced automation.

- **Connectors**: Native integrations with Stripe, Shopify, DHL, etc.
- **Advanced Analytics & BI dashboards**: Deep data insights and visualizations.
- **Workflow Engine**: Customizable, drag-and-drop approval and automation flows.
- **Multi-tenant Support**: SaaS-ready architecture.
- **Enterprise Auth**: OAuth / SAML / SSO authentication integrations.
- **Internationalization (i18n)**: Multi-language support.
- **UI Themes**: Light theme option and customizable branding.
- **Mobile-Optimized Views**: Responsive design for on-the-go access.
- **Real-Time Collaboration**: Collaborative editing and in-app messaging.
- **Custom Report Builder**: User-defined reports and dynamic queries.

## Technical Debt & Known Issues
To ensure long-term stability, the following technical debt items need addressing:

- [ ] **Invoice Status Logic**: Refine and harden transition logic (e.g., Draft -> Sent -> Paid).
- [ ] **Automated Accounting**: Implement automatic journal entry generation from invoices and payments.
- [ ] **GRC Scoring**: Finalize and implement compliance scoring calculations in the Governance module.
- [ ] **Dashboard Flexibility**: Make dashboard widgets configurable per user.
- [ ] **Reporting Engine**: Expand available report types beyond the current limited set.
- [ ] **Accessibility (A11y)**:
  - Add missing `aria-labels` to SVG icons.
  - Implement a global "skip-to-content" link.
  - Ensure custom interactive components have correct ARIA roles.

## Contributing to the Roadmap
This roadmap is a living document. We welcome community input to help shape the future of Odyssey ERP! 

- Have a feature request? Open an issue on our issue tracker.
- Want to contribute? Check out our `CONTRIBUTING.md` guide and pick up a task from the **Phase 2** or **Technical Debt** lists.
- For architectural discussions, join our community forums.

---
*Last updated: August 2026*
