package quotations

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
	ErrNotFound      = errors.New("record not found")
)

type Repository interface {
	WithTx(ctx context.Context, fn func(context.Context, Repository) error) error
	Get(ctx context.Context, id int64) (*Quotation, error)
	GetByDocNumber(ctx context.Context, docNumber string) (*Quotation, error)
	List(ctx context.Context, req ListQuotationsRequest) ([]QuotationWithDetails, int, error)
	Create(ctx context.Context, quotation Quotation) (int64, error)
	Update(ctx context.Context, id int64, updates map[string]interface{}) error
	InsertLine(ctx context.Context, line QuotationLine) (int64, error)
	UpdateStatus(ctx context.Context, id int64, status QuotationStatus, userID int64, reason *string) error
	DeleteLines(ctx context.Context, quotationID int64) error
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

func (r *repository) Get(ctx context.Context, id int64) (*Quotation, error) {
	row, err := r.queries.GetQuotation(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	q := mapQuotationFromSqlc(row)

	lineRows, err := r.queries.GetQuotationLines(ctx, id)
	if err != nil {
		return nil, err
	}
	q.Lines = mapLinesFromSqlc(lineRows)
	return &q, nil
}

func (r *repository) GetByDocNumber(ctx context.Context, docNumber string) (*Quotation, error) {
	row, err := r.queries.GetQuotationByDocNumber(ctx, docNumber)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	q := mapQuotationFromSqlc(row)
	lineRows, err := r.queries.GetQuotationLines(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	q.Lines = mapLinesFromSqlc(lineRows)
	return &q, nil
}

func (r *repository) List(ctx context.Context, req ListQuotationsRequest) ([]QuotationWithDetails, int, error) {
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
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
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

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var quotations []QuotationWithDetails
	for rows.Next() {
		var q QuotationWithDetails
		var quoteDatePG, validUntilPG pgtype.Date 
		var subtotal, taxAmount, totalAmount pgtype.Numeric
		var approvedBy, rejectedBy pgtype.Int8
		var approvedAt, rejectedAt pgtype.Timestamptz
		var notes, rejectionReason, approvedByName, rejectedByName pgtype.Text
		var createdAt, updatedAt pgtype.Timestamptz

		err := rows.Scan(
			&q.ID, &q.DocNumber, &q.CompanyID, &q.CustomerID, &quoteDatePG, &validUntilPG,
			&q.Status, &q.Currency, &subtotal, &taxAmount, &totalAmount, &notes,
			&q.CreatedBy, &approvedBy, &approvedAt, &rejectedBy, &rejectedAt,
			&rejectionReason, &createdAt, &updatedAt,
			&q.CustomerName, &q.CreatedByName, &approvedByName, &rejectedByName,
		)
		if err != nil {
			return nil, 0, err
		}

		if quoteDatePG.Valid { q.QuoteDate = quoteDatePG.Time }
		if validUntilPG.Valid { q.ValidUntil = validUntilPG.Time }
		if subtotal.Valid { f, _ := subtotal.Float64Value(); q.Subtotal = f.Float64 }
		if taxAmount.Valid { f, _ := taxAmount.Float64Value(); q.TaxAmount = f.Float64 }
		if totalAmount.Valid { f, _ := totalAmount.Float64Value(); q.TotalAmount = f.Float64 }
		if notes.Valid { q.Notes = &notes.String }
		if approvedBy.Valid { q.ApprovedBy = &approvedBy.Int64 }
		if approvedAt.Valid { q.ApprovedAt = &approvedAt.Time }
		if rejectedBy.Valid { q.RejectedBy = &rejectedBy.Int64 }
		if rejectedAt.Valid { q.RejectedAt = &rejectedAt.Time }
		if rejectionReason.Valid { q.RejectionReason = &rejectionReason.String }
		if createdAt.Valid { q.CreatedAt = createdAt.Time }
		if updatedAt.Valid { q.UpdatedAt = updatedAt.Time }
		if approvedByName.Valid { q.ApprovedByName = &approvedByName.String }
		if rejectedByName.Valid { q.RejectedByName = &rejectedByName.String }
		
		quotations = append(quotations, q)
	}

	return quotations, total, rows.Err()
}

func (r *repository) Create(ctx context.Context, q Quotation) (int64, error) {
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

	return r.queries.CreateQuotation(ctx, sqlc.CreateQuotationParams{
		DocNumber:   q.DocNumber,
		CompanyID:   q.CompanyID,
		CustomerID:  q.CustomerID,
		QuoteDate:   quoteDate,
		ValidUntil:  validUntil,
		Status:      sqlc.QuotationStatus(q.Status),
		Currency:    q.Currency,
		Subtotal:    subtotal,
		TaxAmount:   taxAmount,
		TotalAmount: totalAmount,
		Notes:       pgtype.Text{String: getString(q.Notes), Valid: q.Notes != nil},
		CreatedBy:   q.CreatedBy,
	})
}

func (r *repository) Update(ctx context.Context, id int64, updates map[string]interface{}) error {
	query := "UPDATE quotations SET updated_at = NOW()"
	var args []interface{}
	argPos := 1
	
	if v, ok := updates["quote_date"]; ok {
		query += fmt.Sprintf(", quote_date = $%d", argPos)
		args = append(args, v)
		argPos++
	}
	if v, ok := updates["valid_until"]; ok {
		query += fmt.Sprintf(", valid_until = $%d", argPos)
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

func (r *repository) InsertLine(ctx context.Context, line QuotationLine) (int64, error) {
	var quantity, unitPrice, discountPercent, discountAmount, taxPercent, taxAmount, lineTotal pgtype.Numeric
	quantity.Scan(fmt.Sprintf("%f", line.Quantity))
	unitPrice.Scan(fmt.Sprintf("%f", line.UnitPrice))
	discountPercent.Scan(fmt.Sprintf("%f", line.DiscountPercent))
	discountAmount.Scan(fmt.Sprintf("%f", line.DiscountAmount))
	taxPercent.Scan(fmt.Sprintf("%f", line.TaxPercent))
	taxAmount.Scan(fmt.Sprintf("%f", line.TaxAmount))
	lineTotal.Scan(fmt.Sprintf("%f", line.LineTotal))
	
	return r.queries.InsertQuotationLine(ctx, sqlc.InsertQuotationLineParams{
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

func (r *repository) UpdateStatus(ctx context.Context, id int64, status QuotationStatus, userID int64, reason *string) error {
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

	return r.queries.UpdateQuotationStatus(ctx, sqlc.UpdateQuotationStatusParams{
		Status:          sqlc.QuotationStatus(status),
		ID:              id,
		ApprovedBy:      approvedBy,
		ApprovedAt:      approvedAt,
		RejectedBy:      rejectedBy,
		RejectedAt:      rejectedAt,
		RejectionReason: rejectionReason,
	})
}

func (r *repository) DeleteLines(ctx context.Context, quotationID int64) error {
	return r.queries.DeleteQuotationLines(ctx, quotationID)
}

func (r *repository) GenerateNumber(ctx context.Context, companyID int64, date time.Time) (string, error) {
	// QT-{YY}{MM}-{SEQ}
	var seq int64
	period := date.Format("200601")
	err := r.db.QueryRow(ctx, `
		INSERT INTO document_sequences (company_id, doc_type, period, seq)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (company_id, doc_type, period)
		DO UPDATE SET seq = document_sequences.seq + 1
		RETURNING seq
	`, companyID, "QT", period).Scan(&seq)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("QT-%s-%04d", date.Format("0601"), seq), nil
}

func mapQuotationFromSqlc(row sqlc.Quotation) Quotation {
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
	return q
}

func mapLinesFromSqlc(rows []sqlc.QuotationLine) []QuotationLine {
	var lines []QuotationLine
	for _, l := range rows {
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
	return lines
}

func getString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
