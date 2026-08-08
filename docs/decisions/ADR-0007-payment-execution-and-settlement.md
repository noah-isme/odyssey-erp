# ADR-0007: Payment execution and settlement

**Status:** Approved

## Decision

Payment scheduling is a treasury workflow separate from `ap_payments`. A proposed,
approved, scheduled, or exported instruction is an intention to pay, not a financial
event. `ap_payments`, allocations, FX journals, tax capture, and bank reconciliation
are created only after the approved authoritative settlement event is confirmed.

```text
DRAFT -> PROPOSED -> APPROVAL -> APPROVED -> SCHEDULED -> PROCESSING
                                                       -> SETTLED
                         \-> REJECTED    \-> CANCELLED  -> PARTIAL / FAILED
```

Provider submission is written to the finance outbox after batch approval commits. A
timeout or ambiguous response must call provider lookup using the same external
instruction reference before any retry. The first production mode may create an
encrypted, checksummed bank file and accept controlled execution-result import; it does
not require a live payment API.

## Separation of duties

The default company policy requires proposer, approver, and executor to be different
actors. Global roles may contain multiple permissions for emergency administration, but
the payment workflow rejects a batch that violates the configured actor separation.
Changing beneficiary details places the supplier on hold until independently verified.

## Exact money and FX

Instructions, provider fees, partial settlements, and payouts use exact amount/currency
contracts and PostgreSQL `NUMERIC`. Settlement uses the existing AP FX valuation rules;
no new payment code may derive cash or journal values with `float64`.
