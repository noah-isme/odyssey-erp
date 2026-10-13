# Portal

## Current status

**Core foundation implemented; employee self-service delivered, other portals remain
read-only.** Odyssey includes a secure external Portal system with authenticated
customer, supplier, and employee views. The core logic is located in
`internal/portal/`; RFQ negotiation, chat, and analytics remain outside the
certified scope.

## Supported scope

- **Authentication & Invitations:** Secure email-based invitation flows with token hashing, expiration, and portal-specific user linking.
- **Customer Portal:** Read-only access to a customer's invoices, sales orders (with delivery status), payments, and credit notes. Includes secure document upload capabilities.
- **Supplier Portal:** Read-only access to a supplier's invoices, purchase orders, deliveries (GRNs), and debit notes. Includes secure document upload capabilities.
- **Employee Portal:** Self-service access to employee timesheets, payroll payslips, leave requests, and attendance records.
- **Document Submissions:** External users can securely upload documents which are automatically routed and classified into the internal Document Management system.

## Employee self-service

Employee self-service endpoints live under `/portal/employee`. Authorization is an
active `hr_employees` link for the session user and company; no extra RBAC
permission is required. The same endpoints serve the employee view of `/portal/app`.

| Endpoint | Purpose |
|---|---|
| `/portal/employee/leave` (+ `/{id}/cancel`) | Request and cancel leave. |
| `/portal/employee/overtime` (+ `/{id}/cancel`) | List, request, and cancel overtime. |
| `/portal/employee/expenses` (+ `/{id}/cancel`) | List, submit, and cancel expense claims. |
| `/portal/employee/profile` | Read own profile, masked: `national_id` and `tax_id` are never returned. |
| `/portal/employee/profile-changes` (+ `/{id}/cancel`) | List, request, and cancel address, bank, or emergency-contact changes. The address form is prefilled from `/portal/employee/profile`; only non-empty fields overwrite on approval. |
| `/portal/employee/holidays?year=` | Company holiday calendar. |

Employees can submit their own requests without HR admin permissions. Decisions
happen in `/approvals`; HR admins monitor and retry in `/hr/overtime`,
`/hr/expenses`, and `/hr/profile-changes`.

### CSRF header

Every POST under `/portal/employee` requires the `X-CSRF-Token` header.
`portal_app.html` emits a `<meta name="csrf-token">` tag and a single `postJSON`
helper that sets the header, same-origin credentials, and JSON headers; all page
writes go through that helper. Custom clients must send the same header or the
request is rejected with 403.

### Scripts and CSP

The app sets `script-src 'self'`, so inline `<script>` blocks and inline
`on*=` handlers never run. `portal_app.html` loads `/static/js/pages/portal.js`
(`defer`) and wires navigation through `data-tab` / `data-rfq-id` attributes.
Do not add inline scripts; pass server data through `data-*` attributes or a
`<script type="application/json">` block. `TestPortalAppHasNoInlineExecutableScript`
enforces this.

### Policy prerequisites

Requests only succeed when an active approval policy resolves for the module. No
approval policies are seeded for `LEAVE`, `OVERTIME`, `EXPENSE`, or `HR_PROFILE`; create them at `/approvals/policies` before enabling
those forms, or submission fails and the request is rolled back. See
[`approvals-hr-core.md`](approvals-hr-core.md#approval-modules) for amount semantics
and the recommended approver steps (use an HR/finance role step for `HR_PROFILE`,
not "Employee manager"). Approved expense claims stop at `APPROVED`; nothing is
posted to AP or payroll. An approved `BANK` change creates a new compensation
assignment effective on the decision date.

## Gaps

Advanced RFQ negotiations/bidding, real-time chat with internal agents, and embedded
analytics/dashboards for external users are not currently supported. Customer and
supplier portals remain read-only apart from document uploads.
