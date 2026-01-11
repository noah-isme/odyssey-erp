# Accounting Runbook – Phase 4.2

This runbook documents the day-to-day and month-end operating procedures for Odyssey ERP's General Ledger module. It aligns with ADR-0004, default account mapping, and the period policy finalised in Phase 4.2.

## Daily Operations

1. **Verify Open Period**
   - Navigate to *Finance → Periods* and confirm the target period is OPEN.
   - If no OPEN period exists, request a finance controller to open the next period.
2. **Review Automated Postings**
   - Monitor procurement and inventory queues for errors. The integration hooks write audit logs with entity `journal_entry`.
   - Use `GET /finance/journals?source_module=<module>` to inspect recent postings.
3. **Manual Journals**
   - Prepare balanced journal payloads (minimum two lines). Debit and credit totals must match.
   - Provide `source_module` and `source_id` values even for manual entries to maintain traceability (use UUID v4).
4. **Materialized View Refresh**
   - Run `make refresh-mv` after large batches of postings to refresh `gl_balances`.
   - Refresh is lightweight and can be executed multiple times per day.
5. **Reporting**
   - Use `GET /finance/reports/trial-balance` for on-screen review.
   - Generate PDF snapshots via `make reports-demo` when finance leadership requests previews.

## Month-End Close Checklist

1. **Inventory & Procurement Completion**
   - Confirm all GRNs are matched to invoices, and pending approvals are resolved.
   - Run inventory reconciliation and ensure no negative stock adjustments remain open.
2. **Trial Balance Review**
   - Execute `make refresh-mv` and open the TB report.
   - Investigate any accounts with unexpected balances using the GL drill-down.
3. **AP/AP Payment Verification**
   - Ensure all approved invoices and payments have corresponding journal entries (idempotency prevents duplicates).
4. **Close Period**
   - With permission `finance.period.close`, change status to CLOSED.
   - Enter justification text; the system records audit metadata.
5. **Lock Period** (optional)
   - After publishing statutory statements, lock the period. Requires `finance.override.lock`.
   - Record approval ticket reference in the justification field.
6. **Archive Reports**
   - Generate and store TB, P&L, and BS PDFs. Upload to secure document management per company policy.

## Troubleshooting

| Symptom | Action |
| --- | --- |
| `409 source already linked` when posting | Locate original journal via `GET /finance/journals?source_id=<uuid>`. Void or reverse if necessary. |
| `period locked` error | Request override approval, unlock period, post adjustment in current open period, then relock. |
| Trial Balance totals mismatch | Run `jobs/gl_integrity` manually. If failure persists, inspect audit logs for manually modified entries. |
| Materialized view stale | Run `make refresh-mv`. Confirm Asynq worker `fin_views_refresh` is scheduled. |

## Emergency Procedures

* **Void Journal Entry** – Use `/finance/journals/{id}/void`. Service transitions status to VOID and records audit log. Only available while period is not LOCKED.
* **Reverse Journal Entry** – Use `/finance/journals/{id}/reverse`. Reverse entry posts into the same period if OPEN, otherwise the next OPEN period.
* **Unlocking a Period** – Requires `finance.override.lock`. Audit log must capture reason; notify compliance team immediately.

## Contacts

* Ledger Tech Lead – ledger@odyssey.local
* Finance Controller – finance-controller@odyssey.local
* DevOps On-call – devops@odyssey.local
