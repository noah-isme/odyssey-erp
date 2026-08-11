# Procurement and Logistics Depth Execution Plan

**Status:** Core sourcing, contract/variance controls, freight persistence, transport
execution, and supplier scorecard calculation are implemented in focused slices. The
atomic PO → freight → receipt → variance → scorecard → AP/payment orchestration and
some operational integrations remain.

## Completion status

- [x] RFQ shared foundation: company-scoped sourcing tables, exact-value bid and FX
  boundaries, RFQ/award permissions, audit events, and idempotent award-to-PO creation.
- [x] Strategic sourcing: RFQ issue email, buyer-entered bids, persisted weighted
  comparisons, split awards, shared approval, and linked draft POs.
- [x] Vendor intelligence core: supplier contracts, applicable-contract selection, PO
  variance controls, price observations, weighted scorecards, evidence sample sizes,
  draft persistence, and publication locking.
- [x] Transport and freight core: company-scoped rate-card/charge/landed-cost/audit
  persistence, guarded charge status transitions, shipment tracking, active-trip
  metrics, and fleet utilization.
- [ ] End-to-end orchestration: receipt-to-landed-cost allocation, real AP/GL posting,
  monthly scorecard scheduling, expiry notifications, and operational SSR workbenches.

## Current implementation slice

The current implementation closes the persistence and calculation gaps that blocked the
business-critical path:

- Freight repository methods now use the existing SQL bindings for rate cards,
  surcharges, charges, landed costs, cost centers, and audit records. Optional filters
  use the empty/zero sentinels passed by the repository, and charge transitions are
  guarded against moving backward from `INVOICED` or `PAID`.
- Logistics tracking is derived from shipment, trip, and stop records. Active trips
  include both `DISPATCHED` and `IN_PROGRESS`, and fleet utilization counts available,
  in-use, and maintenance vehicles plus linked active shipments.
- Scorecard evidence uses the current PO, posted-GRN, and confirmed supplier-return
  schema. Component ratios and the weighted overall score use exact decimal arithmetic;
  evidence-free categories are excluded from the denominator, sample sizes are stored,
  and published scorecards cannot be edited.
- Contract and variance workbench reads are company-scoped. PO price variance is
  calculated as `(PO price - contract price) / contract price × 100`, rather than using
  a comparison result as a percentage.

The remaining integration work is deliberately explicit: a transaction or outbox flow
must connect approved POs to freight charges, receipts, landed-cost allocation, AP
invoices, payment, and real journal entries. Freight GL posting now exposes an injectable
accounting boundary with exact balanced lines, deterministic source identity, and
idempotency checks. Application wiring and the end-to-end orchestration remain open, and
the boundary must call the accounting service and verify journal identity and amount
before it is treated as production posting.

## Summary

Extend Odyssey's existing PR → PO → GRN → AP and delivery-order foundations with:

- Strategic sourcing: RFQs, bid comparison, awards, supplier contracts, price history,
  and performance scorecards.
- Transportation management: carriers, owned fleet, shipments, trips, routes, freight
  reconciliation, and distribution planning.
- Distribution demand from customer delivery orders and inter-warehouse transfers.
- Rules-based planning with deterministic v1 route sequencing; live traffic, external
  route optimization, and carrier APIs remain later integrations.
- Inbound freight capitalized into inventory and outbound freight posted as delivery
  expense.

Deliver this through additive migrations and gated milestones so each capability can
be released independently.

## Implementation changes

### 1. Shared foundations

- Add company-scoped, auditable tables using exact PostgreSQL `NUMERIC` values and
  Odyssey's exact money/FX types. Do not introduce new `float64` monetary boundaries.
- Introduce separate permission families:
  - `procurement.rfq.*`, `procurement.contract.*`, and
    `procurement.supplier_rating.*`.
  - `logistics.carrier.*`, `logistics.fleet.*`, `logistics.plan.*`,
    `logistics.dispatch.*`, and `logistics.freight.*`.
- Seed permissions only to administrator roles; other role assignments remain
  explicit.
- Use the shared approval engine for RFQ awards, contract activation, PO
  contract-price overrides, and freight variances.
- Record lifecycle changes, scoring evidence, overrides, dispatch events, and
  accounting actions in the existing audit system.
- Add idempotency and optimistic/concurrency guards to award, PO generation, dispatch,
  receipt, freight approval, and landed-cost posting.

### 2. RFQ, bids, and awards

- Add RFQ headers, lines, invited suppliers, bid headers/lines, comparison snapshots,
  and awards.
- Use these lifecycles:
  - RFQ: `DRAFT → ISSUED → CLOSED → AWARDED`, with `CANCELLED`.
  - Bid: `DRAFT → SUBMITTED`, with `WITHDRAWN` and `DISQUALIFIED`.
  - Award: `DRAFT → APPROVAL → APPROVED`, with `REJECTED`.
- Create RFQs from submitted PR lines or manually. Issuing freezes the requested
  quantities, commercial terms, response deadline, and invited suppliers.
- Send RFQs through the existing Asynq/SMTP path. Buyers enter received bids and
  retain source references such as email or message identifiers. Supplier portal and
  binary document storage are excluded from v1.
- Capture per-line quantity, unit price, currency, taxes, freight, MOQ, lead time,
  validity, payment terms, and notes.
- Compare bids using an RFQ-configurable weighting that totals 100. Default to price
  50%, lead time 20%, payment/commercial terms 10%, and published supplier rating 20%.
- Normalize comparisons to company base currency using a dated FX snapshot while
  preserving original bid currency and amounts.
- Permit whole-RFQ or line-level split awards. An approved award creates one draft PO
  per winning supplier, links every PO line to its RFQ/bid/award source, and closes
  only fully awarded quantities.

### 3. Supplier contracts, ratings, and price intelligence

- Add versioned supplier contracts with effective dates, currency, payment/incoterms,
  renewal/expiry notice, product price lines, quantity tiers, lead-time commitments,
  and service-level targets.
- Use `DRAFT → APPROVAL → ACTIVE → EXPIRED/TERMINATED` as the contract lifecycle.
  Approved versions are immutable; amendments create new versions.
- PO creation selects an applicable active contract by supplier, product, effective
  date, and quantity tier. Contract terms default onto the PO.
- Non-contract purchases, expired contracts, and price/term variances remain possible
  but generate an exception requiring a reason and approval before the PO can be
  approved.
- Maintain immutable supplier/product price observations sourced from submitted bids,
  approved awards, active contract versions, and approved PO lines. Provide
  native-currency and base-currency trends with source drill-down.
- Publish periodic supplier scorecards using:
  - Delivery/OTIF: 35%.
  - Quality, based on accepted receipts and supplier returns: 25%.
  - Price adherence to award or contract: 20%.
  - RFQ responsiveness: 10%.
  - Reviewer assessment: 10%.
- Renormalize automatic categories when there is no eligible evidence, show sample
  sizes, and require a reviewer to publish each scorecard. Published scores are
  versioned and cannot be edited.
- Add procurement pages under `/procurement/rfqs`, `/procurement/contracts`,
  `/procurement/prices`, and `/procurement/suppliers/{id}/performance`.

The contract list, pending-variance list, applicable-contract lookup, exact PO price
variance, and scorecard calculation/persistence paths are implemented. Expiry
notifications and the monthly background calculation coordinator remain follow-up work.

### 4. Logistics masters and execution

- `internal/logistics` now owns shipment transport execution while `internal/delivery`
  retains sales delivery-order lifecycle. `internal/distribution` owns planning,
  load consolidation, route records, and transfer-order workflow.
- Add:
  - Carriers linked to supplier records for AP.
  - Carrier services, effective-dated lane/zone rate cards, capacity, cutoff, and
    service-level definitions.
  - Vehicles, vehicle classes, capacity, availability, registration/insurance expiry,
    and odometer data.
  - Drivers linked to HR employees where available.
  - Shipment, trip, stop, tracking-event, proof-of-delivery reference, and
    resource-assignment records.
- A trip uses either an owned vehicle/driver or a carrier/service, never both. A
  shipment can consolidate one or more delivery orders or transfer orders with
  compatible origin, destination, dates, and handling constraints.
- Preserve existing delivery-order fields for compatibility. Add nullable shipment and
  trip links and treat legacy free-text driver, vehicle, and tracking values as manual
  historical data.
- Route shipment actions through the logistics service. Distribution delegates the
  final outbound inventory movement to the inventory service so its existing
  idempotency and inventory-to-GL hooks run exactly once.
- Keep carrier execution manual in v1. Provider quote, booking, label, and tracking
  adapters remain governed by the existing external-integrations plan.

### 5. Distribution and route planning

- Add planning horizons that select eligible customer delivery orders and approved
  inter-warehouse transfer demand.
- Add product logistics attributes for gross weight, volume, handling unit, and
  stackability, plus immutable destination/address and delivery-window snapshots.
- Build suggested loads by origin warehouse, requested date, delivery zone, capacity,
  service compatibility, and time window.
- Provide manual stop sequencing with rules-based validation for:
  - Vehicle or carrier weight and volume capacity.
  - Driver, vehicle, and carrier availability.
  - Delivery windows, carrier cutoffs, and duplicate assignments.
  - Warehouse release and sufficiently picked or packed delivery orders.
- Use saved zones, lanes, distance, and duration estimates for v1 costing. The current
  route optimizer may use a deterministic haversine estimate when stop coordinates are
  complete; do not add geocoding, live traffic, or provider routing in this slice.
- Replace the immediate-only inventory transfer workflow with transfer orders:
  - `DRAFT → APPROVED → DISPATCHED → RECEIVED`, with `CANCELLED`.
  - Dispatch removes source availability and records in-transit custody.
  - Receipt posts destination inventory using the original cost basis.
  - The existing immediate-transfer form completes dispatch and receipt transactionally
    for backward compatibility.
- Add planner and dispatcher workbenches under `/logistics/distribution-plans`,
  `/logistics/shipments`, and `/logistics/trips`.

Shipment tracking, trip completion timestamps, active-trip listing, fleet utilization,
and deterministic v1 route resequencing are implemented from persisted transport
records. Inventory movement, related-order closure, audit/notification side effects,
and provider-backed routing remain open integration work.

### 6. Freight planning, reconciliation, and accounting

- Calculate planned freight from versioned rate-card snapshots using flat, distance,
  weight, volume, stop, minimum-charge, and surcharge components.
- Record quoted, approved, and actual freight separately. Default reconciliation
  tolerance is 5%; amounts outside tolerance enter an approval-backed exception queue.
- Link approved carrier bills to the carrier's supplier and create a draft AP invoice
  through the existing AP service. Duplicate invoice/reference detection remains
  enforced.
- For inbound freight:
  - Post the carrier invoice to a freight-clearing account.
  - Allocate to GRN lines by weight, falling back to receipt value and then quantity
    when required data is unavailable.
  - Capitalize the portion still on hand into inventory and post the already-consumed
    portion to COGS variance.
  - Clear the freight-clearing balance exactly once.
- Post outbound freight to a configurable delivery/freight expense account and AP.
- Store allocation basis, FX snapshot, calculation inputs, accounting references, and
  override reasons for audit.
- Add freight quote, bill, allocation, variance, and lane-cost views under
  `/logistics/freight`.

Rate-card selection, exact freight calculation, charge/landed-cost persistence, audit
records, invoice/payment status transitions, and cost-center attributes are implemented.
Carrier-bill-to-AP creation, receipt-line landed-cost allocation, clearing/COGS journals,
application-level GL wiring, and real reconciliation still need to be connected to the
accounting and inventory services.

## Delivery sequence

1. **Foundation:** migrations, exact-value types, permissions, lifecycle primitives,
   and tenant-safe repositories.
2. **Strategic sourcing:** RFQ, bid entry/email, comparison, approval, split award,
   and PO generation.
3. **Vendor intelligence:** contracts, PO variance controls, price history, and
   scorecards are implemented; expiry notifications and the monthly scheduler remain.
4. **Transport execution:** carrier/fleet masters, shipments, trips, stops, tracking,
   and active-trip/fleet metrics are implemented; delivery/WMS side effects remain.
5. **Distribution planning:** planning horizons, load building, shipment linkage,
  dispatch/delivery, manual routes, deterministic v1 route optimization, transfer-order
  lifecycle, and capacity/schedule validation are implemented in the core path; transfer
  inventory posting remains follow-up work.
6. **Freight finance:** rate cards, estimates, charge/landed-cost persistence, and
   invoice/payment status transitions are implemented; bill reconciliation, AP,
   landed-cost allocation, and real GL integration remain.
7. **Hardening and rollout:** reports, exports, observability, documentation, seeded
   acceptance dataset, and operational training.

Each milestone must pass its acceptance suite before exposing its navigation or
permissions outside administrators.

## Test plan

- Migration up/down tests, constraints, indexes, and company-isolation tests for every
  new table and query.
- RFQ lifecycle, late/withdrawn bids, FX normalization, tied bids, partial/split
  awards, concurrent awards, and exactly-once PO creation.
- Contract effective-date and tier selection, amendments, expiration, non-contract
  buying, and approved override behavior.
- Price-history immutability and scorecard formulas for partial receipts, late
  deliveries, returns, missing evidence, manual review, and publication locking.
- Scorecard evidence against posted GRNs, confirmed supplier returns, PO expected dates,
  price-variance math, sample-size persistence, and weighted-score renormalization.
- Carrier/fleet availability, mutually exclusive resource assignment,
  capacity/time-window failures, duplicate shipment assignment, cancellation, and
  redispatch.
- Multi-order load planning, manual stop resequencing, transfer dispatch/receipt,
  in-transit reporting, and exactly-once inventory movements.
- Freight formula and FX tests, tolerance exceptions, duplicate carrier bills,
  allocation fallbacks, partial stock consumption, balanced
  clearing/COGS/inventory journals, and outbound expense posting.
- Freight repository CRUD, optional-filter behavior, exact charge calculations, guarded
  invoice/payment transitions, shipment tracking, active-trip aggregation, and fleet
  utilization.
- SSR route, RBAC, CSRF, validation, template-render, pagination/filter, and audit-event
  tests.
- Run focused package suites throughout, then run:

  ```bash
  ODYSSEY_TEST_MODE=1 GOTENBERG_URL=http://127.0.0.1:0 go test ./...
  go vet ./...
  make lint
  make docs-check
  ```

## Assumptions and defaults

- V1 supports owned fleet and third-party carriers through one workflow.
- Routing is rules-based with manual sequencing; no maps, GPS telemetry, live traffic,
  or optimizer.
- Bid intake is buyer-entered with outbound email, not a supplier portal.
- Supplier ratings combine transaction-derived evidence with a controlled reviewer
  assessment.
- Contracts warn and require approved overrides rather than hard-blocking procurement.
- Distribution planning covers outbound customer deliveries and inter-warehouse
  transfers, not supplier pickup scheduling.
- Inbound freight becomes landed inventory cost; outbound freight is period expense.
- Fleet maintenance/work orders, carrier API integrations, customer freight billing,
  and generic document management remain out of scope.
