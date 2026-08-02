# ADR-0006: Bank ownership and feed ingestion

**Status:** Proposed — requires treasury/controller approval

## Decision

`internal/finance/banking/` owns company bank accounts and normalized bank
transactions. `internal/accounting/banks/` owns imported statements, reconciliation,
and authorized match confirmation. A bank-feed adapter does not write either table
directly: it sends normalized entries through one application service that applies the
same validation and duplicate protection as CSV/OFX import.

```text
Provider poll or signed callback
  -> durable provider inbox
  -> normalized statement-entry command
  -> finance banking transaction + accounting statement
  -> suggested reconciliation -> authorized confirmation
```

Polling and callback events are deduplicated by the provider's stable transaction ID.
Only if the provider supplies no ID may the ingestion service use a documented account,
date, amount, currency, and normalized-reference fingerprint. An external reference is
always `(company, connection, provider, object type, object ID)`; it never relies on an
account number or description alone.

## Consequences

- Provider credentials, cursors, raw payload retention, and event inbox data belong to
  the bank-feed boundary; financial source-of-truth records remain in existing modules.
- Feed ingestion does not post journals or auto-confirm reconciliation.
- CSV/OFX import remains supported and shares the same dedupe path.
- Feed failure, consent expiry, rate limiting, or an ambiguous callback creates a
  visible retry/recovery state without changing accounting state.

## Exact money and FX

All new feed contracts use `accounting/money.Money` plus ISO currency and PostgreSQL
`NUMERIC`; no feed amount, balance, or matching tolerance may be represented as a new
`float64`. FX conversion is deferred to the existing transaction-FX resolver and is
locked by the consuming workflow.
