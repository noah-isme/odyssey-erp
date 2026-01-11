package ar

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides PostgreSQL backed persistence for AR.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ErrNotFound indicates resource not found.
var ErrNotFound = errors.New("ar: not found")

// --- Invoice Operations ---

// CreateARInvoice creates a new AR invoice.
func (r *Repository) CreateARInvoice(ctx context.Context, input CreateARInvoiceInput) (*ARInvoice, error) {
	// Generate number if not provided
	number := input.Number
	if number == "" {
		var err error
		number, err = r.GenerateInvoiceNumber(ctx)
		if err != nil {
			return nil, err
		}
	}

	query := `
		INSERT INTO ar_invoices (
			number, customer_id, so_id, delivery_order_id, currency,
			subtotal, tax_amount, total, status, due_at, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'DRAFT', $9, $10, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	var inv ARInvoice
	var soID, doID, createdBy pgtype.Int8

	if input.SOID > 0 {
		soID = pgtype.Int8{Int64: input.SOID, Valid: true}
	}
	if input.DeliveryOrderID > 0 {
		doID = pgtype.Int8{Int64: input.DeliveryOrderID, Valid: true}
	}
	if input.CreatedBy > 0 {
		createdBy = pgtype.Int8{Int64: input.CreatedBy, Valid: true}
	}

	err := r.pool.QueryRow(ctx, query,
		number,
		input.CustomerID,
		soID,
		doID,
		input.Currency,
		input.Subtotal,
		input.TaxAmount,
		input.Total,
		input.DueDate,
		createdBy,
	).Scan(&inv.ID, &inv.CreatedAt, &inv.UpdatedAt)

	if err != nil {
		return nil, err
	}

	inv.Number = number
	inv.CustomerID = input.CustomerID
	inv.SOID = input.SOID
	inv.DeliveryOrderID = input.DeliveryOrderID
	inv.Currency = input.Currency
	inv.Subtotal = input.Subtotal
	inv.TaxAmount = input.TaxAmount
	inv.Total = input.Total
	inv.Status = ARStatusDraft
	inv.DueAt = input.DueDate
	inv.CreatedBy = input.CreatedBy

	return &inv, nil
}

// CreateARInvoiceLine creates a line item for an invoice.
func (r *Repository) CreateARInvoiceLine(ctx context.Context, invoiceID int64, line CreateARInvoiceLineInput) (*ARInvoiceLine, error) {
	// Calculate line totals
	subtotal := line.Quantity * line.UnitPrice * (1 - line.DiscountPct/100)
	taxAmount := subtotal * (line.TaxPct / 100)
	total := subtotal + taxAmount

	query := `
		INSERT INTO ar_invoice_lines (
			ar_invoice_id, delivery_order_line_id, product_id, description,
			quantity, unit_price, discount_pct, tax_pct,
			subtotal, tax_amount, total, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		RETURNING id, created_at`

	var doLineID pgtype.Int8
	if line.DeliveryOrderLineID > 0 {
		doLineID = pgtype.Int8{Int64: line.DeliveryOrderLineID, Valid: true}
	}

	var result ARInvoiceLine
	err := r.pool.QueryRow(ctx, query,
		invoiceID,
		doLineID,
		line.ProductID,
		line.Description,
		line.Quantity,
		line.UnitPrice,
		line.DiscountPct,
		line.TaxPct,
		subtotal,
		taxAmount,
		total,
	).Scan(&result.ID, &result.CreatedAt)

	if err != nil {
		return nil, err
	}

	result.ARInvoiceID = invoiceID
	result.DeliveryOrderLineID = line.DeliveryOrderLineID
	result.ProductID = line.ProductID
	result.Description = line.Description
	result.Quantity = line.Quantity
	result.UnitPrice = line.UnitPrice
	result.DiscountPct = line.DiscountPct
	result.TaxPct = line.TaxPct
	result.Subtotal = subtotal
	result.TaxAmount = taxAmount
	result.Total = total

	return &result, nil
}

// GetARInvoice retrieves an invoice by ID.
func (r *Repository) GetARInvoice(ctx context.Context, id int64) (*ARInvoice, error) {
	query := `
		SELECT id, number, customer_id, so_id, delivery_order_id, currency,
			subtotal, tax_amount, total, status, due_at,
			posted_at, posted_by, voided_at, voided_by, void_reason,
			created_by, created_at, updated_at
		FROM ar_invoices
		WHERE id = $1`

	var inv ARInvoice
	var soID, doID, postedBy, voidedBy, createdBy pgtype.Int8
	var postedAt, voidedAt pgtype.Timestamptz
	var voidReason pgtype.Text
	var subtotal, taxAmount pgtype.Numeric

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&inv.ID, &inv.Number, &inv.CustomerID, &soID, &doID, &inv.Currency,
		&subtotal, &taxAmount, &inv.Total, &inv.Status, &inv.DueAt,
		&postedAt, &postedBy, &voidedAt, &voidedBy, &voidReason,
		&createdBy, &inv.CreatedAt, &inv.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	inv.SOID = soID.Int64
	inv.DeliveryOrderID = doID.Int64
	inv.Subtotal = numericToFloat64(subtotal)
	inv.TaxAmount = numericToFloat64(taxAmount)
	inv.CreatedBy = createdBy.Int64

	if postedAt.Valid {
		inv.PostedAt = &postedAt.Time
	}
	if postedBy.Valid {
		inv.PostedBy = &postedBy.Int64
	}
	if voidedAt.Valid {
		inv.VoidedAt = &voidedAt.Time
	}
	if voidedBy.Valid {
		inv.VoidedBy = &voidedBy.Int64
	}
	if voidReason.Valid {
		inv.VoidReason = voidReason.String
	}

	return &inv, nil
}

// GetARInvoiceWithDetails retrieves an invoice with lines and payments.
func (r *Repository) GetARInvoiceWithDetails(ctx context.Context, id int64) (*ARInvoiceWithDetails, error) {
	inv, err := r.GetARInvoice(ctx, id)
	if err != nil {
		return nil, err
	}

	// Get customer name
	var customerName string
	_ = r.pool.QueryRow(ctx, "SELECT name FROM customers WHERE id = $1", inv.CustomerID).Scan(&customerName)

	// Get lines
	lines, err := r.ListARInvoiceLines(ctx, id)
	if err != nil {
		return nil, err
	}

	// Get payments
	payments, err := r.ListInvoicePayments(ctx, id)
	if err != nil {
		return nil, err
	}

	// Get balance
	_, paidAmount, balance, _ := r.GetInvoiceBalance(ctx, id)

	return &ARInvoiceWithDetails{
		ARInvoice:    *inv,
		CustomerName: customerName,
		Lines:        lines,
		Payments:     payments,
		PaidAmount:   paidAmount,
		Balance:      balance,
	}, nil
}

// ListARInvoices returns invoices with optional filtering.
func (r *Repository) ListARInvoices(ctx context.Context, req ListARInvoicesRequest) ([]ARInvoice, error) {
	query := `
		SELECT id, number, customer_id, so_id, delivery_order_id, currency,
			subtotal, tax_amount, total, status, due_at,
			posted_at, posted_by, voided_at, voided_by, void_reason,
			created_by, created_at, updated_at
		FROM ar_invoices
		WHERE 1=1`

	args := []any{}
	argNum := 1

	if req.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argNum)
		args = append(args, string(req.Status))
		argNum++
	}
	if req.CustomerID > 0 {
		query += fmt.Sprintf(" AND customer_id = $%d", argNum)
		args = append(args, req.CustomerID)
		argNum++
	}
	if !req.FromDate.IsZero() {
		query += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, req.FromDate)
		argNum++
	}
	if !req.ToDate.IsZero() {
		query += fmt.Sprintf(" AND created_at <= $%d", argNum)
		args = append(args, req.ToDate)
		argNum++
	}

	query += " ORDER BY created_at DESC"

	if req.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, req.Limit)
		argNum++
	}
	if req.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, req.Offset)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invoices []ARInvoice
	for rows.Next() {
		var inv ARInvoice
		var soID, doID, postedBy, voidedBy, createdBy pgtype.Int8
		var postedAt, voidedAt pgtype.Timestamptz
		var voidReason pgtype.Text
		var subtotal, taxAmount pgtype.Numeric

		err := rows.Scan(
			&inv.ID, &inv.Number, &inv.CustomerID, &soID, &doID, &inv.Currency,
			&subtotal, &taxAmount, &inv.Total, &inv.Status, &inv.DueAt,
			&postedAt, &postedBy, &voidedAt, &voidedBy, &voidReason,
			&createdBy, &inv.CreatedAt, &inv.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		inv.SOID = soID.Int64
		inv.DeliveryOrderID = doID.Int64
		inv.Subtotal = numericToFloat64(subtotal)
		inv.TaxAmount = numericToFloat64(taxAmount)
		inv.CreatedBy = createdBy.Int64

		if postedAt.Valid {
			inv.PostedAt = &postedAt.Time
		}
		if postedBy.Valid {
			inv.PostedBy = &postedBy.Int64
		}
		if voidedAt.Valid {
			inv.VoidedAt = &voidedAt.Time
		}
		if voidedBy.Valid {
			inv.VoidedBy = &voidedBy.Int64
		}
		if voidReason.Valid {
			inv.VoidReason = voidReason.String
		}

		invoices = append(invoices, inv)
	}

	return invoices, nil
}

// ListARInvoiceLines returns line items for an invoice.
func (r *Repository) ListARInvoiceLines(ctx context.Context, invoiceID int64) ([]ARInvoiceLine, error) {
	query := `
		SELECT id, ar_invoice_id, delivery_order_line_id, product_id,
			description, quantity, unit_price, discount_pct, tax_pct,
			subtotal, tax_amount, total, created_at
		FROM ar_invoice_lines
		WHERE ar_invoice_id = $1
		ORDER BY id`

	rows, err := r.pool.Query(ctx, query, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []ARInvoiceLine
	for rows.Next() {
		var line ARInvoiceLine
		var doLineID pgtype.Int8

		err := rows.Scan(
			&line.ID, &line.ARInvoiceID, &doLineID, &line.ProductID,
			&line.Description, &line.Quantity, &line.UnitPrice, &line.DiscountPct, &line.TaxPct,
			&line.Subtotal, &line.TaxAmount, &line.Total, &line.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		line.DeliveryOrderLineID = doLineID.Int64
		lines = append(lines, line)
	}

	return lines, nil
}

// PostARInvoice posts a draft invoice.
func (r *Repository) PostARInvoice(ctx context.Context, id int64, postedBy int64) error {
	query := `
		UPDATE ar_invoices
		SET status = 'POSTED', posted_at = NOW(), posted_by = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'DRAFT'`

	result, err := r.pool.Exec(ctx, query, id, postedBy)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("invoice not found or not in DRAFT status")
	}
	return nil
}

// VoidARInvoice voids an invoice.
func (r *Repository) VoidARInvoice(ctx context.Context, id int64, voidedBy int64, reason string) error {
	query := `
		UPDATE ar_invoices
		SET status = 'VOID', voided_at = NOW(), voided_by = $2, void_reason = $3, updated_at = NOW()
		WHERE id = $1 AND status IN ('DRAFT', 'POSTED')`

	result, err := r.pool.Exec(ctx, query, id, voidedBy, reason)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("invoice not found or cannot be voided")
	}
	return nil
}

// GetInvoiceBalance returns the balance for an invoice.
func (r *Repository) GetInvoiceBalance(ctx context.Context, id int64) (total, paid, balance float64, err error) {
	query := `
		SELECT 
			i.total,
			COALESCE(SUM(pa.amount), 0) AS paid_amount,
			i.total - COALESCE(SUM(pa.amount), 0) AS balance
		FROM ar_invoices i
		LEFT JOIN ar_payment_allocations pa ON pa.ar_invoice_id = i.id
		WHERE i.id = $1
		GROUP BY i.id`

	err = r.pool.QueryRow(ctx, query, id).Scan(&total, &paid, &balance)
	if err == pgx.ErrNoRows {
		return 0, 0, 0, ErrNotFound
	}
	return
}

// CountInvoicesByDelivery counts invoices for a delivery order.
func (r *Repository) CountInvoicesByDelivery(ctx context.Context, deliveryOrderID int64) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM ar_invoices WHERE delivery_order_id = $1",
		deliveryOrderID,
	).Scan(&count)
	return count, err
}

// GenerateInvoiceNumber generates a unique invoice number.
func (r *Repository) GenerateInvoiceNumber(ctx context.Context) (string, error) {
	var number string
	err := r.pool.QueryRow(ctx, "SELECT generate_ar_invoice_number()").Scan(&number)
	return number, err
}

// --- Payment Operations ---

// CreateARPayment creates a new payment.
func (r *Repository) CreateARPayment(ctx context.Context, input CreateARPaymentInput) (*ARPayment, error) {
	number := input.Number
	if number == "" {
		var err error
		number, err = r.GeneratePaymentNumber(ctx)
		if err != nil {
			return nil, err
		}
	}

	// Use first allocation's invoice for legacy ar_invoice_id field
	var invoiceID int64
	if len(input.Allocations) > 0 {
		invoiceID = input.Allocations[0].ARInvoiceID
	}

	query := `
		INSERT INTO ar_payments (
			number, ar_invoice_id, amount, paid_at, method, note, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	var createdBy pgtype.Int8
	if input.CreatedBy > 0 {
		createdBy = pgtype.Int8{Int64: input.CreatedBy, Valid: true}
	}

	var payment ARPayment
	err := r.pool.QueryRow(ctx, query,
		number,
		invoiceID,
		input.Amount,
		input.PaidAt,
		input.Method,
		input.Note,
		createdBy,
	).Scan(&payment.ID, &payment.CreatedAt, &payment.UpdatedAt)

	if err != nil {
		return nil, err
	}

	payment.Number = number
	payment.ARInvoiceID = invoiceID
	payment.Amount = input.Amount
	payment.PaidAt = input.PaidAt
	payment.Method = input.Method
	payment.Note = input.Note
	payment.CreatedBy = input.CreatedBy

	return &payment, nil
}

// CreatePaymentAllocation allocates a payment to an invoice.
func (r *Repository) CreatePaymentAllocation(ctx context.Context, paymentID, invoiceID int64, amount float64) error {
	query := `
		INSERT INTO ar_payment_allocations (ar_payment_id, ar_invoice_id, amount, created_at)
		VALUES ($1, $2, $3, NOW())`

	_, err := r.pool.Exec(ctx, query, paymentID, invoiceID, amount)
	return err
}

// ListARPayments returns all payments.
func (r *Repository) ListARPayments(ctx context.Context) ([]ARPayment, error) {
	query := `
		SELECT id, number, ar_invoice_id, amount, paid_at, method, note, 
			created_by, created_at, updated_at
		FROM ar_payments
		ORDER BY paid_at DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payments []ARPayment
	for rows.Next() {
		var p ARPayment
		var createdBy pgtype.Int8

		err := rows.Scan(
			&p.ID, &p.Number, &p.ARInvoiceID, &p.Amount, &p.PaidAt, &p.Method, &p.Note,
			&createdBy, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		p.CreatedBy = createdBy.Int64
		payments = append(payments, p)
	}

	return payments, nil
}

// ListInvoicePayments returns payments allocated to an invoice.
func (r *Repository) ListInvoicePayments(ctx context.Context, invoiceID int64) ([]ARPaymentSummary, error) {
	query := `
		SELECT p.id, p.number, p.amount, p.paid_at, p.method, p.note, pa.amount AS allocated_amount
		FROM ar_payments p
		JOIN ar_payment_allocations pa ON pa.ar_payment_id = p.id
		WHERE pa.ar_invoice_id = $1
		ORDER BY p.paid_at`

	rows, err := r.pool.Query(ctx, query, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payments []ARPaymentSummary
	for rows.Next() {
		var p ARPaymentSummary
		err := rows.Scan(&p.ID, &p.Number, &p.Amount, &p.PaidAt, &p.Method, &p.Note, &p.AllocatedAmount)
		if err != nil {
			return nil, err
		}
		payments = append(payments, p)
	}

	return payments, nil
}

// GeneratePaymentNumber generates a unique payment number.
func (r *Repository) GeneratePaymentNumber(ctx context.Context) (string, error) {
	var number string
	err := r.pool.QueryRow(ctx, "SELECT generate_ar_payment_number()").Scan(&number)
	return number, err
}

// --- Aging Operations ---

// ListAROutstanding returns posted invoices.
func (r *Repository) ListAROutstanding(ctx context.Context) ([]ARInvoice, error) {
	return r.ListARInvoices(ctx, ListARInvoicesRequest{
		Status: ARStatusPosted,
		Limit:  1000,
	})
}

// --- Legacy Compatibility ---

// CreateARInvoiceLegacy creates an invoice using the legacy input format.
func (r *Repository) CreateARInvoiceLegacy(ctx context.Context, input ARInvoiceInput) (*ARInvoice, error) {
	return r.CreateARInvoice(ctx, CreateARInvoiceInput{
		CustomerID: input.CustomerID,
		SOID:       input.SOID,
		Number:     input.Number,
		Currency:   input.Currency,
		Total:      input.Total,
		DueDate:    input.DueDate,
	})
}

// CreateARPaymentLegacy creates a payment using the legacy input format.
func (r *Repository) CreateARPaymentLegacy(ctx context.Context, input ARPaymentInput) (*ARPayment, error) {
	return r.CreateARPayment(ctx, CreateARPaymentInput{
		Number:  input.Number,
		Amount:  input.Amount,
		PaidAt:  input.PaidAt,
		Method:  input.Method,
		Note:    input.Note,
		Allocations: []PaymentAllocationInput{
			{ARInvoiceID: input.ARInvoiceID, Amount: input.Amount},
		},
	})
}

// --- Helpers ---

func numericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, _ := n.Float64Value()
	return f.Float64
}

// --- Transaction Support ---

// TxRepository exposes transactional operations.
type TxRepository interface {
	CreateARInvoice(ctx context.Context, input CreateARInvoiceInput) (*ARInvoice, error)
	CreateARInvoiceLine(ctx context.Context, invoiceID int64, line CreateARInvoiceLineInput) (*ARInvoiceLine, error)
	PostARInvoice(ctx context.Context, id int64, postedBy int64) error
	CreateARPayment(ctx context.Context, input CreateARPaymentInput) (*ARPayment, error)
	CreatePaymentAllocation(ctx context.Context, paymentID, invoiceID int64, amount float64) error
}

// WithTx wraps callback in repeatable-read transaction.
func (r *Repository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	wrapper := &txRepo{tx: tx}

	if err := fn(ctx, wrapper); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type txRepo struct {
	tx pgx.Tx
}

func (t *txRepo) CreateARInvoice(ctx context.Context, input CreateARInvoiceInput) (*ARInvoice, error) {
	// Simplified - use main repo logic with tx
	return nil, errors.New("not implemented in tx")
}

func (t *txRepo) CreateARInvoiceLine(ctx context.Context, invoiceID int64, line CreateARInvoiceLineInput) (*ARInvoiceLine, error) {
	return nil, errors.New("not implemented in tx")
}

func (t *txRepo) PostARInvoice(ctx context.Context, id int64, postedBy int64) error {
	return errors.New("not implemented in tx")
}

func (t *txRepo) CreateARPayment(ctx context.Context, input CreateARPaymentInput) (*ARPayment, error) {
	return nil, errors.New("not implemented in tx")
}

func (t *txRepo) CreatePaymentAllocation(ctx context.Context, paymentID, invoiceID int64, amount float64) error {
	return errors.New("not implemented in tx")
}
