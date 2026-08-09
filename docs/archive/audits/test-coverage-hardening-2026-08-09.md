# Test Coverage Hardening Audit — 2026-08-09

## Scope

This audit records the test-coverage remediation for the previously untested
CMMS, Documents, QMS, Dashboard, Distribution, Logistics, Portal, Reporting,
Search, and outbox packages, together with the high-risk Master Data,
Accounting, Sales, Delivery, Finance, HR, and connector areas.

The percentages below are package statement coverage measured after the test
additions. A non-zero percentage confirms that the package is exercised; it is
not a claim that the package is fully covered.

## Previously uncovered packages

| Area | Package coverage after remediation |
|------|------------------------------------|
| CMMS | `internal/cmms` 7.8%; `internal/cmms/http` 25.1% |
| Documents | `internal/documents` 2.9%; `internal/documents/http` 22.6% |
| QMS | `internal/qms` 10.1%; `internal/qms/http` 18.8% |
| Dashboard | `internal/dashboard` 35.7% |
| Distribution | `internal/distribution` 6.2%; the disabled test was replaced with active service and handler tests |
| Logistics | `internal/logistics` 8.0% |
| Portal | `internal/portal` 11.3% |
| Reporting | `internal/reporting` 55.7%; catalog behavior now has direct tests |
| Search | `internal/search` 15.7% |
| Core outbox | `internal/outbox` 50.0%; dispatcher and domain behavior now have direct tests |

## High-risk package baseline

| Area | Package coverage after remediation |
|------|------------------------------------|
| Master data | `masterdata` 100.0%; branches 10.6%; categories 11.1%; companies 10.4%; products 9.3%; suppliers 11.1%; taxes 11.5%; units 11.1%; warehouses 10.6% |
| Accounting | parent 20.4%; accounts 7.8%; assets 26.7%; dimensions 23.6%; mappings 81.8%; periods 80.0%; schedules 20.8% |
| Sales | parent 64.3%; shared calculations 100.0%; customers 9.2%; orders 6.5%; quotations 8.4% |
| Delivery | parent 85.7%; inventory integration 54.5%; orders 9.2% |
| Finance | forecasting 71.4%; banking 36.0%; treasury 44.3% |
| HR | benefits 90.5%; employees 50.0%; leave 37.4%; performance 87.5% |
| Connectors | parent 12.7%; DHL 100.0%; MockPay 100.0%; OpenAI 100.0% |

## Test strategy

- Service and handler tests use in-memory fakes to cover validation, error
  mapping, routing, and lifecycle behavior without requiring PostgreSQL.
- Repository tests use `pgxmock` where SQL behavior and no-row/error paths are
  the contract under test.
- Connector provider tests cover request construction, response translation,
  signatures, and provider error paths.
- Integration boundaries, including delivery inventory reservation and the
  outbox dispatcher, are tested with explicit fakes and controllable callbacks.

The accounting mappings and periods repositories now depend on narrow query
interfaces. The production constructor still accepts the normal PostgreSQL
pool, while the narrower seam allows repository behavior to be tested without
an external database.

## Verification

The remediation was verified with:

```bash
GOCACHE=/tmp/odyssey-go-cache \
ODYSSEY_TEST_MODE=1 \
GOTENBERG_URL=http://127.0.0.1:0 \
go test -count=1 -p 1 ./...

GOCACHE=/tmp/odyssey-go-cache go vet ./...
git diff --check
make docs-check
```

The serial test flag (`-p 1`) is useful when the local Go build cache or
temporary filesystem is resource-constrained; it does not change test
semantics.

## Follow-up work

The original zero-test gaps are closed, but several package percentages remain
low because SQL-heavy repositories, SSR branches, and database-backed workflows
still need integration coverage. The next coverage pass should prioritize:

1. database-backed repository and migration scenarios for Master Data and the
   remaining Accounting services;
2. authenticated SSR handler paths and validation errors across Sales and
   Delivery; and
3. end-to-end connector and outbox flows using the shared integration test
   environment.

This audit is historical evidence of the remediation completed on 2026-08-09.
The active testing workflow is maintained in
[`docs/guides/testing-runbook.md`](../../guides/testing-runbook.md).
