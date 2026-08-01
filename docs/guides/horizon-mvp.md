# Horizon MVP Foundation (P7)

P7 is the current MVP foundation for multi-company Horizon workflows. It provides
shared persistence, isolation, idempotency, and lifecycle controls for:

- WMS bins, barcode aliases, pick waves, pick tasks, and scans.
- MRP BOMs and work orders.
- POS terminals, sessions, tickets, payments, refunds, and voids.
- Projects, tasks, members, timesheets, and FX snapshots.
- Public API keys, scopes, idempotent project creation, and stable JSON errors.
- Webhook subscriptions, encrypted secrets, signatures, retries, and deduplication.
- Customer, supplier, and employee portals with company-scoped access.

## Local certification

Local P7 gates passed against the clean schema through migration v61. The acceptance
record is [Phase 14/P7 Acceptance Evidence](phase14-p7-acceptance-evidence.md).

The local gate covered focused module tests, the tagged integration suite, the full Go
test suite, lint, build, migration checks, company isolation, idempotency, lifecycle
transitions, API credential handling, webhook secret protection, and timesheet FX
snapshot persistence.

## Isolation and retry policy

Every Horizon operational record carries or derives a `company_id`. Repository lookups
must include the authenticated company, and linked products, warehouses, projects,
customers, suppliers, employees, and portal identities must be validated against it.

Retries use scoped idempotency keys. A repeated key returns the original result and does
not create another record, payment, scan, journal, or webhook delivery. POS payment
insertion locks the ticket while checking the aggregate total to prevent concurrent
overpayment.

## Release gates remaining

Local certification is not production certification. Staging must still verify:

1. Migration execution and all role/account mappings.
2. The daily FX worker in `Asia/Jakarta`.
3. Webhook delivery, signing, retry, and duplicate-event behavior.
4. The cross-feature scenario: USD project/timesheet snapshot, later FX rate, foreign
   currency POS sale, WMS fulfillment, AR payment, and outstanding-balance revaluation.
5. Production migration, smoke tests, and recorded operational evidence.

Full vertical capabilities beyond this foundation remain future scope.
