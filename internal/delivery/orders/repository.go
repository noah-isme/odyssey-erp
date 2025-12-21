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

	// Write operations (transactional)
	WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error

	// Helpers
	GenerateDocNumber(ctx context.Context, companyID int64, date time.Time) (string, error)
	GetSalesOrderDetails(ctx context.Context, salesOrderID int64) (*SalesOrderInfo, error)
	CheckWarehouseExists(ctx context.Context, warehouseID int64) (bool, error)
}

// TxRepository exposes transactional write operations.
type TxRepository interface {
	CreateDeliveryOrder(ctx context.Context, do DeliveryOrder) (int64, error)
	InsertLine(ctx context.Context, line Line) (int64, error)
	UpdateDeliveryOrder(ctx context.Context, id int64, updates map[string]interface{}) error
	UpdateStatus(ctx context.Context, id int64, status Status, updates map[string]interface{}) error
	DeleteLines(ctx context.Context, deliveryOrderID int64) error
	UpdateLineQuantity(ctx context.Context, lineID int64, quantityDelivered float64) error
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
	defer tx.Rollback(ctx)

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
	orderBy := "dor.delivery_date DESC, dor.id DESC"
	if req.SortBy != "" {
		dir := "ASC"
		if strings.ToUpper(req.SortDir) == "DESC" {
			dir = "DESC"
		}
		orderBy = fmt.Sprintf("dor.%s %s", req.SortBy, dir)
	}

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
			confirmedBy pgtype.Int8
			confirmedAt pgtype.Timestamptz
			deliveredAt pgtype.Timestamptz
			driverName, vehicleNumber, trackingNumber, notes pgtype.Text
			confirmedByName pgtype.Text
			totalQty pgtype.Numeric
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
			SalesOrderLineID:  l.SalesOrderLineID,
			SalesOrderID:      l.SalesOrderID,
			ProductID:         l.ProductID,
			ProductCode:       l.ProductCode,
			ProductName:       l.ProductName,
			Quantity:          numericToFloat(l.Quantity),
			QuantityDelivered: numericToFloat(l.QuantityDelivered),
			RemainingQuantity: numericToFloat(l.RemainingQuantity),
			UOM:               l.Uom,
			UnitPrice:         numericToFloat(l.UnitPrice),
			LineOrder:         int(l.LineOrder),
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

func numericToFloat(n pgtype.Numeric) float64 {
	f, _ := n.Float64Value()
	return f.Float64
}

func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	n.Scan(fmt.Sprintf("%f", f))
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
