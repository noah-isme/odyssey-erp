# Procurement and Logistics Depth Execution Plan

**Status:** Partially implemented. The RFQ, bid, comparison, award, approval, and
draft-PO foundation is implemented; supplier contracts/ratings and logistics milestones
remain planned.

## Completion status

- [x] RFQ shared foundation: company-scoped sourcing tables, exact-value bid and FX
  boundaries, RFQ/award permissions, audit events, and idempotent award-to-PO creation.
- [x] Strategic sourcing: RFQ issue email, buyer-entered bids, persisted weighted
  comparisons, split awards, shared approval, and linked draft POs.
- [ ] Vendor intelligence: supplier contracts, PO variance controls, price history,
  ratings, and expiry notifications.
- [ ] Transportation execution, distribution planning, freight accounting, and the
  operational SSR workbenches.

## Summary

Extend Odyssey's existing PR → PO → GRN → AP and delivery-order foundations with:

- Strategic sourcing: RFQs, bid comparison, awards, supplier contracts, price history,
  and performance scorecards.
- Transportation management: carriers, owned fleet, shipments, trips, routes, freight
  reconciliation, and distribution planning.
- Distribution demand from customer delivery orders and inter-warehouse transfers.
- Rules-based planning with manual route sequencing; external route optimization and
  carrier APIs remain later integrations.
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

### 4. Logistics masters and execution

- Create a new internal/logistics module while retaining `internal/delivery` as owner
  of delivery-order lifecycle and inventory effects. The logistics path is proposed
  and does not exist yet.
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
- Route shipment actions through the delivery service so confirmation, shipping,
  completion, cancellation, WMS state, and inventory posting occur exactly once.
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
- Use saved zones, lanes, distance, and duration estimates for v1 costing. Do not add
  geocoding, live traffic, or mathematical route optimization.
- Replace the immediate-only inventory transfer workflow with transfer orders:
  - `DRAFT → APPROVED → DISPATCHED → RECEIVED`, with `CANCELLED`.
  - Dispatch removes source availability and records in-transit custody.
  - Receipt posts destination inventory using the original cost basis.
  - The existing immediate-transfer form completes dispatch and receipt transactionally
    for backward compatibility.
- Add planner and dispatcher workbenches under `/logistics/distribution-plans`,
  `/logistics/shipments`, and `/logistics/trips`.

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

## Delivery sequence

1. **Foundation:** migrations, exact-value types, permissions, lifecycle primitives,
   and tenant-safe repositories.
2. **Strategic sourcing:** RFQ, bid entry/email, comparison, approval, split award,
   and PO generation.
3. **Vendor intelligence:** contracts, PO variance controls, price history,
   scorecards, and expiry notifications.
4. **Transport execution:** carrier/fleet masters, shipments, trips, stops, and
   delivery/WMS integration.
5. **Distribution planning:** transfer-order lifecycle, planning horizon, load
   building, and capacity/schedule validation.
6. **Freight finance:** rate cards, estimates, bills, reconciliation, AP, landed cost,
   and GL integration.
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
- Carrier/fleet availability, mutually exclusive resource assignment,
  capacity/time-window failures, duplicate shipment assignment, cancellation, and
  redispatch.
- Multi-order load planning, manual stop resequencing, transfer dispatch/receipt,
  in-transit reporting, and exactly-once inventory movements.
- Freight formula and FX tests, tolerance exceptions, duplicate carrier bills,
  allocation fallbacks, partial stock consumption, balanced
  clearing/COGS/inventory journals, and outbound expense posting.
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
