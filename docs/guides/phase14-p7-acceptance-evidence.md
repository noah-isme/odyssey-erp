# Phase 14 and P7 Local Acceptance Evidence

**Status:** Local acceptance evidence recorded; staging and production certification remain pending.

**Schema:** This historical evidence run used a clean database migrated through version 61.
Current releases must apply and verify all migrations, including the migrations added
after this evidence was collected.

## Gate results

| Gate | Result |
|---|---|
| Migrations through v61 | Pass |
| FX account mappings | Pass, 4/4 mappings present |
| `make lint` | Pass |
| `make build` | Pass |
| FX, AR, AP, integration, and jobs unit tests | Pass |
| WMS, MRP, POS, projects, API, and portal unit tests | Pass |
| `go test -tags=integration ./...` | Historical local pass; rerun for the current release |
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

## Production Release Evidence

This file is not a staging or production certification record. The following gates
remain open and must be attached to the release record before the relevant matrix rows
can become `production-certified: yes`:

- [ ] Staging execution of migrations `000053`, `000054`, `000055`, `000056`, `000060`,
  and all subsequent migrations, with rollback evidence.
- [ ] Staging verification of FX mappings, Horizon permissions, and tenant isolation.
- [ ] Production worker confirmation for the daily FX job in `Asia/Jakarta`.
- [ ] Provider evidence for webhook delivery, retry, signature, duplicate-event, and
  secret-protection behavior.
- [ ] Cross-feature smoke scenario covering FX, project/timesheet, POS, WMS, AR, and
  revaluation flows.
- [ ] Production migration and post-migration smoke-test evidence without rollbacks.
- [ ] Tax and manufacturing governance sign-off for the exact deployed schema and
  policy configuration.

The [authoritative feature matrix](../reference/feature-matrix.md) is the release
status authority. Local test results here must not be copied into a production release
note as certification.

P7 is an MVP foundation. Full vertical capabilities remain future scope.
