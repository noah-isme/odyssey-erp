# Core Finance Automation Plan

**Priority:** High

**Status:** Partially implemented (foundation, first treasury slice, and provider-neutral payment execution coordinator)

**Scope:** Cash forecasting, automated bank feeds, payment scheduling, end-to-end
purchase-to-pay automation, and fixed-asset maintenance, transfer, location, and
warranty management.

## Execution objective

Move from the current manual/import-assisted finance processes to a controlled loop:

```text
bank data -> reconciled actual cash -> 13-week forecast -> payment proposal
     -> approval -> bank export/provider execution -> settlement -> reconciliation

PR -> PO -> receipt -> invoice intake -> match/exception -> AP posting
     -> scheduled payment -> settlement

capital purchase -> capitalization candidate -> accepted asset -> location/custodian
     -> transfer + warranty + maintenance -> depreciation/disposal
```

This plan is complete when each routine step can proceed without re-keying, while
policy exceptions stop visibly for review. Automation must never bypass company scope,
approval, exact-money, accounting-period, tax, FX, stock, or audit controls.

## Existing baseline

Extend the current modules instead of creating replacement ledgers:

- `internal/finance/banking/` owns bank accounts, transactions, transfers, and CSV/OFX
  imports.
- `internal/finance/bankfeeds/` owns company-scoped provider connections, signed webhook
  inbox records, incremental sync, and convergence into normalized banking imports.
- `internal/finance/forecasting/` owns forecast runs, database-backed source readers,
  daily roll-forward buckets, and persisted FX snapshots.
- `internal/finance/treasury/` owns supplier bank verification, tenant-scoped payment
  batches, beneficiary controls, and batch-total aggregation.
- `internal/accounting/banks/` owns statements and reconciliation.
- `internal/procurement/` owns purchase requests, purchase orders, approvals, goods
  receipts, and supplier returns.
- `internal/ap/` owns supplier invoices, allocations, debit notes, aging, payment
  accounting, tax hooks, and transaction-level FX.
- `internal/fixedassets/` owns the asset register, depreciation, and disposal.
- `internal/approvals/`, `internal/notifications/`, `internal/audit/`, and `jobs/`
  provide shared controls and asynchronous processing.

Two current constraints shape the implementation:

1. Finance banking and accounting banks have different responsibilities. Keep those
   boundaries initially and approve one application contract for external transactions,
   statements, matches, and ledger postings before adding live feeds.
2. `ap_payments` represent recorded financial events. A proposed, scheduled, or
   exported payment must not create an AP payment or journal until settlement is
   confirmed through the approved execution policy.

## Delivery principles

- Every new record and query is company-scoped; cross-company payment batches and
  asset transfers are not supported.
- Monetary values use exact decimal/`NUMERIC` representations and retain transaction
  and base-currency valuation. New finance calculations must not use `float64`.
- Provider messages, worker retries, document conversion, payment execution, matching,
  and journal posting use stable idempotency keys.
- Posted documents and confirmed executions are corrected through controlled reversal,
  cancellation, or adjustment records rather than destructive edits.
- Open-period checks, approvals, maker-checker separation, audit events, notifications,
  observability, and recovery behavior are acceptance requirements for every increment.
- Provider credentials and sensitive payment artifacts use encrypted secret references
  and restricted storage; plaintext values never enter business tables, logs, or job
  payloads.
- External adapters remain separate from the existing in-process accounting adapters in
  `internal/integration/`; the final package boundary follows the external-integrations
  ADR.

## Repository ownership

Extend the current owners and add narrow packages only where a new domain has no owner:

| Capability | Business owner | Implementation location |
|---|---|---|
| Bank accounts and normalized transactions | Finance banking | `internal/finance/banking/` |
| Statements and reconciliation | Accounting banks | `internal/accounting/banks/` |
| Provider connection and ingestion | New bank-feed application service | Proposed internal/finance/bankfeeds package; provider adapter location follows the external-integrations ADR |
| Forecast scenarios and snapshots | Treasury forecast service | `internal/finance/forecasting/` |
| Payment proposals and execution | New treasury payment service | `internal/finance/payments/` now provides the provider-neutral coordinator, versioned PostgreSQL execution snapshots, payment/result outbox commands, durable settlement-result/effect idempotency, and bounded worker registration for `payment.result.import`; live execution/provider adapters and confirmed settlement calls into `internal/ap/` remain |
| PO, receipt, return, and line progress | Procurement | `internal/procurement/` |
| Supplier invoice and allocations | Accounts payable | `internal/ap/` |
| Matching and P2P exception queue | Procurement/AP boundary | Proposed internal/procurement/matching package, with explicit ports into AP |
| Asset lifecycle and operations | Fixed assets | `internal/fixedassets/` |
| Approvals, notifications, audit, jobs | Shared platform | Existing shared modules and `jobs/` |
| SSR pages | Owning module | `web/templates/pages/finance/`, `procurement/`, `ap/`, and fixed-assets pages |
| App wiring | Application composition | `internal/app/`, `cmd/odyssey`, and `cmd/worker` |

Do not put provider code or orchestration SQL into handlers. Handlers validate HTTP
input and call application services; services enforce policy; repositories own scoped
queries; adapters translate external formats.

## Decisions required before implementation

The finance/product owner records these in ADRs or approved configuration stories
during Work Package F0. Defaults may be proposed, but code must not silently decide
material accounting policy.

| Decision | Required owner | Blocks |
|---|---|---|
| Canonical ownership between finance bank transactions and accounting statements | Finance architecture + controller | Bank feeds, reconciliation |
| First bank connection mode: aggregator API, direct bank API, or statement transport | Treasury + security | Provider adapter/certification |
| Authoritative cash balance and freshness threshold | Treasury | Forecast opening position |
| AR expected-date rule and AP payment-date rule | Controller + treasury | Forecast sources |
| Committed/probable scenario policy and minimum-cash threshold | Treasury | Forecast UI and proposal engine |
| Supplier bank-detail verification and maker/checker/executor matrix | AP owner + security | Payment execution |
| Settlement event that authorizes AP payment creation | Controller | Payment execution/accounting |
| Invoice quantity, price, tax, freight, and total tolerances | Procurement + AP | Matching/auto-posting |
| Capitalization threshold, asset split/allocation, and later-cost treatment | Controller + asset custodian | Asset candidates/activation |
| Location hierarchy, transfer custody, warranty-alert window, maintenance SLA | Asset custodian | Asset operations |
| Retention/redaction for provider payloads and payment artifacts | Security + compliance | Bank feeds/payments |

## Dependency map

```text
F0 Shared decisions, controls, settings, and job conventions
├── T1 Bank-feed normalization ──> T2 Provider sync ──> T3 Cash forecast
│                                                   └──> T4 Forecast actuals
├── P1 Supplier payment controls ──> P2 Proposals/approval ──> P3 Export
│                                                        └──> P4 Settlement
├── Q1 PO/receipt/invoice progress ──> Q2 Matching ──> Q3 Exceptions/auto-post
│                                                    └──> Q4 Full orchestration
└── A1 Asset locations/history ──> A2 Transfers/warranty/maintenance
                                 └──> A3 Capitalization candidates

T3 informs P2 cash constraints. P4 and Q4 close the P2P loop.
Q3 is required before policy-controlled auto-posting. A3 depends on Q2 source lineage.
```

Bank feeds, payment export, and asset operations can be released independently. Live
payment submission, automatic AP posting, and automatic asset activation are later
gates, not requirements for the first useful increment.

## Work packages

Each work package should normally be one to three pull requests. A PR must include its
schema/query changes, service behavior, tests, permissions/audit, and minimal operator
documentation rather than leaving controls to a later cleanup.

### F0 — Shared foundation

**Duration:** Weeks 1–2

**Implementation status:** The F0 migration, conservative company settings, RBAC
permission inventory, finance outbox contract, worker-dispatch boundary, and provider-
neutral bank-feed/payment contracts are implemented. The four governing ADRs are
[bank ownership and feed ingestion](../decisions/ADR-0006-bank-ownership-and-feed-ingestion.md),
[payment execution and settlement](../decisions/ADR-0007-payment-execution-and-settlement.md),
[P2P matching and exceptions](../decisions/ADR-0008-p2p-matching-and-exceptions.md),
and [asset capitalization and operations](../decisions/ADR-0009-asset-capitalization-and-operations.md).
They remain proposed until the named treasury, controller, AP, procurement, and asset
custodian approvals are recorded.

#### F0 completion ledger

- [x] Record the four policy ADRs and lifecycle diagrams before adding downstream
  workflow migrations.
- [x] Add provider-neutral bank-feed and payment ports, canonical external references,
  correlation/causation identifiers, typed provider errors, retry policy, and fake-port
  contract coverage.
- [x] Use exact money/currency contracts for every new finance automation boundary; the
  F0 automation, bank-feed, payment, and new forecast-reader paths carry exact decimal
  values. Legacy forecast presentation fields remain an explicit follow-up.
- [x] Add company-scoped, disabled-by-default settings and feature flags, including
  payment execution's dependency on payment scheduling.
- [x] Add finance/P2P/fixed-asset permissions, actor-separation validation, a durable
  idempotent outbox, and the shared Asynq dispatch boundary.
- [x] Add unit and migration coverage for company-scoped settings/outbox behavior,
  conservative defaults, and incompatible payment duties.
- [x] Record named finance-owner approval of ADR-0006 through ADR-0009.
- [x] Apply migration `000076_finance_automation_foundation` in staging and verify
  existing and newly created companies receive disabled settings rows.

#### F0.1 — Architecture and policy contracts

- Record the bank ownership, payment execution, matching, and asset capitalization
  ADRs using the decisions above.
- Define provider-neutral bank-feed and payment ports, canonical external transaction
  identifiers, correlation/source references, and error categories.
- Define lifecycle tables and state diagrams before migrations are written.
- Confirm exact money/FX types and prohibit new `float64` finance calculations.

**Exit:** ADRs are approved by engineering and the named finance operators; interfaces
can be tested with fakes without choosing a production provider.

#### F0.2 — Settings, permissions, and shared delivery records

- Add company finance-automation settings with conservative defaults and feature flags.
- Add permissions for feed management, forecast use, payment proposal/approval/export/
  execution, P2P exceptions, and asset location/transfer/maintenance/warranty work.
- Add shared finance outbox records only for cross-transaction delivery. Reuse existing
  approval, notification, audit, and Asynq patterns.
- Define attempt, retry, dead-letter, replay, and correlation metadata for finance jobs.

**Exit:** Company-isolation and incompatible-role tests pass; no feature is enabled by
migration alone.

### T1–T4 — Treasury: feeds and forecasting

#### T1 — Normalize all statement ingestion

**Duration:** Week 3

- [x] Extract a single normalized statement-entry command from the current CSV/OFX import.
- [x] Route CSV and OFX through the same dedupe and validation service before adding a provider.
- [x] Preserve source filename/hash, external reference, import run, and skip reason.
- [x] Add deterministic fallback fingerprinting only for sources without stable IDs.

**Primary files:** finance banking import/service/repository, accounting bank statement
service, new database constraints, and focused tests.

**Exit:** Re-importing the same CSV/OFX, including concurrent imports, creates no
duplicate statement line or bank transaction and preserves existing behavior.

#### T2 — Bank connections and automated synchronization

**Duration:** Weeks 4–5

- [x] Add connection, external-account mapping, consent status/expiry, cursor, sync run,
  provider-event inbox, and connection-health records.
- [x] Implement a connection-scoped, verified callback inbox. Callback processing claims
  the event, runs the same incremental sync as polling, and marks the event terminal only
  after normalized banking import succeeds.
- [x] Deduplicate repeated callback payloads by `(connection_id, payload_hash)` and
  recover stale `PROCESSING` claims.
- [x] Keep provider callbacks outside browser session/CSRF middleware; require the
  connection-scoped provider verifier and fail closed on expired consent or missing
  banking import dependencies.
- [x] Add a verified statement-transport boundary that checks account identity,
  signature state, checksum, cursor progression, and reuses the normalized banking
  parser/import path for CSV/OFX artifacts.
- Add scheduled sync, token/consent refresh handling, rate-limit backoff, replay, and
  disconnect behavior.
- Provider adapters still need to implement `FeedPort` and `WebhookVerifier`; unsigned
  or provider-only callbacks are rejected.
- Add banking administration pages for connection health, mappings, run history,
  failure detail, retry, and reconnect.
- Preserve manual import as a fallback.

**Exit:** Fake-provider and sandbox contract suites pass connect, incremental sync,
duplicate/out-of-order callback, expired consent, timeout, replay, disconnect, and
company-isolation scenarios.

#### T3 — Forecast engine and snapshots

**Duration:** Weeks 6–7

- [x] Define scenario definitions and forecast engine structures.
- [x] Create `000101_finance_cash_forecast` migration.
- [x] Wire background jobs in `jobs/cash_forecast.go` for periodic snapshots.
- [x] Implement database source readers for cleared/reconciled bank balances, open AR,
  posted AP, and posted payroll. Overdue AR/AP is rolled into the first forecast day.
- [x] Resolve the company base currency from `companies.base_currency` and persist the
  dated FX rate/source snapshot used by the run; missing or stale rates fail the run.
- [x] Roll opening balances and daily inflow/outflow through every day in the 13-week
  horizon instead of calculating each bucket independently.
- [x] Validate scenario ownership before creating a run; persist the exact FX rate/date/
  source set used by the run.
- [x] Add source readers for tax obligations, approved payment batches, and approved
  uninvoiced POs; each source is company-scoped and emits an exact amount with a stable
  source key.
- Add source readers for recurring/manual adjustments.
- Tag each source as committed or probable and calculate base, conservative, and
  optimistic scenarios without mixing categories invisibly.
- Use stable source keys to replace an expected item with its actual payment/bank event
  instead of double counting it.
- Schedule nightly refresh and allow permission-protected on-demand runs.

**Exit:** A seeded 13-week snapshot reconciles every total and source link exactly;
missing/stale inputs mark the run incomplete instead of returning zero.

#### T4 — Forecast operations and actuals

**Duration:** Week 8

- Add treasury summary, daily/weekly drill-down, scenario comparison, source-document
  links, freshness warnings, manual override with reason, and minimum-cash alerts.
- Add CSV/XLSX export and actual-versus-forecast views by week/source.
- Add metrics for freshness, completeness, forecast variance, and failed source readers.

**Exit:** Treasury signs off two parallel forecast cycles against its existing
spreadsheet, with documented variance explanations and no unexplained source gaps.

### P1–P4 — Treasury: payment scheduling and settlement

#### P1 — Supplier payment controls

**Duration:** Week 3; can run alongside T1

- [x] Add effective-dated supplier bank accounts, verification state, independent approval,
  evidence/reference, change audit, and automatic payment hold after sensitive changes.
- Add payment calendars, cut-off times, bank/file format configuration, thresholds, and
  segregation policy.
- Backfill existing supplier/payment references without treating them as verified.

**Exit:** An unverified or newly changed beneficiary cannot enter an executable batch;
maker/checker tests cover all configured incompatible roles.

#### P2 — Proposals, revisions, and approval

**Duration:** Weeks 9–10

- [x] Add payment batches, items, proposed allocations, revisions, approval links, and
  immutable approved snapshots.
- [x] Scope handler identity and every batch/beneficiary operation to the active session
  company. Batch revisions recompute totals from active items in SQL.
- [x] Revalidate beneficiary state and posted, unpaid AP invoice ownership/currency at
  item creation and approval.
- [x] Build proposal rules from posted AP balances, due/discount dates, holds, priority,
  currency, cash thresholds, account, cut-off, and holiday calendar.
- [x] Revalidate invoice balance, supplier, beneficiary, currency/FX, period, approval, and
  cash constraints immediately before approval and execution.
- [x] Add proposal, review, approval/rejection, scheduling, cancellation, and exception UI.

**Exit:** Concurrent proposal/approval tests cannot over-allocate an invoice; editing an
approved value creates a new revision and approval requirement.

#### P3 — Controlled bank export

**Duration:** Week 11

- [x] Implement provider-neutral payment-file generation and one reviewed bank format.
- [x] Encrypt/restrict generated artifacts, record checksums and export actor/time, and
  require a final approval snapshot.
- [x] Add an explicit `EXPORTED`/awaiting-confirmation state. Export must not create an AP
  payment, allocation, journal, or bank transaction.
- [x] Add manual confirmation/import of bank execution results as the production-safe first
  release.

**Exit:** Regenerating or downloading an artifact cannot alter financial state; the file
total and item count reconcile to the approved batch snapshot.

#### P4 — Provider execution and confirmed settlement

**Duration:** Weeks 12–13; may be deferred after export-only production

The provider-neutral coordinator in `internal/finance/payments/` now covers exact-money
proposal, approval, submission, settlement, cancellation, ambiguity lookup, and controlled
export behind injectable persistence and provider ports. A versioned PostgreSQL JSONB
execution store now provides the durable snapshot and optimistic-concurrency boundary.
Production completion still requires provider contract evidence, confirmed settlement
effects in AP/accounting, operations pages, recovery drills, and sandbox certification.
The worker composes live execution and settlement effects only for the isolated
`APP_ENV=finance-sandbox` / `RELEASE_PROFILE=v0.11-finance` profile; other profiles retain
the durable `payment.result.import` boundary and fail closed so a result cannot claim
that AP/GL/tax/FX/bank posting occurred. The payment/result outbox commands,
ambiguous-outcome dead-letter policy, durable result inbox, and effect-key idempotency
boundary remain provider-neutral.

- [x] Define payment execution/result-import outbox commands, stable idempotency keys,
  handler registration, and a terminal ambiguous-outcome path that prevents blind
  resubmission.
- [x] Compose the durable `payment.result.import` handler into the worker with
  PostgreSQL execution/result stores and keep it available outside the finance sandbox.
- [x] Add a durable batch-item producer and Midtrans Iris provider adapter for sandbox
  execution, including restart-safe lookup and cancellation references.
- [x] Add a company-scoped JSONB execution snapshot store with optimistic version checks.
- [x] On timeout or retry, query provider status before any resubmission.
- [x] Ingest partial, rejected, cancelled, failed, and settled provider results into a
  durable, company-scoped result inbox with immutable fingerprints.
- [x] Wire confirmed settlement into AP payment/allocation, journal, and bank transaction
  effects with no duplicate effects on callback replay; tax/FX/reconciliation integration
  and sandbox evidence remain open.
- [ ] Add payment operations pages and alerts for ambiguous, partial, failed, and unmatched
  settlement.

**Exit:** Database-backed and sandbox tests prove exact equality across batch settlement,
AP allocation, bank transaction, and GL cash movement in transaction/base currency.

### Q1–Q4 — Purchase-to-pay automation

#### Q1 — Line progress and invoice intake

**Duration:** Weeks 3–5

- [x] Define accumulated ordered, received, returned, invoiced, debit-noted, and paid
  quantity/value per PO line without duplicating authoritative document totals.
- [x] Add immutable structured/manual invoice intake, attachment hash, supplier document
  number, source metadata, and duplicate-candidate detection.
- [x] Make services/non-receipt POs explicit so they can use two-way matching.
- [x] Add database constraints for supplier document uniqueness where business rules allow;
  route uncertain duplicates to review instead of discarding them.

**Exit:** Partial receipt, return, partial invoice, debit note, and repeated invoice
intake produce deterministic open quantities/values and visible duplicate decisions.

#### Q2 — Matching engine and policy versions

**Duration:** Weeks 6–8

- [x] Add effective-dated matching policies by company with optional supplier/category
  overrides for quantity, price, tax, freight, and total tolerance.
- [x] Persist each two-way/three-way match run, policy version, compared source facts,
  result, reasons, and recommended action.
- [x] Lock source facts consistently during match/acceptance to prevent concurrent
  over-receipt or over-invoicing.
- [x] Produce `MATCHED`, `WITHIN_TOLERANCE`, `EXCEPTION`, or `DUPLICATE_REVIEW` without
  changing the PO, GRN, or AP invoice silently.

**Exit:** Table-driven and database tests cover exact, tolerance-edge, over-tolerance,
partial, service PO, return, tax/freight mismatch, duplicate, and concurrent cases.

#### Q3 — Exception workbench and controlled posting

**Duration:** Weeks 9–11

- [x] Add exception cases with type, severity, owner, SLA, reason, evidence, comments,
  approval/resolution, and immutable decision history.
- [x] Add an operations queue for mismatches, missing mappings, closed periods, supplier/
  payment holds, failed posting, and overdue receipts.
- [x] Allow draft AP creation after a successful match. Auto-post only when an enabled policy
  permits it and tax/account mappings, supplier state, period, duplicate, and approval
  checks all pass in the same controlled service flow.
- [x] Notify owners and escalate overdue exceptions without changing source state.

**Exit:** Every failure stops at a recoverable queue item; retry is idempotent and the
audit timeline explains the policy, source values, decision, actor, and journal result.

#### Q4 — End-to-end orchestration

**Duration:** Weeks 12–14

- [x] Emit durable lifecycle events for approval, PO, receipt/return, matching, AP posting,
  payment approval, settlement, and reconciliation.
- [x] Derive PR/PO closure from accumulated line state and define cancel/reopen rules.
- [x] Build one operational timeline/work queue spanning PR through reconciliation.
- [x] Connect approved AP obligations to P2 proposals and settled payments back to Q1 line
  progress and T4 forecast actuals.

**Exit:** Database-backed scenarios pass happy path, partial receipt/invoice/payment,
return/debit note, tolerance rejection, duplicate invoice, locked period, payment
failure, and reconciliation repair with no partial or duplicate accounting side effects.

### A1–A3 — Asset operations and capitalization

#### A1 — Locations, custody, and history

**Duration:** Weeks 3–5; parallel stream

- Add company-scoped branch/site/building/room hierarchy, effective-dated location and
  custodian assignments, operational condition, tag/barcode, serial, model, and
  manufacturer fields.
- Preserve historical display values or references so asset history remains readable
  after master-data changes.
- Keep financial status separate from operational condition.

**Exit:** Company-isolation, hierarchy-cycle, one-current-assignment, concurrent update,
and historical-read tests pass; current assets can be imported/reconciled safely.

#### A2 — Transfer, warranty, and maintenance operations

**Duration:** Weeks 6–10

The asset-ledger and capitalization boundaries in this section remain authoritative.
The broader operational maintenance module, including non-capitalized equipment, is
defined in the
[`CMMS`](cmms.md), [`QMS`](qms.md), and [`Document Management`](documents.md) guides.

- Add transfer request, approval, dispatch, receipt, cancellation, condition evidence,
  custody update, and timeline.
- Add warranty terms/documents/contacts/claims and configurable expiry alerts.
- Add preventive plans, meters where needed, scheduled/corrective work orders, labor/
  parts/vendor cost, downtime, completion, and next-due calculation.
- Prevent disposed assets from transfer/new work and require controlled handling for
  assets already under maintenance or transfer.
- Expense maintenance by default; any capitalization follows an explicit accounting
  adjustment policy, never a maintenance-form toggle.

**Exit:** Asset-custodian acceptance covers concurrent transfer, failed receipt,
warranty claim, schedule generation, overdue escalation, completion, depreciation
coexistence, and disposal restrictions.

#### A3 — Capitalization candidates and source lineage

**Duration:** Weeks 11–13; depends on Q2 source facts

- Add capitalization intent to eligible PO/AP lines and create a candidate only after
  the configured receipt/invoice conditions are met.
- Require asset acceptance to confirm count, unique tags, in-service date, location,
  custodian, useful life, category, exact cost allocation, and warranty.
- Activate through the existing fixed-asset service and link PO, GRN, AP invoice,
  journals, transfers, maintenance, claims, depreciation, and disposal.
- Handle multi-quantity and allocated freight/tax deterministically with exact decimals.

**Exit:** A multi-unit purchase produces the correct number of assets, exact aggregate
cost, unique tags, and complete source lineage; replay creates no duplicate candidate or
asset.

## Migration and query sequence

Use the next available migration number at implementation time; do not reserve numbers
on long-lived branches. Preserve this logical order so dependencies remain explicit:

1. Finance settings, permissions, and shared outbox/status types.
2. Bank connections, mappings, cursors, sync runs, inbox, and dedupe constraints.
3. Forecast scenarios/runs/buckets/source lines/adjustments.
4. Supplier bank verification, payment batches/items/revisions/attempts/events.
5. P2P intake, line progress, policy versions, match runs/results, and exceptions.
6. Asset locations/history, transfers, warranties/claims, maintenance plans/work orders.
7. Capitalization intent/candidates/source links and final cross-module indexes.

Each migration PR includes up/down behavior where safe, constraint tests, indexes for
worker claiming and work queues, sqlc query updates where used, and migration notes.
Destructive cleanup/backfill enforcement occurs only after the application can read both
old and new states and production backfill evidence is recorded.

## API and UI deliverables

Prefer SSR administration and work queues for operator flows; add JSON APIs only for
provider callbacks, controlled imports, or an explicitly supported public contract.

| Surface | Minimum release capability |
|---|---|
| Banking connections | Connect/map, health, sync history, retry/reconnect, disconnect |
| Reconciliation | Suggested matches, confirmation, duplicate/unmatched exceptions |
| Cash forecast | Scenario summary, daily/weekly drill-down, sources, freshness, overrides, export |
| Payment operations | Propose, revise, approve/reject, schedule, export/execute, settlement exceptions |
| P2P workbench | Intake, match result, evidence, owner/SLA, resolve/escalate, source timeline |
| Asset register | Location/custodian/condition, timeline, tag, source lineage |
| Asset operations | Transfer, warranty/claim, maintenance plan/work order, overdue queue |

All mutating forms require CSRF protection, RBAC, company-scoped object lookup, safe
error messages, audit events, and Post/Redirect/Get behavior. Sensitive bank/provider
values are masked by default.

## Pull-request and release gates

Every work-package PR must pass:

```bash
gofmt -w <changed Go files>
ODYSSEY_TEST_MODE=1 GOTENBERG_URL=http://127.0.0.1:0 go test <changed packages>
go vet <changed packages>
git diff --check
make docs-check
```

Before a release branch is accepted, also run `make test`, `make lint`, `make build`,
migration up/down checks, database-backed integration suites, and relevant HTTP/E2E
tests. Provider increments additionally require sanitized contract fixtures and sandbox
certification.

Required cross-cutting scenarios:

- Company A cannot read, map, approve, replay, export, settle, or update Company B data.
- Worker crash, concurrent claim, duplicate task/callback, retry, and manual replay create
  exactly one domain effect.
- Exact money and FX totals reconcile through AP, bank, and GL; no new finance path uses
  binary floating point.
- Locked/missing periods, mappings, approvals, or tax configuration stop before partial
  posting and leave a visible recovery item.
- Sensitive supplier bank data, provider credentials, payloads, and artifacts are
  encrypted/restricted and absent from logs and user-safe errors.
- Feature disable/disconnect stops new external work while preserving history and
  allowing controlled reconciliation of in-flight operations.

## Sixteen-week delivery schedule

The 16-week target assumes one finance/product owner and two delivery streams, each
with two Go engineers and one UI engineer, plus shared QA/automation and part-time
security/SRE. With a minimum team of two Go engineers and one UI engineer total,
execute the same dependency order in roughly 22–28 elapsed weeks
rather than overlapping the packages below. Provider contracting and sandbox access
start in Week 1 because external approval lead time is outside engineering control.

| Weeks | Treasury/payments stream | P2P/assets stream | Milestone |
|---|---|---|---|
| 1–2 | F0 decisions, ports, settings, permissions, outbox conventions | Asset policy/data discovery and import mapping | M0 architecture/control sign-off |
| 3–5 | T1 normalized import, T2 connection base, P1 supplier bank controls | Q1 line progress/intake and A1 locations/custody/history | M1 manual feeds and asset baseline certified |
| 6–8 | T2 provider sync and T3 forecast engine | Q2 matching and A2 transfer/warranty foundation | M2 feed/forecast shadow pilot |
| 9–10 | T4 forecast UI/actuals and P2 proposals/approval | Q3 exception workbench and A2 maintenance/work orders | M3 treasury and asset operations pilot |
| 11–12 | P3 bank export and settlement scaffolding | Q3 controlled posting, Q4 orchestration start, A3 candidates | M4 export-only payments and match suggestions |
| 13–14 | P4 settlement and cross-module reconciliation | Q4 full loop and A3 activation/source lineage | M5 full database E2E acceptance |
| 15–16 | Hardening, sandbox certification, performance, runbooks, staged flags | Import reconciliation, alerts, operator training | M6 limited production release |

If the provider is not ready, release T1/T3/T4, P1/P2/P3, Q1–Q4, and A1–A3 without
waiting. Keep T2 in manual-import mode and P4 in export/manual-confirmation mode.

## Initial two-week backlog

The first sprint should end with decisions and executable scaffolding, not feature UI:

1. Approve the bank ownership/payment settlement ADR and document the source-of-truth
   query for balances, statements, and reconciliation.
2. Approve the P2P tolerance/auto-post and asset capitalization policy skeletons.
3. Add finance automation settings and feature flags with company-scoped repositories.
4. Add the permission migration and update the role reference with incompatible-role
   guidance.
5. Define bank-feed/payment interfaces and fake adapters; write contract tests first.
6. Define a reusable finance outbox/attempt contract or document why an existing outbox
   is reused unchanged.
7. Create test builders for company, bank, AR/AP, PO/GRN, supplier, and asset scenarios.
8. Capture baseline metrics for import volume, reconciliation rate, forecast effort,
   invoice cycle time, payment timeliness, and asset-data completeness.
9. Secure provider sandbox access and sanitized test accounts/data.
10. Hold an end-of-sprint control review with treasury, AP, procurement, controller,
    asset custodian, security, and operations; unresolved policy decisions block only
    the affected downstream package.

## Program completion criteria

Before implementation begins, capture a company-level baseline and report these success
measures during each pilot:

- Bank-feed freshness, successful sync rate, and reconciliation suggestion/confirmed-
  match rates.
- Thirteen-week forecast completeness and weekly forecast accuracy by source type.
- Invoice match-exception rate, invoices processed without manual correction, and
  median receipt-to-post time.
- Payments made on time, proposal-to-settlement time, and payment failure/duplicate
  rate.
- Percentage of assets with current location, custodian, condition, and warranty data;
  preventive maintenance completed on time; and overdue work-order count.

The program can move from **Planned** to **Implemented** only when:

- Each released capability identifies its exact supported provider/mode; an export-only
  payment flow is not described as live execution.
- Two forecast cycles and at least one payment/P2P accounting close reconcile to source
  documents and GL without unexplained differences.
- Bank sync freshness, forecast completeness, matching/exception age, payment failure,
  outbox depth, warranty expiry, and maintenance SLA metrics have owners and alerts.
- Operator runbooks cover provider outage/disconnect, duplicate/ambiguous payment,
  forecast correction, match exception, payment stop, transfer failure, and maintenance
  rescheduling.
- Staging migration, rollback/recovery, company isolation, permission, security,
  performance, sandbox, and limited-production evidence is recorded.
- The module catalog and user documentation distinguish implemented automation from
  planned providers or policies.
