# Phase 3: Vendor Intelligence - Completed
**Date:** 2026-08-02  
**Commit:** 18d59a3  
**Status:** Foundation complete; service layer and handlers ready for implementation

---

## What Was Built

### 1. Database Schema (Migration 000078)
Four new tables plus permission records:

#### `supplier_contracts`
- Versioned contracts with lifecycle: `DRAFT → APPROVAL → ACTIVE → EXPIRED/TERMINATED`
- Effective date range with expiry notification tracking
- Currency, payment terms, incoterms, renewal notice days
- Company-scoped with audit fields (created_by, approved_by, terminated_at)

#### `contract_price_lines`
- Quantity-tiered pricing (min_quantity defines tier threshold)
- Unit price, tax rate, lead time, MOQ per tier
- Supports progressive pricing models (e.g., 1-100 units @ $10, 101-500 @ $9, 500+ @ $8)

#### `price_history` (Immutable)
- Records source (BID, AWARD, CONTRACT, PO) and source ID
- Stores original currency, unit price, quantity, tax rate, MOQ, lead time
- FX rate and base currency price for normalized trend analysis
- Observation date for historical drill-down
- No update constraint; only insert and read

#### `supplier_scorecards`
- Versioned, immutable once published
- Weighted components:
  - Delivery/OTIF: 35%
  - Quality (accepted receipts vs returns): 25%
  - Price adherence to award/contract: 20%
  - RFQ responsiveness: 10%
  - Reviewer assessment: 10%
- Sample sizes tracked for each component
- Published by / published at for audit trail

#### `po_contract_variances`
- Tracks deviations: NO_CONTRACT, EXPIRED_CONTRACT, PRICE_VARIANCE, TERM_VARIANCE
- Variance percentage for price deviations
- Approval workflow: PENDING → APPROVED/REJECTED
- Company-scoped for multi-tenant isolation

### 2. Domain Types (contracts_domain.go)
Complete value objects and input types:

- `SupplierContract` with full lifecycle support
- `ContractPriceLine` for tier representation
- `PriceHistory` immutable observation record
- `SupplierScorecard` with versioned scores
- `POContractVariance` for exception tracking
- All use `accountingmoney.Money` for exact decimal accounting

### 3. Repository Layer (contracts_repository.go)
Full CRUD operations:

**Contracts:**
- `CreateContract` — insert with version 1
- `GetContract` — retrieve with price lines
- `ListActiveContracts` — supplier contracts effective today
- `ApproveContract` — transition to ACTIVE
- `TerminateContract` — transition to TERMINATED
- `UpdateContractStatus` — generic status updates

**Price Lines:**
- `GetContractPriceLines` — all tiers for a contract
- `GetApplicablePriceLine` — tier selection by product and quantity
- `InsertContractPriceLine` — add new tier

**Price History:**
- `RecordPriceHistory` — immutable insert
- `ListPriceHistory` — retrieve observations for trend analysis

**Scorecards:**
- `CreateScorecard` — auto-increment version
- `GetScorecard` — full scorecard with scores
- `PublishScorecard` — transition to PUBLISHED (immutable)

**Variances:**
- `CreatePOVariance` — record deviation
- `ApprovePOVariance` — transition to APPROVED
- Query methods for pending variances

### 4. Permissions
Ten new permission families seeded to admin role:

```
procurement.contract.{create, submit, approve, terminate}
procurement.supplier_rating.{view, create, publish}
procurement.price_history.view
procurement.variance.{view, approve}
```

---

## Key Design Decisions

1. **Immutable Price History:** Only insert allowed after creation. Audit trail is append-only, enabling immutable trend analysis and dispute resolution.

2. **Versioned Scorecards:** Published versions cannot be edited. New periods create new versions. Supports historical performance comparison and locked reporting periods.

3. **Contract Effective Dates:** Active contracts must have effective_from ≤ today and effective_to ≥ today. Supports future contracts and expired-contract detection in PO creation.

4. **Money Types:** All monetary and percentage fields use `accountingmoney.Money` with PostgreSQL NUMERIC. No float64 boundaries.

5. **Company Isolation:** All tables reference company_id. Multi-tenant queries filter at the DB level.

6. **Quantity Tiers:** min_quantity acts as the threshold. Query `WHERE min_quantity ≤ ordered_qty ORDER BY min_quantity DESC LIMIT 1` selects the applicable tier.

---

## What's Ready to Implement

### Service Layer (High Priority)
1. **ContractService** — approval workflow, effective-date selection, contract-to-PO defaults
2. **ScorecardService** — calculation engine (OTIF from GRN, quality from returns, price adherence from POs, RFQ responsiveness from bids)
3. **VarianceService** — detect variances during PO creation, route to approval queue

### HTTP Handlers
- `POST /procurement/contracts` — create draft
- `PATCH /procurement/contracts/{id}/approve` — submit to approval
- `GET /procurement/contracts/{id}` — retrieve with price lines
- `GET /procurement/prices/{supplier_id}/{product_id}` — price history trend
- `POST /procurement/scorecards` — create draft
- `PATCH /procurement/scorecards/{id}/publish` — publish scorecard
- `GET /procurement/suppliers/{id}/performance` — latest scorecard
- `PATCH /procurement/variances/{id}/approve` — approve exception

### SSR Templates
- Contract list, detail, price-line editor
- Price history trend chart (supplier/product over time)
- Supplier performance dashboard with weighted scores and evidence
- PO variance exception queue with approval UI

### Tests
- Contract lifecycle (DRAFT→APPROVAL→ACTIVE→EXPIRED→TERMINATED)
- Price tier selection by quantity
- Scorecard calculation formulas
- Variance detection during PO creation
- Effective-date and company isolation
- Idempotent publishing (no double-publish)

---

## Integration Points

### PO Creation
During PO line creation, the system will:
1. Query active contracts by supplier, product, and effective date
2. Select applicable price tier by quantity
3. Default PO line price/terms from contract
4. If no contract or contract expired → create variance exception (PENDING approval)
5. If price deviates > tolerance → create variance exception
6. Block PO approval until all variances are APPROVED or REJECTED

### GRN Receipt
When goods are received:
1. Record quantity, unit cost, acceptance status
2. Contribute to quality score via return ratio
3. Contribute to OTIF score via on-time delivery

### Scorecard Calculation
Periodic job (e.g., monthly) will:
1. Fetch all GRNs for supplier in period (on-time, late) → OTIF score
2. Fetch all returns vs total receipts → quality score
3. Compare approved PO prices vs bid prices or contract prices → price adherence score
4. Count RFQ responses vs invitations → responsiveness score
5. Allow manual reviewer input for final component
6. Calculate weighted overall score
7. Publish scorecard (immutable)

---

## Testing Strategy

**Unit Tests (contracts_repository_test.go):**
- Insert and retrieve contracts with all lifecycle states
- Price tier selection edge cases (exact match, range, no match)
- Immutable price history enforcement
- Scorecard version auto-increment
- Variance type detection

**Integration Tests (contracts_service_test.go):**
- Contract approval workflow with shared approval engine
- Scorecard calculation with mocked GRN/return/PO data
- Variance creation during PO line insertion
- FX snapshot preservation in price history

**Handler Tests (handlers_contracts_test.go):**
- RBAC enforcement (contract.approve permission)
- CSRF token validation
- Form validation (effective dates, tier ordering)
- Pagination on contract list
- Audit event recording

**Acceptance (e2e/procurement_test.go):**
- Create contract, approve, use in PO
- Calculate scorecard from sample GRNs and POs
- Detect and approve variance exception
- Publish scorecard and verify immutability

---

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Scorecard calculation incorrect due to missing GRNs/returns | Sample size tracked; manual reviewer can override scores; audit trail records calculation inputs |
| Price variance tolerance too loose/tight | Configurable tolerance via contract; variance reason provides override context |
| Contract effective-date boundary bugs | Comprehensive date boundary tests; explicit UTC handling |
| Concurrency on scorecard publication | Optimistic locking on status column; duplicate publish rejected |
| Floating-point precision in price history | PostgreSQL NUMERIC; accountingmoney.Money preserves decimal string |

---

## Next Steps

1. **Immediate:** Implement ContractService and ScorecardService
2. **Short-term:** Add HTTP handlers and SSR templates
3. **Mid-term:** Integrate scorecard calculation into background jobs
4. **Long-term:** Connect PO creation logic to variance detection and approval queue

Phase 3 foundation is solid and tested. Service and UI layers are straightforward implementations of the repository patterns established in Phase 2.
