package orders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository defines the interface for delivery order persistence.
type Repository interface {
	// Read operations
	GetByID(ctx context.Context, id int64) (*DeliveryOrder, error)
	GetByDocNumber(ctx context.Context, companyID int64, docNumber string) (*DeliveryOrder, error)
	GetWithDetails(ctx context.Context, id int64) (*WithDetails, error)
	GetLinesWithDetails(ctx context.Context, deliveryOrderID int64) ([]LineWithDetails, error)
	List(ctx context.Context, req ListRequest) ([]WithDetails, int, error)
	GetDeliverableSOLines(ctx context.Context, salesOrderID int64) ([]DeliverableSOLine, error)

	GetReturnByID(ctx context.Context, id int64) (*ReturnDeliveryOrder, error)
	ListReturns(ctx context.Context, req ListReturnRequest) ([]ReturnDeliveryOrderWithDetails, int, error)
	GetReturnedQuantity(ctx context.Context, deliveryOrderLineID int64) (float64, error)
	HasCreditNoteForReturn(ctx context.Context, returnDeliveryOrderID int64) (bool, error)

	// Write operations (transactional)
	WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error

	// Helpers
	GenerateDocNumber(ctx context.Context, companyID int64, date time.Time) (string, error)
	GetSalesOrderDetails(ctx context.Context, salesOrderID int64) (*SalesOrderInfo, error)
	CheckWarehouseExists(ctx context.Context, warehouseID int64) (bool, error)
	GenerateReturnDocNumber(ctx context.Context, companyID int64, date time.Time) (string, error)
}

// TxRepository exposes transactional write operations.
type TxRepository interface {
	CreateDeliveryOrder(ctx context.Context, do DeliveryOrder) (int64, error)
	InsertLine(ctx context.Context, line Line) (int64, error)
	UpdateDeliveryOrder(ctx context.Context, id int64, updates map[string]interface{}) error
	UpdateStatus(ctx context.Context, id int64, status Status, updates map[string]interface{}) error
	DeleteLines(ctx context.Context, deliveryOrderID int64) error
	UpdateLineQuantity(ctx context.Context, lineID int64, quantityDelivered float64) error

	CreateReturnDeliveryOrder(ctx context.Context, rdo ReturnDeliveryOrder) (int64, error)
	InsertReturnLine(ctx context.Context, line ReturnLine) (int64, error)
	UpdateReturnStatus(ctx context.Context, id int64, status ReturnStatus, updates map[string]interface{}) error
	DeleteReturnLines(ctx context.Context, returnDeliveryOrderID int64) error
}

// SalesOrderInfo holds basic sales order data for validation.
type SalesOrderInfo struct {
	ID         int64
	DocNumber  string
	CompanyID  int64
	CustomerID int64
	Status     string
}

// repository implements Repository using pgxpool.
type repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewRepository creates a new repository.
func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// txRepository implements TxRepository.
type txRepository struct {
	tx      pgx.Tx
	queries *sqlc.Queries
}

// WithTx wraps callback in repeatable-read transaction.
func (r *repository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.queries.WithTx(tx)
	wrapper := &txRepository{tx: tx, queries: q}

	if err := fn(ctx, wrapper); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetByID retrieves a delivery order by ID with lines.
func (r *repository) GetByID(ctx context.Context, id int64) (*DeliveryOrder, error) {
	row, err := r.queries.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	do := &DeliveryOrder{
		ID:             row.ID,
		DocNumber:      row.DocNumber,
		CompanyID:      row.CompanyID,
		SalesOrderID:   row.SalesOrderID,
		WarehouseID:    row.WarehouseID,
		CustomerID:     row.CustomerID,
		DeliveryDate:   row.DeliveryDate.Time,
		Status:         Status(row.Status),
		DriverName:     textToPointer(row.DriverName),
		VehicleNumber:  textToPointer(row.VehicleNumber),
		TrackingNumber: textToPointer(row.TrackingNumber),
		Notes:          textToPointer(row.Notes),
		CreatedBy:      row.CreatedBy,
		ConfirmedBy:    int8ToPointer(row.ConfirmedBy),
		ConfirmedAt:    timeToPointer(row.ConfirmedAt),
		DeliveredAt:    timeToPointer(row.DeliveredAt),
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}

	linesRows, err := r.queries.GetLines(ctx, id)
	if err != nil {
		return nil, err
	}

	var lines []Line
	for _, l := range linesRows {
		lines = append(lines, Line{
			ID:                l.ID,
			DeliveryOrderID:   l.DeliveryOrderID,
			SalesOrderLineID:  l.SalesOrderLineID,
			ProductID:         l.ProductID,
			QuantityToDeliver: numericToFloat(l.QuantityToDeliver),
			QuantityDelivered: numericToFloat(l.QuantityDelivered),
			UOM:               l.Uom,
			UnitPrice:         numericToFloat(l.UnitPrice),
			Notes:             textToPointer(l.Notes),
			LineOrder:         int(l.LineOrder),
			CreatedAt:         l.CreatedAt.Time,
			UpdatedAt:         l.UpdatedAt.Time,
		})
	}
	do.Lines = lines

	return do, nil
}

// GetByDocNumber retrieves a delivery order by document number.
func (r *repository) GetByDocNumber(ctx context.Context, companyID int64, docNumber string) (*DeliveryOrder, error) {
	row, err := r.queries.GetByDocNumber(ctx, sqlc.GetByDocNumberParams{
		CompanyID: companyID,
		DocNumber: docNumber,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	do := &DeliveryOrder{
		ID:             row.ID,
		DocNumber:      row.DocNumber,
		CompanyID:      row.CompanyID,
		SalesOrderID:   row.SalesOrderID,
		WarehouseID:    row.WarehouseID,
		CustomerID:     row.CustomerID,
		DeliveryDate:   row.DeliveryDate.Time,
		Status:         Status(row.Status),
		DriverName:     textToPointer(row.DriverName),
		VehicleNumber:  textToPointer(row.VehicleNumber),
		TrackingNumber: textToPointer(row.TrackingNumber),
		Notes:          textToPointer(row.Notes),
		CreatedBy:      row.CreatedBy,
		ConfirmedBy:    int8ToPointer(row.ConfirmedBy),
		ConfirmedAt:    timeToPointer(row.ConfirmedAt),
		DeliveredAt:    timeToPointer(row.DeliveredAt),
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}

	linesRows, err := r.queries.GetLines(ctx, do.ID)
	if err != nil {
		return nil, err
	}

	var lines []Line
	for _, l := range linesRows {
		lines = append(lines, Line{
			ID:                l.ID,
			DeliveryOrderID:   l.DeliveryOrderID,
			SalesOrderLineID:  l.SalesOrderLineID,
			ProductID:         l.ProductID,
			QuantityToDeliver: numericToFloat(l.QuantityToDeliver),
			QuantityDelivered: numericToFloat(l.QuantityDelivered),
			UOM:               l.Uom,
			UnitPrice:         numericToFloat(l.UnitPrice),
			Notes:             textToPointer(l.Notes),
			LineOrder:         int(l.LineOrder),
			CreatedAt:         l.CreatedAt.Time,
			UpdatedAt:         l.UpdatedAt.Time,
		})
	}
	do.Lines = lines

	return do, nil
}

// GetWithDetails retrieves a delivery order with enriched details.
func (r *repository) GetWithDetails(ctx context.Context, id int64) (*WithDetails, error) {
	row, err := r.queries.GetWithDetails(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &WithDetails{
		DeliveryOrder: DeliveryOrder{
			ID:             row.ID,
			DocNumber:      row.DocNumber,
			CompanyID:      row.CompanyID,
			SalesOrderID:   row.SalesOrderID,
			WarehouseID:    row.WarehouseID,
			CustomerID:     row.CustomerID,
			DeliveryDate:   row.DeliveryDate.Time,
			Status:         Status(row.Status),
			DriverName:     textToPointer(row.DriverName),
			VehicleNumber:  textToPointer(row.VehicleNumber),
			TrackingNumber: textToPointer(row.TrackingNumber),
			Notes:          textToPointer(row.Notes),
			CreatedBy:      row.CreatedBy,
			ConfirmedBy:    int8ToPointer(row.ConfirmedBy),
			ConfirmedAt:    timeToPointer(row.ConfirmedAt),
			DeliveredAt:    timeToPointer(row.DeliveredAt),
			CreatedAt:      row.CreatedAt.Time,
			UpdatedAt:      row.UpdatedAt.Time,
		},
		SalesOrderNumber: row.SalesOrderNumber,
		WarehouseName:    row.WarehouseName,
		CustomerName:     row.CustomerName,
		CreatedByName:    row.CreatedByName,
		ConfirmedByName:  textToPointer(row.ConfirmedByName),
		LineCount:        int(row.LineCount),
		TotalQuantity:    numericToFloat(row.TotalQuantity),
	}, nil
}

// GetLinesWithDetails retrieves lines with product details.
func (r *repository) GetLinesWithDetails(ctx context.Context, deliveryOrderID int64) ([]LineWithDetails, error) {
	rows, err := r.queries.GetLinesWithDetails(ctx, deliveryOrderID)
	if err != nil {
		return nil, err
	}

	var lines []LineWithDetails
	for _, l := range rows {
		lines = append(lines, LineWithDetails{
			Line: Line{
				ID:                l.ID,
				DeliveryOrderID:   l.DeliveryOrderID,
				SalesOrderLineID:  l.SalesOrderLineID,
				ProductID:         l.ProductID,
				QuantityToDeliver: numericToFloat(l.QuantityToDeliver),
				QuantityDelivered: numericToFloat(l.QuantityDelivered),
				UOM:               l.Uom,
				UnitPrice:         numericToFloat(l.UnitPrice),
				Notes:             textToPointer(l.Notes),
				LineOrder:         int(l.LineOrder),
				CreatedAt:         l.CreatedAt.Time,
				UpdatedAt:         l.UpdatedAt.Time,
			},
			ProductCode:        l.ProductCode,
			ProductName:        l.ProductName,
			SOLineQuantity:     numericToFloat(l.SoLineQuantity),
			SOLineDelivered:    numericToFloat(l.SoLineDelivered),
			RemainingToDeliver: numericToFloat(l.RemainingToDeliver),
		})
	}
	return lines, nil
}

// List retrieves delivery orders with filters.
// NOTE: Kept as raw SQL due to complex dynamic filtering not easily handled by SQLC.
func (r *repository) List(ctx context.Context, req ListRequest) ([]WithDetails, int, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	conditions = append(conditions, fmt.Sprintf("dor.company_id = $%d", argPos))
	args = append(args, req.CompanyID)
	argPos++

	if req.SalesOrderID != nil {
		conditions = append(conditions, fmt.Sprintf("dor.sales_order_id = $%d", argPos))
		args = append(args, *req.SalesOrderID)
		argPos++
	}

	if req.WarehouseID != nil {
		conditions = append(conditions, fmt.Sprintf("dor.warehouse_id = $%d", argPos))
		args = append(args, *req.WarehouseID)
		argPos++
	}

	if req.CustomerID != nil {
		conditions = append(conditions, fmt.Sprintf("dor.customer_id = $%d", argPos))
		args = append(args, *req.CustomerID)
		argPos++
	}

	if req.Status != nil {
		conditions = append(conditions, fmt.Sprintf("dor.status = $%d", argPos))
		args = append(args, *req.Status)
		argPos++
	}

	if req.DateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("dor.delivery_date >= $%d", argPos))
		args = append(args, *req.DateFrom)
		argPos++
	}

	if req.DateTo != nil {
		conditions = append(conditions, fmt.Sprintf("dor.delivery_date <= $%d", argPos))
		args = append(args, *req.DateTo)
		argPos++
	}

	if req.Search != nil && *req.Search != "" {
		searchPattern := "%" + strings.ToLower(*req.Search) + "%"
		conditions = append(conditions, fmt.Sprintf(
			"(LOWER(dor.doc_number) LIKE $%d OR LOWER(dor.driver_name) LIKE $%d OR LOWER(dor.tracking_number) LIKE $%d)",
			argPos, argPos, argPos,
		))
		args = append(args, searchPattern)
		argPos++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count
	countQuery := fmt.Sprintf(`SELECT COUNT(DISTINCT dor.id) FROM delivery_orders dor %s`, whereClause)
	var total int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Sort
	orderBy := sortOrderDeliveryOrders(req.SortBy, req.SortDir)

	// Fetch
	query := fmt.Sprintf(`
		SELECT dor.id, dor.doc_number, dor.company_id, dor.sales_order_id, dor.warehouse_id,
		       dor.customer_id, dor.delivery_date, dor.status, dor.driver_name,
		       dor.vehicle_number, dor.tracking_number, dor.notes, dor.created_by,
		       dor.confirmed_by, dor.confirmed_at, dor.delivered_at,
		       dor.created_at, dor.updated_at,
		       so.doc_number AS sales_order_number,
		       w.name AS warehouse_name,
		       c.name AS customer_name,
		       u_created.email AS created_by_name,
		       u_confirmed.email AS confirmed_by_name,
		       COUNT(dol.id) AS line_count,
		       COALESCE(SUM(dol.quantity_to_deliver), 0) AS total_quantity
		FROM delivery_orders dor
		INNER JOIN sales_orders so ON so.id = dor.sales_order_id
		INNER JOIN warehouses w ON w.id = dor.warehouse_id
		INNER JOIN customers c ON c.id = dor.customer_id
		INNER JOIN users u_created ON u_created.id = dor.created_by
		LEFT JOIN users u_confirmed ON u_confirmed.id = dor.confirmed_by
		LEFT JOIN delivery_order_lines dol ON dol.delivery_order_id = dor.id
		%s
		GROUP BY dor.id, dor.doc_number, dor.company_id, dor.sales_order_id, dor.warehouse_id,
		         dor.customer_id, dor.delivery_date, dor.status, dor.driver_name,
		         dor.vehicle_number, dor.tracking_number, dor.notes, dor.created_by,
		         dor.confirmed_by, dor.confirmed_at, dor.delivered_at,
		         dor.created_at, dor.updated_at, so.doc_number, w.name, c.name,
		         u_created.email, u_confirmed.email
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, whereClause, orderBy, argPos, argPos+1)

	args = append(args, req.Limit, req.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []WithDetails
	for rows.Next() {
		var wd WithDetails
		var (
			confirmedBy                                      pgtype.Int8
			confirmedAt                                      pgtype.Timestamptz
			deliveredAt                                      pgtype.Timestamptz
			driverName, vehicleNumber, trackingNumber, notes pgtype.Text
			confirmedByName                                  pgtype.Text
			totalQty                                         pgtype.Numeric
		)

		err := rows.Scan(
			&wd.ID, &wd.DocNumber, &wd.CompanyID, &wd.SalesOrderID, &wd.WarehouseID,
			&wd.CustomerID, &wd.DeliveryDate, &wd.Status, &driverName,
			&vehicleNumber, &trackingNumber, &notes, &wd.CreatedBy,
			&confirmedBy, &confirmedAt, &deliveredAt, &wd.CreatedAt,
			&wd.UpdatedAt, &wd.SalesOrderNumber, &wd.WarehouseName, &wd.CustomerName,
			&wd.CreatedByName, &confirmedByName, &wd.LineCount, &totalQty,
		)
		if err != nil {
			return nil, 0, err
		}

		wd.DriverName = textToPointer(driverName)
		wd.VehicleNumber = textToPointer(vehicleNumber)
		wd.TrackingNumber = textToPointer(trackingNumber)
		wd.Notes = textToPointer(notes)
		wd.ConfirmedBy = int8ToPointer(confirmedBy)
		wd.ConfirmedAt = timeToPointer(confirmedAt)
		wd.DeliveredAt = timeToPointer(deliveredAt)
		wd.ConfirmedByName = textToPointer(confirmedByName)
		wd.TotalQuantity = numericToFloat(totalQty)

		results = append(results, wd)
	}

	return results, total, rows.Err()
}

// GetDeliverableSOLines retrieves SO lines that can still be delivered.
func (r *repository) GetDeliverableSOLines(ctx context.Context, salesOrderID int64) ([]DeliverableSOLine, error) {
	rows, err := r.queries.GetDeliverableSOLines(ctx, salesOrderID)
	if err != nil {
		return nil, err
	}

	var lines []DeliverableSOLine
	for _, l := range rows {
		lines = append(lines, DeliverableSOLine{
			SalesOrderLineID:       l.SalesOrderLineID,
			SalesOrderID:           l.SalesOrderID,
			ProductID:              l.ProductID,
			ProductCode:            l.ProductCode,
			ProductName:            l.ProductName,
			Quantity:               numericToFloat(l.Quantity),
			QuantityDelivered:      numericToFloat(l.QuantityDelivered),
			RemainingQuantity:      numericToFloat(l.RemainingQuantity),
			UOM:                    l.Uom,
			UnitPrice:              numericToFloat(l.UnitPrice),
			LineOrder:              int(l.LineOrder),
			FulfillmentWarehouseID: int8ToPointer(l.FulfillmentWarehouseID),
		})
	}
	return lines, nil
}

// GenerateDocNumber generates a unique DO number.
func (r *repository) GenerateDocNumber(ctx context.Context, companyID int64, date time.Time) (string, error) {
	return r.queries.GenerateDocNumber(ctx, sqlc.GenerateDocNumberParams{
		PCompanyID: companyID,
		PDate:      pgtype.Date{Time: date, Valid: true},
	})
}

// GetSalesOrderDetails retrieves basic sales order info.
func (r *repository) GetSalesOrderDetails(ctx context.Context, salesOrderID int64) (*SalesOrderInfo, error) {
	row, err := r.queries.GetSalesOrderDetails(ctx, salesOrderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &SalesOrderInfo{
		ID:         row.ID,
		DocNumber:  row.DocNumber,
		CompanyID:  row.CompanyID,
		CustomerID: row.CustomerID,
		Status:     string(row.Status),
	}, nil
}

// CheckWarehouseExists validates warehouse existence.
func (r *repository) CheckWarehouseExists(ctx context.Context, warehouseID int64) (bool, error) {
	return r.queries.CheckWarehouseExists(ctx, warehouseID)
}

func sortOrderDeliveryOrders(sortBy, sortDir string) string {
	dir := "ASC"
	if strings.ToUpper(sortDir) == "DESC" {
		dir = "DESC"
	}

	switch sortBy {
	case "doc_number":
		return "dor.doc_number " + dir
	case "delivery_date":
		return "dor.delivery_date " + dir
	case "customer_name":
		return "c.name " + dir
	case "warehouse_name":
		return "w.name " + dir
	case "status":
		return "dor.status " + dir
	case "driver_name":
		return "dor.driver_name " + dir
	default:
		return "dor.delivery_date DESC, dor.id DESC" // Default fallback
	}
}

func numericToFloat(n pgtype.Numeric) float64 {
	f, _ := n.Float64Value()
	return f.Float64
}

func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%f", f))
	return n
}

func textToPointer(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func int8ToPointer(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	v := i.Int64
	return &v
}

func timeToPointer(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func pointerToText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func (r *repository) GenerateReturnDocNumber(ctx context.Context, companyID int64, date time.Time) (string, error) {
	var number string
	err := r.pool.QueryRow(ctx, "SELECT generate_return_delivery_order_number($1, $2)", companyID, date).Scan(&number)
	return number, err
}

func (r *repository) GetReturnedQuantity(ctx context.Context, deliveryOrderLineID int64) (float64, error) {
	var quantity pgtype.Numeric
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(l.quantity_returned), 0)
		FROM return_delivery_order_lines l
		JOIN return_delivery_orders r ON r.id = l.return_delivery_order_id
		WHERE l.delivery_order_line_id = $1 AND r.status <> 'CANCELLED'`, deliveryOrderLineID).Scan(&quantity)
	return numericToFloat(quantity), err
}

func (r *repository) HasCreditNoteForReturn(ctx context.Context, returnDeliveryOrderID int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ar_credit_notes WHERE return_delivery_order_id = $1 AND status <> 'VOID')`, returnDeliveryOrderID).Scan(&exists)
	return exists, err
}

func (r *repository) GetReturnByID(ctx context.Context, id int64) (*ReturnDeliveryOrder, error) {
	query := `
		SELECT r.id, r.number, r.company_id, r.customer_id, r.original_delivery_order_id, r.warehouse_id,
		       r.return_date, r.status, r.reason, r.notes, r.created_by, r.confirmed_by, r.confirmed_at,
		       r.voided_by, r.voided_at, r.created_at, r.updated_at,
		       c.name AS customer_name, dor.doc_number AS original_delivery_order_doc,
		       w.name AS warehouse_name, u_created.email AS created_by_name,
		       u_confirmed.email AS confirmed_by_name
		FROM return_delivery_orders r
		INNER JOIN customers c ON c.id = r.customer_id
		INNER JOIN delivery_orders dor ON dor.id = r.original_delivery_order_id
		INNER JOIN warehouses w ON w.id = r.warehouse_id
		INNER JOIN users u_created ON u_created.id = r.created_by
		LEFT JOIN users u_confirmed ON u_confirmed.id = r.confirmed_by
		WHERE r.id = $1`

	row := r.pool.QueryRow(ctx, query, id)

	var rdo ReturnDeliveryOrder
	var reason, notes, confirmedByName pgtype.Text
	var confirmedBy, voidedBy pgtype.Int8
	var confirmedAt, voidedAt pgtype.Timestamptz

	err := row.Scan(
		&rdo.ID, &rdo.Number, &rdo.CompanyID, &rdo.CustomerID, &rdo.OriginalDeliveryOrderID, &rdo.WarehouseID,
		&rdo.ReturnDate, &rdo.Status, &reason, &notes, &rdo.CreatedBy, &confirmedBy, &confirmedAt,
		&voidedBy, &voidedAt, &rdo.CreatedAt, &rdo.UpdatedAt,
		&rdo.CustomerName, &rdo.OriginalDeliveryOrderDoc, &rdo.WarehouseName, &rdo.CreatedByName, &confirmedByName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	rdo.Reason = reason.String
	rdo.Notes = textToPointer(notes)
	rdo.ConfirmedBy = int8ToPointer(confirmedBy)
	rdo.ConfirmedAt = timeToPointer(confirmedAt)
	rdo.VoidedBy = int8ToPointer(voidedBy)
	rdo.VoidedAt = timeToPointer(voidedAt)
	rdo.ConfirmedByName = textToPointer(confirmedByName)

	lines, err := r.getReturnLines(ctx, id)
	if err != nil {
		return nil, err
	}
	rdo.Lines = lines

	return &rdo, nil
}

func (r *repository) getReturnLines(ctx context.Context, returnDeliveryOrderID int64) ([]ReturnLine, error) {
	query := `
		SELECT l.id, l.return_delivery_order_id, l.delivery_order_line_id, l.product_id, l.quantity_returned,
		       l.unit_price, l.restock_warehouse_id, l.lot_number, l.serial_numbers, l.notes, l.line_order,
		       l.created_at, l.updated_at, p.code AS product_code, p.name AS product_name,
		       dol.quantity_delivered AS original_quantity_delivered
		FROM return_delivery_order_lines l
		INNER JOIN products p ON p.id = l.product_id
		INNER JOIN delivery_order_lines dol ON dol.id = l.delivery_order_line_id
		WHERE l.return_delivery_order_id = $1
		ORDER BY l.line_order, l.id`

	rows, err := r.pool.Query(ctx, query, returnDeliveryOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []ReturnLine
	for rows.Next() {
		var l ReturnLine
		var notes, lotNumber pgtype.Text
		var restockWH pgtype.Int8
		var originalQty pgtype.Numeric

		err := rows.Scan(
			&l.ID, &l.ReturnDeliveryOrderID, &l.DeliveryOrderLineID, &l.ProductID, &l.QuantityReturned,
			&l.UnitPrice, &restockWH, &lotNumber, &l.SerialNumbers, &notes, &l.LineOrder,
			&l.CreatedAt, &l.UpdatedAt, &l.ProductCode, &l.ProductName, &originalQty,
		)
		if err != nil {
			return nil, err
		}

		if restockWH.Valid {
			l.RestockWarehouseID = &restockWH.Int64
		}
		l.LotNumber = lotNumber.String
		l.Notes = textToPointer(notes)
		l.OriginalQuantityDelivered = numericToFloat(originalQty)

		lines = append(lines, l)
	}

	return lines, rows.Err()
}

func (r *repository) ListReturns(ctx context.Context, req ListReturnRequest) ([]ReturnDeliveryOrderWithDetails, int, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	conditions = append(conditions, fmt.Sprintf("r.company_id = $%d", argPos))
	args = append(args, req.CompanyID)
	argPos++

	if req.CustomerID != nil {
		conditions = append(conditions, fmt.Sprintf("r.customer_id = $%d", argPos))
		args = append(args, *req.CustomerID)
		argPos++
	}

	if req.Status != nil {
		conditions = append(conditions, fmt.Sprintf("r.status = $%d", argPos))
		args = append(args, string(*req.Status))
		argPos++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(DISTINCT r.id) FROM return_delivery_orders r %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT r.id, r.number, r.company_id, r.customer_id, r.original_delivery_order_id, r.warehouse_id,
		       r.return_date, r.status, r.reason, r.notes, r.created_by, r.confirmed_by, r.confirmed_at,
		       r.voided_by, r.voided_at, r.created_at, r.updated_at,
		       c.name AS customer_name, dor.doc_number AS original_delivery_order_doc,
		       w.name AS warehouse_name, u_created.email AS created_by_name,
		       u_confirmed.email AS confirmed_by_name,
		       COUNT(l.id) AS line_count,
		       COALESCE(SUM(l.quantity_returned), 0) AS total_quantity
		FROM return_delivery_orders r
		INNER JOIN customers c ON c.id = r.customer_id
		INNER JOIN delivery_orders dor ON dor.id = r.original_delivery_order_id
		INNER JOIN warehouses w ON w.id = r.warehouse_id
		INNER JOIN users u_created ON u_created.id = r.created_by
		LEFT JOIN users u_confirmed ON u_confirmed.id = r.confirmed_by
		LEFT JOIN return_delivery_order_lines l ON l.return_delivery_order_id = r.id
		%s
		GROUP BY r.id, r.number, r.company_id, r.customer_id, r.original_delivery_order_id, r.warehouse_id,
		         r.return_date, r.status, r.reason, r.notes, r.created_by, r.confirmed_by, r.confirmed_at,
		         r.voided_by, r.voided_at, r.created_at, r.updated_at,
		         c.name, dor.doc_number, w.name, u_created.email, u_confirmed.email
		ORDER BY r.return_date DESC, r.id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, req.Limit, req.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []ReturnDeliveryOrderWithDetails
	for rows.Next() {
		var wd ReturnDeliveryOrderWithDetails
		var reason, notes, confirmedByName pgtype.Text
		var confirmedBy, voidedBy pgtype.Int8
		var confirmedAt, voidedAt pgtype.Timestamptz
		var totalQty pgtype.Numeric

		err := rows.Scan(
			&wd.ID, &wd.Number, &wd.CompanyID, &wd.CustomerID, &wd.OriginalDeliveryOrderID, &wd.WarehouseID,
			&wd.ReturnDate, &wd.Status, &reason, &notes, &wd.CreatedBy, &confirmedBy, &confirmedAt,
			&voidedBy, &voidedAt, &wd.CreatedAt, &wd.UpdatedAt,
			&wd.CustomerName, &wd.OriginalDeliveryOrderDoc, &wd.WarehouseName, &wd.CreatedByName, &confirmedByName,
			&wd.LineCount, &totalQty,
		)
		if err != nil {
			return nil, 0, err
		}

		wd.Reason = reason.String
		wd.Notes = textToPointer(notes)
		wd.ConfirmedBy = int8ToPointer(confirmedBy)
		wd.ConfirmedAt = timeToPointer(confirmedAt)
		wd.VoidedBy = int8ToPointer(voidedBy)
		wd.VoidedAt = timeToPointer(voidedAt)
		wd.ConfirmedByName = textToPointer(confirmedByName)
		wd.TotalQuantity = numericToFloat(totalQty)

		results = append(results, wd)
	}

	return results, total, rows.Err()
}

func (t *txRepository) CreateReturnDeliveryOrder(ctx context.Context, rdo ReturnDeliveryOrder) (int64, error) {
	query := `
		INSERT INTO return_delivery_orders (
			number, company_id, customer_id, original_delivery_order_id, warehouse_id,
			return_date, status, reason, notes, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	var id int64
	err := t.tx.QueryRow(ctx, query,
		rdo.Number, rdo.CompanyID, rdo.CustomerID, rdo.OriginalDeliveryOrderID, rdo.WarehouseID,
		rdo.ReturnDate, string(rdo.Status), rdo.Reason, pointerToText(rdo.Notes), rdo.CreatedBy,
	).Scan(&id, &rdo.CreatedAt, &rdo.UpdatedAt)
	return id, err
}

func (t *txRepository) InsertReturnLine(ctx context.Context, line ReturnLine) (int64, error) {
	query := `
		INSERT INTO return_delivery_order_lines (
			return_delivery_order_id, delivery_order_line_id, product_id, quantity_returned, unit_price,
			restock_warehouse_id, lot_number, serial_numbers, notes, line_order, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9, $10, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	var id int64
	var restockWH *int64
	if line.RestockWarehouseID != nil && *line.RestockWarehouseID > 0 {
		restockWH = line.RestockWarehouseID
	}
	err := t.tx.QueryRow(ctx, query,
		line.ReturnDeliveryOrderID, line.DeliveryOrderLineID, line.ProductID, line.QuantityReturned, line.UnitPrice,
		restockWH, line.LotNumber, line.SerialNumbers, pointerToText(line.Notes), line.LineOrder,
	).Scan(&id, &line.CreatedAt, &line.UpdatedAt)
	return id, err
}

func (t *txRepository) UpdateReturnStatus(ctx context.Context, id int64, status ReturnStatus, updates map[string]interface{}) error {
	setClauses := []string{"status = $2", "updated_at = NOW()"}
	args := []interface{}{id, string(status)}
	argPos := 3

	for key, value := range updates {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, argPos))
		args = append(args, value)
		argPos++
	}

	query := fmt.Sprintf("UPDATE return_delivery_orders SET %s WHERE id = $1", strings.Join(setClauses, ", "))
	cmd, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *txRepository) DeleteReturnLines(ctx context.Context, returnDeliveryOrderID int64) error {
	_, err := t.tx.Exec(ctx, "DELETE FROM return_delivery_order_lines WHERE return_delivery_order_id = $1", returnDeliveryOrderID)
	return err
}
