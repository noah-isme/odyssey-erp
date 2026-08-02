# Point of Sale (POS)

## Current status

**Partial.** POS terminals, sessions, tickets, payments, refunds, and voids are part of the Horizon foundation. The operational route is `/pos`.

## Supported scope

- Company-scoped terminal and cashier-session records.
- Ticket creation and payment records.
- Refund and void lifecycle operations.
- Idempotent payment behavior and links to inventory/AR where applicable.

## Gaps

Barcode scanner and receipt-printer device integration, cashier shift controls,
discount rules, loyalty points, gift cards, and a documented multi-payment tender
model are not currently supported. Loyalty, gift-card, and governed hardware depth is
planned in the
[`Product Workflow Depth Execution Plan`](product-workflow-depth-plan.md).
