package sales

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// MOCK REPOSITORY
// ============================================================================

type mockRepository struct {
	// Customer storage
	customers       map[int64]*Customer
	customersByCode map[string]*Customer
	nextCustomerID  int64
	customerCounter map[int64]int

	// Quotation storage
	quotations       map[int64]*Quotation
	quotationLines   map[int64][]QuotationLine
	nextQuotationID  int64
	quotationCounter map[int64]int

	// Sales Order storage
	salesOrders       map[int64]*SalesOrder
	salesOrderLines   map[int64][]SalesOrderLine
	nextSalesOrderID  int64
	salesOrderCounter map[int64]int

	// Error injection
	txError           error
	getCustomerError  error
	createQuoteError  error
	getQuotationError error
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		customers:         make(map[int64]*Customer),
		customersByCode:   make(map[string]*Customer),
		customerCounter:   make(map[int64]int),
		quotations:        make(map[int64]*Quotation),
		quotationLines:    make(map[int64][]QuotationLine),
		quotationCounter:  make(map[int64]int),
		salesOrders:       make(map[int64]*SalesOrder),
		salesOrderLines:   make(map[int64][]SalesOrderLine),
		salesOrderCounter: make(map[int64]int),
		nextCustomerID:    1,
		nextQuotationID:   1,
		nextSalesOrderID:  1,
	}
}

func (m *mockRepository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	if m.txError != nil {
		return m.txError
	}
	tx := &mockTxRepo{mock: m}
	return fn(ctx, tx)
}

func (m *mockRepository) GetCustomer(ctx context.Context, id int64) (*Customer, error) {
	if m.getCustomerError != nil {
		return nil, m.getCustomerError
	}
	c, ok := m.customers[id]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (m *mockRepository) GetCustomerByCode(ctx context.Context, companyID int64, code string) (*Customer, error) {
	key := makeCodeKey(companyID, code)
	c, ok := m.customersByCode[key]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (m *mockRepository) ListCustomers(ctx context.Context, req ListCustomersRequest) ([]Customer, int, error) {
	result := []Customer{}
	for _, c := range m.customers {
		if c.CompanyID == req.CompanyID {
			if req.IsActive != nil && c.IsActive != *req.IsActive {
				continue
			}
			result = append(result, *c)
		}
	}
	return result, len(result), nil
}

func (m *mockRepository) GenerateCustomerCode(ctx context.Context, companyID int64) (string, error) {
	m.customerCounter[companyID]++
	return formatCode("CUST", companyID, m.customerCounter[companyID]), nil
}

func (m *mockRepository) GetQuotation(ctx context.Context, id int64) (*Quotation, error) {
	if m.getQuotationError != nil {
		return nil, m.getQuotationError
	}
	q, ok := m.quotations[id]
	if !ok {
		return nil, ErrNotFound
	}
	// Attach lines
	if lines, ok := m.quotationLines[id]; ok {
		q.Lines = lines
	}
	return q, nil
}

func (m *mockRepository) ListQuotations(ctx context.Context, req ListQuotationsRequest) ([]QuotationWithDetails, int, error) {
	result := []QuotationWithDetails{}
	for _, q := range m.quotations {
		if q.CompanyID == req.CompanyID {
			if req.CustomerID != nil && q.CustomerID != *req.CustomerID {
				continue
			}
			if req.Status != nil && q.Status != *req.Status {
				continue
			}
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

func (m *mockRepository) GenerateQuotationNumber(ctx context.Context, companyID int64) (string, error) {
	m.quotationCounter[companyID]++
	return formatCode("QUO", companyID, m.quotationCounter[companyID]), nil
}

func (m *mockRepository) GetSalesOrder(ctx context.Context, id int64) (*SalesOrder, error) {
	so, ok := m.salesOrders[id]
	if !ok {
		return nil, ErrNotFound
	}
	// Attach lines
	if lines, ok := m.salesOrderLines[id]; ok {
		so.Lines = lines
	}
	return so, nil
}

func (m *mockRepository) ListSalesOrders(ctx context.Context, req ListSalesOrdersRequest) ([]SalesOrderWithDetails, int, error) {
	result := []SalesOrderWithDetails{}
	for _, so := range m.salesOrders {
		if so.CompanyID == req.CompanyID {
			if req.CustomerID != nil && so.CustomerID != *req.CustomerID {
				continue
			}
			if req.Status != nil && so.Status != *req.Status {
				continue
			}
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

func (m *mockRepository) GenerateSalesOrderNumber(ctx context.Context, companyID int64) (string, error) {
	m.salesOrderCounter[companyID]++
	return formatCode("SO", companyID, m.salesOrderCounter[companyID]), nil
}

// ============================================================================
// MOCK TX REPOSITORY
// ============================================================================

type mockTxRepo struct {
	mock *mockRepository
}

func (tx *mockTxRepo) CreateCustomer(ctx context.Context, customer Customer) (int64, error) {
	id := tx.mock.nextCustomerID
	tx.mock.nextCustomerID++

	customer.ID = id
	customer.CreatedAt = time.Now()
	customer.UpdatedAt = time.Now()

	tx.mock.customers[id] = &customer
	key := makeCodeKey(customer.CompanyID, customer.Code)
	tx.mock.customersByCode[key] = &customer

	return id, nil
}

func (tx *mockTxRepo) UpdateCustomer(ctx context.Context, id int64, updates map[string]interface{}) error {
	c, ok := tx.mock.customers[id]
	if !ok {
		return ErrNotFound
	}

	// Apply updates
	if v, ok := updates["name"].(string); ok {
		c.Name = v
	}
	if v, ok := updates["credit_limit"].(float64); ok {
		c.CreditLimit = v
	}
	if v, ok := updates["payment_terms_days"].(int); ok {
		c.PaymentTermsDays = v
	}
	if v, ok := updates["email"].(*string); ok {
		c.Email = v
	}
	if v, ok := updates["phone"].(*string); ok {
		c.Phone = v
	}
	if v, ok := updates["is_active"].(bool); ok {
		c.IsActive = v
	}
	c.UpdatedAt = time.Now()

	return nil
}

func (tx *mockTxRepo) CreateQuotation(ctx context.Context, quotation Quotation) (int64, error) {
	if tx.mock.createQuoteError != nil {
		return 0, tx.mock.createQuoteError
	}

	id := tx.mock.nextQuotationID
	tx.mock.nextQuotationID++

	quotation.ID = id
	quotation.CreatedAt = time.Now()
	quotation.UpdatedAt = time.Now()

	tx.mock.quotations[id] = &quotation
	return id, nil
}

func (tx *mockTxRepo) InsertQuotationLine(ctx context.Context, line QuotationLine) (int64, error) {
	lineID := int64(len(tx.mock.quotationLines[line.QuotationID]) + 1)
	line.ID = lineID
	line.CreatedAt = time.Now()
	line.UpdatedAt = time.Now()

	tx.mock.quotationLines[line.QuotationID] = append(tx.mock.quotationLines[line.QuotationID], line)
	return lineID, nil
}

func (tx *mockTxRepo) UpdateQuotationStatus(ctx context.Context, id int64, status QuotationStatus, userID int64, reason *string) error {
	q, ok := tx.mock.quotations[id]
	if !ok {
		return ErrNotFound
	}

	now := time.Now()
	q.Status = status
	q.UpdatedAt = now

	switch status {
	case QuotationStatusSubmitted:
		// No additional fields
	case QuotationStatusApproved:
		q.ApprovedBy = &userID
		q.ApprovedAt = &now
	case QuotationStatusRejected:
		q.RejectedBy = &userID
		q.RejectedAt = &now
		q.RejectionReason = reason
	case QuotationStatusConverted:
		// Handled separately
	}

	return nil
}

func (tx *mockTxRepo) DeleteQuotationLines(ctx context.Context, quotationID int64) error {
	delete(tx.mock.quotationLines, quotationID)
	return nil
}

func (tx *mockTxRepo) CreateSalesOrder(ctx context.Context, order SalesOrder) (int64, error) {
	id := tx.mock.nextSalesOrderID
	tx.mock.nextSalesOrderID++

	order.ID = id
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	tx.mock.salesOrders[id] = &order
	return id, nil
}

func (tx *mockTxRepo) InsertSalesOrderLine(ctx context.Context, line SalesOrderLine) (int64, error) {
	lineID := int64(len(tx.mock.salesOrderLines[line.SalesOrderID]) + 1)
	line.ID = lineID
	line.CreatedAt = time.Now()
	line.UpdatedAt = time.Now()

	tx.mock.salesOrderLines[line.SalesOrderID] = append(tx.mock.salesOrderLines[line.SalesOrderID], line)
	return lineID, nil
}

func (tx *mockTxRepo) UpdateSalesOrderStatus(ctx context.Context, id int64, status SalesOrderStatus, userID int64, reason *string) error {
	so, ok := tx.mock.salesOrders[id]
	if !ok {
		return ErrNotFound
	}

	now := time.Now()
	so.Status = status
	so.UpdatedAt = now

	switch status {
	case SalesOrderStatusConfirmed:
		so.ConfirmedBy = &userID
		so.ConfirmedAt = &now
	case SalesOrderStatusCancelled:
		so.CancelledBy = &userID
		so.CancelledAt = &now
		so.CancellationReason = reason
	}

	return nil
}

func (tx *mockTxRepo) DeleteSalesOrderLines(ctx context.Context, salesOrderID int64) error {
	delete(tx.mock.salesOrderLines, salesOrderID)
	return nil
}

func (tx *mockTxRepo) UpdateSalesOrderLineDelivered(ctx context.Context, lineID int64, quantityDelivered float64) error {
	// Not implemented for mock
	return nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func makeCodeKey(companyID int64, code string) string {
	return formatCode("", companyID, 0) + code
}

func formatCode(prefix string, companyID int64, counter int) string {
	if prefix == "" {
		return ""
	}
	return prefix + "-" + time.Now().Format("200601") + "-" + string(rune(counter+1000))
}

// testService wraps mockRepository to provide service-like interface for testing
type testService struct {
	repo *mockRepository
}

func newTestService() *testService {
	return &testService{
		repo: newMockRepository(),
	}
}

func (ts *testService) CreateCustomer(ctx context.Context, req CreateCustomerRequest, createdBy int64) (*Customer, error) {
	// Check if code already exists
	existing, err := ts.repo.GetCustomerByCode(ctx, req.CompanyID, req.Code)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("check existing customer: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("%w: customer code already exists", ErrAlreadyExists)
	}

	customer := Customer{
		Code:             req.Code,
		Name:             req.Name,
		CompanyID:        req.CompanyID,
		Email:            req.Email,
		Phone:            req.Phone,
		TaxID:            req.TaxID,
		CreditLimit:      req.CreditLimit,
		PaymentTermsDays: req.PaymentTermsDays,
		AddressLine1:     req.AddressLine1,
		AddressLine2:     req.AddressLine2,
		City:             req.City,
		State:            req.State,
		PostalCode:       req.PostalCode,
		Country:          req.Country,
		IsActive:         true,
		Notes:            req.Notes,
		CreatedBy:        createdBy,
	}

	var id int64
	err = ts.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		var err error
		id, err = tx.CreateCustomer(ctx, customer)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("create customer: %w", err)
	}

	customer.ID = id
	return &customer, nil
}

func (ts *testService) UpdateCustomer(ctx context.Context, id int64, req UpdateCustomerRequest) (*Customer, error) {
	existing, err := ts.repo.GetCustomer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get customer: %w", err)
	}

	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.CreditLimit != nil {
		updates["credit_limit"] = *req.CreditLimit
	}
	if req.PaymentTermsDays != nil {
		updates["payment_terms_days"] = *req.PaymentTermsDays
	}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if len(updates) == 0 {
		return existing, nil
	}

	err = ts.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateCustomer(ctx, id, updates)
	})
	if err != nil {
		return nil, fmt.Errorf("update customer: %w", err)
	}

	return ts.repo.GetCustomer(ctx, id)
}

func (ts *testService) GetCustomer(ctx context.Context, id int64) (*Customer, error) {
	return ts.repo.GetCustomer(ctx, id)
}

func (ts *testService) ListCustomers(ctx context.Context, req ListCustomersRequest) ([]Customer, int, error) {
	return ts.repo.ListCustomers(ctx, req)
}

func (ts *testService) CreateQuotation(ctx context.Context, req CreateQuotationRequest, createdBy int64) (*Quotation, error) {
	docNumber, _ := ts.repo.GenerateQuotationNumber(ctx, req.CompanyID)

	var subtotal, taxAmount float64
	lines := []QuotationLine{}

	for _, lineReq := range req.Lines {
		lineSubtotal := lineReq.Quantity * lineReq.UnitPrice
		discount := lineSubtotal * lineReq.DiscountPercent / 100
		lineSubtotalAfterDiscount := lineSubtotal - discount
		lineTax := lineSubtotalAfterDiscount * lineReq.TaxPercent / 100
		lineTotal := lineSubtotalAfterDiscount + lineTax

		line := QuotationLine{
			ProductID:       lineReq.ProductID,
			Description:     lineReq.Description,
			Quantity:        lineReq.Quantity,
			UOM:             lineReq.UOM,
			UnitPrice:       lineReq.UnitPrice,
			DiscountPercent: lineReq.DiscountPercent,
			DiscountAmount:  discount,
			TaxPercent:      lineReq.TaxPercent,
			TaxAmount:       lineTax,
			LineTotal:       lineTotal,
			Notes:           lineReq.Notes,
			LineOrder:       lineReq.LineOrder,
		}
		lines = append(lines, line)
		subtotal += lineSubtotalAfterDiscount
		taxAmount += lineTax
	}

	quotation := Quotation{
		DocNumber:   docNumber,
		CompanyID:   req.CompanyID,
		CustomerID:  req.CustomerID,
		QuoteDate:   req.QuoteDate,
		ValidUntil:  req.ValidUntil,
		Status:      QuotationStatusDraft,
		Currency:    req.Currency,
		Subtotal:    subtotal,
		TaxAmount:   taxAmount,
		TotalAmount: subtotal + taxAmount,
		Notes:       req.Notes,
		CreatedBy:   createdBy,
	}

	var id int64
	err := ts.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		var err error
		id, err = tx.CreateQuotation(ctx, quotation)
		if err != nil {
			return err
		}
		for i := range lines {
			lines[i].QuotationID = id
			_, err = tx.InsertQuotationLine(ctx, lines[i])
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	quotation.ID = id
	quotation.Lines = lines
	return &quotation, nil
}

func (ts *testService) SubmitQuotation(ctx context.Context, id int64, userID int64) (*Quotation, error) {
	err := ts.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateQuotationStatus(ctx, id, QuotationStatusSubmitted, userID, nil)
	})
	if err != nil {
		return nil, err
	}
	return ts.repo.GetQuotation(ctx, id)
}

func (ts *testService) ApproveQuotation(ctx context.Context, id int64, userID int64) (*Quotation, error) {
	err := ts.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateQuotationStatus(ctx, id, QuotationStatusApproved, userID, nil)
	})
	if err != nil {
		return nil, err
	}
	return ts.repo.GetQuotation(ctx, id)
}

func (ts *testService) RejectQuotation(ctx context.Context, id int64, userID int64, reason string) (*Quotation, error) {
	err := ts.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateQuotationStatus(ctx, id, QuotationStatusRejected, userID, &reason)
	})
	if err != nil {
		return nil, err
	}
	return ts.repo.GetQuotation(ctx, id)
}

func (ts *testService) UpdateQuotation(ctx context.Context, id int64, req UpdateQuotationRequest) (*Quotation, error) {
	quotation, err := ts.repo.GetQuotation(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.ValidUntil != nil {
		quotation.ValidUntil = *req.ValidUntil
	}
	if req.Notes != nil {
		quotation.Notes = req.Notes
	}
	if req.QuoteDate != nil {
		quotation.QuoteDate = *req.QuoteDate
	}

	// If lines are provided, recalculate totals
	if req.Lines != nil {
		var subtotal, taxAmount float64
		lines := []QuotationLine{}

		for _, lineReq := range *req.Lines {
			lineSubtotal := lineReq.Quantity * lineReq.UnitPrice
			discount := lineSubtotal * lineReq.DiscountPercent / 100
			lineSubtotalAfterDiscount := lineSubtotal - discount
			lineTax := lineSubtotalAfterDiscount * lineReq.TaxPercent / 100
			lineTotal := lineSubtotalAfterDiscount + lineTax

			line := QuotationLine{
				QuotationID:     id,
				ProductID:       lineReq.ProductID,
				Description:     lineReq.Description,
				Quantity:        lineReq.Quantity,
				UOM:             lineReq.UOM,
				UnitPrice:       lineReq.UnitPrice,
				DiscountPercent: lineReq.DiscountPercent,
				DiscountAmount:  discount,
				TaxPercent:      lineReq.TaxPercent,
				TaxAmount:       lineTax,
				LineTotal:       lineTotal,
				Notes:           lineReq.Notes,
				LineOrder:       lineReq.LineOrder,
			}
			lines = append(lines, line)
			subtotal += lineSubtotalAfterDiscount
			taxAmount += lineTax
		}

		err = ts.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			if err := tx.DeleteQuotationLines(ctx, id); err != nil {
				return err
			}
			for i := range lines {
				_, err := tx.InsertQuotationLine(ctx, lines[i])
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		quotation.Subtotal = subtotal
		quotation.TaxAmount = taxAmount
		quotation.TotalAmount = subtotal + taxAmount
		quotation.Lines = lines
	}

	quotation.UpdatedAt = time.Now()
	return quotation, nil
}

func (ts *testService) GetQuotation(ctx context.Context, id int64) (*Quotation, error) {
	return ts.repo.GetQuotation(ctx, id)
}

func (ts *testService) ListQuotations(ctx context.Context, req ListQuotationsRequest) ([]QuotationWithDetails, int, error) {
	return ts.repo.ListQuotations(ctx, req)
}

func (ts *testService) CreateSalesOrder(ctx context.Context, req CreateSalesOrderRequest, createdBy int64) (*SalesOrder, error) {
	docNumber, _ := ts.repo.GenerateSalesOrderNumber(ctx, req.CompanyID)

	var subtotal, taxAmount float64
	lines := []SalesOrderLine{}

	for _, lineReq := range req.Lines {
		lineSubtotal := lineReq.Quantity * lineReq.UnitPrice
		discount := lineSubtotal * lineReq.DiscountPercent / 100
		lineSubtotalAfterDiscount := lineSubtotal - discount
		lineTax := lineSubtotalAfterDiscount * lineReq.TaxPercent / 100
		lineTotal := lineSubtotalAfterDiscount + lineTax

		line := SalesOrderLine{
			ProductID:       lineReq.ProductID,
			Description:     lineReq.Description,
			Quantity:        lineReq.Quantity,
			UOM:             lineReq.UOM,
			UnitPrice:       lineReq.UnitPrice,
			DiscountPercent: lineReq.DiscountPercent,
			DiscountAmount:  discount,
			TaxPercent:      lineReq.TaxPercent,
			TaxAmount:       lineTax,
			LineTotal:       lineTotal,
			Notes:           lineReq.Notes,
			LineOrder:       lineReq.LineOrder,
		}
		lines = append(lines, line)
		subtotal += lineSubtotalAfterDiscount
		taxAmount += lineTax
	}

	order := SalesOrder{
		DocNumber:            docNumber,
		CompanyID:            req.CompanyID,
		CustomerID:           req.CustomerID,
		QuotationID:          req.QuotationID,
		OrderDate:            req.OrderDate,
		ExpectedDeliveryDate: req.ExpectedDeliveryDate,
		Status:               SalesOrderStatusDraft,
		Currency:             req.Currency,
		Subtotal:             subtotal,
		TaxAmount:            taxAmount,
		TotalAmount:          subtotal + taxAmount,
		Notes:                req.Notes,
		CreatedBy:            createdBy,
	}

	var id int64
	err := ts.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		var err error
		id, err = tx.CreateSalesOrder(ctx, order)
		if err != nil {
			return err
		}
		for i := range lines {
			lines[i].SalesOrderID = id
			_, err = tx.InsertSalesOrderLine(ctx, lines[i])
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	order.ID = id
	order.Lines = lines
	return &order, nil
}

func (ts *testService) ConvertQuotationToSalesOrder(ctx context.Context, quotationID int64, createdBy int64, orderDate time.Time) (*SalesOrder, error) {
	quotation, err := ts.repo.GetQuotation(ctx, quotationID)
	if err != nil {
		return nil, err
	}

	docNumber, _ := ts.repo.GenerateSalesOrderNumber(ctx, quotation.CompanyID)

	order := SalesOrder{
		DocNumber:   docNumber,
		CompanyID:   quotation.CompanyID,
		CustomerID:  quotation.CustomerID,
		QuotationID: &quotationID,
		OrderDate:   orderDate,
		Status:      SalesOrderStatusDraft,
		Currency:    quotation.Currency,
		Subtotal:    quotation.Subtotal,
		TaxAmount:   quotation.TaxAmount,
		TotalAmount: quotation.TotalAmount,
		CreatedBy:   createdBy,
	}

	var id int64
	err = ts.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		var err error
		id, err = tx.CreateSalesOrder(ctx, order)
		if err != nil {
			return err
		}
		for _, qLine := range quotation.Lines {
			line := SalesOrderLine{
				SalesOrderID:    id,
				ProductID:       qLine.ProductID,
				Description:     qLine.Description,
				Quantity:        qLine.Quantity,
				UOM:             qLine.UOM,
				UnitPrice:       qLine.UnitPrice,
				DiscountPercent: qLine.DiscountPercent,
				DiscountAmount:  qLine.DiscountAmount,
				TaxPercent:      qLine.TaxPercent,
				TaxAmount:       qLine.TaxAmount,
				LineTotal:       qLine.LineTotal,
				Notes:           qLine.Notes,
				LineOrder:       qLine.LineOrder,
			}
			_, err = tx.InsertSalesOrderLine(ctx, line)
			if err != nil {
				return err
			}
		}
		return tx.UpdateQuotationStatus(ctx, quotationID, QuotationStatusConverted, createdBy, nil)
	})
	if err != nil {
		return nil, err
	}

	return ts.repo.GetSalesOrder(ctx, id)
}

func (ts *testService) ConfirmSalesOrder(ctx context.Context, id int64, userID int64) (*SalesOrder, error) {
	err := ts.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateSalesOrderStatus(ctx, id, SalesOrderStatusConfirmed, userID, nil)
	})
	if err != nil {
		return nil, err
	}
	return ts.repo.GetSalesOrder(ctx, id)
}

func (ts *testService) CancelSalesOrder(ctx context.Context, id int64, userID int64, reason string) (*SalesOrder, error) {
	err := ts.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateSalesOrderStatus(ctx, id, SalesOrderStatusCancelled, userID, &reason)
	})
	if err != nil {
		return nil, err
	}
	return ts.repo.GetSalesOrder(ctx, id)
}

func (ts *testService) UpdateSalesOrder(ctx context.Context, id int64, req UpdateSalesOrderRequest) (*SalesOrder, error) {
	order, err := ts.repo.GetSalesOrder(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.OrderDate != nil {
		order.OrderDate = *req.OrderDate
	}
	if req.ExpectedDeliveryDate != nil {
		order.ExpectedDeliveryDate = req.ExpectedDeliveryDate
	}
	if req.Notes != nil {
		order.Notes = req.Notes
	}

	order.UpdatedAt = time.Now()
	return order, nil
}

func (ts *testService) GetSalesOrder(ctx context.Context, id int64) (*SalesOrder, error) {
	return ts.repo.GetSalesOrder(ctx, id)
}

func (ts *testService) ListSalesOrders(ctx context.Context, req ListSalesOrdersRequest) ([]SalesOrderWithDetails, int, error) {
	return ts.repo.ListSalesOrders(ctx, req)
}

func ptr[T any](v T) *T {
	return &v
}

// ============================================================================
// CUSTOMER TESTS
// ============================================================================

func TestCreateCustomer(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	req := CreateCustomerRequest{
		Code:             "CUST001",
		Name:             "Test Customer Inc",
		CompanyID:        1,
		Email:            ptr("test@example.com"),
		Phone:            ptr("+1234567890"),
		CreditLimit:      50000.00,
		PaymentTermsDays: 30,
		Country:          "US",
	}

	customer, err := svc.CreateCustomer(ctx, req, 100)
	require.NoError(t, err)
	require.NotNil(t, customer)

	assert.Equal(t, int64(1), customer.ID)
	assert.Equal(t, "CUST001", customer.Code)
	assert.Equal(t, "Test Customer Inc", customer.Name)
	assert.Equal(t, int64(1), customer.CompanyID)
	assert.Equal(t, 50000.00, customer.CreditLimit)
	assert.Equal(t, 30, customer.PaymentTermsDays)
	assert.True(t, customer.IsActive)
	assert.Equal(t, int64(100), customer.CreatedBy)
}

func TestCreateCustomerDuplicateCode(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	req := CreateCustomerRequest{
		Code:             "CUST001",
		Name:             "First Customer",
		CompanyID:        1,
		CreditLimit:      50000.00,
		PaymentTermsDays: 30,
		Country:          "US",
	}

	// Create first customer
	_, err := svc.CreateCustomer(ctx, req, 100)
	require.NoError(t, err)

	// Try to create duplicate
	req.Name = "Second Customer"
	_, err = svc.CreateCustomer(ctx, req, 100)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAlreadyExists))
}

func TestUpdateCustomer(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create customer first
	req := CreateCustomerRequest{
		Code:             "CUST001",
		Name:             "Original Name",
		CompanyID:        1,
		CreditLimit:      50000.00,
		PaymentTermsDays: 30,
		Country:          "US",
	}
	customer, err := svc.CreateCustomer(ctx, req, 100)
	require.NoError(t, err)

	// Update customer
	updateReq := UpdateCustomerRequest{
		Name:        ptr("Updated Name"),
		CreditLimit: ptr(75000.00),
		IsActive:    ptr(false),
	}

	updated, err := svc.UpdateCustomer(ctx, customer.ID, updateReq)
	require.NoError(t, err)
	require.NotNil(t, updated)

	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, 75000.00, updated.CreditLimit)
	assert.False(t, updated.IsActive)
}

func TestGetCustomer(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create customer
	req := CreateCustomerRequest{
		Code:             "CUST001",
		Name:             "Test Customer",
		CompanyID:        1,
		CreditLimit:      50000.00,
		PaymentTermsDays: 30,
		Country:          "US",
	}
	created, err := svc.CreateCustomer(ctx, req, 100)
	require.NoError(t, err)

	// Retrieve customer
	customer, err := svc.GetCustomer(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, customer.ID)
	assert.Equal(t, "Test Customer", customer.Name)
}

func TestServiceGetCustomerNotFound(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	_, err := svc.GetCustomer(ctx, 999)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestListCustomers(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create multiple customers
	for i := 1; i <= 3; i++ {
		req := CreateCustomerRequest{
			Code:             formatCode("CUST", 1, i),
			Name:             "Customer " + string(rune(i+'0')),
			CompanyID:        1,
			CreditLimit:      50000.00,
			PaymentTermsDays: 30,
			Country:          "US",
		}
		_, err := svc.CreateCustomer(ctx, req, 100)
		require.NoError(t, err)
	}

	// List customers
	listReq := ListCustomersRequest{
		CompanyID: 1,
		Limit:     10,
		Offset:    0,
	}
	customers, total, err := svc.ListCustomers(ctx, listReq)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, customers, 3)
}

// ============================================================================
// QUOTATION TESTS
// ============================================================================

func TestCreateQuotation(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create customer first
	customerReq := CreateCustomerRequest{
		Code:             "CUST001",
		Name:             "Test Customer",
		CompanyID:        1,
		CreditLimit:      50000.00,
		PaymentTermsDays: 30,
		Country:          "US",
	}
	customer, err := svc.CreateCustomer(ctx, customerReq, 100)
	require.NoError(t, err)

	// Create quotation
	quoteDate := time.Now()
	validUntil := quoteDate.AddDate(0, 0, 30)

	req := CreateQuotationRequest{
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
			{
				ProductID:       2,
				Description:     ptr("Product B"),
				Quantity:        5,
				UOM:             "PCS",
				UnitPrice:       200.00,
				DiscountPercent: 5,
				TaxPercent:      10,
				LineOrder:       2,
			},
		},
	}

	quotation, err := svc.CreateQuotation(ctx, req, 100)
	require.NoError(t, err)
	require.NotNil(t, quotation)

	assert.Equal(t, int64(1), quotation.ID)
	assert.Equal(t, QuotationStatusDraft, quotation.Status)
	assert.Equal(t, customer.ID, quotation.CustomerID)
	assert.Equal(t, "USD", quotation.Currency)
	assert.Len(t, quotation.Lines, 2)

	// Verify calculations
	// Line 1: 10 * 100 = 1000, tax = 100, total = 1100
	// Line 2: 5 * 200 = 1000, discount 5% = 50, subtotal = 950, tax = 95, total = 1045
	// Expected subtotal: 1000 + 950 = 1950
	// Expected tax: 100 + 95 = 195
	// Expected total: 1950 + 195 = 2145
	assert.InDelta(t, 1950.00, quotation.Subtotal, 0.01)
	assert.InDelta(t, 195.00, quotation.TaxAmount, 0.01)
	assert.InDelta(t, 2145.00, quotation.TotalAmount, 0.01)
}

func TestSubmitQuotation(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create quotation
	quotation := createTestQuotation(t, svc, ctx)

	// Submit quotation
	submitted, err := svc.SubmitQuotation(ctx, quotation.ID, 100)
	require.NoError(t, err)
	assert.Equal(t, QuotationStatusSubmitted, submitted.Status)
}

func TestApproveQuotation(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create and submit quotation
	quotation := createTestQuotation(t, svc, ctx)
	_, err := svc.SubmitQuotation(ctx, quotation.ID, 100)
	require.NoError(t, err)

	// Approve quotation
	approved, err := svc.ApproveQuotation(ctx, quotation.ID, 200)
	require.NoError(t, err)
	assert.Equal(t, QuotationStatusApproved, approved.Status)
	assert.NotNil(t, approved.ApprovedBy)
	assert.NotNil(t, approved.ApprovedAt)
	assert.Equal(t, int64(200), *approved.ApprovedBy)
}

func TestRejectQuotation(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create and submit quotation
	quotation := createTestQuotation(t, svc, ctx)
	_, err := svc.SubmitQuotation(ctx, quotation.ID, 100)
	require.NoError(t, err)

	// Reject quotation
	reason := "Price too high"
	rejected, err := svc.RejectQuotation(ctx, quotation.ID, 200, reason)
	require.NoError(t, err)
	assert.Equal(t, QuotationStatusRejected, rejected.Status)
	assert.NotNil(t, rejected.RejectedBy)
	assert.NotNil(t, rejected.RejectedAt)
	assert.NotNil(t, rejected.RejectionReason)
	assert.Equal(t, reason, *rejected.RejectionReason)
}

func TestUpdateQuotation(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create quotation
	quotation := createTestQuotation(t, svc, ctx)

	// Update quotation
	newValidUntil := time.Now().AddDate(0, 0, 60)
	newNote := "Updated notes"
	updateReq := UpdateQuotationRequest{
		ValidUntil: &newValidUntil,
		Notes:      &newNote,
	}

	updated, err := svc.UpdateQuotation(ctx, quotation.ID, updateReq)
	require.NoError(t, err)
	assert.NotNil(t, updated.Notes)
	assert.Equal(t, newNote, *updated.Notes)
}

// ============================================================================
// SALES ORDER TESTS
// ============================================================================

func TestCreateSalesOrder(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create customer first
	customer := createTestCustomer(t, svc, ctx)

	// Create sales order
	orderDate := time.Now()
	deliveryDate := orderDate.AddDate(0, 0, 7)

	req := CreateSalesOrderRequest{
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

	order, err := svc.CreateSalesOrder(ctx, req, 100)
	require.NoError(t, err)
	require.NotNil(t, order)

	assert.Equal(t, int64(1), order.ID)
	assert.Equal(t, SalesOrderStatusDraft, order.Status)
	assert.Equal(t, customer.ID, order.CustomerID)
	assert.Equal(t, "USD", order.Currency)
	assert.Len(t, order.Lines, 1)
}

func TestConvertQuotationToSalesOrder(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create and approve quotation
	quotation := createTestQuotation(t, svc, ctx)
	_, err := svc.SubmitQuotation(ctx, quotation.ID, 100)
	require.NoError(t, err)
	_, err = svc.ApproveQuotation(ctx, quotation.ID, 200)
	require.NoError(t, err)

	// Convert to sales order
	orderDate := time.Now()

	order, err := svc.ConvertQuotationToSalesOrder(ctx, quotation.ID, 100, orderDate)
	require.NoError(t, err)
	require.NotNil(t, order)

	assert.Equal(t, SalesOrderStatusDraft, order.Status)
	assert.NotNil(t, order.QuotationID)
	assert.Equal(t, quotation.ID, *order.QuotationID)
	assert.Equal(t, quotation.CustomerID, order.CustomerID)
	assert.Equal(t, quotation.Currency, order.Currency)
	assert.Equal(t, quotation.TotalAmount, order.TotalAmount)
	assert.Len(t, order.Lines, len(quotation.Lines))

	// Verify quotation is marked as converted
	convertedQuote, err := svc.GetQuotation(ctx, quotation.ID)
	require.NoError(t, err)
	assert.Equal(t, QuotationStatusConverted, convertedQuote.Status)
}

func TestConfirmSalesOrder(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create sales order
	order := createTestSalesOrder(t, svc, ctx)

	// Confirm order
	confirmed, err := svc.ConfirmSalesOrder(ctx, order.ID, 100)
	require.NoError(t, err)
	assert.Equal(t, SalesOrderStatusConfirmed, confirmed.Status)
	assert.NotNil(t, confirmed.ConfirmedBy)
	assert.NotNil(t, confirmed.ConfirmedAt)
	assert.Equal(t, int64(100), *confirmed.ConfirmedBy)
}

func TestCancelSalesOrder(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create sales order
	order := createTestSalesOrder(t, svc, ctx)

	// Cancel order
	reason := "Customer requested cancellation"
	cancelled, err := svc.CancelSalesOrder(ctx, order.ID, 100, reason)
	require.NoError(t, err)
	assert.Equal(t, SalesOrderStatusCancelled, cancelled.Status)
	assert.NotNil(t, cancelled.CancelledBy)
	assert.NotNil(t, cancelled.CancelledAt)
	assert.NotNil(t, cancelled.CancellationReason)
	assert.Equal(t, reason, *cancelled.CancellationReason)
}

func TestUpdateSalesOrder(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create sales order
	order := createTestSalesOrder(t, svc, ctx)

	// Update order
	newNote := "Updated delivery instructions"
	updateReq := UpdateSalesOrderRequest{
		Notes: &newNote,
	}

	updated, err := svc.UpdateSalesOrder(ctx, order.ID, updateReq)
	require.NoError(t, err)
	assert.NotNil(t, updated.Notes)
	assert.Equal(t, newNote, *updated.Notes)
}

func TestListSalesOrders(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Create multiple sales orders
	for i := 0; i < 3; i++ {
		_ = createTestSalesOrder(t, svc, ctx)
	}

	// List sales orders
	listReq := ListSalesOrdersRequest{
		CompanyID: 1,
		Limit:     10,
		Offset:    0,
	}
	orders, total, err := svc.ListSalesOrders(ctx, listReq)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, orders, 3)
}

// ============================================================================
// HELPER FUNCTIONS FOR TESTS
// ============================================================================

func createTestCustomer(t *testing.T, svc *testService, ctx context.Context) *Customer {
	t.Helper()
	// Generate unique customer code using timestamp to avoid conflicts
	uniqueCode := fmt.Sprintf("CUST%d", time.Now().UnixNano()%1000000)
	req := CreateCustomerRequest{
		Code:             uniqueCode,
		Name:             "Test Customer",
		CompanyID:        1,
		CreditLimit:      50000.00,
		PaymentTermsDays: 30,
		Country:          "US",
	}
	customer, err := svc.CreateCustomer(ctx, req, 100)
	require.NoError(t, err)
	return customer
}

func createTestQuotation(t *testing.T, svc *testService, ctx context.Context) *Quotation {
	t.Helper()
	customer := createTestCustomer(t, svc, ctx)

	quoteDate := time.Now()
	validUntil := quoteDate.AddDate(0, 0, 30)

	req := CreateQuotationRequest{
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

	quotation, err := svc.CreateQuotation(ctx, req, 100)
	require.NoError(t, err)
	return quotation
}

func createTestSalesOrder(t *testing.T, svc *testService, ctx context.Context) *SalesOrder {
	t.Helper()
	customer := createTestCustomer(t, svc, ctx)

	orderDate := time.Now()
	deliveryDate := orderDate.AddDate(0, 0, 7)

	req := CreateSalesOrderRequest{
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

	order, err := svc.CreateSalesOrder(ctx, req, 100)
	require.NoError(t, err)
	return order
}
