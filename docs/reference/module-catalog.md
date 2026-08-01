# Odyssey ERP Module Catalog

**Authority:** This is the current documentation status source for the ERP capability inventory. It was reviewed 2026-08-01. `Implemented` means the capability is documented as user-facing and has an implementation reference; `Partial` means only part of the requested scope is available; `Planned` means it is a roadmap item; `Unsupported` means no current implementation or committed plan is documented.

Archived status files do not override this catalog.

| Requirement area | Status | Current scope | Primary evidence / next gap |
|---|---|---|---|
| Finance & accounting | Partial | GL, AR, AP, banking, reconciliation, cash flow, tax, budgets, fixed assets, FX, consolidation | [`accounting.md`](../guides/accounting.md), [`ROADMAP.md`](../ROADMAP.md); cash forecasting, online banking, payment scheduling, asset maintenance remain |
| Sales management | Partial | Customers, quotations, orders, delivery, AR invoicing, returns | [`v0.9.0`](../releases/v0.9.0.md); installments, replacement/refund detail, and delivery scheduling need a guide |
| CRM | Implemented | Leads, contacts, pipeline, opportunities, activities, reminders, conversion, win/loss | [`crm.md`](../guides/crm.md); campaigns, SMS campaigns, segmentation are unsupported |
| Procurement | Partial | PR, approval, PO, GRN, vendor invoice, payment | [`procurement.md`](../guides/procurement.md); RFQ, comparison, contracts, rating, price history are planned/undocumented |
| Inventory | Partial | Warehouses, movements, stock takes, adjustments, lot/serial, AVG/FIFO, replenishment | [`inventory-replenishment.md`](../guides/inventory-replenishment.md); RFID and LIFO are unsupported |
| Manufacturing / MRP | Partial | BOM and work orders | [`manufacturing-mrp.md`](../guides/manufacturing-mrp.md); scheduling, capacity, MRP calculation, WIP and yield are planned |
| HRM | Partial | Employees, organization, attendance, leave, approvals, payroll | [`approvals-hr-core.md`](../guides/approvals-hr-core.md), [`payroll.md`](../guides/payroll.md); recruitment, performance, training are unsupported |
| Projects | Partial | Projects, tasks, members, timesheets, FX snapshots | [`projects.md`](../guides/projects.md); milestones, Gantt, Kanban, budget and resource allocation are planned |
| Asset management | Partial | Asset register, categories, depreciation, disposal | [`fixed-assets.md`](../guides/fixed-assets.md); warranty, maintenance, location and transfer are planned |
| POS | Partial | Terminals, sessions, tickets, payments, refunds, voids | [`pos.md`](../guides/pos.md); loyalty, gift cards, printer/scanner operations are not documented |
| E-commerce | Planned | Candidate marketplace/store integrations only | [`integrations.md`](../guides/integrations.md); no sync connector is committed |
| Supply chain | Partial | WMS bins, picking, barcode, warehouse and fulfillment foundations | [`supply-chain.md`](../guides/supply-chain.md); fleet and route optimization are unsupported |
| CMMS | Unsupported | No current maintenance module | [`cmms.md`](../guides/cmms.md) |
| QMS | Unsupported | No current quality module | [`qms.md`](../guides/qms.md) |
| Document management | Planned | Transaction attachments and e-signature are roadmap items | [`document-management.md`](../guides/document-management.md) |
| BI | Partial | Analytics, insights, KPIs, board packs, finance exports and scheduled reports | [`reporting-catalog.md`](reporting-catalog.md); custom report builder/widgets are not started |
| Workflow automation | Partial | Shared approvals, lifecycle transitions, notifications, payroll and tax outboxes | [`lifecycles.md`](../architecture/lifecycles.md); full purchase-to-pay automation is not documented as one workflow |
| Notifications | Partial | In-app notifications and transactional email | [`notifications.md`](../guides/notifications.md); SMS, push and WhatsApp are planned/unsupported |
| RBAC | Implemented | Permission middleware, module permissions, roles and segregation | [`rbac.md`](rbac.md); newer-module role matrix needs expansion |
| Reporting | Partial | Finance, consolidation, analytics, board pack, audit exports | [`reporting-catalog.md`](reporting-catalog.md); operational and HR report coverage is incomplete |
| Audit trail | Implemented | Timeline, exports, source audit events, immutable tax/audit records | [`security.md`](../guides/security.md), [`observability.md`](observability.md) |
| Multi-everything | Partial | Company isolation, branches/warehouses in data model, FX, tax, periods | [`horizon-mvp.md`](../guides/horizon-mvp.md); language, timezone and fiscal-year policy need explicit documentation |
| APIs & integrations | Partial | Public API, API keys, webhooks, SMTP, PDF, Coretax export, portals | [`integrations.md`](../guides/integrations.md); payment, shipping, SSO, BI and AI connectors are not implemented |
| Security | Partial | RBAC, CSRF, secure headers, sessions, bcrypt, rate limiting, audit | [`security.md`](../guides/security.md); 2FA, SSO, encryption, DR and compliance controls remain gaps |
