package orders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/platform/db"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

var (
	ErrNotFound = errors.New("record not found")
)

type Repository interface {
	WithTx(ctx context.Context, fn func(context.Context, Repository) error) error
	Get(ctx context.Context, id int64) (*SalesOrder, error)
	GetByDocNumber(ctx context.Context, docNumber string) (*SalesOrder, error)
	List(ctx context.Context, req ListSalesOrdersRequest) ([]SalesOrderWithDetails, int, error)
	Create(ctx context.Context, order SalesOrder) (int64, error)
	Update(ctx context.Context, id int64, updates map[string]interface{}) error
	InsertLine(ctx context.Context, line SalesOrderLine) (int64, error)
	UpdateStatus(ctx context.Context, id int64, status SalesOrderStatus, userID int64, reason *string) error
	DeleteLines(ctx context.Context, orderID int64) error
	GenerateNumber(ctx context.Context, companyID int64, date time.Time) (string, error)
}

type dbtx interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

type repository struct {
	db      dbtx
	queries *sqlc.Queries
	pool    *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{
		db:      pool,
		queries: sqlc.New(pool),
		pool:    pool,
	}
}

func (r *repository) WithTx(ctx context.Context, fn func(context.Context, Repository) error) error {
	return db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		repoTx := &repository{
			db:      tx,
			queries: r.queries.WithTx(tx),
			pool:    r.pool,
		}
		return fn(ctx, repoTx)
	})
}

func (r *repository) Get(ctx context.Context, id int64) (*SalesOrder, error) {
	row, err := r.queries.GetSalesOrder(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	o := mapOrderFromSqlc(row)
	
	lineRows, err := r.queries.GetSalesOrderLines(ctx, id)
	if err != nil {
		return nil, err
	}
	o.Lines = mapLinesFromSqlc(lineRows)
	
	return &o, nil
}

func (r *repository) GetByDocNumber(ctx context.Context, docNumber string) (*SalesOrder, error) {
	row, err := r.queries.GetSalesOrderByDocNumber(ctx, docNumber)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	o := mapOrderFromSqlc(row)
	lineRows, err := r.queries.GetSalesOrderLines(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	o.Lines = mapLinesFromSqlc(lineRows)
	return &o, nil
}

func (r *repository) List(ctx context.Context, req ListSalesOrdersRequest) ([]SalesOrderWithDetails, int, error) {
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

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM sales_orders so %s", whereClause)
	var total int
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT so.id, so.doc_number, so.company_id, so.customer_id, so.quotation_id,
		       so.order_date, so.expected_delivery_date, so.status, so.currency,
		       so.subtotal, so.tax_amount, so.total_amount, so.notes,
		       so.created_by, so.confirmed_by, so.confirmed_at,
		       so.cancelled_by, so.cancelled_at, so.cancellation_reason,
		       so.created_at, so.updated_at,
		       c.name as customer_name,
		       u1.full_name as created_by_name,
		       u2.full_name as confirmed_by_name,
		       u3.full_name as cancelled_by_name
		FROM sales_orders so
		JOIN customers c ON so.customer_id = c.id
		JOIN users u1 ON so.created_by = u1.id
		LEFT JOIN users u2 ON so.confirmed_by = u2.id
		LEFT JOIN users u3 ON so.cancelled_by = u3.id
		%s
		ORDER BY so.order_date DESC, so.id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, req.Limit, req.Offset)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var orders []SalesOrderWithDetails
	for rows.Next() {
		var o SalesOrderWithDetails
		var quotationID, confirmedBy, cancelledBy pgtype.Int8
		var expectedDelivery, confirmedAt, cancelledAt pgtype.Timestamptz
		var orderDatePG pgtype.Date
		var notes, cancellationReason, confirmedByName, cancelledByName pgtype.Text
		var subtotal, taxAmount, totalAmount pgtype.Numeric
		var createdAt, updatedAt pgtype.Timestamptz

		err := rows.Scan(
			&o.ID, &o.DocNumber, &o.CompanyID, &o.CustomerID, &quotationID,
			&orderDatePG, &expectedDelivery, &o.Status, &o.Currency,
			&subtotal, &taxAmount, &totalAmount, &notes,
			&o.CreatedBy, &confirmedBy, &confirmedAt,
			&cancelledBy, &cancelledAt, &cancellationReason,
			&createdAt, &updatedAt,
			&o.CustomerName, &o.CreatedByName, &confirmedByName, &cancelledByName,
		)
		if err != nil {
			return nil, 0, err
		}

		if quotationID.Valid { o.QuotationID = &quotationID.Int64 }
		if orderDatePG.Valid { o.OrderDate = orderDatePG.Time }
		if expectedDelivery.Valid { o.ExpectedDeliveryDate = &expectedDelivery.Time }
		if subtotal.Valid { f, _ := subtotal.Float64Value(); o.Subtotal = f.Float64 }
		if taxAmount.Valid { f, _ := taxAmount.Float64Value(); o.TaxAmount = f.Float64 }
		if totalAmount.Valid { f, _ := totalAmount.Float64Value(); o.TotalAmount = f.Float64 }
		if notes.Valid { o.Notes = &notes.String }
		if confirmedBy.Valid { o.ConfirmedBy = &confirmedBy.Int64 }
		if confirmedAt.Valid { o.ConfirmedAt = &confirmedAt.Time }
		if cancelledBy.Valid { o.CancelledBy = &cancelledBy.Int64 }
		if cancelledAt.Valid { o.CancelledAt = &cancelledAt.Time }
		if cancellationReason.Valid { o.CancellationReason = &cancellationReason.String }
		if createdAt.Valid { o.CreatedAt = createdAt.Time }
		if updatedAt.Valid { o.UpdatedAt = updatedAt.Time }
		if confirmedByName.Valid { o.ConfirmedByName = &confirmedByName.String }
		if cancelledByName.Valid { o.CancelledByName = &cancelledByName.String }

		orders = append(orders, o)
	}

	return orders, total, rows.Err()
}

func (r *repository) Create(ctx context.Context, o SalesOrder) (int64, error) {
	var quotationID pgtype.Int8
	if o.QuotationID != nil {
		quotationID = pgtype.Int8{Int64: *o.QuotationID, Valid: true}
	}
	var expectedDelivery pgtype.Date
	if o.ExpectedDeliveryDate != nil {
		expectedDelivery = pgtype.Date{Time: *o.ExpectedDeliveryDate, Valid: true}
	}
	var subtotal, taxAmount, totalAmount pgtype.Numeric
	subtotal.Scan(fmt.Sprintf("%f", o.Subtotal))
	taxAmount.Scan(fmt.Sprintf("%f", o.TaxAmount))
	totalAmount.Scan(fmt.Sprintf("%f", o.TotalAmount))

	var orderDate pgtype.Date
	if !o.OrderDate.IsZero() {
		orderDate = pgtype.Date{Time: o.OrderDate, Valid: true}
	}

	return r.queries.CreateSalesOrder(ctx, sqlc.CreateSalesOrderParams{
		DocNumber:            o.DocNumber,
		CompanyID:            o.CompanyID,
		CustomerID:           o.CustomerID,
		QuotationID:          quotationID,
		OrderDate:            orderDate,
		ExpectedDeliveryDate: expectedDelivery,
		Status:               sqlc.SalesOrderStatus(o.Status),
		Currency:             o.Currency,
		Subtotal:             subtotal,
		TaxAmount:            taxAmount,
		TotalAmount:          totalAmount,
		Notes:                pgtype.Text{String: getString(o.Notes), Valid: o.Notes != nil},
		CreatedBy:            o.CreatedBy,
	})
}

func (r *repository) Update(ctx context.Context, id int64, updates map[string]interface{}) error {
	query := "UPDATE sales_orders SET updated_at = NOW()"
	var args []interface{}
	argPos := 1
	
	if v, ok := updates["order_date"]; ok {
		query += fmt.Sprintf(", order_date = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["expected_delivery_date"]; ok {
		query += fmt.Sprintf(", expected_delivery_date = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["notes"]; ok {
		query += fmt.Sprintf(", notes = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["subtotal"]; ok {
		query += fmt.Sprintf(", subtotal = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["tax_amount"]; ok {
		query += fmt.Sprintf(", tax_amount = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["total_amount"]; ok {
		query += fmt.Sprintf(", total_amount = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	
	query += fmt.Sprintf(" WHERE id = $%d", argPos)
	args = append(args, id)
	
	_, err := r.db.Exec(ctx, query, args...)
	return err
}

func (r *repository) InsertLine(ctx context.Context, line SalesOrderLine) (int64, error) {
	var quantity, unitPrice, discountPercent, discountAmount, taxPercent, taxAmount, lineTotal pgtype.Numeric
	quantity.Scan(fmt.Sprintf("%f", line.Quantity))
	unitPrice.Scan(fmt.Sprintf("%f", line.UnitPrice))
	discountPercent.Scan(fmt.Sprintf("%f", line.DiscountPercent))
	discountAmount.Scan(fmt.Sprintf("%f", line.DiscountAmount))
	taxPercent.Scan(fmt.Sprintf("%f", line.TaxPercent))
	taxAmount.Scan(fmt.Sprintf("%f", line.TaxAmount))
	lineTotal.Scan(fmt.Sprintf("%f", line.LineTotal))
	
	return r.queries.InsertSalesOrderLine(ctx, sqlc.InsertSalesOrderLineParams{
		SalesOrderID:    line.SalesOrderID,
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

func (r *repository) UpdateStatus(ctx context.Context, id int64, status SalesOrderStatus, userID int64, reason *string) error {
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

	return r.queries.UpdateSalesOrderStatus(ctx, sqlc.UpdateSalesOrderStatusParams{
		Status:             sqlc.SalesOrderStatus(status),
		ID:                 id,
		ConfirmedBy:        confirmedBy,
		ConfirmedAt:        confirmedAt,
		CancelledBy:        cancelledBy,
		CancelledAt:        cancelledAt,
		CancellationReason: cancellationReason,
	})
}

func (r *repository) DeleteLines(ctx context.Context, orderID int64) error {
	return r.queries.DeleteSalesOrderLines(ctx, orderID)
}

func (r *repository) GenerateNumber(ctx context.Context, companyID int64, date time.Time) (string, error) {
	// SO-{YY}{MM}-{SEQ}
	var count int64
	err := r.db.QueryRow(ctx, "SELECT count(*) FROM sales_orders WHERE company_id = $1", companyID).Scan(&count)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("SO-%s-%04d", date.Format("0601"), count+1), nil
}

func mapOrderFromSqlc(row sqlc.SalesOrder) SalesOrder {
	o := SalesOrder{
		ID:          row.ID,
		DocNumber:   row.DocNumber,
		CompanyID:   row.CompanyID,
		CustomerID:  row.CustomerID,
		Status:      SalesOrderStatus(row.Status),
		Currency:    row.Currency,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
	if row.QuotationID.Valid {
		val := row.QuotationID.Int64
		o.QuotationID = &val
	}
	if row.OrderDate.Valid {
		o.OrderDate = row.OrderDate.Time
	}
	if row.ExpectedDeliveryDate.Valid {
		val := row.ExpectedDeliveryDate.Time
		o.ExpectedDeliveryDate = &val
	}
	if row.Subtotal.Valid {
		f, _ := row.Subtotal.Float64Value()
		o.Subtotal = f.Float64
	}
	if row.TaxAmount.Valid {
		f, _ := row.TaxAmount.Float64Value()
		o.TaxAmount = f.Float64
	}
	if row.TotalAmount.Valid {
		f, _ := row.TotalAmount.Float64Value()
		o.TotalAmount = f.Float64
	}
	if row.Notes.Valid {
		val := row.Notes.String
		o.Notes = &val
	}
	if row.ConfirmedBy.Valid {
		val := row.ConfirmedBy.Int64
		o.ConfirmedBy = &val
	}
	if row.ConfirmedAt.Valid {
		val := row.ConfirmedAt.Time
		o.ConfirmedAt = &val
	}
	if row.CancelledBy.Valid {
		val := row.CancelledBy.Int64
		o.CancelledBy = &val
	}
	if row.CancelledAt.Valid {
		val := row.CancelledAt.Time
		o.CancelledAt = &val
	}
	if row.CancellationReason.Valid {
		val := row.CancellationReason.String
		o.CancellationReason = &val
	}
	return o
}

func mapLinesFromSqlc(rows []sqlc.SalesOrderLine) []SalesOrderLine {
	var lines []SalesOrderLine
	for _, l := range rows {
		line := SalesOrderLine{
			ID:              l.ID,
			SalesOrderID:    l.SalesOrderID,
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
	return lines
}

func getString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
