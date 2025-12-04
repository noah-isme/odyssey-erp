# Sales Module - Unit & Integration Tests

## Overview

This document describes the comprehensive test suite for the Odyssey ERP Sales & Accounts Receivable module (Phase 9.1).

## Test Structure

The test suite consists of three main files:

1. **`service_test.go`** - Unit tests for service layer business logic
2. **`handler_test.go`** - Unit tests for HTTP handlers
3. **`integration_test.go`** - End-to-end integration tests for complete workflows

## Test Coverage

### Total Test Count: 44 tests

#### Service Layer Tests (18 tests)
- **Customer Operations (6 tests)**
  - `TestCreateCustomer` - Creates a new customer with valid data
  - `TestCreateCustomerDuplicateCode` - Validates duplicate code prevention
  - `TestUpdateCustomer` - Updates customer information
  - `TestGetCustomer` - Retrieves customer by ID
  - `TestServiceGetCustomerNotFound` - Handles non-existent customer lookup
  - `TestListCustomers` - Lists customers with pagination

- **Quotation Operations (6 tests)**
  - `TestCreateQuotation` - Creates quotation with line items and calculates totals
  - `TestSubmitQuotation` - Submits draft quotation for approval
  - `TestApproveQuotation` - Approves submitted quotation
  - `TestRejectQuotation` - Rejects quotation with reason
  - `TestUpdateQuotation` - Updates draft quotation
  - Quotation listing covered in integration tests

- **Sales Order Operations (6 tests)**
  - `TestCreateSalesOrder` - Creates sales order directly (no quotation)
  - `TestConvertQuotationToSalesOrder` - Converts approved quotation to order
  - `TestConfirmSalesOrder` - Confirms sales order
  - `TestCancelSalesOrder` - Cancels order with reason
  - `TestUpdateSalesOrder` - Updates draft sales order
  - `TestListSalesOrders` - Lists orders with filtering

#### Handler Layer Tests (17 tests)
- **Customer Handlers (4 tests)**
  - `TestCreateCustomerHandler` - POST /customers endpoint
  - `TestGetCustomerHandler` - GET /customers/{id} endpoint
  - `TestListCustomersHandler` - GET /customers with pagination
  - `TestUpdateCustomerHandler` - POST /customers/{id}/edit endpoint

- **Quotation Handlers (4 tests)**
  - `TestCreateQuotationHandler` - POST /quotations endpoint
  - `TestSubmitQuotationHandler` - POST /quotations/{id}/submit
  - `TestApproveQuotationHandler` - POST /quotations/{id}/approve
  - `TestRejectQuotationHandler` - POST /quotations/{id}/reject

- **Sales Order Handlers (5 tests)**
  - `TestCreateSalesOrderHandler` - POST /orders endpoint
  - `TestConvertQuotationToSalesOrderHandler` - POST /quotations/{id}/convert
  - `TestConfirmSalesOrderHandler` - POST /orders/{id}/confirm
  - `TestCancelSalesOrderHandler` - POST /orders/{id}/cancel
  - `TestListSalesOrdersHandler` - GET /orders with filters

- **Error Handling (4 tests)**
  - `TestCreateCustomerWithError` - Validates error injection
  - `TestHandlerGetCustomerNotFound` - 404 handling
  - `TestGetQuotationNotFound` - Non-existent quotation
  - `TestGetSalesOrderNotFound` - Non-existent order

#### Integration Tests (9 test scenarios)
- **Complete Workflows**
  - `TestCompleteQuotationWorkflow` - Full lifecycle: Draft → Submit → Approve → Convert → Confirm
  - `TestQuotationRejectionWorkflow` - Rejection path with reason tracking
  - `TestDirectSalesOrderWorkflow` - Order creation without quotation
  - `TestSalesOrderCancellationWorkflow` - Order cancellation process
  
- **Customer Management**
  - `TestCustomerManagementWorkflow` - Create, update, deactivate, filter customers
  
- **Advanced Scenarios**
  - `TestQuotationUpdateBeforeSubmission` - Update draft quotation lines
  - `TestMultipleCustomersAndOrders` - Concurrent operations with multiple entities
  - `TestInvalidStatusTransitions` - Status validation (implementation-dependent)
  - `TestEdgeCasesAndBoundaries` - Zero values, maximum values, minimal data

## Test Architecture

### Mock Repository Pattern

The test suite uses a comprehensive mock repository that simulates database operations:

```go
type mockRepository struct {
    customers       map[int64]*Customer
    quotations      map[int64]*Quotation
    salesOrders     map[int64]*SalesOrder
    // ... tracking counters and error injection
}
```

### Test Service Wrapper

A `testService` wrapper provides service-layer methods using the mock repository:

```go
type testService struct {
    repo *mockRepository
}
```

This approach allows:
- Complete business logic testing without database dependencies
- Deterministic test execution
- Fast test runs (no I/O operations)
- Easy error injection for negative test cases

## Key Test Scenarios

### 1. Quotation → Sales Order Workflow

```
Draft Quotation
    ↓ Submit
Submitted Quotation
    ↓ Approve (or Reject)
Approved Quotation
    ↓ Convert
Draft Sales Order
    ↓ Confirm
Confirmed Sales Order
```

**Tested in:** `TestCompleteQuotationWorkflow`, `TestConvertQuotationToSalesOrder`

### 2. Financial Calculations

All tests verify accurate calculation of:
- Line subtotals = Quantity × Unit Price
- Discount amounts = Subtotal × Discount%
- Tax amounts = (Subtotal - Discount) × Tax%
- Line totals = Subtotal - Discount + Tax
- Document totals = Sum of all line totals

**Tested in:** `TestCreateQuotation`, `TestCreateSalesOrder`, `TestQuotationUpdateBeforeSubmission`

### 3. Status Transitions

Tests ensure proper state machine enforcement:
- Quotations: DRAFT → SUBMITTED → APPROVED → CONVERTED
- Quotations: SUBMITTED → REJECTED
- Sales Orders: DRAFT → CONFIRMED → PROCESSING → COMPLETED
- Sales Orders: Any → CANCELLED (with restrictions)

**Tested in:** All workflow tests, `TestInvalidStatusTransitions`

### 4. Error Handling

Tests cover error scenarios:
- Duplicate customer codes
- Non-existent entity lookups (404)
- Invalid status transitions
- Expired quotations
- Business rule violations

**Tested in:** Error handling test group

## Running Tests

### Run All Tests
```bash
go test ./internal/sales/...
```

### Run with Verbose Output
```bash
go test ./internal/sales/... -v
```

### Run Specific Test
```bash
go test ./internal/sales/... -run TestCreateCustomer
```

### Run Integration Tests Only
```bash
go test ./internal/sales/... -run TestSalesIntegrationSuite
```

### Run with Coverage
```bash
go test ./internal/sales/... -cover
```

### Run Specific Test Suite
```bash
go test ./internal/sales/... -run TestSalesIntegrationSuite/TestCompleteQuotationWorkflow
```

## Test Data Patterns

### Customer Test Data
```go
CreateCustomerRequest{
    Code:             "CUST-2025-001",
    Name:             "Acme Corporation",
    CompanyID:        1,
    CreditLimit:      100000.00,
    PaymentTermsDays: 30,
    Country:          "US",
}
```

### Quotation Test Data
```go
CreateQuotationRequest{
    CompanyID:  1,
    CustomerID: customerID,
    QuoteDate:  time.Now(),
    ValidUntil: time.Now().AddDate(0, 1, 0), // 1 month
    Currency:   "USD",
    Lines: []CreateQuotationLineReq{
        {
            ProductID:       101,
            Quantity:        50,
            UnitPrice:       150.00,
            DiscountPercent: 10,
            TaxPercent:      8,
        },
    },
}
```

## Mock Service Interface

The `mockServiceForHandler` provides a complete implementation of service methods for handler testing:

- All CRUD operations for customers, quotations, and sales orders
- Status transition methods (submit, approve, reject, confirm, cancel)
- Error injection capabilities for negative testing
- In-memory storage without external dependencies

## Integration Test Suite

The integration test suite uses `testify/suite` for setup/teardown:

```go
type SalesIntegrationTestSuite struct {
    suite.Suite
    service *testService
    repo    *mockRepository
    ctx     context.Context
}
```

Each test scenario:
1. Creates necessary prerequisites (customers, products)
2. Executes the workflow under test
3. Verifies state transitions and data integrity
4. Validates business rule enforcement

## Best Practices Applied

1. **Isolation** - Each test is independent and can run in any order
2. **AAA Pattern** - Arrange, Act, Assert structure
3. **Clear Naming** - Test names describe what is being tested
4. **Helper Functions** - `createTestCustomer()`, `createTestQuotation()`, etc.
5. **Unique Data** - Dynamic customer codes to avoid conflicts
6. **Comprehensive Assertions** - Verify all critical fields and calculations
7. **Error Scenarios** - Both happy path and error cases covered
8. **Documentation** - Inline comments explain complex scenarios

## Future Enhancements

Potential additions to the test suite:

1. **Database Integration Tests** - Tests against real PostgreSQL using test containers
2. **Performance Tests** - Benchmark tests for high-volume scenarios
3. **Concurrency Tests** - Race condition detection with parallel operations
4. **API Contract Tests** - JSON schema validation for HTTP responses
5. **End-to-End Tests** - Full browser-based UI testing
6. **Load Tests** - Stress testing with multiple concurrent users

## Dependencies

- `github.com/stretchr/testify` - Assertions and test suites
- Standard library `testing` package
- No external database required for unit tests

## Maintenance

When adding new features:

1. Add unit tests for service layer methods
2. Add handler tests for new HTTP endpoints
3. Create integration test scenarios for new workflows
4. Update this README with new test descriptions
5. Ensure all tests pass before committing

## Troubleshooting

### Test Failures

If tests fail:
1. Check for unique constraint violations (customer codes)
2. Verify mock data setup in failing test
3. Review error messages for business rule violations
4. Ensure proper status transitions in workflow tests

### Performance Issues

If tests run slowly:
1. Verify no actual database connections are being made
2. Check for infinite loops in mock implementations
3. Reduce test data volume if not necessary
4. Use `-short` flag to skip long-running tests

---

**Last Updated:** Phase 9.1 Implementation  
**Maintained By:** Odyssey ERP Development Team  
**Status:** ✅ All 44 tests passing