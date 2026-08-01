# Cross-Module Lifecycle Reference

This document is the authoritative summary of documented lifecycle states. A state not listed here must not be presented as a supported workflow without an implementation reference.

```text
Sales quotation
  DRAFT -> SUBMITTED -> APPROVED -> CONVERTED TO SALES ORDER
                         \-> REJECTED

Sales order
  DRAFT -> CONFIRMED -> DELIVERY ORDER -> SHIPPED -> DELIVERED
       \-> CANCELLED

Purchase request / purchase order
  DRAFT -> APPROVAL -> APPROVED -> GOODS RECEIPT -> VENDOR INVOICE -> PAYMENT
       \-> REJECTED / CANCELLED

Accounting document
  DRAFT -> POSTED -> VOIDED
                 \-> CREDIT/DEBIT NOTE or reversal, depending on module

Payroll run
  DRAFT -> APPROVAL -> POSTED

Tax period
  OPEN -> LOCKED
```

## Module notes

- Sales quotation approval is permission-protected; only approved quotations convert to orders. See [`rbac.md`](../reference/rbac.md).
- Procurement PO approval is recorded in the approvals table. GRN posting feeds inventory and AP workflows. See [`procurement.md`](../guides/procurement.md).
- AR/AP posting is immutable at the accounting/tax boundary; voiding, cancellation, replacement, and credit/debit notes append correcting records. See [`tax-compliance.md`](../guides/tax-compliance.md).
- Inventory receiving, transfer, adjustment, stock take, and delivery completion create traceable movements. Costing is AVG or FIFO; LIFO is excluded. See [`inventory-replenishment.md`](../guides/inventory-replenishment.md).
- Approval decisions, lifecycle transitions, notifications, and audit records should carry the same company scope and actor identity.

## Explicitly not covered

Manufacturing production completion, project delivery/billing, POS end-of-day close, maintenance work orders, and QMS holds/releases do not yet have an authoritative lifecycle here.
