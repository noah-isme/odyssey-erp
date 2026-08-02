# Odyssey ERP Module Catalog

**Authority:** This is the current documentation status source for the ERP capability inventory. It was reviewed 2026-08-01. `Implemented` means the capability is documented as user-facing and has an implementation reference; `Partial` means only part of the requested scope is available; `Planned` means it is a roadmap item; `Unsupported` means no current implementation or committed plan is documented.

Archived status files do not override this catalog.

| Requirement area | Status | Current scope | Primary evidence / next gap |
|---|---|---|---|
| Finance & accounting | Partial | GL, AR, AP, banking, reconciliation, cash flow, tax, budgets, fixed assets, FX, consolidation | [`accounting.md`](../guides/accounting.md), [`core-finance-automation-plan.md`](../guides/core-finance-automation-plan.md); cash forecasting, online banking, payment scheduling, and asset operations are planned |
| Sales management | Partial | Customers, quotations, orders, delivery, AR invoicing, returns | [`v0.9.0`](../releases/v0.9.0.md); installments, replacement/refund detail, and delivery scheduling need a guide |
| CRM | Implemented | Leads, contacts, pipeline, opportunities, activities, reminders, conversion, win/loss | [`crm.md`](../guides/crm.md); campaigns, SMS campaigns, segmentation are unsupported |
| Procurement | Partial | PR, approval, PO, GRN, vendor invoice, payment | [`procurement.md`](../guides/procurement.md); RFQ, comparison, contracts, rating, price history are planned/undocumented |
| Inventory | Partial | Warehouses, movements, stock takes, adjustments, lot/serial, AVG/FIFO, replenishment | [`inventory-replenishment.md`](../guides/inventory-replenishment.md); RFID and LIFO are unsupported |
| Manufacturing / MRP | Partial | Approved BOM revisions, warehouse-aware planning/firming, routing operations, WIP cost transfer, finite-capacity scheduling, exceptions, quality/genealogy, analytics, and compliance foundations | [`manufacturing-mrp.md`](../guides/manufacturing-mrp.md); staging validation and mandatory controlled-record policy enforcement remain |
| HRM | Partial | Employees, organization, attendance, leave, approvals, payroll | [`approvals-hr-core.md`](../guides/approvals-hr-core.md), [`payroll.md`](../guides/payroll.md); recruitment, performance, training are unsupported |
| Projects | Partial | Projects, tasks, members, timesheets, FX snapshots | [`projects.md`](../guides/projects.md); milestones, Gantt, Kanban, budget and resource allocation are planned |
| Asset management | Partial | Asset register, categories, depreciation, disposal | [`fixed-assets.md`](../guides/fixed-assets.md), [`core-finance-automation-plan.md`](../guides/core-finance-automation-plan.md); warranty, maintenance, location and transfer are planned |
| POS | Partial | Terminals, sessions, tickets, payments, refunds, voids | [`pos.md`](../guides/pos.md); loyalty, gift cards, printer/scanner operations are not documented |
| E-commerce | Planned | Provider-neutral marketplace/store integration plan | [`external-integrations-plan.md`](../guides/external-integrations-plan.md); no sync connector is implemented |
| Supply chain | Partial | WMS bins, picking, barcode, warehouse and fulfillment foundations | [`supply-chain.md`](../guides/supply-chain.md); fleet and route optimization are unsupported |
| CMMS | Unsupported | No current maintenance module | Preventive/corrective maintenance and work orders are not implemented |
| QMS | Unsupported | No current quality-management module | Inspection plans, NCR/CAPA, and quality reporting are not implemented |
| Document management | Planned | Transaction attachments and e-signature are roadmap items | Central storage, versioning, retention, and document-level permissions remain roadmap work |
| BI | Partial | Analytics, insights, KPIs, board packs, finance exports and scheduled reports | [`reporting-catalog.md`](reporting-catalog.md), [`external-integrations-plan.md`](../guides/external-integrations-plan.md); managed external BI delivery is planned |
| Workflow automation | Partial | Shared approvals, lifecycle transitions, notifications, payroll and tax outboxes | [`lifecycles.md`](../architecture/lifecycles.md), [`core-finance-automation-plan.md`](../guides/core-finance-automation-plan.md); full purchase-to-pay implementation remains planned |
| Notifications | Partial | In-app notifications and transactional email | [`notifications.md`](../guides/notifications.md), [`external-integrations-plan.md`](../guides/external-integrations-plan.md); SMS, push and WhatsApp are planned |
| RBAC | Implemented | Permission middleware, module permissions, roles and segregation | [`rbac.md`](rbac.md); newer-module role matrix needs expansion |
| Reporting | Partial | Finance, consolidation, analytics, board pack, audit exports | [`reporting-catalog.md`](reporting-catalog.md); operational and HR report coverage is incomplete |
| Audit trail | Implemented | Timeline, exports, source audit events, immutable tax/audit records | [`security.md`](../guides/security.md), [`observability.md`](observability.md) |
| Multi-everything | Partial | Company isolation, branches/warehouses in data model, FX, tax, periods | [`horizon-mvp.md`](../guides/horizon-mvp.md); language, timezone and fiscal-year policy need explicit documentation |
| APIs & integrations | Partial | Public API, API keys, webhooks, SMTP, PDF, Coretax export, portals | [`integrations.md`](../guides/integrations.md), [`external-integrations-plan.md`](../guides/external-integrations-plan.md); payment, shipping, marketplace, messaging, SSO, BI, and AI connectors are planned but not implemented |
| Security | Partial | RBAC, CSRF, secure headers, sessions, bcrypt, rate limiting, audit | [`security.md`](../guides/security.md); 2FA, SSO, encryption, DR and compliance controls remain gaps |
