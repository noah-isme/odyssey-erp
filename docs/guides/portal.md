# Portal

## Current status

**Core foundation implemented; self-service depth is partial.** Odyssey includes a
secure external Portal system with authenticated customer, supplier, and employee
views. The core logic is located in `internal/portal/`; profile changes, RFQ
negotiation, chat, analytics, and broader self-service remain outside the certified
scope.

## Supported scope

- **Authentication & Invitations:** Secure email-based invitation flows with token hashing, expiration, and portal-specific user linking.
- **Customer Portal:** Read-only access to a customer's invoices, sales orders (with delivery status), payments, and credit notes. Includes secure document upload capabilities.
- **Supplier Portal:** Read-only access to a supplier's invoices, purchase orders, deliveries (GRNs), and debit notes. Includes secure document upload capabilities.
- **Employee Portal:** Self-service access to employee timesheets, payroll payslips, leave requests, and attendance records.
- **Document Submissions:** External users can securely upload documents which are automatically routed and classified into the internal Document Management system.

## Gaps

Self-service profile and banking detail updates (with approval workflows), advanced RFQ negotiations/bidding, real-time chat with internal agents, and embedded analytics/dashboards for external users are not currently supported.
