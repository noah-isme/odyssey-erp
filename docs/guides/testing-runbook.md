# Testing Runbook

This document summarises the steps required to run the Odyssey ERP automated
checks without contacting external infrastructure.

## Test mode environment

The application now honours the `ODYSSEY_TEST_MODE` environment flag. When the
flag is set to `1` the runtime skips expensive side effects such as opening
PostgreSQL/Redis connections or initialising background workers. The helper
package located at `github.com/odyssey-erp/odyssey-erp/testing` enables the flag
for unit tests, and CI also exports it before executing the suite.

Set the following variables when running tools manually:

```bash
export ODYSSEY_TEST_MODE=1
export GOTENBERG_URL="http://127.0.0.1:0"
```

`GOTENBERG_URL` is pointed at a non-routable address so that any code paths that
accidentally reach the HTTP client fail fast instead of hanging on network
timeouts.

## Local lint, test, and build

After exporting the environment variables above you can execute the complete Go
workflow:

```bash
go vet ./...
go test ./...
go build ./...
```

These commands should finish in a few seconds now that runtime hooks are
suppressed in test mode.

## Phase 1–6 verification

The focused regression suite for the completed ERP phases can be run with:

```bash
go test ./internal/notifications ./internal/auth ./internal/rbac ./internal/app \
  ./internal/approvals ./internal/ar ./internal/ap ./internal/procurement \
  ./internal/hr/... ./internal/payroll ./internal/tax ./internal/crm \
  ./internal/boardpack ./jobs ./migrations
```

The suite covers the following boundaries:

| Phase | Primary coverage |
| --- | --- |
| P1 Returns & credit/debit notes | Return lifecycles, stock reversal, allocations, balanced journals, tax, SSR/RBAC, PDFs, and end-to-end posting flows |
| P2 Notifications & email | Notification isolation/read state, preferences, event emission, enqueueing, retries, SMTP failures, and HTTP/worker behavior |
| P3 Approval engine & HR core | Policy resolution, decisions/delegation/escalation, finalizers, HR workflows, attendance imports, and cross-module notifications |
| P4 Indonesia payroll | TER/PTKP, BPJS, overtime, THR, effective-dated rules, lifecycle/audit immutability, journals, bank files, payslips, and outbox retries |
| P5 Tax compliance | Tax rules/codes/identities, immutable ledgers, invoice and note capture, reversals, locked periods, exact recaps, and export outboxes |
| P6 CRM | Company scoping, pipeline/activity workflows, conversion, idempotency, reporting, HTTP/RBAC, and repository constraints/transactions |

The optional CRM repository integration tests require a disposable database
DSN and are enabled separately:

```bash
CRM_TEST_DSN="postgres://..." go test -tags=integration ./internal/crm
```

Two release checks are intentionally blocked rather than treated as ordinary
unit-test failures:

- `internal/payroll/annual_reconciliation_release_test.go` remains skipped
  until approved December PPh 21 reconciliation examples and the release
  strategy exist.
- `internal/tax/external_release_test.go` remains skipped until the current
  official DJP/Coretax validator and an approved representative-month fixture
  are available. Follow `docs/guides/coretax-validation.md` for that sign-off.

## Troubleshooting

## RBAC repair diagnostics

Inspect the effective permissions for the seeded administrator:

```sql
SELECT
    u.email,
    r.name AS role_name,
    p.name AS permission_name
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN roles r ON r.id = ur.role_id
JOIN role_permissions rp ON rp.role_id = r.id
JOIN permissions p ON p.id = rp.permission_id
WHERE u.email = 'admin@odyssey.local'
ORDER BY p.name;
```

Verify the Phase 1–6 administrator grants specifically:

```sql
SELECT p.name
FROM permissions p
JOIN role_permissions rp ON rp.permission_id = p.id
JOIN roles r ON r.id = rp.role_id
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
  AND (
    p.name LIKE 'delivery.return.%'
    OR p.name LIKE 'finance.ar.credit_note.%'
    OR p.name LIKE 'finance.ap.debit_note.%'
    OR p.name LIKE 'procurement.return.%'
    OR p.name LIKE 'approvals.%'
    OR p.name LIKE 'hr.%'
    OR p.name LIKE 'payroll.%'
    OR p.name LIKE 'tax.%'
    OR p.name LIKE 'crm.%'
  )
ORDER BY p.name;
```

If vet or test still appears to hang:

- Verify that `ODYSSEY_TEST_MODE` is set to `1` in the shell.
- Ensure that no process is attempting to connect to PostgreSQL or Redis by
  checking `ps` output for `psql` or `redis-cli` commands.
- Run packages individually with `go test -run ^$ <package>` to identify any
  remaining integration-style code paths that require further guards.

Document any new findings in this runbook so that the next engineer can resolve
similar issues quickly.
