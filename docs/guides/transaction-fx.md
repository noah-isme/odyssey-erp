# Transaction-level FX operations

Phase 14 adds daily transaction FX for AR/AP. It is separate from the monthly
`fx_rates` table used by consolidation.

## Accounting policy

- Company base currency is configurable and defaults to `IDR`.
- Quotes use `1 transaction currency = N base currency`.
- Invoice rates are locked when invoices are posted.
- Payment rates are locked when payments are recorded.
- Revaluation uses the period closing rate and never changes the original invoice journal.
- Amounts and rates are persisted as PostgreSQL `NUMERIC`; AR/AP arithmetic uses exact decimals.
- Revaluation uses the reversal model: reverse the prior period valuation, then value the new closing balance.

## Daily rate operations

The provider is configured with:

```text
FX_PROVIDER=exchangerate-api
FX_API_BASE_URL=https://open.er-api.com/v6
FX_API_KEY=
FX_BASE_CURRENCY=IDR
FX_FETCH_TIMEOUT=10s
FX_MAX_RATE_AGE=48h
```

Fetches are stored in `fx_daily_rates` and audited in `fx_fetch_runs`. The
worker determines the business date in `Asia/Jakarta`, fetches one rate set for
each configured company base currency, and retries transient failures.

Manual operations:

```bash
odyssey fx fetch --date 2026-08-01
odyssey fx status --date 2026-08-01
```

The Makefile equivalents are:

```bash
FX_DATE=2026-08-01 make fx-fetch
FX_DATE=2026-08-01 make fx-status
```

Do not expose `FX_API_KEY` to templates, browser code, or logs.

## AR/AP behavior

Invoice posting stores the original currency amount, base amount, rate, rate
date, source, and lock timestamp. Missing or stale local rates reject posting.

Payment allocations calculate realized FX only for the allocated amount. Journal
source keys are deterministic and protected by `fx_journal_idempotency`:

```text
AR_PAYMENT_FX:<payment-id>:<allocation-id>
AP_PAYMENT_FX:<payment-id>:<allocation-id>
```

Period revaluation stores details in `fx_revaluations`, claims each document
before journal creation, and records reversal journals in
`fx_revaluation_reversals`.

## Deployment checklist

1. Back up the database.
2. Run `make migrate-up` with the selected `PG_DSN`.
3. Verify migration version and the four `FX` account mappings.
4. Start the application and worker processes.
5. Fetch rates and inspect `fx_fetch_runs`.
6. Run a controlled USD AR/AP posting and payment smoke test.

The remaining release gate is a database-backed USD AR/AP end-to-end suite and
execution of migration `000053_transaction_fx` against staging and production.
