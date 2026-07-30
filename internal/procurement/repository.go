package procurement

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository provides PostgreSQL backed persistence.
type Repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewRepository constructs a repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// TxRepository exposes transactional operations.
type TxRepository interface {
	CreatePR(ctx context.Context, pr PurchaseRequest) (int64, error)
	InsertPRLine(ctx context.Context, line PRLine) error
	UpdatePRStatus(ctx context.Context, id int64, status PRStatus) error
	CreatePO(ctx context.Context, po PurchaseOrder) (int64, error)
	InsertPOLine(ctx context.Context, line POLine) error
	UpdatePOStatus(ctx context.Context, id int64, status POStatus) error
	SetPOApproval(ctx context.Context, id int64, approvedBy int64, approvedAt time.Time) error
	CreateGRN(ctx context.Context, grn GoodsReceipt) (int64, error)
	InsertGRNLine(ctx context.Context, line GRNLine) error
	UpdateGRNStatus(ctx context.Context, id int64, status GRNStatus) error

	// Goods return operations
	CreateGoodsReturnGRN(ctx context.Context, ret GoodsReturnGRN) (int64, error)
	InsertGoodsReturnGRNLine(ctx context.Context, line GoodsReturnGRNLine) error
	ConfirmGoodsReturnGRN(ctx context.Context, id int64, actorID int64) error
	CancelGoodsReturnGRN(ctx context.Context, id int64, actorID int64) error
	GenerateGoodsReturnGRNNumber(ctx context.Context) (string, error)
}

type txRepo struct {
	queries *sqlc.Queries
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

// Fetch helpers

// GetPR returns purchase request and lines.
func (r *Repository) GetPR(ctx context.Context, id int64) (PurchaseRequest, []PRLine, error) {
	row, err := r.queries.GetPR(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PurchaseRequest{}, nil, ErrNotFound
		}
		return PurchaseRequest{}, nil, err
	}
	pr := PurchaseRequest{
		ID:        row.ID,
		Number:    row.Number,
		RequestBy: row.RequestBy,
		Status:    PRStatus(row.Status),
		Note:      row.Note,
	}
	if row.SupplierID.Valid {
		pr.SupplierID = row.SupplierID.Int64
	}
	if row.CompanyID.Valid {
		pr.CompanyID = row.CompanyID.Int64
	}

	lineRows, err := r.queries.GetPRLines(ctx, id)
	if err != nil {
		return PurchaseRequest{}, nil, err
	}
	var lines []PRLine
	for _, l := range lineRows {
		line := PRLine{
			ID:        l.ID,
			PRID:      l.PrID,
			ProductID: l.ProductID,
			Note:      l.Note,
		}
		if l.Qty.Valid {
			f, _ := l.Qty.Float64Value()
			line.Qty = f.Float64
		}
		lines = append(lines, line)
	}
	return pr, lines, nil
}

// GetPO returns purchase order and lines.
func (r *Repository) GetPO(ctx context.Context, id int64) (PurchaseOrder, []POLine, error) {
	row, err := r.queries.GetPO(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PurchaseOrder{}, nil, ErrNotFound
		}
		return PurchaseOrder{}, nil, err
	}
	po := PurchaseOrder{
		ID:         row.ID,
		Number:     row.Number,
		SupplierID: row.SupplierID,
		Status:     POStatus(row.Status),
		Currency:   row.Currency,
		Note:       row.Note,
	}
	if row.ExpectedDate.Valid {
		po.ExpectedDate = row.ExpectedDate.Time
	}
	if row.CompanyID.Valid {
		po.CompanyID = row.CompanyID.Int64
	}

	lineRows, err := r.queries.GetPOLines(ctx, id)
	if err != nil {
		return PurchaseOrder{}, nil, err
	}
	var lines []POLine
	for _, l := range lineRows {
		line := POLine{
			ID:        l.ID,
			POID:      l.PoID,
			ProductID: l.ProductID,
			Note:      l.Note,
		}
		if l.Qty.Valid {
			f, _ := l.Qty.Float64Value()
			line.Qty = f.Float64
		}
		if l.Price.Valid {
			f, _ := l.Price.Float64Value()
			line.Price = f.Float64
		}
		if l.TaxID.Valid {
			line.TaxID = l.TaxID.Int64
		}
		lines = append(lines, line)
	}
	return po, lines, nil
}

// GetGRN returns GRN and lines.
func (r *Repository) GetGRN(ctx context.Context, id int64) (GoodsReceipt, []GRNLine, error) {
	row, err := r.queries.GetGRN(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GoodsReceipt{}, nil, ErrNotFound
		}
		return GoodsReceipt{}, nil, err
	}
	grn := GoodsReceipt{
		ID:          row.ID,
		Number:      row.Number,
		SupplierID:  row.SupplierID,
		WarehouseID: row.WarehouseID,
		Status:      GRNStatus(row.Status),
		Note:        row.Note,
	}
	if row.PoID.Valid {
		grn.POID = row.PoID.Int64
	}
	if row.ReceivedAt.Valid {
		grn.ReceivedAt = row.ReceivedAt.Time
	}

	lineRows, err := r.pool.Query(ctx, `SELECT id, grn_id, product_id, qty, unit_cost, lot_number, expiry_date, serial_numbers FROM grn_lines WHERE grn_id = $1 ORDER BY id`, id)
	if err != nil {
		return GoodsReceipt{}, nil, err
	}
	defer lineRows.Close()
	var lines []GRNLine
	for lineRows.Next() {
		line := GRNLine{}
		var qty, unitCost pgtype.Numeric
		var lot pgtype.Text
		var expiry pgtype.Date
		if err := lineRows.Scan(&line.ID, &line.GRNID, &line.ProductID, &qty, &unitCost, &lot, &expiry, &line.SerialNumbers); err != nil {
			return GoodsReceipt{}, nil, err
		}
		quantity, _ := qty.Float64Value()
		cost, _ := unitCost.Float64Value()
		line.Qty = quantity.Float64
		line.UnitCost = cost.Float64
		if lot.Valid {
			line.LotNumber = lot.String
		}
		if expiry.Valid {
			value := expiry.Time
			line.ExpiryDate = &value
		}
		lines = append(lines, line)
	}
	if err := lineRows.Err(); err != nil {
		return GoodsReceipt{}, nil, err
	}
	return grn, lines, nil
}

// POExistsByNumber checks if a PO with the given number already exists.
func (r *Repository) POExistsByNumber(ctx context.Context, number string) (bool, error) {
	return r.queries.POExistsByNumber(ctx, number)
}

// ListPOs returns purchase orders with supplier name and total.
func (r *Repository) ListPOs(ctx context.Context, limit, offset int, filters ListFilters) ([]POListItem, int, error) {
	// Count query
	countSQL := `SELECT COUNT(*) FROM pos p WHERE 1=1`
	args := []any{}
	argNum := 1

	if filters.Status != "" {
		countSQL += ` AND p.status = $` + itoa(argNum)
		args = append(args, filters.Status)
		argNum++
	}
	if filters.SupplierID > 0 {
		countSQL += ` AND p.supplier_id = $` + itoa(argNum)
		args = append(args, filters.SupplierID)
		argNum++
	}
	if filters.Search != "" {
		countSQL += ` AND p.number ILIKE $` + itoa(argNum)
		args = append(args, "%"+filters.Search+"%")
	}

	var total int
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data query with JOIN
	dataSQL := `SELECT p.id, p.number, p.supplier_id, COALESCE(s.name, '') AS supplier_name,
		p.status, p.currency, COALESCE(p.expected_date, CURRENT_DATE), p.created_at,
		COALESCE((SELECT SUM(qty * price) FROM po_lines WHERE po_id = p.id), 0) AS total
	FROM pos p
	LEFT JOIN suppliers s ON s.id = p.supplier_id
	WHERE 1=1`

	args2 := []any{}
	argNum2 := 1
	if filters.Status != "" {
		dataSQL += ` AND p.status = $` + itoa(argNum2)
		args2 = append(args2, filters.Status)
		argNum2++
	}
	if filters.SupplierID > 0 {
		dataSQL += ` AND p.supplier_id = $` + itoa(argNum2)
		args2 = append(args2, filters.SupplierID)
		argNum2++
	}
	if filters.Search != "" {
		dataSQL += ` AND p.number ILIKE $` + itoa(argNum2)
		args2 = append(args2, "%"+filters.Search+"%")
		argNum2++
	}

	// ORDER BY with sorting
	orderBy := sortOrderPO(filters.SortBy, filters.SortDir)
	dataSQL += ` ORDER BY ` + orderBy + ` LIMIT $` + itoa(argNum2) + ` OFFSET $` + itoa(argNum2+1)
	args2 = append(args2, limit, offset)

	rows, err := r.pool.Query(ctx, dataSQL, args2...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []POListItem
	for rows.Next() {
		var item POListItem
		if err := rows.Scan(&item.ID, &item.Number, &item.SupplierID, &item.SupplierName,
			&item.Status, &item.Currency, &item.ExpectedDate, &item.CreatedAt, &item.Total); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListGRNs returns goods receipts with supplier and warehouse names.
func (r *Repository) ListGRNs(ctx context.Context, limit, offset int, filters ListFilters) ([]GRNListItem, int, error) {
	// Count query
	countSQL := `SELECT COUNT(*) FROM grns g WHERE 1=1`
	args := []any{}
	argNum := 1

	if filters.Status != "" {
		countSQL += ` AND g.status = $` + itoa(argNum)
		args = append(args, filters.Status)
		argNum++
	}
	if filters.SupplierID > 0 {
		countSQL += ` AND g.supplier_id = $` + itoa(argNum)
		args = append(args, filters.SupplierID)
		argNum++
	}
	if filters.Search != "" {
		countSQL += ` AND g.number ILIKE $` + itoa(argNum)
		args = append(args, "%"+filters.Search+"%")
	}

	var total int
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data query with JOINs
	dataSQL := `SELECT g.id, g.number, COALESCE(g.po_id, 0), COALESCE(p.number, '') AS po_number,
		g.supplier_id, COALESCE(s.name, '') AS supplier_name,
		g.warehouse_id, COALESCE(w.name, '') AS warehouse_name,
		g.status, g.received_at, g.created_at
	FROM grns g
	LEFT JOIN pos p ON p.id = g.po_id
	LEFT JOIN suppliers s ON s.id = g.supplier_id
	LEFT JOIN warehouses w ON w.id = g.warehouse_id
	WHERE 1=1`

	args2 := []any{}
	argNum2 := 1
	if filters.Status != "" {
		dataSQL += ` AND g.status = $` + itoa(argNum2)
		args2 = append(args2, filters.Status)
		argNum2++
	}
	if filters.SupplierID > 0 {
		dataSQL += ` AND g.supplier_id = $` + itoa(argNum2)
		args2 = append(args2, filters.SupplierID)
		argNum2++
	}
	if filters.Search != "" {
		dataSQL += ` AND g.number ILIKE $` + itoa(argNum2)
		args2 = append(args2, "%"+filters.Search+"%")
		argNum2++
	}

	// ORDER BY with sorting
	orderBy := sortOrderGRN(filters.SortBy, filters.SortDir)
	dataSQL += ` ORDER BY ` + orderBy + ` LIMIT $` + itoa(argNum2) + ` OFFSET $` + itoa(argNum2+1)
	args2 = append(args2, limit, offset)

	rows, err := r.pool.Query(ctx, dataSQL, args2...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []GRNListItem
	for rows.Next() {
		var item GRNListItem
		if err := rows.Scan(&item.ID, &item.Number, &item.POID, &item.PONumber,
			&item.SupplierID, &item.SupplierName, &item.WarehouseID, &item.WarehouseName,
			&item.Status, &item.ReceivedAt, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// itoa converts int to string for dynamic query building.
func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

// numericOf converts a float64 into a pgtype.Numeric. Scanning a formatted
// finite float cannot fail, so the (impossible) error is deliberately ignored.
func numericOf(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%f", f))
	return n
}

// sortOrderPO returns a safe ORDER BY clause for PO queries.
func sortOrderPO(sortBy, sortDir string) string {
	dir := "DESC"
	if sortDir == "asc" {
		dir = "ASC"
	}
	switch sortBy {
	case "number":
		return "p.number " + dir
	case "supplier":
		return "supplier_name " + dir
	case "expected_date":
		return "p.expected_date " + dir
	case "total":
		return "total " + dir
	case "status":
		return "p.status " + dir
	default:
		return "p.created_at DESC"
	}
}

// sortOrderGRN returns a safe ORDER BY clause for GRN queries.
func sortOrderGRN(sortBy, sortDir string) string {
	dir := "DESC"
	if sortDir == "asc" {
		dir = "ASC"
	}
	switch sortBy {
	case "number":
		return "g.number " + dir
	case "supplier":
		return "supplier_name " + dir
	case "received_at":
		return "g.received_at " + dir
	case "status":
		return "g.status " + dir
	default:
		return "g.created_at DESC"
	}
}

func (tx *txRepo) CreatePR(ctx context.Context, pr PurchaseRequest) (int64, error) {
	var supplierID pgtype.Int8
	if pr.SupplierID != 0 {
		supplierID = pgtype.Int8{Int64: pr.SupplierID, Valid: true}
	}
	return tx.queries.CreatePR(ctx, sqlc.CreatePRParams{
		Number:     pr.Number,
		SupplierID: supplierID,
		RequestBy:  pr.RequestBy,
		Status:     string(pr.Status),
		Note:       pr.Note,
		CompanyID:  pgtype.Int8{Int64: pr.CompanyID, Valid: pr.CompanyID > 0},
	})
}

func (tx *txRepo) InsertPRLine(ctx context.Context, line PRLine) error {
	qty := numericOf(line.Qty)

	return tx.queries.InsertPRLine(ctx, sqlc.InsertPRLineParams{
		PrID:      line.PRID,
		ProductID: line.ProductID,
		Qty:       qty,
		Note:      line.Note,
	})
}

func (tx *txRepo) UpdatePRStatus(ctx context.Context, id int64, status PRStatus) error {
	return tx.queries.UpdatePRStatus(ctx, sqlc.UpdatePRStatusParams{
		Status: string(status),
		ID:     id,
	})
}

func (tx *txRepo) CreatePO(ctx context.Context, po PurchaseOrder) (int64, error) {
	var expectedDate pgtype.Date
	if !po.ExpectedDate.IsZero() {
		expectedDate = pgtype.Date{Time: po.ExpectedDate, Valid: true}
	}
	return tx.queries.CreatePO(ctx, sqlc.CreatePOParams{
		Number:       po.Number,
		SupplierID:   po.SupplierID,
		Status:       string(po.Status),
		Currency:     po.Currency,
		ExpectedDate: expectedDate,
		Note:         po.Note,
		CompanyID:    pgtype.Int8{Int64: po.CompanyID, Valid: po.CompanyID > 0},
	})
}

func (tx *txRepo) InsertPOLine(ctx context.Context, line POLine) error {
	qty := numericOf(line.Qty)
	price := numericOf(line.Price)
	var taxID pgtype.Int8
	if line.TaxID != 0 {
		taxID = pgtype.Int8{Int64: line.TaxID, Valid: true}
	}

	return tx.queries.InsertPOLine(ctx, sqlc.InsertPOLineParams{
		PoID:      line.POID,
		ProductID: line.ProductID,
		Qty:       qty,
		Price:     price,
		TaxID:     taxID,
		Note:      line.Note,
	})
}

func (tx *txRepo) UpdatePOStatus(ctx context.Context, id int64, status POStatus) error {
	return tx.queries.UpdatePOStatus(ctx, sqlc.UpdatePOStatusParams{
		Status: string(status),
		ID:     id,
	})
}

func (tx *txRepo) SetPOApproval(ctx context.Context, id int64, approvedBy int64, approvedAt time.Time) error {
	var appBy pgtype.Int8
	if approvedBy != 0 {
		appBy = pgtype.Int8{Int64: approvedBy, Valid: true}
	}
	var appAt pgtype.Timestamptz
	if !approvedAt.IsZero() {
		appAt = pgtype.Timestamptz{Time: approvedAt, Valid: true}
	}

	return tx.queries.SetPOApproval(ctx, sqlc.SetPOApprovalParams{
		ApprovedBy: appBy,
		ApprovedAt: appAt,
		ID:         id,
	})
}

func (tx *txRepo) CreateGRN(ctx context.Context, grn GoodsReceipt) (int64, error) {
	var poID pgtype.Int8
	if grn.POID != 0 {
		poID = pgtype.Int8{Int64: grn.POID, Valid: true}
	}
	var receivedAt pgtype.Timestamptz
	if !grn.ReceivedAt.IsZero() {
		receivedAt = pgtype.Timestamptz{Time: grn.ReceivedAt, Valid: true}
	}

	return tx.queries.CreateGRN(ctx, sqlc.CreateGRNParams{
		Number:      grn.Number,
		PoID:        poID,
		SupplierID:  grn.SupplierID,
		WarehouseID: grn.WarehouseID,
		Status:      string(grn.Status),
		ReceivedAt:  receivedAt,
		Note:        grn.Note,
	})
}

func (tx *txRepo) InsertGRNLine(ctx context.Context, line GRNLine) error {
	var expiry any
	if line.ExpiryDate != nil {
		expiry = *line.ExpiryDate
	}
	_, err := tx.tx.Exec(ctx, `INSERT INTO grn_lines (grn_id, product_id, qty, unit_cost, lot_number, expiry_date, serial_numbers)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7)`, line.GRNID, line.ProductID, line.Qty, line.UnitCost, line.LotNumber, expiry, line.SerialNumbers)
	return err
}

func (tx *txRepo) UpdateGRNStatus(ctx context.Context, id int64, status GRNStatus) error {
	return tx.queries.UpdateGRNStatus(ctx, sqlc.UpdateGRNStatusParams{
		Status: string(status),
		ID:     id,
	})
}

func (tx *txRepo) CreateGoodsReturnGRN(ctx context.Context, ret GoodsReturnGRN) (int64, error) {
	return tx.queries.CreateGoodsReturnGRN(ctx, sqlc.CreateGoodsReturnGRNParams{
		Number:      ret.Number,
		CompanyID:   ret.CompanyID,
		SupplierID:  ret.SupplierID,
		GrnID:       ret.GRNID,
		WarehouseID: ret.WarehouseID,
		ReturnDate:  pgtype.Date{Time: ret.ReturnDate, Valid: true},
		Status:      sqlc.GoodsReturnGrnStatus(ret.Status),
		Reason:      ret.Reason,
		Notes:       toText(ret.Notes),
		CreatedBy:   ret.CreatedBy,
	})
}

func (tx *txRepo) InsertGoodsReturnGRNLine(ctx context.Context, line GoodsReturnGRNLine) error {
	_, err := tx.queries.CreateGoodsReturnGRNLine(ctx, sqlc.CreateGoodsReturnGRNLineParams{
		GoodsReturnGrnID: line.GoodsReturnGRNID,
		GrnLineID:        line.GRNLineID,
		ProductID:        line.ProductID,
		QuantityReturned: numericOf(line.QuantityReturned),
		UnitCost:         numericOf(line.UnitCost),
		Notes:            toText(line.Notes),
		LineOrder:        int32(line.LineOrder),
	})
	return err
}

func (tx *txRepo) ConfirmGoodsReturnGRN(ctx context.Context, id int64, actorID int64) error {
	return tx.queries.ConfirmGoodsReturnGRN(ctx, sqlc.ConfirmGoodsReturnGRNParams{
		ID:          id,
		ConfirmedBy: pgtype.Int8{Int64: actorID, Valid: true},
	})
}

func (tx *txRepo) CancelGoodsReturnGRN(ctx context.Context, id int64, actorID int64) error {
	return tx.queries.CancelGoodsReturnGRN(ctx, sqlc.CancelGoodsReturnGRNParams{
		ID:       id,
		VoidedBy: pgtype.Int8{Int64: actorID, Valid: true},
	})
}

func (tx *txRepo) GenerateGoodsReturnGRNNumber(ctx context.Context) (string, error) {
	return tx.queries.GenerateGoodsReturnGRNNumber(ctx)
}

func (r *Repository) GetGoodsReturnGRN(ctx context.Context, id int64) (GoodsReturnGRN, []GoodsReturnGRNLine, error) {
	row, err := r.queries.GetGoodsReturnGRN(ctx, id)
	if err != nil {
		return GoodsReturnGRN{}, nil, ErrGoodsReturnNotFound
	}
	ret := GoodsReturnGRN{
		ID:          row.ID,
		Number:      row.Number,
		CompanyID:   row.CompanyID,
		SupplierID:  row.SupplierID,
		GRNID:       row.GrnID,
		WarehouseID: row.WarehouseID,
		Status:      GoodsReturnGRNStatus(row.Status),
		Reason:      row.Reason,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   safeTime(row.CreatedAt),
		UpdatedAt:   safeTime(row.UpdatedAt),
	}
	if row.ReturnDate.Valid {
		ret.ReturnDate = row.ReturnDate.Time
	}
	if row.Notes.Valid {
		v := row.Notes.String
		ret.Notes = &v
	}
	if row.ConfirmedBy.Valid {
		v := row.ConfirmedBy.Int64
		ret.ConfirmedBy = &v
	}
	if row.ConfirmedAt.Valid {
		v := row.ConfirmedAt.Time
		ret.ConfirmedAt = &v
	}
	if row.VoidedBy.Valid {
		v := row.VoidedBy.Int64
		ret.VoidedBy = &v
	}
	if row.VoidedAt.Valid {
		v := row.VoidedAt.Time
		ret.VoidedAt = &v
	}

	lineRows, err := r.queries.ListGoodsReturnGRNLines(ctx, id)
	if err != nil {
		return GoodsReturnGRN{}, nil, err
	}
	lines := make([]GoodsReturnGRNLine, len(lineRows))
	for i, l := range lineRows {
		lines[i] = GoodsReturnGRNLine{
			ID:               l.ID,
			GoodsReturnGRNID: l.GoodsReturnGrnID,
			GRNLineID:        l.GrnLineID,
			ProductID:        l.ProductID,
			QuantityReturned: numericToFloat(l.QuantityReturned),
			UnitCost:         numericToFloat(l.UnitCost),
			LineOrder:        int(l.LineOrder),
			CreatedAt:        safeTime(l.CreatedAt),
			UpdatedAt:        safeTime(l.UpdatedAt),
		}
		if l.Notes.Valid {
			v := l.Notes.String
			lines[i].Notes = &v
		}
	}
	return ret, lines, nil
}

func (r *Repository) ListGoodsReturnGRNs(ctx context.Context) ([]GoodsReturnGRN, error) {
	rows, err := r.queries.ListGoodsReturnGRNs(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]GoodsReturnGRN, len(rows))
	for i, row := range rows {
		items[i] = GoodsReturnGRN{
			ID:          row.ID,
			Number:      row.Number,
			CompanyID:   row.CompanyID,
			SupplierID:  row.SupplierID,
			GRNID:       row.GrnID,
			WarehouseID: row.WarehouseID,
			Status:      GoodsReturnGRNStatus(row.Status),
			Reason:      row.Reason,
			CreatedBy:   row.CreatedBy,
			CreatedAt:   safeTime(row.CreatedAt),
			UpdatedAt:   safeTime(row.UpdatedAt),
		}
		if row.ReturnDate.Valid {
			items[i].ReturnDate = row.ReturnDate.Time
		}
	}
	return items, nil
}

func toText(ptr *string) pgtype.Text {
	if ptr == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *ptr, Valid: true}
}

func numericToFloat(n pgtype.Numeric) float64 {
	f, _ := n.Float64Value()
	return f.Float64
}

func safeTime(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}
