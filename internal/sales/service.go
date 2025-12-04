package sales

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service provides business logic for sales operations.
type Service struct {
	repo *Repository
	pool *pgxpool.Pool
}

// NewService constructs a sales service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		repo: NewRepository(pool),
		pool: pool,
	}
}

// ============================================================================
// CUSTOMER OPERATIONS
// ============================================================================

// CreateCustomer creates a new customer.
func (s *Service) CreateCustomer(ctx context.Context, req CreateCustomerRequest, createdBy int64) (*Customer, error) {
	// Check if code already exists
	existing, err := s.repo.GetCustomerByCode(ctx, req.CompanyID, req.Code)
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
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
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

// UpdateCustomer updates an existing customer.
func (s *Service) UpdateCustomer(ctx context.Context, id int64, req UpdateCustomerRequest) (*Customer, error) {
	// Check if customer exists
	existing, err := s.repo.GetCustomer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get customer: %w", err)
	}

	// Build updates map
	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.TaxID != nil {
		updates["tax_id"] = *req.TaxID
	}
	if req.CreditLimit != nil {
		updates["credit_limit"] = *req.CreditLimit
	}
	if req.PaymentTermsDays != nil {
		updates["payment_terms_days"] = *req.PaymentTermsDays
	}
	if req.AddressLine1 != nil {
		updates["address_line1"] = *req.AddressLine1
	}
	if req.AddressLine2 != nil {
		updates["address_line2"] = *req.AddressLine2
	}
	if req.City != nil {
		updates["city"] = *req.City
	}
	if req.State != nil {
		updates["state"] = *req.State
	}
	if req.PostalCode != nil {
		updates["postal_code"] = *req.PostalCode
	}
	if req.Country != nil {
		updates["country"] = *req.Country
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if req.Notes != nil {
		updates["notes"] = *req.Notes
	}

	if len(updates) == 0 {
		return existing, nil
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateCustomer(ctx, id, updates)
	})
	if err != nil {
		return nil, fmt.Errorf("update customer: %w", err)
	}

	return s.repo.GetCustomer(ctx, id)
}

// GetCustomer retrieves a customer by ID.
func (s *Service) GetCustomer(ctx context.Context, id int64) (*Customer, error) {
	return s.repo.GetCustomer(ctx, id)
}

// ListCustomers returns a paginated list of customers.
func (s *Service) ListCustomers(ctx context.Context, req ListCustomersRequest) ([]Customer, int, error) {
	return s.repo.ListCustomers(ctx, req)
}

// GenerateCustomerCode generates a unique customer code.
func (s *Service) GenerateCustomerCode(ctx context.Context, companyID int64) (string, error) {
	return s.repo.GenerateCustomerCode(ctx, companyID)
}

// ============================================================================
// QUOTATION OPERATIONS
// ============================================================================

// CreateQuotation creates a new quotation.
func (s *Service) CreateQuotation(ctx context.Context, req CreateQuotationRequest, createdBy int64) (*Quotation, error) {
	// Validate dates
	if req.ValidUntil.Before(req.QuoteDate) {
		return nil, errors.New("valid_until must be after quote_date")
	}

	// Verify customer exists
	_, err := s.repo.GetCustomer(ctx, req.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("verify customer: %w", err)
	}

	// Generate document number
	docNumber, err := s.repo.GenerateQuotationNumber(ctx, req.CompanyID, req.QuoteDate)
	if err != nil {
		return nil, fmt.Errorf("generate doc number: %w", err)
	}

	// Calculate totals
	var subtotal, taxAmount, totalAmount float64
	for _, lineReq := range req.Lines {
		discount, tax, lineTotal := CalculateLineTotals(
			lineReq.Quantity,
			lineReq.UnitPrice,
			lineReq.DiscountPercent,
			lineReq.TaxPercent,
		)
		subtotal += (lineReq.Quantity * lineReq.UnitPrice) - discount
		taxAmount += tax
		totalAmount += lineTotal
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
		TotalAmount: totalAmount,
		Notes:       req.Notes,
		CreatedBy:   createdBy,
	}

	var quotationID int64
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		// Create quotation
		id, err := tx.CreateQuotation(ctx, quotation)
		if err != nil {
			return fmt.Errorf("create quotation: %w", err)
		}
		quotationID = id

		// Insert lines
		for i, lineReq := range req.Lines {
			discount, tax, lineTotal := CalculateLineTotals(
				lineReq.Quantity,
				lineReq.UnitPrice,
				lineReq.DiscountPercent,
				lineReq.TaxPercent,
			)

			line := QuotationLine{
				QuotationID:     quotationID,
				ProductID:       lineReq.ProductID,
				Description:     lineReq.Description,
				Quantity:        lineReq.Quantity,
				UOM:             lineReq.UOM,
				UnitPrice:       lineReq.UnitPrice,
				DiscountPercent: lineReq.DiscountPercent,
				DiscountAmount:  discount,
				TaxPercent:      lineReq.TaxPercent,
				TaxAmount:       tax,
				LineTotal:       lineTotal,
				Notes:           lineReq.Notes,
				LineOrder:       lineReq.LineOrder,
			}
			if line.LineOrder == 0 {
				line.LineOrder = i + 1
			}

			_, err := tx.InsertQuotationLine(ctx, line)
			if err != nil {
				return fmt.Errorf("insert quotation line: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.repo.GetQuotation(ctx, quotationID)
}

// UpdateQuotation updates an existing quotation (only DRAFT status).
func (s *Service) UpdateQuotation(ctx context.Context, id int64, req UpdateQuotationRequest) (*Quotation, error) {
	// Get existing quotation
	existing, err := s.repo.GetQuotation(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get quotation: %w", err)
	}

	// Only DRAFT quotations can be updated
	if existing.Status != QuotationStatusDraft {
		return nil, fmt.Errorf("%w: only DRAFT quotations can be updated", ErrInvalidStatus)
	}

	// Update basic fields if provided
	if req.QuoteDate != nil {
		existing.QuoteDate = *req.QuoteDate
	}
	if req.ValidUntil != nil {
		existing.ValidUntil = *req.ValidUntil
	}
	if req.Notes != nil {
		existing.Notes = req.Notes
	}

	// Validate dates
	if existing.ValidUntil.Before(existing.QuoteDate) {
		return nil, errors.New("valid_until must be after quote_date")
	}

	// If lines are provided, recalculate everything
	if req.Lines != nil {
		var subtotal, taxAmount, totalAmount float64
		for _, lineReq := range *req.Lines {
			discount, tax, lineTotal := CalculateLineTotals(
				lineReq.Quantity,
				lineReq.UnitPrice,
				lineReq.DiscountPercent,
				lineReq.TaxPercent,
			)
			subtotal += (lineReq.Quantity * lineReq.UnitPrice) - discount
			taxAmount += tax
			totalAmount += lineTotal
		}

		existing.Subtotal = subtotal
		existing.TaxAmount = taxAmount
		existing.TotalAmount = totalAmount

		err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			// Delete existing lines
			if err := tx.DeleteQuotationLines(ctx, id); err != nil {
				return fmt.Errorf("delete quotation lines: %w", err)
			}

			// Insert new lines
			for i, lineReq := range *req.Lines {
				discount, tax, lineTotal := CalculateLineTotals(
					lineReq.Quantity,
					lineReq.UnitPrice,
					lineReq.DiscountPercent,
					lineReq.TaxPercent,
				)

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
					TaxAmount:       tax,
					LineTotal:       lineTotal,
					Notes:           lineReq.Notes,
					LineOrder:       lineReq.LineOrder,
				}
				if line.LineOrder == 0 {
					line.LineOrder = i + 1
				}

				_, err := tx.InsertQuotationLine(ctx, line)
				if err != nil {
					return fmt.Errorf("insert quotation line: %w", err)
				}
			}

			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return s.repo.GetQuotation(ctx, id)
}

// SubmitQuotation changes quotation status from DRAFT to SUBMITTED.
func (s *Service) SubmitQuotation(ctx context.Context, id int64, userID int64) (*Quotation, error) {
	existing, err := s.repo.GetQuotation(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get quotation: %w", err)
	}

	if existing.Status != QuotationStatusDraft {
		return nil, fmt.Errorf("%w: can only submit DRAFT quotations", ErrInvalidStatus)
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateQuotationStatus(ctx, id, QuotationStatusSubmitted, userID, nil)
	})
	if err != nil {
		return nil, fmt.Errorf("submit quotation: %w", err)
	}

	return s.repo.GetQuotation(ctx, id)
}

// ApproveQuotation approves a quotation (SUBMITTED → APPROVED).
func (s *Service) ApproveQuotation(ctx context.Context, id int64, approvedBy int64) (*Quotation, error) {
	existing, err := s.repo.GetQuotation(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get quotation: %w", err)
	}

	if existing.Status != QuotationStatusSubmitted {
		return nil, fmt.Errorf("%w: can only approve SUBMITTED quotations", ErrInvalidStatus)
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateQuotationStatus(ctx, id, QuotationStatusApproved, approvedBy, nil)
	})
	if err != nil {
		return nil, fmt.Errorf("approve quotation: %w", err)
	}

	return s.repo.GetQuotation(ctx, id)
}

// RejectQuotation rejects a quotation (SUBMITTED → REJECTED).
func (s *Service) RejectQuotation(ctx context.Context, id int64, rejectedBy int64, reason string) (*Quotation, error) {
	existing, err := s.repo.GetQuotation(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get quotation: %w", err)
	}

	if existing.Status != QuotationStatusSubmitted {
		return nil, fmt.Errorf("%w: can only reject SUBMITTED quotations", ErrInvalidStatus)
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateQuotationStatus(ctx, id, QuotationStatusRejected, rejectedBy, &reason)
	})
	if err != nil {
		return nil, fmt.Errorf("reject quotation: %w", err)
	}

	return s.repo.GetQuotation(ctx, id)
}

// GetQuotation retrieves a quotation by ID.
func (s *Service) GetQuotation(ctx context.Context, id int64) (*Quotation, error) {
	return s.repo.GetQuotation(ctx, id)
}

// ListQuotations returns a paginated list of quotations.
func (s *Service) ListQuotations(ctx context.Context, req ListQuotationsRequest) ([]QuotationWithDetails, int, error) {
	return s.repo.ListQuotations(ctx, req)
}

// ============================================================================
// SALES ORDER OPERATIONS
// ============================================================================

// CreateSalesOrder creates a new sales order.
func (s *Service) CreateSalesOrder(ctx context.Context, req CreateSalesOrderRequest, createdBy int64) (*SalesOrder, error) {
	// Verify customer exists
	_, err := s.repo.GetCustomer(ctx, req.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("verify customer: %w", err)
	}

	// Generate document number
	docNumber, err := s.repo.GenerateSalesOrderNumber(ctx, req.CompanyID, req.OrderDate)
	if err != nil {
		return nil, fmt.Errorf("generate doc number: %w", err)
	}

	// Calculate totals
	var subtotal, taxAmount, totalAmount float64
	for _, lineReq := range req.Lines {
		discount, tax, lineTotal := CalculateLineTotals(
			lineReq.Quantity,
			lineReq.UnitPrice,
			lineReq.DiscountPercent,
			lineReq.TaxPercent,
		)
		subtotal += (lineReq.Quantity * lineReq.UnitPrice) - discount
		taxAmount += tax
		totalAmount += lineTotal
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
		TotalAmount:          totalAmount,
		Notes:                req.Notes,
		CreatedBy:            createdBy,
	}

	var orderID int64
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		// Create sales order
		id, err := tx.CreateSalesOrder(ctx, order)
		if err != nil {
			return fmt.Errorf("create sales order: %w", err)
		}
		orderID = id

		// Insert lines
		for i, lineReq := range req.Lines {
			discount, tax, lineTotal := CalculateLineTotals(
				lineReq.Quantity,
				lineReq.UnitPrice,
				lineReq.DiscountPercent,
				lineReq.TaxPercent,
			)

			line := SalesOrderLine{
				SalesOrderID:      orderID,
				ProductID:         lineReq.ProductID,
				Description:       lineReq.Description,
				Quantity:          lineReq.Quantity,
				QuantityDelivered: 0,
				QuantityInvoiced:  0,
				UOM:               lineReq.UOM,
				UnitPrice:         lineReq.UnitPrice,
				DiscountPercent:   lineReq.DiscountPercent,
				DiscountAmount:    discount,
				TaxPercent:        lineReq.TaxPercent,
				TaxAmount:         tax,
				LineTotal:         lineTotal,
				Notes:             lineReq.Notes,
				LineOrder:         lineReq.LineOrder,
			}
			if line.LineOrder == 0 {
				line.LineOrder = i + 1
			}

			_, err := tx.InsertSalesOrderLine(ctx, line)
			if err != nil {
				return fmt.Errorf("insert sales order line: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.repo.GetSalesOrder(ctx, orderID)
}

// ConvertQuotationToSalesOrder converts an approved quotation to a sales order.
func (s *Service) ConvertQuotationToSalesOrder(ctx context.Context, quotationID int64, createdBy int64, orderDate time.Time) (*SalesOrder, error) {
	// Get quotation
	quotation, err := s.repo.GetQuotation(ctx, quotationID)
	if err != nil {
		return nil, fmt.Errorf("get quotation: %w", err)
	}

	// Only APPROVED quotations can be converted
	if quotation.Status != QuotationStatusApproved {
		return nil, fmt.Errorf("%w: can only convert APPROVED quotations", ErrInvalidStatus)
	}

	// Check if quotation is still valid
	if time.Now().After(quotation.ValidUntil) {
		return nil, errors.New("quotation has expired")
	}

	// Generate document number
	docNumber, err := s.repo.GenerateSalesOrderNumber(ctx, quotation.CompanyID, orderDate)
	if err != nil {
		return nil, fmt.Errorf("generate doc number: %w", err)
	}

	order := SalesOrder{
		DocNumber:            docNumber,
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
		Notes:                quotation.Notes,
		CreatedBy:            createdBy,
	}

	var orderID int64
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		// Create sales order
		id, err := tx.CreateSalesOrder(ctx, order)
		if err != nil {
			return fmt.Errorf("create sales order: %w", err)
		}
		orderID = id

		// Copy lines from quotation
		for _, quoteLine := range quotation.Lines {
			line := SalesOrderLine{
				SalesOrderID:      orderID,
				ProductID:         quoteLine.ProductID,
				Description:       quoteLine.Description,
				Quantity:          quoteLine.Quantity,
				QuantityDelivered: 0,
				QuantityInvoiced:  0,
				UOM:               quoteLine.UOM,
				UnitPrice:         quoteLine.UnitPrice,
				DiscountPercent:   quoteLine.DiscountPercent,
				DiscountAmount:    quoteLine.DiscountAmount,
				TaxPercent:        quoteLine.TaxPercent,
				TaxAmount:         quoteLine.TaxAmount,
				LineTotal:         quoteLine.LineTotal,
				Notes:             quoteLine.Notes,
				LineOrder:         quoteLine.LineOrder,
			}

			_, err := tx.InsertSalesOrderLine(ctx, line)
			if err != nil {
				return fmt.Errorf("insert sales order line: %w", err)
			}
		}

		// Mark quotation as CONVERTED
		err = tx.UpdateQuotationStatus(ctx, quotationID, QuotationStatusConverted, createdBy, nil)
		if err != nil {
			return fmt.Errorf("update quotation status: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.repo.GetSalesOrder(ctx, orderID)
}

// UpdateSalesOrder updates an existing sales order (only DRAFT status).
func (s *Service) UpdateSalesOrder(ctx context.Context, id int64, req UpdateSalesOrderRequest) (*SalesOrder, error) {
	// Get existing order
	existing, err := s.repo.GetSalesOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get sales order: %w", err)
	}

	// Only DRAFT orders can be updated
	if existing.Status != SalesOrderStatusDraft {
		return nil, fmt.Errorf("%w: only DRAFT sales orders can be updated", ErrInvalidStatus)
	}

	// Update basic fields if provided
	if req.OrderDate != nil {
		existing.OrderDate = *req.OrderDate
	}
	if req.ExpectedDeliveryDate != nil {
		existing.ExpectedDeliveryDate = req.ExpectedDeliveryDate
	}
	if req.Notes != nil {
		existing.Notes = req.Notes
	}

	// If lines are provided, recalculate everything
	if req.Lines != nil {
		var subtotal, taxAmount, totalAmount float64
		for _, lineReq := range *req.Lines {
			discount, tax, lineTotal := CalculateLineTotals(
				lineReq.Quantity,
				lineReq.UnitPrice,
				lineReq.DiscountPercent,
				lineReq.TaxPercent,
			)
			subtotal += (lineReq.Quantity * lineReq.UnitPrice) - discount
			taxAmount += tax
			totalAmount += lineTotal
		}

		existing.Subtotal = subtotal
		existing.TaxAmount = taxAmount
		existing.TotalAmount = totalAmount

		err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			// Delete existing lines
			if err := tx.DeleteSalesOrderLines(ctx, id); err != nil {
				return fmt.Errorf("delete sales order lines: %w", err)
			}

			// Insert new lines
			for i, lineReq := range *req.Lines {
				discount, tax, lineTotal := CalculateLineTotals(
					lineReq.Quantity,
					lineReq.UnitPrice,
					lineReq.DiscountPercent,
					lineReq.TaxPercent,
				)

				line := SalesOrderLine{
					SalesOrderID:      id,
					ProductID:         lineReq.ProductID,
					Description:       lineReq.Description,
					Quantity:          lineReq.Quantity,
					QuantityDelivered: 0,
					QuantityInvoiced:  0,
					UOM:               lineReq.UOM,
					UnitPrice:         lineReq.UnitPrice,
					DiscountPercent:   lineReq.DiscountPercent,
					DiscountAmount:    discount,
					TaxPercent:        lineReq.TaxPercent,
					TaxAmount:         tax,
					LineTotal:         lineTotal,
					Notes:             lineReq.Notes,
					LineOrder:         lineReq.LineOrder,
				}
				if line.LineOrder == 0 {
					line.LineOrder = i + 1
				}

				_, err := tx.InsertSalesOrderLine(ctx, line)
				if err != nil {
					return fmt.Errorf("insert sales order line: %w", err)
				}
			}

			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return s.repo.GetSalesOrder(ctx, id)
}

// ConfirmSalesOrder confirms a sales order (DRAFT → CONFIRMED).
func (s *Service) ConfirmSalesOrder(ctx context.Context, id int64, confirmedBy int64) (*SalesOrder, error) {
	existing, err := s.repo.GetSalesOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get sales order: %w", err)
	}

	if existing.Status != SalesOrderStatusDraft {
		return nil, fmt.Errorf("%w: can only confirm DRAFT sales orders", ErrInvalidStatus)
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateSalesOrderStatus(ctx, id, SalesOrderStatusConfirmed, confirmedBy, nil)
	})
	if err != nil {
		return nil, fmt.Errorf("confirm sales order: %w", err)
	}

	return s.repo.GetSalesOrder(ctx, id)
}

// CancelSalesOrder cancels a sales order.
func (s *Service) CancelSalesOrder(ctx context.Context, id int64, cancelledBy int64, reason string) (*SalesOrder, error) {
	existing, err := s.repo.GetSalesOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get sales order: %w", err)
	}

	// Can't cancel already completed or cancelled orders
	if existing.Status == SalesOrderStatusCompleted || existing.Status == SalesOrderStatusCancelled {
		return nil, fmt.Errorf("%w: cannot cancel %s sales orders", ErrInvalidStatus, existing.Status)
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateSalesOrderStatus(ctx, id, SalesOrderStatusCancelled, cancelledBy, &reason)
	})
	if err != nil {
		return nil, fmt.Errorf("cancel sales order: %w", err)
	}

	return s.repo.GetSalesOrder(ctx, id)
}

// GetSalesOrder retrieves a sales order by ID.
func (s *Service) GetSalesOrder(ctx context.Context, id int64) (*SalesOrder, error) {
	return s.repo.GetSalesOrder(ctx, id)
}

// ListSalesOrders returns a paginated list of sales orders.
func (s *Service) ListSalesOrders(ctx context.Context, req ListSalesOrdersRequest) ([]SalesOrderWithDetails, int, error) {
	return s.repo.ListSalesOrders(ctx, req)
}
