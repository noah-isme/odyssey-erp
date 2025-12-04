package sales

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// MOCK DEPENDENCIES
// ============================================================================

type mockServiceForHandler struct {
	customers      map[int64]*Customer
	quotations     map[int64]*Quotation
	salesOrders    map[int64]*SalesOrder
	createError    error
	getError       error
	updateError    error
	nextCustomerID int64
	nextQuoteID    int64
	nextOrderID    int64
}

func newMockServiceForHandler() *mockServiceForHandler {
	return &mockServiceForHandler{
		customers:      make(map[int64]*Customer),
		quotations:     make(map[int64]*Quotation),
		salesOrders:    make(map[int64]*SalesOrder),
		nextCustomerID: 1,
		nextQuoteID:    1,
		nextOrderID:    1,
	}
}

func (m *mockServiceForHandler) CreateCustomer(ctx context.Context, req CreateCustomerRequest, createdBy int64) (*Customer, error) {
	if m.createError != nil {
		return nil, m.createError
	}
	customer := &Customer{
		ID:               m.nextCustomerID,
		Code:             req.Code,
		Name:             req.Name,
		CompanyID:        req.CompanyID,
		Email:            req.Email,
		Phone:            req.Phone,
		CreditLimit:      req.CreditLimit,
		PaymentTermsDays: req.PaymentTermsDays,
		Country:          req.Country,
		IsActive:         true,
		CreatedBy:        createdBy,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	m.customers[customer.ID] = customer
	m.nextCustomerID++
	return customer, nil
}

func (m *mockServiceForHandler) UpdateCustomer(ctx context.Context, id int64, req UpdateCustomerRequest) (*Customer, error) {
	if m.updateError != nil {
		return nil, m.updateError
	}
	customer, ok := m.customers[id]
	if !ok {
		return nil, ErrNotFound
	}
	if req.Name != nil {
		customer.Name = *req.Name
	}
	if req.CreditLimit != nil {
		customer.CreditLimit = *req.CreditLimit
	}
	customer.UpdatedAt = time.Now()
	return customer, nil
}

func (m *mockServiceForHandler) GetCustomer(ctx context.Context, id int64) (*Customer, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	customer, ok := m.customers[id]
	if !ok {
		return nil, ErrNotFound
	}
	return customer, nil
}

func (m *mockServiceForHandler) ListCustomers(ctx context.Context, req ListCustomersRequest) ([]Customer, int, error) {
	result := []Customer{}
	for _, c := range m.customers {
		if c.CompanyID == req.CompanyID {
			result = append(result, *c)
		}
	}
	return result, len(result), nil
}

func (m *mockServiceForHandler) GenerateCustomerCode(ctx context.Context, companyID int64) (string, error) {
	return "CUST-" + time.Now().Format("200601") + "-001", nil
}

func (m *mockServiceForHandler) CreateQuotation(ctx context.Context, req CreateQuotationRequest, createdBy int64) (*Quotation, error) {
	if m.createError != nil {
		return nil, m.createError
	}
	quotation := &Quotation{
		ID:          m.nextQuoteID,
		DocNumber:   "QUO-" + time.Now().Format("200601") + "-" + strconv.FormatInt(m.nextQuoteID, 10),
		CompanyID:   req.CompanyID,
		CustomerID:  req.CustomerID,
		QuoteDate:   req.QuoteDate,
		ValidUntil:  req.ValidUntil,
		Status:      QuotationStatusDraft,
		Currency:    req.Currency,
		Subtotal:    1000.00,
		TaxAmount:   100.00,
		TotalAmount: 1100.00,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.quotations[quotation.ID] = quotation
	m.nextQuoteID++
	return quotation, nil
}

func (m *mockServiceForHandler) UpdateQuotation(ctx context.Context, id int64, req UpdateQuotationRequest) (*Quotation, error) {
	if m.updateError != nil {
		return nil, m.updateError
	}
	quotation, ok := m.quotations[id]
	if !ok {
		return nil, ErrNotFound
	}
	if req.ValidUntil != nil {
		quotation.ValidUntil = *req.ValidUntil
	}
	quotation.UpdatedAt = time.Now()
	return quotation, nil
}

func (m *mockServiceForHandler) SubmitQuotation(ctx context.Context, id int64, userID int64) (*Quotation, error) {
	quotation, ok := m.quotations[id]
	if !ok {
		return nil, ErrNotFound
	}
	quotation.Status = QuotationStatusSubmitted
	quotation.UpdatedAt = time.Now()
	return quotation, nil
}

func (m *mockServiceForHandler) ApproveQuotation(ctx context.Context, id int64, userID int64) (*Quotation, error) {
	quotation, ok := m.quotations[id]
	if !ok {
		return nil, ErrNotFound
	}
	quotation.Status = QuotationStatusApproved
	quotation.ApprovedBy = &userID
	now := time.Now()
	quotation.ApprovedAt = &now
	quotation.UpdatedAt = time.Now()
	return quotation, nil
}

func (m *mockServiceForHandler) RejectQuotation(ctx context.Context, id int64, userID int64, reason string) (*Quotation, error) {
	quotation, ok := m.quotations[id]
	if !ok {
		return nil, ErrNotFound
	}
	quotation.Status = QuotationStatusRejected
	quotation.RejectedBy = &userID
	now := time.Now()
	quotation.RejectedAt = &now
	quotation.RejectionReason = &reason
	quotation.UpdatedAt = time.Now()
	return quotation, nil
}

func (m *mockServiceForHandler) GetQuotation(ctx context.Context, id int64) (*Quotation, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	quotation, ok := m.quotations[id]
	if !ok {
		return nil, ErrNotFound
	}
	return quotation, nil
}

func (m *mockServiceForHandler) ListQuotations(ctx context.Context, req ListQuotationsRequest) ([]QuotationWithDetails, int, error) {
	result := []QuotationWithDetails{}
	for _, q := range m.quotations {
		if q.CompanyID == req.CompanyID {
			qwd := QuotationWithDetails{
				Quotation:     *q,
				CustomerName:  "Test Customer",
				CreatedByName: "Test User",
			}
			result = append(result, qwd)
		}
	}
	return result, len(result), nil
}

func (m *mockServiceForHandler) CreateSalesOrder(ctx context.Context, req CreateSalesOrderRequest, createdBy int64) (*SalesOrder, error) {
	if m.createError != nil {
		return nil, m.createError
	}
	order := &SalesOrder{
		ID:                   m.nextOrderID,
		DocNumber:            "SO-" + time.Now().Format("200601") + "-" + strconv.FormatInt(m.nextOrderID, 10),
		CompanyID:            req.CompanyID,
		CustomerID:           req.CustomerID,
		OrderDate:            req.OrderDate,
		ExpectedDeliveryDate: req.ExpectedDeliveryDate,
		Status:               SalesOrderStatusDraft,
		Currency:             req.Currency,
		Subtotal:             1000.00,
		TaxAmount:            100.00,
		TotalAmount:          1100.00,
		CreatedBy:            createdBy,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	m.salesOrders[order.ID] = order
	m.nextOrderID++
	return order, nil
}

func (m *mockServiceForHandler) ConvertQuotationToSalesOrder(ctx context.Context, quotationID int64, userID int64, orderDate time.Time) (*SalesOrder, error) {
	quotation, ok := m.quotations[quotationID]
	if !ok {
		return nil, ErrNotFound
	}
	order := &SalesOrder{
		ID:                   m.nextOrderID,
		DocNumber:            "SO-" + time.Now().Format("200601") + "-" + strconv.FormatInt(m.nextOrderID, 10),
		CompanyID:            quotation.CompanyID,
		CustomerID:           quotation.CustomerID,
		QuotationID:          &quotationID,
		OrderDate:            orderDate,
		ExpectedDeliveryDate: nil,
		Status:               SalesOrderStatusDraft,
		Currency:             quotation.Currency,
		Subtotal:             quotation.Subtotal,
		TaxAmount:            quotation.TaxAmount,
		TotalAmount:          quotation.TotalAmount,
		CreatedBy:            userID,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	m.salesOrders[order.ID] = order
	m.nextOrderID++
	quotation.Status = QuotationStatusConverted
	return order, nil
}

func (m *mockServiceForHandler) UpdateSalesOrder(ctx context.Context, id int64, req UpdateSalesOrderRequest) (*SalesOrder, error) {
	if m.updateError != nil {
		return nil, m.updateError
	}
	order, ok := m.salesOrders[id]
	if !ok {
		return nil, ErrNotFound
	}
	if req.ExpectedDeliveryDate != nil {
		order.ExpectedDeliveryDate = req.ExpectedDeliveryDate
	}
	order.UpdatedAt = time.Now()
	return order, nil
}

func (m *mockServiceForHandler) ConfirmSalesOrder(ctx context.Context, id int64, userID int64) (*SalesOrder, error) {
	order, ok := m.salesOrders[id]
	if !ok {
		return nil, ErrNotFound
	}
	order.Status = SalesOrderStatusConfirmed
	order.ConfirmedBy = &userID
	now := time.Now()
	order.ConfirmedAt = &now
	order.UpdatedAt = time.Now()
	return order, nil
}

func (m *mockServiceForHandler) CancelSalesOrder(ctx context.Context, id int64, userID int64, reason string) (*SalesOrder, error) {
	order, ok := m.salesOrders[id]
	if !ok {
		return nil, ErrNotFound
	}
	order.Status = SalesOrderStatusCancelled
	order.CancelledBy = &userID
	now := time.Now()
	order.CancelledAt = &now
	order.CancellationReason = &reason
	order.UpdatedAt = time.Now()
	return order, nil
}

func (m *mockServiceForHandler) GetSalesOrder(ctx context.Context, id int64) (*SalesOrder, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	order, ok := m.salesOrders[id]
	if !ok {
		return nil, ErrNotFound
	}
	return order, nil
}

func (m *mockServiceForHandler) ListSalesOrders(ctx context.Context, req ListSalesOrdersRequest) ([]SalesOrderWithDetails, int, error) {
	result := []SalesOrderWithDetails{}
	for _, so := range m.salesOrders {
		if so.CompanyID == req.CompanyID {
			sowd := SalesOrderWithDetails{
				SalesOrder:    *so,
				CustomerName:  "Test Customer",
				CreatedByName: "Test User",
			}
			result = append(result, sowd)
		}
	}
	return result, len(result), nil
}

// ============================================================================
// TEST HELPERS
// ============================================================================

func setupTestHandler() *mockServiceForHandler {
	mockSvc := newMockServiceForHandler()
	return mockSvc
}

// ============================================================================
// CUSTOMER HANDLER TESTS
// ============================================================================

func TestCreateCustomerHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Test the service method directly
	customerReq := CreateCustomerRequest{
		Code:             "CUST001",
		Name:             "Test Customer Inc",
		CompanyID:        1,
		CreditLimit:      50000.00,
		PaymentTermsDays: 30,
		Country:          "US",
	}

	customer, err := mockSvc.CreateCustomer(context.Background(), customerReq, 100)
	require.NoError(t, err)
	require.NotNil(t, customer)

	assert.Equal(t, "CUST001", customer.Code)
	assert.Equal(t, "Test Customer Inc", customer.Name)
	assert.Equal(t, 50000.00, customer.CreditLimit)
}

func TestGetCustomerHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create a customer first
	customerReq := CreateCustomerRequest{
		Code:             "CUST001",
		Name:             "Test Customer",
		CompanyID:        1,
		CreditLimit:      50000.00,
		PaymentTermsDays: 30,
		Country:          "US",
	}
	customer, err := mockSvc.CreateCustomer(context.Background(), customerReq, 100)
	require.NoError(t, err)

	// Retrieve customer
	retrieved, err := mockSvc.GetCustomer(context.Background(), customer.ID)
	require.NoError(t, err)
	assert.Equal(t, customer.ID, retrieved.ID)
	assert.Equal(t, "Test Customer", retrieved.Name)
}

func TestListCustomersHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create multiple customers
	for i := 1; i <= 3; i++ {
		req := CreateCustomerRequest{
			Code:             "CUST00" + strconv.Itoa(i),
			Name:             "Customer " + strconv.Itoa(i),
			CompanyID:        1,
			CreditLimit:      50000.00,
			PaymentTermsDays: 30,
			Country:          "US",
		}
		_, err := mockSvc.CreateCustomer(context.Background(), req, 100)
		require.NoError(t, err)
	}

	// List customers
	listReq := ListCustomersRequest{
		CompanyID: 1,
		Limit:     10,
		Offset:    0,
	}
	customers, total, err := mockSvc.ListCustomers(context.Background(), listReq)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, customers, 3)
}

func TestUpdateCustomerHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create customer
	customerReq := CreateCustomerRequest{
		Code:             "CUST001",
		Name:             "Original Name",
		CompanyID:        1,
		CreditLimit:      50000.00,
		PaymentTermsDays: 30,
		Country:          "US",
	}
	customer, err := mockSvc.CreateCustomer(context.Background(), customerReq, 100)
	require.NoError(t, err)

	// Update customer
	updateReq := UpdateCustomerRequest{
		Name:        ptr("Updated Name"),
		CreditLimit: ptr(75000.00),
	}
	updated, err := mockSvc.UpdateCustomer(context.Background(), customer.ID, updateReq)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, 75000.00, updated.CreditLimit)
}

// ============================================================================
// QUOTATION HANDLER TESTS
// ============================================================================

func TestCreateQuotationHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create customer first
	customerReq := CreateCustomerRequest{
		Code:             "CUST001",
		Name:             "Test Customer",
		CompanyID:        1,
		CreditLimit:      50000.00,
		PaymentTermsDays: 30,
		Country:          "US",
	}
	customer, err := mockSvc.CreateCustomer(context.Background(), customerReq, 100)
	require.NoError(t, err)

	// Create quotation
	quoteDate := time.Now()
	validUntil := quoteDate.AddDate(0, 0, 30)

	quotationReq := CreateQuotationRequest{
		CompanyID:  1,
		CustomerID: customer.ID,
		QuoteDate:  quoteDate,
		ValidUntil: validUntil,
		Currency:   "USD",
		Lines: []CreateQuotationLineReq{
			{
				ProductID:       1,
				Description:     ptr("Product A"),
				Quantity:        10,
				UOM:             "PCS",
				UnitPrice:       100.00,
				DiscountPercent: 0,
				TaxPercent:      10,
				LineOrder:       1,
			},
		},
	}

	quotation, err := mockSvc.CreateQuotation(context.Background(), quotationReq, 100)
	require.NoError(t, err)
	assert.Equal(t, QuotationStatusDraft, quotation.Status)
	assert.Equal(t, customer.ID, quotation.CustomerID)
}

func TestSubmitQuotationHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create quotation
	customer, _ := mockSvc.CreateCustomer(context.Background(), CreateCustomerRequest{
		Code: "CUST001", Name: "Test", CompanyID: 1, CreditLimit: 50000, PaymentTermsDays: 30, Country: "US",
	}, 100)

	quotation, _ := mockSvc.CreateQuotation(context.Background(), CreateQuotationRequest{
		CompanyID:  1,
		CustomerID: customer.ID,
		QuoteDate:  time.Now(),
		ValidUntil: time.Now().AddDate(0, 0, 30),
		Currency:   "USD",
		Lines: []CreateQuotationLineReq{
			{ProductID: 1, Quantity: 10, UOM: "PCS", UnitPrice: 100, LineOrder: 1},
		},
	}, 100)

	// Submit quotation
	submitted, err := mockSvc.SubmitQuotation(context.Background(), quotation.ID, 100)
	require.NoError(t, err)
	assert.Equal(t, QuotationStatusSubmitted, submitted.Status)
}

func TestApproveQuotationHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create and submit quotation
	customer, _ := mockSvc.CreateCustomer(context.Background(), CreateCustomerRequest{
		Code: "CUST001", Name: "Test", CompanyID: 1, CreditLimit: 50000, PaymentTermsDays: 30, Country: "US",
	}, 100)

	quotation, _ := mockSvc.CreateQuotation(context.Background(), CreateQuotationRequest{
		CompanyID:  1,
		CustomerID: customer.ID,
		QuoteDate:  time.Now(),
		ValidUntil: time.Now().AddDate(0, 0, 30),
		Currency:   "USD",
		Lines: []CreateQuotationLineReq{
			{ProductID: 1, Quantity: 10, UOM: "PCS", UnitPrice: 100, LineOrder: 1},
		},
	}, 100)

	mockSvc.SubmitQuotation(context.Background(), quotation.ID, 100)

	// Approve quotation
	approved, err := mockSvc.ApproveQuotation(context.Background(), quotation.ID, 200)
	require.NoError(t, err)
	assert.Equal(t, QuotationStatusApproved, approved.Status)
	assert.NotNil(t, approved.ApprovedBy)
	assert.Equal(t, int64(200), *approved.ApprovedBy)
}

func TestRejectQuotationHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create and submit quotation
	customer, _ := mockSvc.CreateCustomer(context.Background(), CreateCustomerRequest{
		Code: "CUST001", Name: "Test", CompanyID: 1, CreditLimit: 50000, PaymentTermsDays: 30, Country: "US",
	}, 100)

	quotation, _ := mockSvc.CreateQuotation(context.Background(), CreateQuotationRequest{
		CompanyID:  1,
		CustomerID: customer.ID,
		QuoteDate:  time.Now(),
		ValidUntil: time.Now().AddDate(0, 0, 30),
		Currency:   "USD",
		Lines: []CreateQuotationLineReq{
			{ProductID: 1, Quantity: 10, UOM: "PCS", UnitPrice: 100, LineOrder: 1},
		},
	}, 100)

	mockSvc.SubmitQuotation(context.Background(), quotation.ID, 100)

	// Reject quotation
	reason := "Price too high"
	rejected, err := mockSvc.RejectQuotation(context.Background(), quotation.ID, 200, reason)
	require.NoError(t, err)
	assert.Equal(t, QuotationStatusRejected, rejected.Status)
	assert.NotNil(t, rejected.RejectionReason)
	assert.Equal(t, reason, *rejected.RejectionReason)
}

// ============================================================================
// SALES ORDER HANDLER TESTS
// ============================================================================

func TestCreateSalesOrderHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create customer
	customer, _ := mockSvc.CreateCustomer(context.Background(), CreateCustomerRequest{
		Code: "CUST001", Name: "Test Customer", CompanyID: 1, CreditLimit: 50000, PaymentTermsDays: 30, Country: "US",
	}, 100)

	// Create sales order
	orderDate := time.Now()
	deliveryDate := orderDate.AddDate(0, 0, 7)

	orderReq := CreateSalesOrderRequest{
		CompanyID:            1,
		CustomerID:           customer.ID,
		OrderDate:            orderDate,
		ExpectedDeliveryDate: &deliveryDate,
		Currency:             "USD",
		Lines: []CreateSalesOrderLineReq{
			{
				ProductID:       1,
				Description:     ptr("Product A"),
				Quantity:        10,
				UOM:             "PCS",
				UnitPrice:       100.00,
				DiscountPercent: 0,
				TaxPercent:      10,
				LineOrder:       1,
			},
		},
	}

	order, err := mockSvc.CreateSalesOrder(context.Background(), orderReq, 100)
	require.NoError(t, err)
	assert.Equal(t, SalesOrderStatusDraft, order.Status)
	assert.Equal(t, customer.ID, order.CustomerID)
}

func TestConvertQuotationToSalesOrderHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create customer and quotation
	customer, _ := mockSvc.CreateCustomer(context.Background(), CreateCustomerRequest{
		Code: "CUST001", Name: "Test", CompanyID: 1, CreditLimit: 50000, PaymentTermsDays: 30, Country: "US",
	}, 100)

	quotation, _ := mockSvc.CreateQuotation(context.Background(), CreateQuotationRequest{
		CompanyID:  1,
		CustomerID: customer.ID,
		QuoteDate:  time.Now(),
		ValidUntil: time.Now().AddDate(0, 0, 30),
		Currency:   "USD",
		Lines: []CreateQuotationLineReq{
			{ProductID: 1, Quantity: 10, UOM: "PCS", UnitPrice: 100, LineOrder: 1},
		},
	}, 100)

	mockSvc.SubmitQuotation(context.Background(), quotation.ID, 100)
	mockSvc.ApproveQuotation(context.Background(), quotation.ID, 200)

	// Convert to sales order
	orderDate := time.Now()

	order, err := mockSvc.ConvertQuotationToSalesOrder(context.Background(), quotation.ID, 100, orderDate)
	require.NoError(t, err)
	assert.Equal(t, SalesOrderStatusDraft, order.Status)
	assert.NotNil(t, order.QuotationID)
	assert.Equal(t, quotation.ID, *order.QuotationID)
}

func TestConfirmSalesOrderHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create sales order
	customer, _ := mockSvc.CreateCustomer(context.Background(), CreateCustomerRequest{
		Code: "CUST001", Name: "Test", CompanyID: 1, CreditLimit: 50000, PaymentTermsDays: 30, Country: "US",
	}, 100)

	order, _ := mockSvc.CreateSalesOrder(context.Background(), CreateSalesOrderRequest{
		CompanyID:  1,
		CustomerID: customer.ID,
		OrderDate:  time.Now(),
		Currency:   "USD",
		Lines: []CreateSalesOrderLineReq{
			{ProductID: 1, Quantity: 10, UOM: "PCS", UnitPrice: 100, LineOrder: 1},
		},
	}, 100)

	// Confirm order
	confirmed, err := mockSvc.ConfirmSalesOrder(context.Background(), order.ID, 100)
	require.NoError(t, err)
	assert.Equal(t, SalesOrderStatusConfirmed, confirmed.Status)
	assert.NotNil(t, confirmed.ConfirmedBy)
	assert.Equal(t, int64(100), *confirmed.ConfirmedBy)
}

func TestCancelSalesOrderHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create sales order
	customer, _ := mockSvc.CreateCustomer(context.Background(), CreateCustomerRequest{
		Code: "CUST001", Name: "Test", CompanyID: 1, CreditLimit: 50000, PaymentTermsDays: 30, Country: "US",
	}, 100)

	order, _ := mockSvc.CreateSalesOrder(context.Background(), CreateSalesOrderRequest{
		CompanyID:  1,
		CustomerID: customer.ID,
		OrderDate:  time.Now(),
		Currency:   "USD",
		Lines: []CreateSalesOrderLineReq{
			{ProductID: 1, Quantity: 10, UOM: "PCS", UnitPrice: 100, LineOrder: 1},
		},
	}, 100)

	// Cancel order
	reason := "Customer requested cancellation"
	cancelled, err := mockSvc.CancelSalesOrder(context.Background(), order.ID, 100, reason)
	require.NoError(t, err)
	assert.Equal(t, SalesOrderStatusCancelled, cancelled.Status)
	assert.NotNil(t, cancelled.CancellationReason)
	assert.Equal(t, reason, *cancelled.CancellationReason)
}

func TestListSalesOrdersHandler(t *testing.T) {
	mockSvc := setupTestHandler()

	// Create customer
	customer, _ := mockSvc.CreateCustomer(context.Background(), CreateCustomerRequest{
		Code: "CUST001", Name: "Test", CompanyID: 1, CreditLimit: 50000, PaymentTermsDays: 30, Country: "US",
	}, 100)

	// Create multiple sales orders
	for i := 0; i < 3; i++ {
		_, err := mockSvc.CreateSalesOrder(context.Background(), CreateSalesOrderRequest{
			CompanyID:  1,
			CustomerID: customer.ID,
			OrderDate:  time.Now(),
			Currency:   "USD",
			Lines: []CreateSalesOrderLineReq{
				{ProductID: 1, Quantity: 10, UOM: "PCS", UnitPrice: 100, LineOrder: 1},
			},
		}, 100)
		require.NoError(t, err)
	}

	// List orders
	listReq := ListSalesOrdersRequest{
		CompanyID: 1,
		Limit:     10,
		Offset:    0,
	}
	orders, total, err := mockSvc.ListSalesOrders(context.Background(), listReq)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, orders, 3)
}

// ============================================================================
// ERROR HANDLING TESTS
// ============================================================================

func TestCreateCustomerWithError(t *testing.T) {
	mockSvc := setupTestHandler()

	// Inject error
	mockSvc.createError = ErrAlreadyExists

	req := CreateCustomerRequest{
		Code:             "CUST001",
		Name:             "Test",
		CompanyID:        1,
		CreditLimit:      50000,
		PaymentTermsDays: 30,
		Country:          "US",
	}

	_, err := mockSvc.CreateCustomer(context.Background(), req, 100)
	require.Error(t, err)
	assert.Equal(t, ErrAlreadyExists, err)
}

func TestHandlerGetCustomerNotFound(t *testing.T) {
	mockSvc := setupTestHandler()

	_, err := mockSvc.GetCustomer(context.Background(), 999)
	require.Error(t, err)
	assert.Equal(t, ErrNotFound, err)
}

func TestGetQuotationNotFound(t *testing.T) {
	mockSvc := setupTestHandler()

	_, err := mockSvc.GetQuotation(context.Background(), 999)
	require.Error(t, err)
	assert.Equal(t, ErrNotFound, err)
}

func TestGetSalesOrderNotFound(t *testing.T) {
	mockSvc := setupTestHandler()

	_, err := mockSvc.GetSalesOrder(context.Background(), 999)
	require.Error(t, err)
	assert.Equal(t, ErrNotFound, err)
}
