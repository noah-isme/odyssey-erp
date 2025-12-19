package sales

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	salesdb "github.com/odyssey-erp/odyssey-erp/internal/sales/db"
)

var (
	ErrNotFound      = errors.New("record not found")
	ErrInvalidStatus = errors.New("invalid status transition")
	ErrAlreadyExists = errors.New("record already exists")
)

// Repository provides PostgreSQL backed persistence for sales operations.
type Repository struct {
	pool    *pgxpool.Pool
	queries *salesdb.Queries
}

// NewRepository constructs a repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: salesdb.New(pool),
	}
}

// TxRepository exposes transactional operations.
type TxRepository interface {
	// Customer operations
	CreateCustomer(ctx context.Context, customer Customer) (int64, error)
	UpdateCustomer(ctx context.Context, id int64, updates map[string]interface{}) error

	// Quotation operations
	CreateQuotation(ctx context.Context, quotation Quotation) (int64, error)
	InsertQuotationLine(ctx context.Context, line QuotationLine) (int64, error)
	UpdateQuotationStatus(ctx context.Context, id int64, status QuotationStatus, userID int64, reason *string) error
	DeleteQuotationLines(ctx context.Context, quotationID int64) error

	// Sales Order operations
	CreateSalesOrder(ctx context.Context, order SalesOrder) (int64, error)
	InsertSalesOrderLine(ctx context.Context, line SalesOrderLine) (int64, error)
	UpdateSalesOrderStatus(ctx context.Context, id int64, status SalesOrderStatus, userID int64, reason *string) error
	DeleteSalesOrderLines(ctx context.Context, salesOrderID int64) error
	UpdateSalesOrderLineDelivered(ctx context.Context, lineID int64, quantityDelivered float64) error
}

type txRepo struct {
	queries *salesdb.Queries
	tx      pgx.Tx
}

// WithTx wraps callback in repeatable-read transaction.
func (r *Repository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	wrapper := &txRepo{
		queries: r.queries.WithTx(tx),
		tx:      tx,
	}
	if err := fn(ctx, wrapper); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// ============================================================================
// CUSTOMER OPERATIONS
// ============================================================================

func (r *Repository) GetCustomer(ctx context.Context, id int64) (*Customer, error) {
	row, err := r.queries.GetCustomer(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c := Customer{
		ID:               row.ID,
		Code:             row.Code,
		Name:             row.Name,
		CompanyID:        row.CompanyID,
		PaymentTermsDays: int(row.PaymentTermsDays),
		Country:          row.Country,
		IsActive:         row.IsActive,
		CreatedBy:        row.CreatedBy,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
	if row.TaxID.Valid {
		val := row.TaxID.String
		c.TaxID = &val
	}
	if row.Email.Valid {
		val := row.Email.String
		c.Email = &val
	}
	if row.Phone.Valid {
		val := row.Phone.String
		c.Phone = &val
	}
	if row.CreditLimit.Valid {
		f, _ := row.CreditLimit.Float64Value()
		c.CreditLimit = f.Float64
	}
	if row.AddressLine1.Valid {
		val := row.AddressLine1.String
		c.AddressLine1 = &val
	}
	if row.AddressLine2.Valid {
		val := row.AddressLine2.String
		c.AddressLine2 = &val
	}
	if row.City.Valid {
		val := row.City.String
		c.City = &val
	}
	if row.State.Valid {
		val := row.State.String
		c.State = &val
	}
	if row.PostalCode.Valid {
		val := row.PostalCode.String
		c.PostalCode = &val
	}
	if row.Notes.Valid {
		val := row.Notes.String
		c.Notes = &val
	}
	return &c, nil
}

func (r *Repository) GetCustomerByCode(ctx context.Context, companyID int64, code string) (*Customer, error) {
	row, err := r.queries.GetCustomerByCode(ctx, salesdb.GetCustomerByCodeParams{
		CompanyID: companyID,
		Code:      code,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c := Customer{
		ID:               row.ID,
		Code:             row.Code,
		Name:             row.Name,
		CompanyID:        row.CompanyID,
		PaymentTermsDays: int(row.PaymentTermsDays),
		Country:          row.Country,
		IsActive:         row.IsActive,
		CreatedBy:        row.CreatedBy,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
	if row.TaxID.Valid {
		val := row.TaxID.String
		c.TaxID = &val
	}
	if row.Email.Valid {
		val := row.Email.String
		c.Email = &val
	}
	if row.Phone.Valid {
		val := row.Phone.String
		c.Phone = &val
	}
	if row.CreditLimit.Valid {
		f, _ := row.CreditLimit.Float64Value()
		c.CreditLimit = f.Float64
	}
	if row.AddressLine1.Valid {
		val := row.AddressLine1.String
		c.AddressLine1 = &val
	}
	if row.AddressLine2.Valid {
		val := row.AddressLine2.String
		c.AddressLine2 = &val
	}
	if row.City.Valid {
		val := row.City.String
		c.City = &val
	}
	if row.State.Valid {
		val := row.State.String
		c.State = &val
	}
	if row.PostalCode.Valid {
		val := row.PostalCode.String
		c.PostalCode = &val
	}
	if row.Notes.Valid {
		val := row.Notes.String
		c.Notes = &val
	}
	return &c, nil
}

func (r *Repository) ListCustomers(ctx context.Context, req ListCustomersRequest) ([]Customer, int, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	conditions = append(conditions, fmt.Sprintf("company_id = $%d", argPos))
	args = append(args, req.CompanyID)
	argPos++

	if req.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *req.IsActive)
		argPos++
	}

	if req.Search != nil && *req.Search != "" {
		searchPattern := "%" + *req.Search + "%"
		conditions = append(conditions, fmt.Sprintf("(code ILIKE $%d OR name ILIKE $%d OR email ILIKE $%d)", argPos, argPos, argPos))
		args = append(args, searchPattern)
		argPos++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + conditions[0]
		for i := 1; i < len(conditions); i++ {
			whereClause += " AND " + conditions[i]
		}
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM customers %s", whereClause)
	var total int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Fetch records
	query := fmt.Sprintf(`
		SELECT id, code, name, company_id, email, phone, tax_id,
		       credit_limit, payment_terms_days, address_line1, address_line2,
		       city, state, postal_code, country, is_active, notes,
		       created_by, created_at, updated_at
		FROM customers
		%s
		ORDER BY code
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, req.Limit, req.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var c Customer
		err := rows.Scan(
			&c.ID, &c.Code, &c.Name, &c.CompanyID, &c.Email, &c.Phone, &c.TaxID,
			&c.CreditLimit, &c.PaymentTermsDays, &c.AddressLine1, &c.AddressLine2,
			&c.City, &c.State, &c.PostalCode, &c.Country, &c.IsActive, &c.Notes,
			&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		customers = append(customers, c)
	}

	return customers, total, rows.Err()
}

func (t *txRepo) CreateCustomer(ctx context.Context, customer Customer) (int64, error) {
	var creditLimit pgtype.Numeric
	creditLimit.Scan(fmt.Sprintf("%f", customer.CreditLimit))
	
	return t.queries.CreateCustomer(ctx, salesdb.CreateCustomerParams{
		Code:             customer.Code,
		Name:             customer.Name,
		CompanyID:        customer.CompanyID,
		Email:            pgtype.Text{String: getString(customer.Email), Valid: customer.Email != nil},
		Phone:            pgtype.Text{String: getString(customer.Phone), Valid: customer.Phone != nil},
		TaxID:            pgtype.Text{String: getString(customer.TaxID), Valid: customer.TaxID != nil},
		CreditLimit:      creditLimit,
		PaymentTermsDays: int32(customer.PaymentTermsDays),
		AddressLine1:     pgtype.Text{String: getString(customer.AddressLine1), Valid: customer.AddressLine1 != nil},
		AddressLine2:     pgtype.Text{String: getString(customer.AddressLine2), Valid: customer.AddressLine2 != nil},
		City:             pgtype.Text{String: getString(customer.City), Valid: customer.City != nil},
		State:            pgtype.Text{String: getString(customer.State), Valid: customer.State != nil},
		PostalCode:       pgtype.Text{String: getString(customer.PostalCode), Valid: customer.PostalCode != nil},
		Country:          customer.Country,
		IsActive:         customer.IsActive,
		Notes:            pgtype.Text{String: getString(customer.Notes), Valid: customer.Notes != nil},
		CreatedBy:        customer.CreatedBy,
	})
}

func (t *txRepo) UpdateCustomer(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	params := salesdb.UpdateCustomerParams{ID: id}
	
	if v, ok := updates["name"].(string); ok {
		params.Name = pgtype.Text{String: v, Valid: true}
	}
	if v, ok := updates["email"].(string); ok {
		params.Email = pgtype.Text{String: v, Valid: true}
	}
	if v, ok := updates["phone"].(string); ok {
		params.Phone = pgtype.Text{String: v, Valid: true}
	}
	if v, ok := updates["address_line1"].(string); ok {
		params.AddressLine1 = pgtype.Text{String: v, Valid: true}
	}
	if v, ok := updates["address_line2"].(string); ok {
		params.AddressLine2 = pgtype.Text{String: v, Valid: true}
	}
	if v, ok := updates["city"].(string); ok {
		params.City = pgtype.Text{String: v, Valid: true}
	}
	if v, ok := updates["state"].(string); ok {
		params.State = pgtype.Text{String: v, Valid: true}
	}
	if v, ok := updates["postal_code"].(string); ok {
		params.PostalCode = pgtype.Text{String: v, Valid: true}
	}
	if v, ok := updates["country"].(string); ok {
		params.Country = pgtype.Text{String: v, Valid: true}
	}
	if v, ok := updates["is_active"].(bool); ok {
		params.IsActive = pgtype.Bool{Bool: v, Valid: true}
	}
	if v, ok := updates["notes"].(string); ok {
		params.Notes = pgtype.Text{String: v, Valid: true}
	}

	// This assumes that fields not in the map are not updated (COALESCE/NULL handling in query)
	// But generated Params fields are zero-valued (Valid=false) by default if not set above,
	// so sqlc query using COALESCE(NULL, old_val) works correctly.
	
	// However, we can't easily check RowsAffected with pure sqlc :exec unless we check err.
	// But Update query with ID should return RowsAffected if we used Exec on driver.
	// sqlc generated usage of db.Exec returns error only.
	// We can rely on error being nil = success. ErrNoRows is impossible for UPDATE usually unless checking logic.
	
	return t.queries.UpdateCustomer(ctx, params)
}

// ============================================================================
// QUOTATION OPERATIONS
// ============================================================================

func (r *Repository) GetQuotation(ctx context.Context, id int64) (*Quotation, error) {
	row, err := r.queries.GetQuotation(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	q := Quotation{
		ID:          row.ID,
		DocNumber:   row.DocNumber,
		CompanyID:   row.CompanyID,
		CustomerID:  row.CustomerID,
		Status:      QuotationStatus(row.Status),
		Currency:    row.Currency,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
	if row.QuoteDate.Valid {
		q.QuoteDate = row.QuoteDate.Time
	}
	if row.ValidUntil.Valid {
		q.ValidUntil = row.ValidUntil.Time
	}
	if row.Subtotal.Valid {
		f, _ := row.Subtotal.Float64Value()
		q.Subtotal = f.Float64
	}
	if row.TaxAmount.Valid {
		f, _ := row.TaxAmount.Float64Value()
		q.TaxAmount = f.Float64
	}
	if row.TotalAmount.Valid {
		f, _ := row.TotalAmount.Float64Value()
		q.TotalAmount = f.Float64
	}
	if row.Notes.Valid {
		val := row.Notes.String
		q.Notes = &val
	}
	if row.ApprovedBy.Valid {
		val := row.ApprovedBy.Int64
		q.ApprovedBy = &val
	}
	if row.ApprovedAt.Valid {
		val := row.ApprovedAt.Time
		q.ApprovedAt = &val
	}
	if row.RejectedBy.Valid {
		val := row.RejectedBy.Int64
		q.RejectedBy = &val
	}
	if row.RejectedAt.Valid {
		val := row.RejectedAt.Time
		q.RejectedAt = &val
	}
	if row.RejectionReason.Valid {
		val := row.RejectionReason.String
		q.RejectionReason = &val
	}

	lineRows, err := r.queries.GetQuotationLines(ctx, id)
	if err != nil {
		return nil, err
	}
	var lines []QuotationLine
	for _, l := range lineRows {
		line := QuotationLine{
			ID:              l.ID,
			QuotationID:     l.QuotationID,
			ProductID:       l.ProductID,
			UOM:             l.Uom,
			LineOrder:       int(l.LineOrder),
		}
		if l.Description.Valid {
			val := l.Description.String
			line.Description = &val
		}
		if l.Quantity.Valid {
			f, _ := l.Quantity.Float64Value()
			line.Quantity = f.Float64
		}
		if l.UnitPrice.Valid {
			f, _ := l.UnitPrice.Float64Value()
			line.UnitPrice = f.Float64
		}
		if l.DiscountPercent.Valid {
			f, _ := l.DiscountPercent.Float64Value()
			line.DiscountPercent = f.Float64
		}
		if l.DiscountAmount.Valid {
			f, _ := l.DiscountAmount.Float64Value()
			line.DiscountAmount = f.Float64
		}
		if l.TaxPercent.Valid {
			f, _ := l.TaxPercent.Float64Value()
			line.TaxPercent = f.Float64
		}
		if l.TaxAmount.Valid {
			f, _ := l.TaxAmount.Float64Value()
			line.TaxAmount = f.Float64
		}
		if l.LineTotal.Valid {
			f, _ := l.LineTotal.Float64Value()
			line.LineTotal = f.Float64
		}
		if l.Notes.Valid {
			val := l.Notes.String
			line.Notes = &val
		}
		lines = append(lines, line)
	}
	q.Lines = lines
	return &q, nil
}

func (r *Repository) GetQuotationByDocNumber(ctx context.Context, docNumber string) (*Quotation, error) {
	row, err := r.queries.GetQuotationByDocNumber(ctx, docNumber)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	q := Quotation{
		ID:          row.ID,
		DocNumber:   row.DocNumber,
		CompanyID:   row.CompanyID,
		CustomerID:  row.CustomerID,
		Status:      QuotationStatus(row.Status),
		Currency:    row.Currency,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
	if row.QuoteDate.Valid {
		q.QuoteDate = row.QuoteDate.Time
	}
	if row.ValidUntil.Valid {
		q.ValidUntil = row.ValidUntil.Time
	}
	if row.Subtotal.Valid {
		f, _ := row.Subtotal.Float64Value()
		q.Subtotal = f.Float64
	}
	if row.TaxAmount.Valid {
		f, _ := row.TaxAmount.Float64Value()
		q.TaxAmount = f.Float64
	}
	if row.TotalAmount.Valid {
		f, _ := row.TotalAmount.Float64Value()
		q.TotalAmount = f.Float64
	}
	if row.Notes.Valid {
		val := row.Notes.String
		q.Notes = &val
	}
	if row.ApprovedBy.Valid {
		val := row.ApprovedBy.Int64
		q.ApprovedBy = &val
	}
	if row.ApprovedAt.Valid {
		val := row.ApprovedAt.Time
		q.ApprovedAt = &val
	}
	if row.RejectedBy.Valid {
		val := row.RejectedBy.Int64
		q.RejectedBy = &val
	}
	if row.RejectedAt.Valid {
		val := row.RejectedAt.Time
		q.RejectedAt = &val
	}
	if row.RejectionReason.Valid {
		val := row.RejectionReason.String
		q.RejectionReason = &val
	}

	lineRows, err := r.queries.GetQuotationLines(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	var lines []QuotationLine
	for _, l := range lineRows {
		line := QuotationLine{
			ID:              l.ID,
			QuotationID:     l.QuotationID,
			ProductID:       l.ProductID,
			UOM:             l.Uom,
			LineOrder:       int(l.LineOrder),
		}
		if l.Description.Valid {
			val := l.Description.String
			line.Description = &val
		}
		if l.Quantity.Valid {
			f, _ := l.Quantity.Float64Value()
			line.Quantity = f.Float64
		}
		if l.UnitPrice.Valid {
			f, _ := l.UnitPrice.Float64Value()
			line.UnitPrice = f.Float64
		}
		if l.DiscountPercent.Valid {
			f, _ := l.DiscountPercent.Float64Value()
			line.DiscountPercent = f.Float64
		}
		if l.DiscountAmount.Valid {
			f, _ := l.DiscountAmount.Float64Value()
			line.DiscountAmount = f.Float64
		}
		if l.TaxPercent.Valid {
			f, _ := l.TaxPercent.Float64Value()
			line.TaxPercent = f.Float64
		}
		if l.TaxAmount.Valid {
			f, _ := l.TaxAmount.Float64Value()
			line.TaxAmount = f.Float64
		}
		if l.LineTotal.Valid {
			f, _ := l.LineTotal.Float64Value()
			line.LineTotal = f.Float64
		}
		if l.Notes.Valid {
			val := l.Notes.String
			line.Notes = &val
		}
		lines = append(lines, line)
	}
	q.Lines = lines
	return &q, nil
}

func (r *Repository) ListQuotations(ctx context.Context, req ListQuotationsRequest) ([]QuotationWithDetails, int, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	conditions = append(conditions, fmt.Sprintf("q.company_id = $%d", argPos))
	args = append(args, req.CompanyID)
	argPos++

	if req.CustomerID != nil {
		conditions = append(conditions, fmt.Sprintf("q.customer_id = $%d", argPos))
		args = append(args, *req.CustomerID)
		argPos++
	}

	if req.Status != nil {
		conditions = append(conditions, fmt.Sprintf("q.status = $%d", argPos))
		args = append(args, *req.Status)
		argPos++
	}

	if req.DateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("q.quote_date >= $%d", argPos))
		args = append(args, *req.DateFrom)
		argPos++
	}

	if req.DateTo != nil {
		conditions = append(conditions, fmt.Sprintf("q.quote_date <= $%d", argPos))
		args = append(args, *req.DateTo)
		argPos++
	}

	whereClause := "WHERE " + conditions[0]
	for i := 1; i < len(conditions); i++ {
		whereClause += " AND " + conditions[i]
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM quotations q %s", whereClause)
	var total int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Fetch records
	query := fmt.Sprintf(`
		SELECT q.id, q.doc_number, q.company_id, q.customer_id, q.quote_date, q.valid_until,
		       q.status, q.currency, q.subtotal, q.tax_amount, q.total_amount, q.notes,
		       q.created_by, q.approved_by, q.approved_at, q.rejected_by, q.rejected_at,
		       q.rejection_reason, q.created_at, q.updated_at,
		       c.name as customer_name,
		       u1.full_name as created_by_name,
		       u2.full_name as approved_by_name,
		       u3.full_name as rejected_by_name
		FROM quotations q
		JOIN customers c ON q.customer_id = c.id
		JOIN users u1 ON q.created_by = u1.id
		LEFT JOIN users u2 ON q.approved_by = u2.id
		LEFT JOIN users u3 ON q.rejected_by = u3.id
		%s
		ORDER BY q.quote_date DESC, q.id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, req.Limit, req.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var quotations []QuotationWithDetails
	for rows.Next() {
		var q QuotationWithDetails
		err := rows.Scan(
			&q.ID, &q.DocNumber, &q.CompanyID, &q.CustomerID, &q.QuoteDate, &q.ValidUntil,
			&q.Status, &q.Currency, &q.Subtotal, &q.TaxAmount, &q.TotalAmount, &q.Notes,
			&q.CreatedBy, &q.ApprovedBy, &q.ApprovedAt, &q.RejectedBy, &q.RejectedAt,
			&q.RejectionReason, &q.CreatedAt, &q.UpdatedAt,
			&q.CustomerName, &q.CreatedByName, &q.ApprovedByName, &q.RejectedByName,
		)
		if err != nil {
			return nil, 0, err
		}
		quotations = append(quotations, q)
	}

	return quotations, total, rows.Err()
}

func (t *txRepo) CreateQuotation(ctx context.Context, q Quotation) (int64, error) {
	var quoteDate, validUntil pgtype.Date
	if !q.QuoteDate.IsZero() {
		quoteDate = pgtype.Date{Time: q.QuoteDate, Valid: true}
	}
	if !q.ValidUntil.IsZero() {
		validUntil = pgtype.Date{Time: q.ValidUntil, Valid: true}
	}
	
	var subtotal, taxAmount, totalAmount pgtype.Numeric
	subtotal.Scan(fmt.Sprintf("%f", q.Subtotal))
	taxAmount.Scan(fmt.Sprintf("%f", q.TaxAmount))
	totalAmount.Scan(fmt.Sprintf("%f", q.TotalAmount))

	return t.queries.CreateQuotation(ctx, salesdb.CreateQuotationParams{
		DocNumber:   q.DocNumber,
		CompanyID:   q.CompanyID,
		CustomerID:  q.CustomerID,
		QuoteDate:   quoteDate,
		ValidUntil:  validUntil,
		Status:      salesdb.QuotationStatus(q.Status),
		Currency:    q.Currency,
		Subtotal:    subtotal,
		TaxAmount:   taxAmount,
		TotalAmount: totalAmount,
		Notes:       pgtype.Text{String: getString(q.Notes), Valid: q.Notes != nil},
		CreatedBy:   q.CreatedBy,
	})
}

func (t *txRepo) InsertQuotationLine(ctx context.Context, line QuotationLine) (int64, error) {
	var quantity, unitPrice, discountPercent, discountAmount, taxPercent, taxAmount, lineTotal pgtype.Numeric
	quantity.Scan(fmt.Sprintf("%f", line.Quantity))
	unitPrice.Scan(fmt.Sprintf("%f", line.UnitPrice))
	discountPercent.Scan(fmt.Sprintf("%f", line.DiscountPercent))
	discountAmount.Scan(fmt.Sprintf("%f", line.DiscountAmount))
	taxPercent.Scan(fmt.Sprintf("%f", line.TaxPercent))
	taxAmount.Scan(fmt.Sprintf("%f", line.TaxAmount))
	lineTotal.Scan(fmt.Sprintf("%f", line.LineTotal))
	
	return t.queries.InsertQuotationLine(ctx, salesdb.InsertQuotationLineParams{
		QuotationID:     line.QuotationID,
		ProductID:       line.ProductID,
		Description:     pgtype.Text{String: getString(line.Description), Valid: line.Description != nil},
		Quantity:        quantity,
		Uom:             line.UOM,
		UnitPrice:       unitPrice,
		DiscountPercent: discountPercent,
		DiscountAmount:  discountAmount,
		TaxPercent:      taxPercent,
		TaxAmount:       taxAmount,
		LineTotal:       lineTotal,
		Notes:           pgtype.Text{String: getString(line.Notes), Valid: line.Notes != nil},
		LineOrder:       int32(line.LineOrder),
	})
}

func (t *txRepo) UpdateQuotationStatus(ctx context.Context, id int64, status QuotationStatus, userID int64, reason *string) error {
	var approvedBy, rejectedBy pgtype.Int8
	var approvedAt, rejectedAt pgtype.Timestamptz
	var rejectionReason pgtype.Text

	if status == QuotationStatusApproved {
		approvedBy = pgtype.Int8{Int64: userID, Valid: true}
		approvedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	} else if status == QuotationStatusRejected {
		rejectedBy = pgtype.Int8{Int64: userID, Valid: true}
		rejectedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
		if reason != nil {
			rejectionReason = pgtype.Text{String: *reason, Valid: true}
		}
	}

	return t.queries.UpdateQuotationStatus(ctx, salesdb.UpdateQuotationStatusParams{
		Status:          salesdb.QuotationStatus(status),
		ID:              id,
		ApprovedBy:      approvedBy,
		ApprovedAt:      approvedAt,
		RejectedBy:      rejectedBy,
		RejectedAt:      rejectedAt,
		RejectionReason: rejectionReason,
	})
}

func (t *txRepo) DeleteQuotationLines(ctx context.Context, quotationID int64) error {
	return t.queries.DeleteQuotationLines(ctx, quotationID)
}

// ============================================================================
// SALES ORDER OPERATIONS
// ============================================================================

func (r *Repository) GetSalesOrder(ctx context.Context, id int64) (*SalesOrder, error) {
	row, err := r.queries.GetSalesOrder(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	so := SalesOrder{
		ID:        row.ID,
		DocNumber: row.DocNumber,
		CompanyID: row.CompanyID,
		CustomerID: row.CustomerID,
		Status:    SalesOrderStatus(row.Status),
		Currency:  row.Currency,
		CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
	if row.QuotationID.Valid {
		val := row.QuotationID.Int64
		so.QuotationID = &val
	}
	if row.OrderDate.Valid {
		so.OrderDate = row.OrderDate.Time
	}
	if row.ExpectedDeliveryDate.Valid {
		val := row.ExpectedDeliveryDate.Time
		so.ExpectedDeliveryDate = &val
	}
	if row.Subtotal.Valid {
		f, _ := row.Subtotal.Float64Value()
		so.Subtotal = f.Float64
	}
	if row.TaxAmount.Valid {
		f, _ := row.TaxAmount.Float64Value()
		so.TaxAmount = f.Float64
	}
	if row.TotalAmount.Valid {
		f, _ := row.TotalAmount.Float64Value()
		so.TotalAmount = f.Float64
	}
	if row.Notes.Valid {
		val := row.Notes.String
		so.Notes = &val
	}
	if row.ConfirmedBy.Valid {
		val := row.ConfirmedBy.Int64
		so.ConfirmedBy = &val
	}
	if row.ConfirmedAt.Valid {
		val := row.ConfirmedAt.Time
		so.ConfirmedAt = &val
	}
	if row.CancelledBy.Valid {
		val := row.CancelledBy.Int64
		so.CancelledBy = &val
	}
	if row.CancelledAt.Valid {
		val := row.CancelledAt.Time
		so.CancelledAt = &val
	}
	if row.CancellationReason.Valid {
		val := row.CancellationReason.String
		so.CancellationReason = &val
	}

	lineRows, err := r.queries.GetSalesOrderLines(ctx, id)
	if err != nil {
		return nil, err
	}
	var lines []SalesOrderLine
	for _, l := range lineRows {
		line := SalesOrderLine{
			ID:           l.ID,
			SalesOrderID: l.SalesOrderID,
			ProductID:    l.ProductID,
			UOM:          l.Uom,
			LineOrder:    int(l.LineOrder),
		}
		if l.Description.Valid {
			val := l.Description.String
			line.Description = &val
		}
		if l.Quantity.Valid {
			f, _ := l.Quantity.Float64Value()
			line.Quantity = f.Float64
		}
		if l.QuantityDelivered.Valid {
			f, _ := l.QuantityDelivered.Float64Value()
			line.QuantityDelivered = f.Float64
		}
		if l.QuantityInvoiced.Valid {
			f, _ := l.QuantityInvoiced.Float64Value()
			line.QuantityInvoiced = f.Float64
		}
		if l.UnitPrice.Valid {
			f, _ := l.UnitPrice.Float64Value()
			line.UnitPrice = f.Float64
		}
		if l.DiscountPercent.Valid {
			f, _ := l.DiscountPercent.Float64Value()
			line.DiscountPercent = f.Float64
		}
		if l.DiscountAmount.Valid {
			f, _ := l.DiscountAmount.Float64Value()
			line.DiscountAmount = f.Float64
		}
		if l.TaxPercent.Valid {
			f, _ := l.TaxPercent.Float64Value()
			line.TaxPercent = f.Float64
		}
		if l.TaxAmount.Valid {
			f, _ := l.TaxAmount.Float64Value()
			line.TaxAmount = f.Float64
		}
		if l.LineTotal.Valid {
			f, _ := l.LineTotal.Float64Value()
			line.LineTotal = f.Float64
		}
		if l.Notes.Valid {
			val := l.Notes.String
			line.Notes = &val
		}
		lines = append(lines, line)
	}
	so.Lines = lines
	return &so, nil
}

func (r *Repository) GetSalesOrderByDocNumber(ctx context.Context, docNumber string) (*SalesOrder, error) {
	row, err := r.queries.GetSalesOrderByDocNumber(ctx, docNumber)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	so := SalesOrder{
		ID:        row.ID,
		DocNumber: row.DocNumber,
		CompanyID: row.CompanyID,
		CustomerID: row.CustomerID,
		Status:    SalesOrderStatus(row.Status),
		Currency:  row.Currency,
		CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
	if row.QuotationID.Valid {
		val := row.QuotationID.Int64
		so.QuotationID = &val
	}
	if row.OrderDate.Valid {
		so.OrderDate = row.OrderDate.Time
	}
	if row.ExpectedDeliveryDate.Valid {
		val := row.ExpectedDeliveryDate.Time
		so.ExpectedDeliveryDate = &val
	}
	if row.Subtotal.Valid {
		f, _ := row.Subtotal.Float64Value()
		so.Subtotal = f.Float64
	}
	if row.TaxAmount.Valid {
		f, _ := row.TaxAmount.Float64Value()
		so.TaxAmount = f.Float64
	}
	if row.TotalAmount.Valid {
		f, _ := row.TotalAmount.Float64Value()
		so.TotalAmount = f.Float64
	}
	if row.Notes.Valid {
		val := row.Notes.String
		so.Notes = &val
	}
	if row.ConfirmedBy.Valid {
		val := row.ConfirmedBy.Int64
		so.ConfirmedBy = &val
	}
	if row.ConfirmedAt.Valid {
		val := row.ConfirmedAt.Time
		so.ConfirmedAt = &val
	}
	if row.CancelledBy.Valid {
		val := row.CancelledBy.Int64
		so.CancelledBy = &val
	}
	if row.CancelledAt.Valid {
		val := row.CancelledAt.Time
		so.CancelledAt = &val
	}
	if row.CancellationReason.Valid {
		val := row.CancellationReason.String
		so.CancellationReason = &val
	}

	lineRows, err := r.queries.GetSalesOrderLines(ctx, so.ID)
	if err != nil {
		return nil, err
	}
	var lines []SalesOrderLine
	for _, l := range lineRows {
		line := SalesOrderLine{
			ID:           l.ID,
			SalesOrderID: l.SalesOrderID,
			ProductID:    l.ProductID,
			UOM:          l.Uom,
			LineOrder:    int(l.LineOrder),
		}
		if l.Description.Valid {
			val := l.Description.String
			line.Description = &val
		}
		if l.Quantity.Valid {
			f, _ := l.Quantity.Float64Value()
			line.Quantity = f.Float64
		}
		if l.QuantityDelivered.Valid {
			f, _ := l.QuantityDelivered.Float64Value()
			line.QuantityDelivered = f.Float64
		}
		if l.QuantityInvoiced.Valid {
			f, _ := l.QuantityInvoiced.Float64Value()
			line.QuantityInvoiced = f.Float64
		}
		if l.UnitPrice.Valid {
			f, _ := l.UnitPrice.Float64Value()
			line.UnitPrice = f.Float64
		}
		if l.DiscountPercent.Valid {
			f, _ := l.DiscountPercent.Float64Value()
			line.DiscountPercent = f.Float64
		}
		if l.DiscountAmount.Valid {
			f, _ := l.DiscountAmount.Float64Value()
			line.DiscountAmount = f.Float64
		}
		if l.TaxPercent.Valid {
			f, _ := l.TaxPercent.Float64Value()
			line.TaxPercent = f.Float64
		}
		if l.TaxAmount.Valid {
			f, _ := l.TaxAmount.Float64Value()
			line.TaxAmount = f.Float64
		}
		if l.LineTotal.Valid {
			f, _ := l.LineTotal.Float64Value()
			line.LineTotal = f.Float64
		}
		if l.Notes.Valid {
			val := l.Notes.String
			line.Notes = &val
		}
		lines = append(lines, line)
	}
	so.Lines = lines
	return &so, nil
}

func (r *Repository) ListSalesOrders(ctx context.Context, req ListSalesOrdersRequest) ([]SalesOrderWithDetails, int, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	conditions = append(conditions, fmt.Sprintf("so.company_id = $%d", argPos))
	args = append(args, req.CompanyID)
	argPos++

	if req.CustomerID != nil {
		conditions = append(conditions, fmt.Sprintf("so.customer_id = $%d", argPos))
		args = append(args, *req.CustomerID)
		argPos++
	}

	if req.Status != nil {
		conditions = append(conditions, fmt.Sprintf("so.status = $%d", argPos))
		args = append(args, *req.Status)
		argPos++
	}

	if req.DateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("so.order_date >= $%d", argPos))
		args = append(args, *req.DateFrom)
		argPos++
	}

	if req.DateTo != nil {
		conditions = append(conditions, fmt.Sprintf("so.order_date <= $%d", argPos))
		args = append(args, *req.DateTo)
		argPos++
	}

	whereClause := "WHERE " + conditions[0]
	for i := 1; i < len(conditions); i++ {
		whereClause += " AND " + conditions[i]
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM sales_orders so %s", whereClause)
	var total int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Fetch records
	query := fmt.Sprintf(`
		SELECT so.id, so.doc_number, so.company_id, so.customer_id, so.quotation_id, so.order_date,
		       so.expected_delivery_date, so.status, so.currency, so.subtotal, so.tax_amount, so.total_amount,
		       so.notes, so.created_by, so.confirmed_by, so.confirmed_at, so.cancelled_by, so.cancelled_at,
		       so.cancellation_reason, so.created_at, so.updated_at,
		       c.name as customer_name,
		       u1.full_name as created_by_name,
		       u2.full_name as confirmed_by_name,
		       u3.full_name as cancelled_by_name,
		       q.doc_number as quotation_number
		FROM sales_orders so
		JOIN customers c ON so.customer_id = c.id
		JOIN users u1 ON so.created_by = u1.id
		LEFT JOIN users u2 ON so.confirmed_by = u2.id
		LEFT JOIN users u3 ON so.cancelled_by = u3.id
		LEFT JOIN quotations q ON so.quotation_id = q.id
		%s
		ORDER BY so.order_date DESC, so.id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, req.Limit, req.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var orders []SalesOrderWithDetails
	for rows.Next() {
		var so SalesOrderWithDetails
		err := rows.Scan(
			&so.ID, &so.DocNumber, &so.CompanyID, &so.CustomerID, &so.QuotationID, &so.OrderDate,
			&so.ExpectedDeliveryDate, &so.Status, &so.Currency, &so.Subtotal, &so.TaxAmount,
			&so.TotalAmount, &so.Notes, &so.CreatedBy, &so.ConfirmedBy, &so.ConfirmedAt,
			&so.CancelledBy, &so.CancelledAt, &so.CancellationReason, &so.CreatedAt, &so.UpdatedAt,
			&so.CustomerName, &so.CreatedByName, &so.ConfirmedByName, &so.CancelledByName,
			&so.QuotationNumber,
		)
		if err != nil {
			return nil, 0, err
		}
		orders = append(orders, so)
	}

	return orders, total, rows.Err()
}

func (t *txRepo) CreateSalesOrder(ctx context.Context, order SalesOrder) (int64, error) {
	var quotationID pgtype.Int8
	if order.QuotationID != nil {
		quotationID = pgtype.Int8{Int64: *order.QuotationID, Valid: true}
	}
	var orderDate, expectedDeliveryDate pgtype.Date
	if !order.OrderDate.IsZero() {
		orderDate = pgtype.Date{Time: order.OrderDate, Valid: true}
	}
	if order.ExpectedDeliveryDate != nil && !order.ExpectedDeliveryDate.IsZero() {
		expectedDeliveryDate = pgtype.Date{Time: *order.ExpectedDeliveryDate, Valid: true}
	}

	var subtotal, taxAmount, totalAmount pgtype.Numeric
	subtotal.Scan(fmt.Sprintf("%f", order.Subtotal))
	taxAmount.Scan(fmt.Sprintf("%f", order.TaxAmount))
	totalAmount.Scan(fmt.Sprintf("%f", order.TotalAmount))

	return t.queries.CreateSalesOrder(ctx, salesdb.CreateSalesOrderParams{
		DocNumber:          order.DocNumber,
		CompanyID:          order.CompanyID,
		CustomerID:         order.CustomerID,
		QuotationID:        quotationID,
		OrderDate:          orderDate,
		ExpectedDeliveryDate: expectedDeliveryDate,
		Status:             salesdb.SalesOrderStatus(order.Status),
		Currency:           order.Currency,
		Subtotal:           subtotal,
		TaxAmount:          taxAmount,
		TotalAmount:        totalAmount,
		Notes:              pgtype.Text{String: getString(order.Notes), Valid: order.Notes != nil},
		CreatedBy:          order.CreatedBy,
	})
}

func (t *txRepo) InsertSalesOrderLine(ctx context.Context, line SalesOrderLine) (int64, error) {
	var quantity, quantityDelivered, quantityInvoiced, unitPrice, discountPercent, discountAmount, taxPercent, taxAmount, lineTotal pgtype.Numeric
	quantity.Scan(fmt.Sprintf("%f", line.Quantity))
	quantityDelivered.Scan(fmt.Sprintf("%f", line.QuantityDelivered))
	quantityInvoiced.Scan(fmt.Sprintf("%f", line.QuantityInvoiced))
	unitPrice.Scan(fmt.Sprintf("%f", line.UnitPrice))
	discountPercent.Scan(fmt.Sprintf("%f", line.DiscountPercent))
	discountAmount.Scan(fmt.Sprintf("%f", line.DiscountAmount))
	taxPercent.Scan(fmt.Sprintf("%f", line.TaxPercent))
	taxAmount.Scan(fmt.Sprintf("%f", line.TaxAmount))
	lineTotal.Scan(fmt.Sprintf("%f", line.LineTotal))

	return t.queries.InsertSalesOrderLine(ctx, salesdb.InsertSalesOrderLineParams{
		SalesOrderID:      line.SalesOrderID,
		ProductID:         line.ProductID,
		Description:       pgtype.Text{String: getString(line.Description), Valid: line.Description != nil},
		Quantity:          quantity,
		QuantityDelivered: quantityDelivered,
		QuantityInvoiced:  quantityInvoiced,
		Uom:               line.UOM,
		UnitPrice:         unitPrice,
		DiscountPercent:   discountPercent,
		DiscountAmount:    discountAmount,
		TaxPercent:        taxPercent,
		TaxAmount:         taxAmount,
		LineTotal:         lineTotal,
		Notes:             pgtype.Text{String: getString(line.Notes), Valid: line.Notes != nil},
		LineOrder:         int32(line.LineOrder),
	})
}

func (t *txRepo) UpdateSalesOrderStatus(ctx context.Context, id int64, status SalesOrderStatus, userID int64, reason *string) error {
	var confirmedBy, cancelledBy pgtype.Int8
	var confirmedAt, cancelledAt pgtype.Timestamptz
	var cancellationReason pgtype.Text

	if status == SalesOrderStatusConfirmed {
		confirmedBy = pgtype.Int8{Int64: userID, Valid: true}
		confirmedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	} else if status == SalesOrderStatusCancelled {
		cancelledBy = pgtype.Int8{Int64: userID, Valid: true}
		cancelledAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
		if reason != nil {
			cancellationReason = pgtype.Text{String: *reason, Valid: true}
		}
	}

	return t.queries.UpdateSalesOrderStatus(ctx, salesdb.UpdateSalesOrderStatusParams{
		Status:           salesdb.SalesOrderStatus(status),
		ID:               id,
		ConfirmedBy:      confirmedBy,
		ConfirmedAt:      confirmedAt,
		CancelledBy:      cancelledBy,
		CancelledAt:      cancelledAt,
		CancellationReason: cancellationReason,
	})
}

func (t *txRepo) DeleteSalesOrderLines(ctx context.Context, salesOrderID int64) error {
	return t.queries.DeleteSalesOrderLines(ctx, salesOrderID)
}

func (t *txRepo) UpdateSalesOrderLineDelivered(ctx context.Context, id int64, delivered float64) error {
	var qty pgtype.Numeric
	qty.Scan(fmt.Sprintf("%f", delivered))

	return t.queries.UpdateSalesOrderLineDelivered(ctx, salesdb.UpdateSalesOrderLineDeliveredParams{
		QuantityDelivered: qty,
		ID:                id,
	})
}

func getString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func (r *Repository) GenerateCustomerCode(ctx context.Context, companyID int64) (string, error) {
	var code string
	err := r.pool.QueryRow(ctx, "SELECT generate_customer_code($1)", companyID).Scan(&code)
	return code, err
}

func (r *Repository) GenerateQuotationNumber(ctx context.Context, companyID int64, quoteDate time.Time) (string, error) {
	var docNumber string
	err := r.pool.QueryRow(ctx, "SELECT generate_quotation_number($1, $2)", companyID, quoteDate).Scan(&docNumber)
	return docNumber, err
}

func (r *Repository) GenerateSalesOrderNumber(ctx context.Context, companyID int64, orderDate time.Time) (string, error) {
	var docNumber string
	err := r.pool.QueryRow(ctx, "SELECT generate_sales_order_number($1, $2)", companyID, orderDate).Scan(&docNumber)
	return docNumber, err
}

// CalculateLineTotals calculates discount, tax, and line total for a quotation/SO line
func CalculateLineTotals(quantity, unitPrice, discountPercent, taxPercent float64) (discountAmount, taxAmount, lineTotal float64) {
	subtotal := quantity * unitPrice
	discountAmount = roundTo2(subtotal * discountPercent / 100)
	taxableAmount := subtotal - discountAmount
	taxAmount = roundTo2(taxableAmount * taxPercent / 100)
	lineTotal = taxableAmount + taxAmount
	return
}

func roundTo2(val float64) float64 {
	return float64(int64(val*100+0.5)) / 100
}
