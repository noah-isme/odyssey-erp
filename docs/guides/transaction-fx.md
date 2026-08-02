# Transaction-level FX operations

Daily transaction FX supports AR/AP. It is separate from the monthly
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

## Local acceptance result

Local FX acceptance gates are complete. The clean schema was migrated through version 61;
the FX account mappings, AR/AP FX columns, fetch-run audit behavior, CLI redaction,
focused tests, tagged integration tests, full test suite, lint, and build all passed.
The exact checklist and command evidence are recorded in
`docs/guides/phase14-p7-acceptance-evidence.md`.

The development `fx fetch` command may return `FAILED` when no live
`EXCHANGERATE_API` key is configured. This is expected safe-failure behavior when the
failure is recorded in `fx_fetch_runs` and the key is absent from output and logs.

## Deployment and acceptance checklist

The implementation is locally certified. It is not production-certified until all of
the following staging and production checks are completed:

1. Back up the database.
2. Deploy the corrected application and worker code.
3. Run `make migrate-up` with the selected `PG_DSN`, including
   `000053_transaction_fx`.
4. Verify migration version and the four `FX` account mappings on staging and
   production.
5. Run database-backed USD AR and USD AP end-to-end tests against PostgreSQL.
6. Start the application and worker processes; confirm the daily FX job is
   deployed and running in `Asia/Jakarta`.
7. Fetch rates and inspect `fx_fetch_runs`.
8. Run post-migration smoke tests for invoice posting, partial payment,
   revaluation, and reversal.

The release gate remains open until staging and production migration execution,
account mapping verification, worker confirmation, and post-migration smoke evidence
are recorded.
