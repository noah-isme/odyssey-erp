package ar

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) GenerateCreditNoteNumber(ctx context.Context) (string, error) {
	var number string
	err := r.pool.QueryRow(ctx, "SELECT generate_ar_credit_note_number()").Scan(&number)
	return number, err
}

func (r *Repository) GetInvoiceCreditAvailable(ctx context.Context, invoiceID int64) (float64, error) {
	var available float64
	err := r.pool.QueryRow(ctx, `
		SELECT i.total - COALESCE(SUM(a.amount), 0)
		FROM ar_invoices i
		LEFT JOIN ar_credit_note_allocations a ON a.ar_invoice_id = i.id
		WHERE i.id = $1
		GROUP BY i.id`, invoiceID).Scan(&available)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrInvoiceNotFound
	}
	return available, err
}

func (r *Repository) CreateARCreditNote(ctx context.Context, input CreateARCreditNoteInput) (*ARCreditNote, error) {
	if len(input.Lines) == 0 {
		return nil, errors.New("ar: credit note lines required")
	}
	var subtotal, taxAmount, total float64
	for _, line := range input.Lines {
		if line.Quantity <= 0 {
			return nil, errors.New("ar: credit note quantity must be positive")
		}
		lineSubtotal := line.Quantity * line.UnitPrice * (1 - line.DiscountPct/100)
		lineTax := lineSubtotal * line.TaxPct / 100
		subtotal += lineSubtotal
		taxAmount += lineTax
		total += lineSubtotal + lineTax
	}
	if total <= 0 {
		return nil, errors.New("ar: credit note total must be positive")
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const headerSQL = `
		INSERT INTO ar_credit_notes (
			number, customer_id, ar_invoice_id, return_delivery_order_id, currency, reason,
			subtotal, tax_amount, total, status, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'DRAFT',$10,NOW(),NOW())
		RETURNING id, created_at, updated_at`

	creditNote := &ARCreditNote{
		Number: input.Number, CustomerID: input.CustomerID, ARInvoiceID: input.ARInvoiceID,
		ReturnDeliveryOrderID: input.ReturnDeliveryOrderID, Currency: input.Currency, Reason: input.Reason,
		Subtotal: subtotal, TaxAmount: taxAmount, Total: total, Status: ARCreditNoteStatusDraft, CreatedBy: input.CreatedBy,
	}
	if err := tx.QueryRow(ctx, headerSQL, input.Number, input.CustomerID, input.ARInvoiceID,
		nullInt64(input.ReturnDeliveryOrderID), input.Currency, input.Reason, subtotal, taxAmount, total, input.CreatedBy,
	).Scan(&creditNote.ID, &creditNote.CreatedAt, &creditNote.UpdatedAt); err != nil {
		return nil, err
	}

	const lineSQL = `
		INSERT INTO ar_credit_note_lines (
			ar_credit_note_id, ar_invoice_line_id, return_delivery_order_line_id, product_id,
			description, quantity, unit_price, discount_pct, tax_pct, subtotal, tax_amount, total, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW())`
	for _, line := range input.Lines {
		lineSubtotal := line.Quantity * line.UnitPrice * (1 - line.DiscountPct/100)
		lineTax := lineSubtotal * line.TaxPct / 100
		if _, err := tx.Exec(ctx, lineSQL, creditNote.ID, nullInt64(line.ARInvoiceLineID),
			nullInt64(line.ReturnDeliveryOrderLineID), line.ProductID, line.Description, line.Quantity,
			line.UnitPrice, line.DiscountPct, line.TaxPct, lineSubtotal, lineTax, lineSubtotal+lineTax); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return creditNote, nil
}

func (r *Repository) GetARCreditNote(ctx context.Context, id int64) (*ARCreditNote, error) {
	const query = `
		SELECT cn.id, cn.number, cn.customer_id, cn.ar_invoice_id, cn.return_delivery_order_id,
		       cn.currency, cn.reason, cn.subtotal, cn.tax_amount, cn.total, cn.status,
		       cn.posted_at, cn.posted_by, cn.voided_at, cn.voided_by, cn.void_reason,
		       cn.created_by, cn.created_at, cn.updated_at, c.name, i.number
		FROM ar_credit_notes cn
		JOIN customers c ON c.id = cn.customer_id
		JOIN ar_invoices i ON i.id = cn.ar_invoice_id
		WHERE cn.id = $1`
	var note ARCreditNote
	var returnID, postedBy, voidedBy pgtype.Int8
	var postedAt, voidedAt pgtype.Timestamptz
	var voidReason pgtype.Text
	if err := r.pool.QueryRow(ctx, query, id).Scan(
		&note.ID, &note.Number, &note.CustomerID, &note.ARInvoiceID, &returnID,
		&note.Currency, &note.Reason, &note.Subtotal, &note.TaxAmount, &note.Total, &note.Status,
		&postedAt, &postedBy, &voidedAt, &voidedBy, &voidReason,
		&note.CreatedBy, &note.CreatedAt, &note.UpdatedAt, &note.CustomerName, &note.InvoiceNumber,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCreditNoteNotFound
		}
		return nil, err
	}
	note.ReturnDeliveryOrderID = returnID.Int64
	if postedAt.Valid {
		note.PostedAt = &postedAt.Time
	}
	if postedBy.Valid {
		note.PostedBy = &postedBy.Int64
	}
	if voidedAt.Valid {
		note.VoidedAt = &voidedAt.Time
	}
	if voidedBy.Valid {
		note.VoidedBy = &voidedBy.Int64
	}
	note.VoidReason = voidReason.String
	return &note, nil
}

func (r *Repository) GetARCreditNoteWithDetails(ctx context.Context, id int64) (*ARCreditNoteWithDetails, error) {
	note, err := r.GetARCreditNote(ctx, id)
	if err != nil {
		return nil, err
	}
	lines, err := r.listARCreditNoteLines(ctx, id)
	if err != nil {
		return nil, err
	}
	return &ARCreditNoteWithDetails{ARCreditNote: *note, Lines: lines}, nil
}

func (r *Repository) listARCreditNoteLines(ctx context.Context, id int64) ([]ARCreditNoteLine, error) {
	const query = `
		SELECT id, ar_credit_note_id, ar_invoice_line_id, return_delivery_order_line_id, product_id,
		       description, quantity, unit_price, discount_pct, tax_pct, subtotal, tax_amount, total, created_at
		FROM ar_credit_note_lines WHERE ar_credit_note_id = $1 ORDER BY id`
	rows, err := r.pool.Query(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []ARCreditNoteLine
	for rows.Next() {
		var line ARCreditNoteLine
		var invoiceLineID, returnLineID pgtype.Int8
		if err := rows.Scan(&line.ID, &line.ARCreditNoteID, &invoiceLineID, &returnLineID, &line.ProductID,
			&line.Description, &line.Quantity, &line.UnitPrice, &line.DiscountPct, &line.TaxPct,
			&line.Subtotal, &line.TaxAmount, &line.Total, &line.CreatedAt); err != nil {
			return nil, err
		}
		line.ARInvoiceLineID = invoiceLineID.Int64
		line.ReturnDeliveryOrderLineID = returnLineID.Int64
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func (r *Repository) ListARCreditNotes(ctx context.Context, req ListARCreditNotesRequest) ([]ARCreditNote, error) {
	query := `
		SELECT cn.id, cn.number, cn.customer_id, cn.ar_invoice_id, cn.return_delivery_order_id,
		       cn.currency, cn.reason, cn.subtotal, cn.tax_amount, cn.total, cn.status,
		       cn.created_by, cn.created_at, cn.updated_at, c.name, i.number
		FROM ar_credit_notes cn
		JOIN customers c ON c.id = cn.customer_id
		JOIN ar_invoices i ON i.id = cn.ar_invoice_id WHERE 1=1`
	var args []any
	if req.Status != "" {
		args = append(args, string(req.Status))
		query += fmt.Sprintf(" AND cn.status = $%d", len(args))
	}
	if req.CustomerID != 0 {
		args = append(args, req.CustomerID)
		query += fmt.Sprintf(" AND cn.customer_id = $%d", len(args))
	}
	if req.InvoiceID != 0 {
		args = append(args, req.InvoiceID)
		query += fmt.Sprintf(" AND cn.ar_invoice_id = $%d", len(args))
	}
	query += " ORDER BY cn.created_at DESC"
	if req.Limit > 0 {
		args = append(args, req.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	if req.Offset > 0 {
		args = append(args, req.Offset)
		query += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notes []ARCreditNote
	for rows.Next() {
		var note ARCreditNote
		var returnID pgtype.Int8
		if err := rows.Scan(&note.ID, &note.Number, &note.CustomerID, &note.ARInvoiceID, &returnID,
			&note.Currency, &note.Reason, &note.Subtotal, &note.TaxAmount, &note.Total, &note.Status,
			&note.CreatedBy, &note.CreatedAt, &note.UpdatedAt, &note.CustomerName, &note.InvoiceNumber); err != nil {
			return nil, err
		}
		note.ReturnDeliveryOrderID = returnID.Int64
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func (r *Repository) PostARCreditNote(ctx context.Context, id, invoiceID, postedBy int64, amount float64) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var available float64
	if err := tx.QueryRow(ctx, `
		SELECT i.total - COALESCE((SELECT SUM(a.amount) FROM ar_credit_note_allocations a WHERE a.ar_invoice_id = i.id), 0)
		FROM ar_invoices i WHERE i.id = $1 FOR UPDATE`, invoiceID).Scan(&available); err != nil {
		return err
	}
	if amount > available {
		return ErrCreditExceedsBalance
	}
	cmd, err := tx.Exec(ctx, `UPDATE ar_credit_notes SET status='POSTED', posted_at=NOW(), posted_by=$2, updated_at=NOW() WHERE id=$1 AND status='DRAFT'`, id, postedBy)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ar_credit_note_allocations (ar_credit_note_id, ar_invoice_id, amount) VALUES ($1,$2,$3)`, id, invoiceID, amount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) VoidARCreditNote(ctx context.Context, id, voidedBy int64, reason string) error {
	cmd, err := r.pool.Exec(ctx, `UPDATE ar_credit_notes SET status='VOID', voided_at=NOW(), voided_by=$2, void_reason=$3, updated_at=NOW() WHERE id=$1 AND status='DRAFT'`, id, voidedBy, strings.TrimSpace(reason))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvalidStatus
	}
	return nil
}

func nullInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}
