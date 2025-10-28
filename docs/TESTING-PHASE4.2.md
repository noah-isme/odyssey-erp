# Testing Plan – Phase 4.2 Accounting

This document outlines the QA scope for the accounting implementation. Tests extend the coverage introduced in Phase 3 and verify integrations, ledger posting rules, and reporting accuracy.

## Unit & Service Tests

1. **Posting Validation**
   - Table-driven tests for `accounting.Service.PostJournal` covering:
     - Balanced vs. imbalanced payloads
     - Period status (OPEN, CLOSED, LOCKED)
     - Idempotency conflict via `source_links`
   - Ensure audit logger invoked with correct metadata.
2. **Void & Reverse**
   - Verify VOID marks entry status and writes audit log.
   - Reverse generates mirrored journal lines in next OPEN period.
3. **Period Helpers**
   - `shared.PeriodGuard` tests for transition rules and permission checks.

## Integration Tests

1. **GRN Posting**
   - Simulate GRN approval → expect Inventory Dr / GRIR Cr entry with mapping keys.
2. **AP Invoice Posting**
   - Balanced entry with tax component; ensure `source_links` prevents duplicates.
3. **AP Payment Posting**
   - Payment reduces AP and cash. Void path reopens invoice balance.
4. **Inventory Adjustment**
   - Positive adjustment hits gain account; negative hits loss.

Each integration test seeds CoA using `samples/coa.csv` and mapping table.

## Reporting Verification

1. Refresh MV (`make refresh-mv`) and call TB/P&L/BS handlers.
2. Snapshot totals and compare to known fixture dataset.
3. Ensure PDF endpoints return `Content-Type: application/pdf` and size > 1 KB.

## Regression Matrix

| Scenario | Modules | Expected |
| --- | --- | --- |
| Period close with pending GRN | Procurement, Accounting | Close rejected with explanation message |
| Duplicate AP invoice posting | Procurement, Accounting | HTTP 409 with reference to existing JE |
| Reverse JE in locked period | Accounting | HTTP 403 `period locked` |
| Trial balance after reversal | Accounting, Reports | Closing balance remains zero |

## Tooling

* `go test ./...` – unit tests
* `make migrate-up` – apply schema changes
* `make seed-phase4` – load baseline CoA & mappings
* `make refresh-mv` – refresh reporting snapshot
* `make reports-demo` – generate PDF samples

Document updates and test evidence are stored in `docs/testing-phase4.2/` (to be attached in release notes).
