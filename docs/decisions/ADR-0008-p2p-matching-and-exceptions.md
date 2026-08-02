# ADR-0008: Purchase-to-pay matching and exceptions

**Status:** Proposed — requires procurement and AP approval

## Decision

Procurement remains the source of purchase request, purchase order, receipt, return,
and line-progress facts. AP remains the source of supplier invoice, allocation, tax,
and accounting posting. A matching service reads those facts through explicit ports,
records the evaluated policy and evidence, and never silently mutates a PO, receipt, or
invoice to erase a mismatch.

```text
Invoice intake -> duplicate review -> two-way/three-way match
    -> MATCHED / WITHIN_TOLERANCE -> draft AP -> policy-controlled posting
    -> EXCEPTION -> owner + evidence + approval -> resolution or cancellation
```

Policies are effective-dated and company-scoped. They define whether a service PO uses
two-way matching and the quantity, price, tax, freight, and total tolerances. A match
captures source facts and policy version so a later policy change cannot rewrite a past
decision.

## Consequences

- Partial receipts, returns, debit notes, and partial invoices derive open quantities
  and values from accumulated document facts.
- Duplicate candidates are visible review items; uncertain duplicates are not dropped.
- Auto-posting is disabled by default and requires one transactionally consistent
  validation of policy, duplicate state, supplier hold, accounting period, and mappings.
- Exceptions are durable, company-scoped work items with owner, SLA, evidence, decision,
  notification, and audit history.

## Exact money and FX

Match calculations use exact decimal quantities and money. Foreign-currency AP facts
retain their transaction/base values and locked FX references; comparison never uses
binary floating point.
