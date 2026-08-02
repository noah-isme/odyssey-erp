# CRM

The `/crm` module is a sales front-end to the existing customer, quotation,
and sales-order modules. It does not copy pricing, quotation approval, or order
conversion logic.

## Model and ownership

Migration `000050_crm` creates company-scoped pipeline stages, leads, contacts,
opportunities, activities, and event history. Lead, contact, and
customer are separate records:

- A lead is an unqualified prospect with source and owner.
- Qualification creates one contact and one opportunity. Contact email is unique
  per company, so duplicate contact data is rejected.
- A customer remains sales master data. Conversion links an existing matching
  customer or creates one through `sales/customers` with deterministic code
  `CRM-Lnnnnnn`.

Sales representatives see and edit their own records. `crm.team.view` allows a
sales manager to see the company pipeline, while `crm.manage` allows company-wide
administration and reassignment. Every repository read and mutation remains
scoped to the active company.

## Workflow

1. Create a lead in the pipeline board/list.
2. Qualify it to create a contact and an opportunity in the first open stage.
3. Schedule calls, emails, meetings, tasks, or notes on the lead/opportunity.
4. Move opportunities forward through ordered open stages. Backward movement is
   rejected; won/lost states are terminal, and loss requires a reason.
5. For a won opportunity, explicitly link/create a customer and optionally
   create a draft quotation. Quotation lines are passed to the existing
   quotation service, so its calculations and later approval/order lifecycle
   remain authoritative.

Repeated qualification and completed conversion return their existing links.
CRM-created quotations carry a unique opportunity reference. Customer and
quotation links are finalized in one CRM transaction, so a retry reuses the
same quotation and cannot leave a customer-only partial CRM conversion.
Expected opportunity values preserve the database's two-decimal precision.
The win/loss dashboard summarizes visible opportunity counts, values, and loss
reasons.

## Activities and reminders

The worker scans every minute. A reminder is sent to the activity owner when
`reminder_at` is reached. When an incomplete activity passes `due_at`, an
escalation is sent to the owner's active HR manager; if no mapped manager exists,
it falls back to the owner. Notification preferences apply independently to:

- `crm_activity_reminder`
- `crm_activity_escalated`
- `crm_owner_reassigned`

## Permissions

- `crm.view`: owned CRM records.
- `crm.create`: leads, qualification, and activities.
- `crm.edit`: forward stage updates.
- `crm.convert`: won-opportunity conversion.
- `crm.team.view`: all records for the active company.
- `crm.manage`: company-wide administration and owner reassignment.

Run focused verification with:

```bash
ODYSSEY_TEST_MODE=1 GOTENBERG_URL=http://127.0.0.1:0 \
  go test ./internal/crm ./internal/sales/... ./internal/notifications ./jobs
```
