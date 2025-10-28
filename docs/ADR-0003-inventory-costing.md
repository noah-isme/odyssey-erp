# ADR-0003 – Inventory Costing Strategy

## Status
Accepted – Phase 3

## Context
Inventory valuation underpins procurement, fulfillment, and financial reporting. The ERP must support transactional stock movements, integrate with procurement receipts, and expose reliable running balances for reporting and AP integration. We selected Average Moving Cost (AVCO) for the initial release to prioritise simplicity while ensuring deterministic costing in multi-user scenarios.

## Decision
* Use Average Moving Cost across all warehouses. Each inbound movement recalculates `avg_cost` by weighting existing balance with incoming quantity.
* Enforce strict non-negative balances by default. Transfers and adjustments run inside repeatable-read transactions to eliminate race conditions.
* Store detailed movements in `inventory_tx`/`inventory_tx_lines` and running balances in `inventory_balances` for fast lookup. A lightweight `inventory_cards` table captures pre-aggregated ledger rows to power reports.
* Defer FIFO/LIFO support to a future ADR; design keeps transaction headers/lines generic to allow alternative costing engines later.

## Consequences
* Integration with procurement simply calls the inventory service with inbound payloads; no costing logic leaks into procurement.
* Average cost recalculation happens inside the transaction pipeline ensuring atomicity but requires consistent unit cost inputs for adjustments.
* Nightly revaluation job (`jobs/inventory_reval.go`) validates balances and flags inconsistencies for operators.
* Moving to FIFO later will require new tables for layers but the existing schema can coexist thanks to explicit transaction headers.
