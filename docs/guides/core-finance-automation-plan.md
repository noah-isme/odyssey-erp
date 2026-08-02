# Core Finance Automation Plan

**Priority:** High

**Status:** Planned

**Scope:** Cash forecasting, automated bank feeds, payment scheduling, end-to-end
purchase-to-pay automation, and fixed-asset maintenance, transfer, location, and
warranty management.

## Outcome

Deliver a controlled finance operations loop in which bank activity is imported
automatically, expected cash is visible for a rolling 13-week horizon, approved
liabilities become scheduled payments, purchasing exceptions are routed rather
than handled off-system, and assets remain traceable after capitalization.

The target flow is:

```text
PR -> approval -> PO -> receipt -> invoice capture -> three-way match
   -> payment proposal -> approval -> schedule/export/provider execution
   -> AP allocation -> bank reconciliation
                          |
                          +-> cash forecast actualization

Capitalizable receipt/invoice -> asset registration -> location/assignment
   -> maintenance + warranty -> transfer -> disposal
```

## Existing baseline

Extend the current modules instead of creating replacement ledgers:

- `internal/procurement/` owns PR, PO, approval, GRN, and supplier-return flows.
- `internal/ap/` owns vendor invoices, allocations, debit notes, aging, payment
  accounting, tax hooks, and transaction-level FX.
- `internal/finance/banking/` owns bank accounts, transactions, transfers, and
  CSV/OFX imports.
- `internal/accounting/banks/` owns statements and reconciliation.
- `internal/fixedassets/` owns the asset register, depreciation, and disposal.
- `internal/approvals/`, `internal/notifications/`, `internal/audit/`, and `jobs/`
  provide shared controls and asynchronous processing.

Two constraints shape the design:

1. The two existing banking packages have different responsibilities. Keep the
   account/transaction and statement/reconciliation boundaries initially, but
   define one application service contract between them before adding feeds.
2. `ap_payments` currently represent a recorded financial event. A scheduled or
   merely exported payment must not create an AP payment or journal entry until
   execution is confirmed.

## Delivery principles

- Every record and query is company-scoped; cross-company payment batches and
  asset transfers are not supported.
- Monetary values use exact decimal/`NUMERIC` values and retain transaction and
  base currency valuation. Do not add new accounting calculations with `float64`.
- Provider messages, worker retries, document conversion, payment execution, and
  journal posting use stable idempotency keys.
- Posted accounting documents and confirmed executions are corrected by
  reversal/cancellation records, never in-place edits.
- Open-period checks, approval policies, maker-checker separation, audit events,
  and notifications are part of each increment rather than a final hardening pass.
- Provider credentials are secret references supplied through deployment
  configuration; tokens and raw credentials are never stored in business tables
  or written to logs.

## Phase 0 — Shared controls and contracts

**Estimate:** 1–2 weeks

**Exit condition:** The later workstreams share one set of lifecycle, provider,
money, approval, and observability conventions.

1. Write an ADR for the bank boundary and payment execution model. Define the
   canonical ownership of external transactions, statements, matches, and ledger
   postings across the two existing banking packages.
2. Define provider-neutral ports for bank-feed polling/webhooks, balance lookup,
   payment submission, payment-status lookup, and payment-file generation. Keep
   provider implementations under `internal/integration/`.
3. Add permissions for feed administration, forecast viewing/editing, payment
   proposal/approval/execution, P2P exceptions, and asset operations. Add explicit
   incompatible-role guidance for proposer, approver, and executor.
4. Standardize correlation IDs, source references, outbox records, retry policy,
   and audit metadata for finance automation jobs.
5. Add company-level finance automation settings: forecast horizon, bank-feed
   cadence, matching tolerances, payment cut-off/calendar, invoice tolerances, and
   feature flags.

## Phase 1 — Automated bank feeds

**Estimate:** 2–3 weeks

**Depends on:** Phase 0

### Data and services

- Add bank connections, external-account mappings, consent/connection status,
  cursor state, feed runs, and provider-event inbox records.
- Normalize provider entries into the current statement import path. Enforce a
  unique provider transaction ID, with a deterministic account/date/amount/hash
  fallback only when the provider supplies no stable ID.
- Persist raw provider payloads only when operationally required, encrypted or
  access-restricted, and with a documented retention period.
- Add scheduled polling and signed webhook ingestion. Polling and webhooks must
  converge through the same idempotent ingestion service.
- Reuse and extend reconciliation matching. Suggested matches may consider amount,
  date window, reference, counterparty, and known AP payment identifiers; only an
  authorized confirmation changes reconciliation state.

### User experience and operations

- Add connection health, last successful synchronization, next run, consent
  expiry, failed-run reason, and retry controls to banking administration.
- Notify finance operators about expired consent, stale feeds, repeated failures,
  duplicate anomalies, and unreconciled high-value transactions.
- Preserve CSV/OFX import as a supported fallback and run it through the same
  duplicate protection.

### Acceptance gate

- Replaying a webhook, page cursor, or worker task creates no duplicate statement
  line or bank transaction.
- A provider sandbox account completes connect, incremental sync, reconnect,
  error recovery, reconciliation, and audit-trail scenarios.
- Company-isolation tests prove that connection metadata and transactions cannot
  cross tenants; logs and error pages contain no credentials.

## Phase 2 — Rolling cash forecasting

**Estimate:** 2–3 weeks

**Depends on:** Phase 0; Phase 1 is recommended for reliable actual balances.

### Forecast model

- Produce daily positions for 13 weeks, with configurable weekly/monthly summary
  views and base-currency consolidation.
- Start from the most recent reconciled bank balance, falling back to the current
  bank-account balance with an explicit freshness warning.
- Include committed sources: open AR by expected receipt date, posted AP by due or
  scheduled payment date, approved payroll, tax obligations, and approved payment
  batches.
- Include probable sources separately: approved POs not yet invoiced, recurring
  cash items, and user-entered forecast adjustments. Probable items must never be
  mixed silently into the committed scenario.
- Store scenario definitions, forecast runs/snapshots, source lines, manual
  overrides with reasons, and actual-versus-forecast outcomes. Source documents
  remain authoritative; snapshots provide reproducibility.
- Prevent double counting when an expected item becomes a payment, bank
  transaction, or reconciled actual. Lock FX rates to the forecast run and show
  rate/freshness metadata.

### Product surface

- Add base, conservative, and optimistic scenarios; cash-in/cash-out drill-down;
  minimum-cash threshold alerts; and CSV/XLSX export.
- Show opening cash, inflows, outflows, closing cash, headroom, stale-data flags,
  and the source document behind every line.
- Run a nightly refresh and allow an authorized on-demand refresh. A failed source
  must mark the forecast incomplete rather than silently return zero.

### Acceptance gate

- A seeded 13-week scenario reconciles opening cash and all source totals exactly
  to bank, AR, AP, payroll, and tax records.
- Payment rescheduling, partial receipts/payments, invoice voids, and FX-rate
  changes produce deterministic forecast deltas without duplicate lines.
- Actual-versus-forecast reporting supports forecast accuracy measurement by week
  and source type.

## Phase 3 — Payment scheduling and execution

**Estimate:** 3–4 weeks

**Depends on:** Phase 0; integrates with Phase 2 when available.

### Lifecycle and data

Use an explicit batch lifecycle:

```text
DRAFT -> PROPOSED -> APPROVAL -> APPROVED -> SCHEDULED -> PROCESSING
                                                        -> SETTLED
                                                        -> PARTIAL / FAILED
          \-> REJECTED                     \-> CANCELLED before submission
```

- Add payment batches, batch items, proposed invoice allocations, approval links,
  execution attempts, provider/file artifacts, status events, and settlement
  references.
- Generate proposals from posted AP balances using due date, discount date,
  supplier hold, priority, currency, available cash, bank account, cut-off time,
  holiday calendar, and configured minimum-cash constraints.
- Revalidate invoice balance, supplier bank details, approval state, accounting
  period, and cash constraint immediately before submission.
- Freeze approved batch content. Any amount, date, bank, beneficiary, or allocation
  change creates a new approval revision.
- Support a controlled bank-file/export mode first, then provider API submission.
  Encrypt generated artifacts at rest and restrict download by permission.
- Create `ap_payments`, allocations, FX journals, and tax capture only on confirmed
  settlement. Partial settlement creates only the confirmed amount; failures leave
  invoices open. Reconciliation records the provider/end-to-end reference.

### Controls

- Enforce maker-checker-executor segregation and optional dual approval above a
  configured threshold.
- Require verified supplier bank details and audit every change. A bank-detail
  change places the supplier on payment hold until independently approved.
- Use a transactional outbox between approved batches and provider/file execution;
  retries must query provider status before resubmission.

### Acceptance gate

- Tests cover full, partial, rejected, cancelled, timed-out, duplicate callback,
  and ambiguous provider responses without duplicate payment or journal records.
- The sum of settled batch items equals AP allocations, provider settlement, bank
  transaction, and GL cash movement in both transaction and base currency.
- An actor cannot propose and approve or approve and execute the same batch when
  segregation is enabled.

## Phase 4 — Purchase-to-pay orchestration

**Estimate:** 4–6 weeks

**Depends on:** Phases 0 and 3 for the complete path.

### Matching and exceptions

- Add invoice intake through structured upload/API and a controlled manual route.
  OCR/email capture may be a later provider adapter; it is not required for the
  first production release.
- Add immutable source attachments and duplicate-invoice detection by company,
  supplier, supplier invoice number, date, amount, currency, and content hash.
- Track ordered, received, returned, invoiced, debit-noted, and paid quantities and
  values per PO line. Support partial receipts and partial invoices explicitly.
- Implement two-way matching for service/non-receipt POs and three-way matching for
  goods. Policies define quantity, unit-price, tax, freight, and total tolerances by
  company and optionally supplier/category.
- Add match results and exception cases with reason, owner, evidence, approval,
  resolution, and timestamps. Exceptions never silently alter the PO or receipt.
- Auto-create a draft AP invoice after successful intake/match, and allow policy-
  controlled auto-posting only when the match passes, tax configuration is valid,
  the period is open, and no duplicate or supplier hold exists.

### Orchestration

- Emit durable lifecycle events/outbox records for PR submission, PO approval,
  receipt posting, return confirmation, invoice matching/posting, payment approval,
  settlement, and reconciliation.
- Close PR/PO lines from accumulated quantities, not a single downstream event.
  Define cancellation/reopen behavior and prevent over-receipt or over-invoicing
  outside approved tolerances.
- Add an operations work queue showing documents blocked by approval, overdue
  receipt, invoice mismatch, missing tax/account mapping, payment hold, failed
  execution, or unreconciled settlement.
- Use notifications and escalation timers for each queue state; retries remain
  idempotent and visible.

### Acceptance gate

- Database-backed tests prove the happy path and partial/return/debit-note paths
  from PR through reconciled payment.
- Tolerance breach, duplicate invoice, supplier hold, closed period, missing tax
  mapping, and payment failure all stop at a visible exception without partial
  accounting side effects.
- The audit timeline can reconstruct every automated decision, policy version,
  actor, source document, and resulting journal.

## Phase 5 — Asset operations and P2P capitalization

**Estimate:** 3–4 weeks

**Depends on:** Phase 0; capitalization integration depends on Phase 4.

### Asset model

- Add asset locations with branch/site/building/room hierarchy; assignments or
  custodians; condition; serial/model/manufacturer fields; and effective-dated
  location history.
- Add transfer requests, approvals, dispatch/receipt confirmation, condition notes,
  and transfer history. Intra-company transfers update custody/location and audit
  history; they do not create an accounting journal unless a reviewed accounting
  policy explicitly requires one.
- Add warranties with provider, policy/reference, start/end dates, coverage,
  documents, contacts, and expiry alerts.
- Add maintenance plans, meter readings where applicable, work orders, preventive
  schedules, corrective incidents, parts/labor/external cost, downtime, completion,
  and next-due calculation.
- Keep financial status (active/fully depreciated/disposed) separate from operational
  condition (available/in use/under maintenance/lost). Maintenance must not mutate
  depreciation state.

### Procurement and accounting integration

- Mark eligible PO/AP lines with capitalization intent, asset category, quantity,
  and expected location. On successful receipt and posted invoice, create a
  capitalization candidate rather than an active asset silently.
- Require asset acceptance to confirm asset count, numbers/tags, in-service date,
  location, custodian, useful life, cost allocation, and warranty before activation.
- Preserve the existing depreciation/disposal service. Activation supplies its
  acquisition basis and source links; later maintenance costs are expensed unless
  an explicitly approved capitalization policy creates a separate adjustment.
- Support barcode/QR labels and a full asset timeline linking PO, GRN, AP invoice,
  journals, transfers, maintenance, warranty claims, and disposal.

### Acceptance gate

- A multi-quantity capital purchase creates the correct number of uniquely tagged
  assets, with exact allocated cost and complete PO/GRN/AP lineage.
- Transfer approval, dispatch, receipt, cancellation, concurrent updates, warranty
  alerts, preventive scheduling, maintenance completion, depreciation, and disposal
  are covered by unit and database-backed tests.
- Asset history remains readable after location, user, supplier, or warranty data
  changes, and disposed assets cannot be transferred or receive new work orders.

## Phase 6 — Hardening and rollout

**Estimate:** 2 weeks plus provider certification lead time

**Depends on:** Each feature can enter this gate independently.

1. Run `gofmt`, `go vet`, `make lint`, focused package tests, `make test`, migration
   up/down checks, `make build`, and `make docs-check`.
2. Add PostgreSQL integration tests for company isolation, unique/idempotency
   constraints, row locking, period enforcement, exact money, and rollback behavior.
3. Add HTTP/E2E coverage for the main SSR work queues and permissions. Add provider
   contract tests using recorded sanitized fixtures and sandbox smoke tests.
4. Publish dashboards and alerts for feed freshness/failure, forecast freshness,
   unmatched statement lines, exception age, payment execution latency/failure,
   outbox depth, warranty expiry, and overdue maintenance.
5. Roll out per company behind feature flags:
   - bank feeds in shadow/import-only mode before enabling automatic ingestion;
   - forecasts beside spreadsheet forecasts for at least two cycles;
   - payment scheduling in export-only mode before provider submission;
   - P2P auto-posting only after a period of match suggestions and reviewed results;
   - asset import/reconciliation before enabling automated maintenance schedules.
6. Document disconnect, payment-stop, provider-outage, duplicate/ambiguous payment,
   forecast correction, invoice-exception, and asset-transfer recovery runbooks.

## Delivery sequence and staffing

For one cross-functional team, plan **17–24 engineering weeks** sequentially. With
two implementation streams after Phase 0, target **12–16 elapsed weeks**, excluding
bank onboarding/certification lead time:

| Release | Scope | Suggested sequence |
|---|---|---|
| A | Controls, bank feeds, initial cash forecast | Phases 0 → 1 → 2 |
| B | Payment proposals, approvals, export, settlement | Phase 3 |
| C | Matching, exceptions, orchestration, auto-post policy | Phase 4 |
| D | Asset operations and capitalization integration | Phase 5; may start after Phase 0 |
| E | Provider execution and production hardening | Phase 6 throughout, final certification last |

Minimum team shape: one finance/product owner, two Go engineers, one UI engineer,
shared QA/automation support, and part-time security/operations review. A treasury
operator, AP operator, procurement approver, and asset custodian must sign off the
relevant acceptance scenarios.

## Success measures

Establish a baseline before Release A and report by company:

- bank-feed freshness and successful sync rate;
- automatic reconciliation suggestion and confirmed-match rates;
- 13-week forecast completeness and weekly accuracy by source type;
- invoices processed without manual correction, match-exception rate, and median
  receipt-to-post time;
- payments made on time, payment failure/duplicate rate, and proposal-to-settlement
  time;
- percentage of fixed assets with current location/custodian/warranty data,
  preventive maintenance completed on time, and overdue work-order count.

Do not define “full automation” as zero human involvement. The release is complete
when routine documents move without re-keying, risky decisions stop for policy-based
approval, exceptions are visible and recoverable, and every financial result remains
traceable to its source and control decision.
