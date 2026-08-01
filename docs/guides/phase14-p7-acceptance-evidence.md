# Phase 14 and P7 Local Acceptance Evidence

**Status:** Local gates complete; staging and production certification pending.

**Schema:** Clean database migrated successfully through version 61.

## Gate results

| Gate | Result |
|---|---|
| Migrations through v61 | Pass |
| FX account mappings | Pass, 4/4 mappings present |
| `make lint` | Pass |
| `make build` | Pass |
| FX, AR, AP, integration, and jobs unit tests | Pass |
| WMS, MRP, POS, projects, API, and portal unit tests | Pass |
| `go test -tags=integration ./...` | Pass |
| Full `go test ./...` suite | Pass |
| AR/AP FX columns | Pass, 7 columns each |
| Timesheet FX snapshot columns | Pass, 3 columns |
| `fx_fetch_runs` idempotency and error recording | Pass |
| CLI/API-key secret redaction | Pass |

## Expected provider failure

The development `fx fetch` command returns `FAILED` when no live
`EXCHANGERATE_API` key is configured. The failure is recorded in `fx_fetch_runs`, and
the provider key does not appear in command output, logs, templates, browser responses,
or returned errors. This is the expected safe-failure behavior; it is not a successful
live-provider smoke test.

## Remaining release evidence

The following must be recorded before production certification:

- Staging execution of migrations `000053`, `000054`, `000055`, `000056`, and `000060`.
- Staging verification of the four FX mappings and Horizon permissions.
- Production worker confirmation for the daily FX job in `Asia/Jakarta`.
- Webhook delivery, retry, signature, duplicate-event, and secret-protection evidence.
- Cross-feature smoke scenario: USD project/timesheet FX snapshot, FX rate change,
  foreign-currency POS sale, WMS fulfillment, AR payment, and revaluation.
- Production migration and post-migration smoke-test evidence.

P7 is an MVP foundation. Full vertical capabilities remain future scope.
