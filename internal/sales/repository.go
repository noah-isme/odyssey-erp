package sales

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("record not found")
	ErrInvalidStatus = errors.New("invalid status transition")
	ErrAlreadyExists = errors.New("record already exists")
)

// Repository provides PostgreSQL backed persistence for sales operations.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
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
	tx pgx.Tx
}

// WithTx wraps callback in repeatable-read transaction.
func (r *Repository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	wrapper := &txRepo{tx: tx}
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
	query := `
		SELECT id, code, name, company_id, email, phone, tax_id,
		       credit_limit, payment_terms_days, address_line1, address_line2,
		       city, state, postal_code, country, is_active, notes,
		       created_by, created_at, updated_at
		FROM customers
		WHERE id = $1
	`
	var c Customer
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&c.ID, &c.Code, &c.Name, &c.CompanyID, &c.Email, &c.Phone, &c.TaxID,
		&c.CreditLimit, &c.PaymentTermsDays, &c.AddressLine1, &c.AddressLine2,
		&c.City, &c.State, &c.PostalCode, &c.Country, &c.IsActive, &c.Notes,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repository) GetCustomerByCode(ctx context.Context, companyID int64, code string) (*Customer, error) {
	query := `
		SELECT id, code, name, company_id, email, phone, tax_id,
		       credit_limit, payment_terms_days, address_line1, address_line2,
		       city, state, postal_code, country, is_active, notes,
		       created_by, created_at, updated_at
		FROM customers
		WHERE company_id = $1 AND code = $2
	`
	var c Customer
	err := r.pool.QueryRow(ctx, query, companyID, code).Scan(
		&c.ID, &c.Code, &c.Name, &c.CompanyID, &c.Email, &c.Phone, &c.TaxID,
		&c.CreditLimit, &c.PaymentTermsDays, &c.AddressLine1, &c.AddressLine2,
		&c.City, &c.State, &c.PostalCode, &c.Country, &c.IsActive, &c.Notes,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
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
	query := `
		INSERT INTO customers (
			code, name, company_id, email, phone, tax_id,
			credit_limit, payment_terms_days, address_line1, address_line2,
			city, state, postal_code, country, is_active, notes, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING id
	`
	var id int64
	err := t.tx.QueryRow(ctx, query,
		customer.Code, customer.Name, customer.CompanyID, customer.Email, customer.Phone, customer.TaxID,
		customer.CreditLimit, customer.PaymentTermsDays, customer.AddressLine1, customer.AddressLine2,
		customer.City, customer.State, customer.PostalCode, customer.Country, customer.IsActive,
		customer.Notes, customer.CreatedBy,
	).Scan(&id)
	return id, err
}

func (t *txRepo) UpdateCustomer(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	query := "UPDATE customers SET "
	var args []interface{}
	argPos := 1

	for field, value := range updates {
		if argPos > 1 {
			query += ", "
		}
		query += fmt.Sprintf("%s = $%d", field, argPos)
		args = append(args, value)
		argPos++
	}

	query += fmt.Sprintf(", updated_at = NOW() WHERE id = $%d", argPos)
	args = append(args, id)

	result, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================================
// QUOTATION OPERATIONS
// ============================================================================

func (r *Repository) GetQuotation(ctx context.Context, id int64) (*Quotation, error) {
	query := `
		SELECT id, doc_number, company_id, customer_id, quote_date, valid_until,
		       status, currency, subtotal, tax_amount, total_amount, notes,
		       created_by, approved_by, approved_at, rejected_by, rejected_at,
		       rejection_reason, created_at, updated_at
		FROM quotations
		WHERE id = $1
	`
	var q Quotation
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&q.ID, &q.DocNumber, &q.CompanyID, &q.CustomerID, &q.QuoteDate, &q.ValidUntil,
		&q.Status, &q.Currency, &q.Subtotal, &q.TaxAmount, &q.TotalAmount, &q.Notes,
		&q.CreatedBy, &q.ApprovedBy, &q.ApprovedAt, &q.RejectedBy, &q.RejectedAt,
		&q.RejectionReason, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Fetch lines
	lines, err := r.getQuotationLines(ctx, id)
	if err != nil {
		return nil, err
	}
	q.Lines = lines

	return &q, nil
}

func (r *Repository) GetQuotationByDocNumber(ctx context.Context, docNumber string) (*Quotation, error) {
	query := `
		SELECT id, doc_number, company_id, customer_id, quote_date, valid_until,
		       status, currency, subtotal, tax_amount, total_amount, notes,
		       created_by, approved_by, approved_at, rejected_by, rejected_at,
		       rejection_reason, created_at, updated_at
		FROM quotations
		WHERE doc_number = $1
	`
	var q Quotation
	err := r.pool.QueryRow(ctx, query, docNumber).Scan(
		&q.ID, &q.DocNumber, &q.CompanyID, &q.CustomerID, &q.QuoteDate, &q.ValidUntil,
		&q.Status, &q.Currency, &q.Subtotal, &q.TaxAmount, &q.TotalAmount, &q.Notes,
		&q.CreatedBy, &q.ApprovedBy, &q.ApprovedAt, &q.RejectedBy, &q.RejectedAt,
		&q.RejectionReason, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	lines, err := r.getQuotationLines(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	q.Lines = lines

	return &q, nil
}

func (r *Repository) getQuotationLines(ctx context.Context, quotationID int64) ([]QuotationLine, error) {
	query := `
		SELECT id, quotation_id, product_id, description, quantity, uom,
		       unit_price, discount_percent, discount_amount, tax_percent,
		       tax_amount, line_total, notes, line_order, created_at, updated_at
		FROM quotation_lines
		WHERE quotation_id = $1
		ORDER BY line_order, id
	`
	rows, err := r.pool.Query(ctx, query, quotationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []QuotationLine
	for rows.Next() {
		var line QuotationLine
		err := rows.Scan(
			&line.ID, &line.QuotationID, &line.ProductID, &line.Description, &line.Quantity, &line.UOM,
			&line.UnitPrice, &line.DiscountPercent, &line.DiscountAmount, &line.TaxPercent,
			&line.TaxAmount, &line.LineTotal, &line.Notes, &line.LineOrder, &line.CreatedAt, &line.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
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

func (t *txRepo) CreateQuotation(ctx context.Context, quotation Quotation) (int64, error) {
	query := `
		INSERT INTO quotations (
			doc_number, company_id, customer_id, quote_date, valid_until,
			status, currency, subtotal, tax_amount, total_amount, notes, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`
	var id int64
	err := t.tx.QueryRow(ctx, query,
		quotation.DocNumber, quotation.CompanyID, quotation.CustomerID, quotation.QuoteDate,
		quotation.ValidUntil, quotation.Status, quotation.Currency, quotation.Subtotal,
		quotation.TaxAmount, quotation.TotalAmount, quotation.Notes, quotation.CreatedBy,
	).Scan(&id)
	return id, err
}

func (t *txRepo) InsertQuotationLine(ctx context.Context, line QuotationLine) (int64, error) {
	query := `
		INSERT INTO quotation_lines (
			quotation_id, product_id, description, quantity, uom,
			unit_price, discount_percent, discount_amount, tax_percent,
			tax_amount, line_total, notes, line_order
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`
	var id int64
	err := t.tx.QueryRow(ctx, query,
		line.QuotationID, line.ProductID, line.Description, line.Quantity, line.UOM,
		line.UnitPrice, line.DiscountPercent, line.DiscountAmount, line.TaxPercent,
		line.TaxAmount, line.LineTotal, line.Notes, line.LineOrder,
	).Scan(&id)
	return id, err
}

func (t *txRepo) UpdateQuotationStatus(ctx context.Context, id int64, status QuotationStatus, userID int64, reason *string) error {
	now := time.Now()

	var query string
	var args []interface{}

	switch status {
	case QuotationStatusSubmitted:
		query = "UPDATE quotations SET status = $1, updated_at = $2 WHERE id = $3"
		args = []interface{}{status, now, id}
	case QuotationStatusApproved:
		query = "UPDATE quotations SET status = $1, approved_by = $2, approved_at = $3, updated_at = $3 WHERE id = $4"
		args = []interface{}{status, userID, now, id}
	case QuotationStatusRejected:
		query = "UPDATE quotations SET status = $1, rejected_by = $2, rejected_at = $3, rejection_reason = $4, updated_at = $3 WHERE id = $5"
		args = []interface{}{status, userID, now, reason, id}
	case QuotationStatusConverted:
		query = "UPDATE quotations SET status = $1, updated_at = $2 WHERE id = $3"
		args = []interface{}{status, now, id}
	default:
		query = "UPDATE quotations SET status = $1, updated_at = $2 WHERE id = $3"
		args = []interface{}{status, now, id}
	}

	result, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *txRepo) DeleteQuotationLines(ctx context.Context, quotationID int64) error {
	_, err := t.tx.Exec(ctx, "DELETE FROM quotation_lines WHERE quotation_id = $1", quotationID)
	return err
}

// ============================================================================
// SALES ORDER OPERATIONS
// ============================================================================

func (r *Repository) GetSalesOrder(ctx context.Context, id int64) (*SalesOrder, error) {
	query := `
		SELECT id, doc_number, company_id, customer_id, quotation_id, order_date,
		       expected_delivery_date, status, currency, subtotal, tax_amount, total_amount,
		       notes, created_by, confirmed_by, confirmed_at, cancelled_by, cancelled_at,
		       cancellation_reason, created_at, updated_at
		FROM sales_orders
		WHERE id = $1
	`
	var so SalesOrder
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&so.ID, &so.DocNumber, &so.CompanyID, &so.CustomerID, &so.QuotationID, &so.OrderDate,
		&so.ExpectedDeliveryDate, &so.Status, &so.Currency, &so.Subtotal, &so.TaxAmount,
		&so.TotalAmount, &so.Notes, &so.CreatedBy, &so.ConfirmedBy, &so.ConfirmedAt,
		&so.CancelledBy, &so.CancelledAt, &so.CancellationReason, &so.CreatedAt, &so.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Fetch lines
	lines, err := r.getSalesOrderLines(ctx, id)
	if err != nil {
		return nil, err
	}
	so.Lines = lines

	return &so, nil
}

func (r *Repository) GetSalesOrderByDocNumber(ctx context.Context, docNumber string) (*SalesOrder, error) {
	query := `
		SELECT id, doc_number, company_id, customer_id, quotation_id, order_date,
		       expected_delivery_date, status, currency, subtotal, tax_amount, total_amount,
		       notes, created_by, confirmed_by, confirmed_at, cancelled_by, cancelled_at,
		       cancellation_reason, created_at, updated_at
		FROM sales_orders
		WHERE doc_number = $1
	`
	var so SalesOrder
	err := r.pool.QueryRow(ctx, query, docNumber).Scan(
		&so.ID, &so.DocNumber, &so.CompanyID, &so.CustomerID, &so.QuotationID, &so.OrderDate,
		&so.ExpectedDeliveryDate, &so.Status, &so.Currency, &so.Subtotal, &so.TaxAmount,
		&so.TotalAmount, &so.Notes, &so.CreatedBy, &so.ConfirmedBy, &so.ConfirmedAt,
		&so.CancelledBy, &so.CancelledAt, &so.CancellationReason, &so.CreatedAt, &so.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	lines, err := r.getSalesOrderLines(ctx, so.ID)
	if err != nil {
		return nil, err
	}
	so.Lines = lines

	return &so, nil
}

func (r *Repository) getSalesOrderLines(ctx context.Context, salesOrderID int64) ([]SalesOrderLine, error) {
	query := `
		SELECT id, sales_order_id, product_id, description, quantity,
		       quantity_delivered, quantity_invoiced, uom, unit_price,
		       discount_percent, discount_amount, tax_percent, tax_amount,
		       line_total, notes, line_order, created_at, updated_at
		FROM sales_order_lines
		WHERE sales_order_id = $1
		ORDER BY line_order, id
	`
	rows, err := r.pool.Query(ctx, query, salesOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []SalesOrderLine
	for rows.Next() {
		var line SalesOrderLine
		err := rows.Scan(
			&line.ID, &line.SalesOrderID, &line.ProductID, &line.Description, &line.Quantity,
			&line.QuantityDelivered, &line.QuantityInvoiced, &line.UOM, &line.UnitPrice,
			&line.DiscountPercent, &line.DiscountAmount, &line.TaxPercent, &line.TaxAmount,
			&line.LineTotal, &line.Notes, &line.LineOrder, &line.CreatedAt, &line.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
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
	query := `
		INSERT INTO sales_orders (
			doc_number, company_id, customer_id, quotation_id, order_date,
			expected_delivery_date, status, currency, subtotal, tax_amount,
			total_amount, notes, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`
	var id int64
	err := t.tx.QueryRow(ctx, query,
		order.DocNumber, order.CompanyID, order.CustomerID, order.QuotationID, order.OrderDate,
		order.ExpectedDeliveryDate, order.Status, order.Currency, order.Subtotal,
		order.TaxAmount, order.TotalAmount, order.Notes, order.CreatedBy,
	).Scan(&id)
	return id, err
}

func (t *txRepo) InsertSalesOrderLine(ctx context.Context, line SalesOrderLine) (int64, error) {
	query := `
		INSERT INTO sales_order_lines (
			sales_order_id, product_id, description, quantity,
			quantity_delivered, quantity_invoiced, uom, unit_price,
			discount_percent, discount_amount, tax_percent, tax_amount,
			line_total, notes, line_order
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id
	`
	var id int64
	err := t.tx.QueryRow(ctx, query,
		line.SalesOrderID, line.ProductID, line.Description, line.Quantity,
		line.QuantityDelivered, line.QuantityInvoiced, line.UOM, line.UnitPrice,
		line.DiscountPercent, line.DiscountAmount, line.TaxPercent, line.TaxAmount,
		line.LineTotal, line.Notes, line.LineOrder,
	).Scan(&id)
	return id, err
}

func (t *txRepo) UpdateSalesOrderStatus(ctx context.Context, id int64, status SalesOrderStatus, userID int64, reason *string) error {
	now := time.Now()

	var query string
	var args []interface{}

	switch status {
	case SalesOrderStatusConfirmed:
		query = "UPDATE sales_orders SET status = $1, confirmed_by = $2, confirmed_at = $3, updated_at = $3 WHERE id = $4"
		args = []interface{}{status, userID, now, id}
	case SalesOrderStatusCancelled:
		query = "UPDATE sales_orders SET status = $1, cancelled_by = $2, cancelled_at = $3, cancellation_reason = $4, updated_at = $3 WHERE id = $5"
		args = []interface{}{status, userID, now, reason, id}
	default:
		query = "UPDATE sales_orders SET status = $1, updated_at = $2 WHERE id = $3"
		args = []interface{}{status, now, id}
	}

	result, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *txRepo) DeleteSalesOrderLines(ctx context.Context, salesOrderID int64) error {
	_, err := t.tx.Exec(ctx, "DELETE FROM sales_order_lines WHERE sales_order_id = $1", salesOrderID)
	return err
}

func (t *txRepo) UpdateSalesOrderLineDelivered(ctx context.Context, lineID int64, quantityDelivered float64) error {
	query := "UPDATE sales_order_lines SET quantity_delivered = $1, updated_at = NOW() WHERE id = $2"
	result, err := t.tx.Exec(ctx, query, quantityDelivered, lineID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
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
