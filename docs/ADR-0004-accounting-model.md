# ADR-0004 – Accounting Model & Ledger Architecture

## Status
Draft – Phase 4

## Context
The accounting phase introduces double-entry bookkeeping, statutory financial reporting, and automated integration with procurem
ent. The ledger must remain consistent with upstream modules (inventory, procurement, accounts payable) and provide auditable tr
aces from source transactions to journal entries and back. We also need to support period control (open/close/lock) while allowin
g controlled overrides, provide performant queries for Trial Balance (TB), Profit & Loss (P&L), and Balance Sheet (BS), and prod
uce PDF outputs via Gotenberg.

Constraints include:
* Double-entry journals must balance (total debit = total credit) and persist atomically (header, lines, source link).
* Period locking rules disallow posting to closed or locked periods without elevated permission.
* Reporting must aggregate rapidly over large datasets while supporting drill-down to journal lines.
* Prior phases (inventory, procurement, AP) already emit auditable transactions; the accounting service may not duplicate business
  logic – it consumes normalized events.

## Decision
* **Chart of Accounts (CoA)** – Use a hierarchical structure with enforced unique codes (alphanumeric with dot/dash separators, m
  ax 20 chars). Accounts store type (ASSET, LIABILITY, EQUITY, REVENUE, EXPENSE), `parent_id` for roll-up, and activity flag. This
  schema mirrors regulatory expectations and enables tree traversal for reporting.
* **Accounting Periods** – Model fiscal periods with explicit `status` ENUM (OPEN, CLOSED, LOCKED). Only OPEN periods accept new po
  stings. Closed periods freeze new entries but allow void/reversal via guarded service. Locked periods require `finance.override
  .lock`. Closing a period records timestamps and actor for audit.
* **Journal Entries** – Represented by header (`journal_entries`) and line (`journal_lines`). Header stores source module + ID, me
  mo, posting metadata, and status (POSTED, VOID). Lines enforce either debit or credit per row (non-negative numeric). Domain ser
  vice validates balancing and period checks before commit.
* **Source Linkage** – Maintain `source_links` table with unique `(module, ref_id)` constraint to guarantee idempotent posting and
  allow traceability from journals to originating document and vice versa.
* **Account Mapping** – `account_mappings` table provides configurable mapping keys (e.g., `grn.inventory`, `ap.invoice.tax`). Map
  ping is required for automated postings. Integrations resolve mappings before creating journal payloads.
* **Reporting Structures** – Materialized view `gl_balances` holds aggregated opening/debit/credit/closing per account/period to s
  peed up TB. Additional helper tables `pl_structure` and `bs_structure` capture report ordering and aggregation rules referencing
  accounts.
* **Audit Trail** – Reuse central `audit_logs` table, capturing posting, void, reversal, period close/lock, and override events wit
  h metadata (source IDs, actor IDs, diffs). Services emit logs within the same transaction to guarantee alignment.
* **Service Boundaries** – Accounting domain package exposes posting, void, reverse, and query operations. Integrations adapt sour
  ce modules to ledger-specific DTOs. Shared period/lock helpers centralize RBAC checks.
* **Performance & Jobs** – Nightly Asynq jobs refresh materialized views and run integrity checks (balance equality, orphan links).
  Reporting endpoints always read the latest MV snapshot but allow manual refresh for finance operators.

## Consequences
* Introducing hierarchical CoA increases complexity of CRUD UI but enables regulatory reporting, consolidations, and alignment wi
th industry practice.
* Period management requires RBAC changes and more rigorous operator workflows but prevents back-dated edits without approval.
* Source linkage prevents duplicate postings and simplifies debugging; however, all integrations must handle 409 conflicts and pr
ovide clear remediation steps.
* Materialized views speed up financial reports but introduce refresh mechanics; jobs and manual refresh endpoints must be relia
ble.
* Audit logging expands storage requirements, but consistent metadata ensures forensic capabilities and compliance with SOX-like
 controls.
* Future modules (AR, GL adjustments) can reuse the same posting pipeline, preserving architectural consistency.
