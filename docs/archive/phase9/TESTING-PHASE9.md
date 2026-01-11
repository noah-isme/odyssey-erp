# Testing Plan – Phase 9 (Sales & AR)

## Overview

Dokumen ini menjabarkan strategi testing komprehensif untuk Phase 9 yang mencakup Sales Quotation, Sales Order, Delivery Order, AR Invoice, dan AR Payment. Testing dilakukan secara bertahap per cycle dengan fokus pada unit tests, integration tests, E2E tests, dan manual QA.

---

## Testing Strategy

### Test Pyramid

```
        /\
       /  \        E2E Tests (10%)
      /----\       - Critical user journeys
     /      \      - Full stack integration
    /--------\     Integration Tests (30%)
   /          \    - Repository + DB
  /------------\   - Service + dependencies
 /--------------\  Unit Tests (60%)
/                \ - Business logic
------------------  - Validators
                   - Calculations
```

### Coverage Targets

- **Unit Tests**: ≥ 70% code coverage per module
- **Integration Tests**: All critical flows covered
- **E2E Tests**: Top 5 user journeys covered
- **Manual QA**: 100% of UI pages tested

---

## Cycle 9.1 – Quotation & Sales Order Testing

### Unit Tests

#### `internal/sales/service_test.go`

**Quotation Tests**:
- [x] `TestCreateQuotation_Success` – valid quotation creation
- [x] `TestCreateQuotation_InvalidCustomer` – non-existent customer ID
- [x] `TestCreateQuotation_InvalidDates` – quote_date > valid_until
- [x] `TestCreateQuotation_EmptyLines` – quotation with no line items
- [x] `TestCreateQuotation_NegativeQuantity` – line with qty < 0
- [x] `TestCreateQuotation_InvalidDiscount` – discount > 100%
- [x] `TestCalculateTotals` – subtotal, tax, total calculations
- [x] `TestSubmitQuotation` – DRAFT → SUBMITTED
- [x] `TestSubmitQuotation_AlreadySubmitted` – idempotency check
- [x] `TestApproveQuotation_Success` – SUBMITTED → APPROVED
- [x] `TestApproveQuotation_NotSubmitted` – cannot approve DRAFT
- [x] `TestApproveQuotation_Unauthorized` – user without approve permission
- [x] `TestRejectQuotation` – SUBMITTED → REJECTED with reason
- [x] `TestConvertToSO_Success` – APPROVED quotation → SO DRAFT
- [x] `TestConvertToSO_NotApproved` – cannot convert DRAFT/REJECTED
- [x] `TestConvertToSO_AlreadyConverted` – prevent duplicate conversion

**Sales Order Tests**:
- [x] `TestCreateSalesOrder_Success` – valid SO creation
- [x] `TestCreateSalesOrder_FromQuotation` – SO inherits quotation lines
- [x] `TestConfirmSalesOrder_Success` – DRAFT → CONFIRMED
- [x] `TestConfirmSalesOrder_InsufficientStock` – warning (soft check)
- [x] `TestConfirmSalesOrder_AlreadyConfirmed` – idempotency
- [x] `TestCancelSalesOrder` – CONFIRMED → CANCELLED with reason
- [x] `TestCancelSalesOrder_PartiallyDelivered` – cannot cancel if delivered
- [x] `TestUpdateSalesOrder_DraftOnly` – can only edit DRAFT
- [x] `TestDeleteSalesOrder_DraftOnly` – can only delete DRAFT

#### `internal/sales/validator_test.go`

- [x] `TestValidateQuotationInput` – all field validations
- [x] `TestValidateSalesOrderInput` – all field validations
- [x] `TestValidateLineItems` – quantity, price, discount ranges
- [x] `TestSanitizeInput` – XSS prevention in notes/description

### Integration Tests

#### `internal/sales/repository_test.go`

- [x] `TestCreateQuotation_DB` – insert & retrieve from DB
- [x] `TestListQuotations_Filter` – filter by status, customer, date range
- [x] `TestListQuotations_Pagination` – page size & offset
- [x] `TestGetQuotationByID` – retrieve with line items (JOIN)
- [x] `TestUpdateQuotationStatus` – status transitions
- [x] `TestDeleteQuotation_Soft` – soft delete preserves audit trail
- [x] `TestCreateSalesOrder_WithLines` – transaction integrity
- [x] `TestListSalesOrders_ComplexFilter` – multiple filters & sort
- [x] `TestGetSOWithDeliveryStatus` – JOIN with delivery_orders

**Setup**: Use Docker testcontainer PostgreSQL with test fixtures.

### HTTP Handler Tests

#### `internal/sales/http/handler_test.go`

**Quotation Endpoints**:
- [x] `TestGetQuotationList_200` – authorized user
- [x] `TestGetQuotationList_403` – unauthorized user
- [x] `TestGetQuotationDetail_200` – valid ID
- [x] `TestGetQuotationDetail_404` – non-existent ID
- [x] `TestCreateQuotation_302` – redirect after success (PRG)
- [x] `TestCreateQuotation_422` – validation errors
- [x] `TestCreateQuotation_403` – no create permission
- [x] `TestApproveQuotation_200` – with approve permission
- [x] `TestApproveQuotation_403` – without approve permission
- [x] `TestConvertToSO_302` – redirect to new SO

**Sales Order Endpoints**:
- [x] `TestGetSOList_200` – authorized
- [x] `TestGetSODetail_200` – with line items
- [x] `TestCreateSO_302` – success redirect
- [x] `TestConfirmSO_200` – with confirm permission
- [x] `TestConfirmSO_403` – unauthorized
- [x] `TestCancelSO_200` – with reason

**Tools**: `httpexpect` for HTTP testing.

### RBAC Tests

- [x] `TestQuotationPermissions` – verify all permission checks
- [x] `TestSOPermissions` – verify all permission checks
- [x] `TestCrossCompanyAccess` – user cannot access other company's data
- [x] `TestOwnDraftAccess` – user can edit own drafts

### E2E Scenario Tests

#### `internal/e2e/sales_quotation_test.go`

**Scenario 1: Happy Path**
```
1. Sales user creates quotation (DRAFT)
2. Sales user submits quotation (SUBMITTED)
3. Manager approves quotation (APPROVED)
4. Sales user converts to SO (SO DRAFT)
5. Sales user confirms SO (CONFIRMED)
```

**Scenario 2: Rejection Flow**
```
1. Sales user creates quotation
2. Sales user submits quotation
3. Manager rejects quotation with reason
4. Verify quotation status = REJECTED
5. Verify cannot convert to SO
```

**Scenario 3: Edit & Resubmit**
```
1. Create & submit quotation
2. Manager rejects
3. Sales creates new quotation (copy data)
4. Submit & approve
5. Convert to SO
```

### Manual QA Checklist (Cycle 9.1)

#### Quotation Management
- [ ] Navigate to `/sales/quotations` – list displays with filters
- [ ] Click "New Quotation" – form renders correctly
- [ ] Select customer from dropdown – populates customer info
- [ ] Add product line items – quantity/price/discount calculations work
- [ ] Submit form – redirects to detail page (PRG pattern)
- [ ] Verify quotation appears in list with DRAFT status
- [ ] Click "Submit" – status changes to SUBMITTED
- [ ] Login as manager – approve quotation
- [ ] Verify status = APPROVED
- [ ] Click "Convert to SO" – redirects to new SO page
- [ ] Verify SO has all quotation line items
- [ ] Verify original quotation status = CONVERTED

#### Sales Order Management
- [ ] Navigate to `/sales/orders` – list with filters
- [ ] Create SO manually (without quotation)
- [ ] Add line items – calculations correct
- [ ] Save as DRAFT – can edit
- [ ] Click "Confirm" – status = CONFIRMED
- [ ] Verify cannot edit confirmed SO
- [ ] Click "Cancel" with reason – status = CANCELLED
- [ ] Verify audit log entry created

#### RBAC Verification
- [ ] Login as sales staff – can create/edit own quotations
- [ ] Verify cannot approve own quotations
- [ ] Login as sales manager – can approve quotations
- [ ] Login as warehouse staff – can view but not edit
- [ ] Login without permissions – 403 on all pages

#### Validation & Error Handling
- [ ] Try invalid date (quote_date > valid_until) – validation error
- [ ] Try negative quantity – validation error
- [ ] Try discount > 100% – validation error
- [ ] Try submitting without line items – validation error
- [ ] Try XSS in notes field – input sanitized/escaped

---

## Cycle 9.2 – Delivery & Fulfillment Testing

### Unit Tests

#### `internal/delivery/service_test.go`

- [x] `TestCreateDO_FromSO` – populate DO from SO lines
- [x] `TestCreateDO_PartialQuantity` – user adjusts qty_to_deliver
- [x] `TestCreateDO_InvalidSO` – SO not CONFIRMED
- [x] `TestConfirmDO_Success` – creates inventory_tx, updates SO
- [x] `TestConfirmDO_InsufficientStock` – validation error
- [x] `TestConfirmDO_UpdateSOStatus` – SO → PROCESSING/COMPLETED
- [x] `TestConfirmDO_PartialDelivery` – multiple DOs for one SO
- [x] `TestConfirmDO_Idempotency` – prevent double-confirm
- [x] `TestCancelDO` – DRAFT → CANCELLED
- [x] `TestCancelDO_AlreadyConfirmed` – cannot cancel confirmed DO
- [x] `TestCalculateRemainingQty` – remaining qty on SO lines

#### `internal/delivery/inventory_integration_test.go`

- [x] `TestStockReduction` – inventory_tx created with correct qty
- [x] `TestStockBalance` – product stock updated after DO confirm
- [x] `TestNegativeStockPrevention` – error if stock would go negative
- [x] `TestMultiProductDO` – batch stock reduction
- [x] `TestConcurrentDO` – race condition handling (stock locking)

### Integration Tests

#### `internal/delivery/repository_test.go`

- [x] `TestCreateDO_WithLines` – transaction integrity
- [x] `TestGetDOWithSOInfo` – JOIN with sales_orders
- [x] `TestListDOs_Filter` – by status, SO, warehouse, date
- [x] `TestUpdateQuantityDelivered` – update SO lines after confirm

### Job Tests

#### `internal/delivery/job_test.go`

- [x] `TestConfirmDOJob_Success` – async confirm flow
- [x] `TestConfirmDOJob_Retry` – retry on transient errors
- [x] `TestConfirmDOJob_Rollback` – rollback on stock validation failure
- [x] `TestConfirmDOJob_MaxRetries` – move to DLQ after 3 retries

### PDF Generation Tests

#### `internal/delivery/pdf_test.go`

- [x] `TestGeneratePackingList` – PDF generated successfully
- [x] `TestPackingListContent` – verify DO number, customer, products
- [x] `TestPackingListTimeout` – Gotenberg timeout handling (10s)
- [x] `TestPackingListSizeValidation` – PDF size > 1KB

### HTTP Handler Tests

- [x] `TestGetDOList_200` – list with filters
- [x] `TestCreateDO_FromSO_302` – success redirect
- [x] `TestCreateDO_InvalidSO_422` – validation error
- [x] `TestConfirmDO_200` – with permission
- [x] `TestConfirmDO_403` – unauthorized
- [x] `TestDownloadPackingList_200` – PDF download
- [x] `TestDownloadPackingList_403` – no download permission

### E2E Scenario Tests

#### `internal/e2e/delivery_flow_test.go`

**Scenario 1: Full Delivery**
```
1. Confirm SO (from Cycle 9.1)
2. Create DO for all SO lines
3. Confirm DO
4. Verify stock reduced in inventory
5. Verify SO status = COMPLETED
6. Download packing list PDF
```

**Scenario 2: Partial Delivery**
```
1. Confirm SO for 100 units
2. Create DO1 for 60 units → confirm
3. Verify SO status = PROCESSING
4. Verify SO line qty_delivered = 60
5. Create DO2 for 40 units → confirm
6. Verify SO status = COMPLETED
7. Verify total qty_delivered = 100
```

**Scenario 3: Stock Validation**
```
1. Confirm SO for 100 units
2. Reduce stock to 50 units (manual adjustment)
3. Try to confirm DO for 100 units
4. Verify error: insufficient stock
5. Adjust DO to 50 units
6. Confirm DO successfully
```

### Manual QA Checklist (Cycle 9.2)

#### Delivery Order Management
- [ ] Navigate to `/sales/deliveries` – list displays
- [ ] Click "New Delivery" – form with SO selection
- [ ] Select SO – lines populate automatically
- [ ] Adjust quantities for partial delivery
- [ ] Enter driver name & vehicle number
- [ ] Submit – redirects to detail page
- [ ] Verify DO status = DRAFT
- [ ] Click "Confirm" – status = CONFIRMED
- [ ] Verify stock reduced in inventory module
- [ ] Verify SO line qty_delivered updated
- [ ] Click "Download Packing List" – PDF downloads

#### Stock Integration Verification
- [ ] Check inventory before DO confirm – note stock qty
- [ ] Confirm DO with 10 units
- [ ] Check inventory after confirm – stock reduced by 10
- [ ] View inventory_tx – entry with type SALES_OUT exists
- [ ] Try confirming DO with insufficient stock – error displayed

#### Partial Delivery Flow
- [ ] Create SO with 100 units
- [ ] Create DO1 for 60 units – confirm
- [ ] Verify SO status = PROCESSING (not COMPLETED)
- [ ] Create DO2 for 40 units – confirm
- [ ] Verify SO status = COMPLETED
- [ ] Verify total qty_delivered = 100 on SO line

#### PDF Generation
- [ ] Download packing list – opens in browser
- [ ] Verify PDF contains: DO number, date, customer info
- [ ] Verify product list with quantities
- [ ] Verify driver & vehicle info
- [ ] Test with 50+ line items – PDF still generates

---

## Cycle 9.3 – AR Invoice & Payment Testing

### Unit Tests

#### `internal/ar/service_test.go`

**Invoice Tests**:
- [x] `TestCreateInvoice_FromDO` – populate from delivery order
- [x] `TestCreateInvoice_FromSO` – populate from sales order
- [x] `TestCreateInvoice_Manual` – manual line items
- [x] `TestPostInvoice_Success` – creates journal entry
- [x] `TestPostInvoice_JournalValidation` – debits = credits
- [x] `TestPostInvoice_Idempotency` – prevent double-posting
- [x] `TestPostInvoice_GLAccountMapping` – revenue account per line
- [x] `TestCancelInvoice` – ISSUED → CANCELLED with reason
- [x] `TestCancelInvoice_PartiallyPaid` – cannot cancel if paid
- [x] `TestUpdateInvoiceStatus_Overdue` – auto-update on due_date
- [x] `TestCalculateOutstanding` – total - paid = outstanding

**Payment Tests**:
- [x] `TestCreatePayment` – valid payment record
- [x] `TestPostPayment_SingleInvoice` – allocate to one invoice
- [x] `TestPostPayment_MultipleInvoices` – allocate to multiple
- [x] `TestPostPayment_PartialAllocation` – payment < invoice total
- [x] `TestPostPayment_ExactAllocation` – payment = invoice total
- [x] `TestPostPayment_Overpayment` – payment > invoice (credit balance)
- [x] `TestPostPayment_JournalEntry` – DR Cash, CR AR
- [x] `TestUpdateInvoiceAfterPayment` – status PAID/PARTIALLY_PAID
- [x] `TestValidateAllocation` – allocations <= payment amount

**Aging Tests**:
- [x] `TestCalculateAgingBucket` – current, 1-30, 31-60, 61-90, 90+
- [x] `TestAgingReport_GroupByCustomer` – totals per customer
- [x] `TestAgingReport_EmptyData` – no invoices scenario

### Integration Tests

#### `internal/ar/repository_test.go`

- [x] `TestCreateInvoice_WithLines` – transaction integrity
- [x] `TestGetInvoiceWithPayments` – JOIN allocations
- [x] `TestListInvoices_OverdueFilter` – where due_date < today
- [x] `TestRefreshAgingMV` – materialized view refresh
- [x] `TestGetAgingReport` – query mv_ar_aging
- [x] `TestCreatePaymentWithAllocations` – multi-table insert

#### `internal/ar/accounting_integration_test.go`

- [x] `TestInvoicePosting_CreatesJE` – journal_entry created
- [x] `TestInvoicePosting_JELines` – debit AR, credit Revenue/Tax
- [x] `TestInvoicePosting_JEBalance` – debits = credits
- [x] `TestPaymentPosting_CreatesJE` – journal_entry created
- [x] `TestPaymentPosting_JELines` – debit Cash, credit AR
- [x] `TestMultiPayment_JEAccuracy` – multiple payments → multiple JEs

### Job Tests

#### `internal/ar/job_test.go`

- [x] `TestPostInvoiceJob_Success` – async posting
- [x] `TestPostInvoiceJob_Rollback` – rollback on JE error
- [x] `TestPostPaymentJob_Success` – async payment posting
- [x] `TestPostPaymentJob_AllocationUpdate` – updates invoice amounts
- [x] `TestCheckOverdueJob_Daily` – cron job identifies overdue invoices
- [x] `TestRefreshAgingJob_Daily` – cron refreshes mv_ar_aging

### Report Tests

#### `internal/ar/aging_report_test.go`

- [x] `TestAgingReport_Buckets` – verify bucket calculations
- [x] `TestAgingReport_Export_CSV` – CSV format correct
- [x] `TestAgingReport_Export_PDF` – PDF generates via Gotenberg
- [x] `TestAgingReport_CustomerSummary` – totals by customer
- [x] `TestAgingReport_EmptyResult` – no overdue invoices

### HTTP Handler Tests

**Invoice Endpoints**:
- [x] `TestGetInvoiceList_200` – list with filters
- [x] `TestGetInvoiceDetail_200` – with payment history
- [x] `TestCreateInvoice_302` – success redirect
- [x] `TestPostInvoice_200` – with post permission
- [x] `TestPostInvoice_403` – unauthorized
- [x] `TestCancelInvoice_200` – with reason
- [x] `TestDownloadInvoicePDF_200` – PDF download

**Payment Endpoints**:
- [x] `TestGetPaymentList_200` – list with filters
- [x] `TestCreatePayment_302` – with allocation UI
- [x] `TestPostPayment_200` – with allocations
- [x] `TestPostPayment_422` – allocation > payment amount

**Aging Report Endpoints**:
- [x] `TestGetAgingReport_200` – SSR page with pivot table
- [x] `TestExportAgingCSV_200` – CSV download
- [x] `TestExportAgingPDF_200` – PDF download
- [x] `TestAgingReport_RateLimit_429` – 10 req/min limit

### E2E Scenario Tests

#### `internal/e2e/ar_flow_test.go`

**Scenario 1: Invoice → Payment (Full)**
```
1. Create invoice from DO (from Cycle 9.2)
2. Post invoice → verify journal entry created
3. Verify invoice status = ISSUED
4. Verify AR balance increased (query GL)
5. Create payment for full amount
6. Post payment → allocate to invoice
7. Verify invoice status = PAID
8. Verify payment journal entry created
9. Verify AR balance reduced
```

**Scenario 2: Partial Payment**
```
1. Post invoice for $1000
2. Create payment for $600
3. Allocate $600 to invoice
4. Verify invoice status = PARTIALLY_PAID
5. Verify outstanding_amount = $400
6. Create second payment for $400
7. Allocate to invoice
8. Verify invoice status = PAID
9. Verify outstanding_amount = $0
```

**Scenario 3: Multiple Invoice Payment**
```
1. Post invoice1 for $500
2. Post invoice2 for $300
3. Create payment for $800
4. Allocate $500 to invoice1, $300 to invoice2
5. Verify both invoices status = PAID
6. Verify payment fully allocated
```

**Scenario 4: Overdue Detection**
```
1. Create invoice with due_date = yesterday
2. Run overdue check job
3. Verify invoice status = OVERDUE
4. Verify appears in aging report (1-30 bucket)
5. Post payment
6. Verify status changes to PAID
7. Verify removed from aging report
```

### Manual QA Checklist (Cycle 9.3)

#### Invoice Management
- [ ] Navigate to `/finance/ar/invoices` – list displays
- [ ] Click "New Invoice" – form renders
- [ ] Select "From Delivery Order" – DO dropdown appears
- [ ] Select DO – lines populate from DO
- [ ] Verify calculations: subtotal, tax, total
- [ ] Save as DRAFT – can edit
- [ ] Click "Post Invoice" – status = ISSUED
- [ ] Verify cannot edit after posting
- [ ] Check accounting module – journal entry created
- [ ] Verify journal: DR AR, CR Revenue, CR Tax
- [ ] Download invoice PDF – opens correctly

#### Payment Management
- [ ] Navigate to `/finance/ar/payments` – list displays
- [ ] Click "New Payment" – form with allocation UI
- [ ] Select customer – outstanding invoices appear
- [ ] Enter payment amount
- [ ] Allocate to invoice(s) – running total updates
- [ ] Verify cannot over-allocate (validation)
- [ ] Submit payment – redirects to detail
- [ ] Click "Post Payment" – journal entry created
- [ ] Check invoice detail – paid_amount updated
- [ ] Verify invoice status changed (PARTIALLY_PAID or PAID)

#### AR Aging Report
- [ ] Navigate to `/finance/ar/aging`
- [ ] View aging buckets: Current, 1-30, 31-60, 61-90, 90+
- [ ] Filter by customer – results filtered
- [ ] Click "Export CSV" – downloads CSV
- [ ] Open CSV – verify data accuracy
- [ ] Click "Export PDF" – downloads PDF
- [ ] Verify PDF formatting & totals

#### Overdue Invoices
- [ ] Create invoice with due_date = today
- [ ] Wait until tomorrow (or manually update date)
- [ ] Run overdue check job (or wait for cron)
- [ ] Verify invoice status = OVERDUE
- [ ] Check aging report – appears in 1-30 bucket
- [ ] Post payment – status changes to PAID

#### Accounting Integration
- [ ] Post invoice – check journal_entries table
- [ ] Verify JE has debit AR line
- [ ] Verify JE has credit revenue line(s)
- [ ] Verify JE has credit tax line (if applicable)
- [ ] Verify debits = credits
- [ ] Post payment – check journal_entries table
- [ ] Verify JE has debit cash line
- [ ] Verify JE has credit AR line
- [ ] Run GL balance query – verify AR balance correct

---

## Performance Testing

### Load Tests (Apache Bench / k6)

#### Quotation Creation
- **Scenario**: 100 users create quotations concurrently
- **Expected**: <500ms response time (p95)
- **Expected**: 0% error rate

#### Invoice Posting
- **Scenario**: 50 users post invoices concurrently
- **Expected**: <1s response time (p95) including journal creation
- **Expected**: No duplicate journal entries

#### Aging Report
- **Scenario**: 10 users generate aging report with 10,000 invoices
- **Expected**: <2s query time
- **Expected**: <5s PDF generation time

#### Payment Allocation
- **Scenario**: 30 users record payments concurrently
- **Expected**: <800ms response time (p95)
- **Expected**: Correct allocation under concurrent updates

### Stress Tests

- **Max Concurrent DO Confirms**: Test stock locking under 100 concurrent confirms
- **Large Quotation**: 500 line items on single quotation
- **Bulk Invoice Export**: Export 1,000 invoices to CSV
- **Aging Report with 50K Invoices**: Performance degradation point

---

## Security Testing

### RBAC Tests (Automated)

- [x] `TestUnauthorizedAccess` – 403 on all protected endpoints
- [x] `TestCrossCompanyAccess` – user cannot access other company data
- [x] `TestOwnDraftEdit` – user can only edit own drafts
- [x] `TestApprovalPermission` – only managers can approve
- [x] `TestPostPermission` – only finance can post invoices/payments

### Penetration Tests (Manual)

- [ ] **SQL Injection**: Try injecting SQL in filter fields
- [ ] **XSS**: Inject `<script>alert(1)</script>` in notes
- [ ] **CSRF**: Submit form without CSRF token
- [ ] **IDOR**: Try accessing other user's invoice by ID
- [ ] **Path Traversal**: Download file with `../` in path
- [ ] **Rate Limit Bypass**: Exceed 10 req/min on export

### Audit Trail Verification

- [ ] Create quotation → verify audit log entry
- [ ] Approve quotation → verify audit with approver ID
- [ ] Post invoice → verify audit with journal_entry_id
- [ ] Cancel payment → verify audit with reason
- [ ] Query audit logs → all events captured

---

## Regression Testing

### Existing Module Impact

#### Inventory Module
- [ ] Stock balances correct after DO confirms
- [ ] Inventory transactions created properly
- [ ] No negative stock allowed
- [ ] Reconciliation report still accurate

#### Accounting Module
- [ ] Journal entries created correctly
- [ ] GL balances updated properly
- [ ] Trial balance still balanced
- [ ] Financial reports (P&L, BS) include AR/Revenue

#### Auth/RBAC Module
- [ ] New permissions seeded correctly
- [ ] Existing roles not broken
- [ ] Session handling unchanged

---

## Test Data Management

### Fixtures

Create reusable test fixtures:
- `fixtures/customers.sql` – 10 sample customers
- `fixtures/products.sql` – 20 sample products
- `fixtures/quotations.sql` – 5 quotations (various statuses)
- `fixtures/sales_orders.sql` – 3 sales orders
- `fixtures/deliveries.sql` – 2 delivery orders
- `fixtures/invoices.sql` – 5 invoices (various statuses)
- `fixtures/payments.sql` – 3 payments with allocations

### Seed Script

```bash
make seed-phase9
```

This script:
1. Seeds RBAC permissions for Phase 9
2. Seeds sample customers (if not exist)
3. Seeds sample products (if not exist)
4. Seeds default GL accounts for AR
5. Creates test quotations, orders, deliveries
6. Creates test invoices & payments
7. Refreshes aging materialized view

---

## CI/CD Integration

### GitHub Actions Workflow

```yaml
name: Phase 9 Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: testpass
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      redis:
        image: redis:7
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s

    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.23'

      - name: Run migrations
        run: make migrate-up

      - name: Seed test data
        run: make seed-phase9

      - name: Run unit tests
        run: go test -v -cover ./internal/sales/... ./internal/delivery/... ./internal/ar/...

      - name: Run integration tests
        run: go test -v -tags=integration ./internal/sales/... ./internal/delivery/... ./internal/ar/...

      - name: Run E2E tests
        run: go test -v -tags=e2e ./internal/e2e/...

      - name: Check coverage
        run: go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

      - name: Lint
        run: golangci-lint run ./internal/sales/... ./internal/delivery/... ./internal/ar/...
```

---

## Test Execution Schedule

### Development Phase

- **Daily**: Unit tests on every commit
- **Daily**: Integration tests on PR
- **Before merge**: E2E tests + manual QA checklist

### Pre-Production

- **Week 1**: Cycle 9.1 complete + full test suite
- **Week 2**: Cycle 9.2 complete + integration tests
- **Week 3**: Cycle 9.3 complete + E2E scenarios
- **Week 4**: Regression + performance + security tests

### Production

- **Post-deployment**: Smoke tests (critical paths)
- **Daily**: Automated regression suite
- **Weekly**: Performance benchmarks
- **Monthly**: Security pen tests

---

## Bug Tracking & Reporting

### Severity Levels

- **Critical**: Data loss, security vulnerability, system crash
- **High**: Feature unusable, incorrect calculations, RBAC bypass
- **Medium**: UI glitch, slow performance, minor validation issue
- **Low**: Cosmetic issue, typo, enhancement request

### Bug Report Template

```markdown
**Title**: [Module] Brief description

**Severity**: Critical / High / Medium / Low

**Steps to Reproduce**:
1. Navigate to...
2. Click on...
3. Observe...

**Expected**: What should happen

**Actual**: What actually happened

**Environment**:
- OS: Linux/Mac/Windows
- Browser: Chrome 120 / Firefox 121
- Go version: 1.23
- Commit: abc123

**Screenshots**: (if applicable)

**Logs**: (paste relevant logs)
```

---

## Acceptance Criteria (Phase 9 Complete)

### Functional
- ✅ All 3 cycles implemented and tested
- ✅ All user stories completed
- ✅ All acceptance criteria met per cycle

### Quality
- ✅ Unit test coverage ≥ 70%
- ✅ Zero critical or high bugs in production
- ✅ All E2E scenarios passing
- ✅ Performance benchmarks met

### Security
- ✅ Security checklist completed (see `security-checklist-phase9.md`)
- ✅ Pen test passed (no critical vulnerabilities)
- ✅ RBAC enforced on all endpoints
- ✅ Audit trail complete

### Documentation
- ✅ All docs updated (howto, runbook, architecture)
- ✅ API documentation complete
- ✅ User guides reviewed by stakeholders

### Operations
- ✅ Runbooks reviewed by ops team
- ✅ Monitoring dashboards configured
- ✅ Alerting rules deployed
- ✅ Backup/restore tested

---

## Post-Launch Monitoring

### Metrics to Watch (First 30 Days)

- **Error Rate**: <0.1% for all endpoints
- **Response Time**: p95 <500ms for reads, <1s for writes
- **Job Success Rate**: >99% for async jobs
- **User Adoption**: # of quotations, orders, invoices created
- **AR Aging Accuracy**: Spot-check against manual calculations

### Known Issues Log

Maintain `KNOWN-ISSUES-PHASE9.md` with:
- Issue description
- Workaround (if any)
- Planned fix version
- Impact assessment

---

## Lessons Learned & Retrospective

### Post-Phase 9 Retro Questions

1. What went well in testing process?
2. What could be improved?
3. Were test coverage targets realistic?
4. Did we catch bugs early enough?
5. How was collaboration between dev and QA?
6. What testing tools should we adopt?
7. What edge cases did we miss?

### Action Items for Phase 10

- [ ] Improve test data generation (faker library?)
- [ ] Add visual regression testing (Percy, Chromatic)
- [ ] Implement contract testing for APIs
- [ ] Automate more security tests (OWASP ZAP CI integration)
- [ ] Create load testing dashboards (Grafana + k6)

---

**Document Version**: 1.0  
**Last Updated**: 2025-01-16  
**Maintained By**: QA Lead & Tech Lead  
**Next Review**: End of Cycle 9.3