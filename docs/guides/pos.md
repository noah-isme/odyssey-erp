# Point of Sale (POS)

## Current status

**Partial.** POS terminals, sessions, tickets, payments, refunds, and voids are part of the Horizon foundation. The operational route is `/pos` and core logic is in `internal/pos/`.

## Supported scope

- **Terminals & Sessions:** Company-scoped terminal and cashier-session records.
- **Tickets:** Ticket creation with individual line items, supporting unit pricing, discounts, and tax calculation on a per-line basis.
- **Payments:** Idempotent payment recording linked to tickets, with support for tracking paid totals and automatic status progression to COMPLETED when fully paid.
- **Lifecycle Operations:** Refund and void lifecycle operations.

## Gaps

Barcode scanner and receipt-printer device integration, cashier shift controls,
discount rules, loyalty points, gift cards, and a fully governed multi-payment tender
model (handling complex split tenders natively in UI) are limited. Loyalty, gift-card, and governed hardware depth is
planned in the
[`Product Workflow Depth Execution Plan`](product-workflow-depth-plan.md).
