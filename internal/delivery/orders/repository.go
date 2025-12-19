package orders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	pool *pgxpool.Pool
}

// NewRepository creates a new repository.
func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{pool: pool}
}

// txRepository implements TxRepository.
type txRepository struct {
	tx pgx.Tx
}

// WithTx wraps callback in repeatable-read transaction.
func (r *repository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	wrapper := &txRepository{tx: tx}
	if err := fn(ctx, wrapper); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// GetByID retrieves a delivery order by ID with lines.
func (r *repository) GetByID(ctx context.Context, id int64) (*DeliveryOrder, error) {
	query := `
		SELECT id, doc_number, company_id, sales_order_id, warehouse_id, customer_id,
		       delivery_date, status, driver_name, vehicle_number, tracking_number,
		       notes, created_by, confirmed_by, confirmed_at, delivered_at,
		       created_at, updated_at
		FROM delivery_orders
		WHERE id = $1
	`
	var do DeliveryOrder
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&do.ID, &do.DocNumber, &do.CompanyID, &do.SalesOrderID, &do.WarehouseID,
		&do.CustomerID, &do.DeliveryDate, &do.Status, &do.DriverName,
		&do.VehicleNumber, &do.TrackingNumber, &do.Notes, &do.CreatedBy,
		&do.ConfirmedBy, &do.ConfirmedAt, &do.DeliveredAt, &do.CreatedAt, &do.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	lines, err := r.getLines(ctx, id)
	if err != nil {
		return nil, err
	}
	do.Lines = lines

	return &do, nil
}

// GetByDocNumber retrieves a delivery order by document number.
func (r *repository) GetByDocNumber(ctx context.Context, companyID int64, docNumber string) (*DeliveryOrder, error) {
	query := `
		SELECT id, doc_number, company_id, sales_order_id, warehouse_id, customer_id,
		       delivery_date, status, driver_name, vehicle_number, tracking_number,
		       notes, created_by, confirmed_by, confirmed_at, delivered_at,
		       created_at, updated_at
		FROM delivery_orders
		WHERE company_id = $1 AND doc_number = $2
	`
	var do DeliveryOrder
	err := r.pool.QueryRow(ctx, query, companyID, docNumber).Scan(
		&do.ID, &do.DocNumber, &do.CompanyID, &do.SalesOrderID, &do.WarehouseID,
		&do.CustomerID, &do.DeliveryDate, &do.Status, &do.DriverName,
		&do.VehicleNumber, &do.TrackingNumber, &do.Notes, &do.CreatedBy,
		&do.ConfirmedBy, &do.ConfirmedAt, &do.DeliveredAt, &do.CreatedAt, &do.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	lines, err := r.getLines(ctx, do.ID)
	if err != nil {
		return nil, err
	}
	do.Lines = lines

	return &do, nil
}

func (r *repository) getLines(ctx context.Context, deliveryOrderID int64) ([]Line, error) {
	query := `
		SELECT id, delivery_order_id, sales_order_line_id, product_id,
		       quantity_to_deliver, quantity_delivered, uom, unit_price,
		       notes, line_order, created_at, updated_at
		FROM delivery_order_lines
		WHERE delivery_order_id = $1
		ORDER BY line_order, id
	`
	rows, err := r.pool.Query(ctx, query, deliveryOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []Line
	for rows.Next() {
		var line Line
		err := rows.Scan(
			&line.ID, &line.DeliveryOrderID, &line.SalesOrderLineID, &line.ProductID,
			&line.QuantityToDeliver, &line.QuantityDelivered, &line.UOM, &line.UnitPrice,
			&line.Notes, &line.LineOrder, &line.CreatedAt, &line.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}

	return lines, rows.Err()
}

// GetWithDetails retrieves a delivery order with enriched details.
func (r *repository) GetWithDetails(ctx context.Context, id int64) (*WithDetails, error) {
	query := `
		SELECT do.id, do.doc_number, do.company_id, do.sales_order_id, do.warehouse_id,
		       do.customer_id, do.delivery_date, do.status, do.driver_name,
		       do.vehicle_number, do.tracking_number, do.notes, do.created_by,
		       do.confirmed_by, do.confirmed_at, do.delivered_at,
		       do.created_at, do.updated_at,
		       so.doc_number AS sales_order_number,
		       w.name AS warehouse_name,
		       c.name AS customer_name,
		       u_created.email AS created_by_name,
		       u_confirmed.email AS confirmed_by_name,
		       COUNT(dol.id) AS line_count,
		       COALESCE(SUM(dol.quantity_to_deliver), 0) AS total_quantity
		FROM delivery_orders do
		INNER JOIN sales_orders so ON so.id = do.sales_order_id
		INNER JOIN warehouses w ON w.id = do.warehouse_id
		INNER JOIN customers c ON c.id = do.customer_id
		INNER JOIN users u_created ON u_created.id = do.created_by
		LEFT JOIN users u_confirmed ON u_confirmed.id = do.confirmed_by
		LEFT JOIN delivery_order_lines dol ON dol.delivery_order_id = do.id
		WHERE do.id = $1
		GROUP BY do.id, do.doc_number, do.company_id, do.sales_order_id, do.warehouse_id,
		         do.customer_id, do.delivery_date, do.status, do.driver_name,
		         do.vehicle_number, do.tracking_number, do.notes, do.created_by,
		         do.confirmed_by, do.confirmed_at, do.delivered_at,
		         do.created_at, do.updated_at, so.doc_number, w.name, c.name,
		         u_created.email, u_confirmed.email
	`
	var wd WithDetails
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&wd.ID, &wd.DocNumber, &wd.CompanyID, &wd.SalesOrderID, &wd.WarehouseID,
		&wd.CustomerID, &wd.DeliveryDate, &wd.Status, &wd.DriverName,
		&wd.VehicleNumber, &wd.TrackingNumber, &wd.Notes, &wd.CreatedBy,
		&wd.ConfirmedBy, &wd.ConfirmedAt, &wd.DeliveredAt, &wd.CreatedAt,
		&wd.UpdatedAt, &wd.SalesOrderNumber, &wd.WarehouseName, &wd.CustomerName,
		&wd.CreatedByName, &wd.ConfirmedByName, &wd.LineCount, &wd.TotalQuantity,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &wd, nil
}

// GetLinesWithDetails retrieves lines with product details.
func (r *repository) GetLinesWithDetails(ctx context.Context, deliveryOrderID int64) ([]LineWithDetails, error) {
	query := `
		SELECT dol.id, dol.delivery_order_id, dol.sales_order_line_id, dol.product_id,
		       dol.quantity_to_deliver, dol.quantity_delivered, dol.uom, dol.unit_price,
		       dol.notes, dol.line_order, dol.created_at, dol.updated_at,
		       p.sku AS product_code,
		       p.name AS product_name,
		       sol.quantity AS so_line_quantity,
		       sol.quantity_delivered AS so_line_delivered,
		       (sol.quantity - sol.quantity_delivered) AS remaining_to_deliver
		FROM delivery_order_lines dol
		INNER JOIN products p ON p.id = dol.product_id
		INNER JOIN sales_order_lines sol ON sol.id = dol.sales_order_line_id
		WHERE dol.delivery_order_id = $1
		ORDER BY dol.line_order, dol.id
	`
	rows, err := r.pool.Query(ctx, query, deliveryOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []LineWithDetails
	for rows.Next() {
		var line LineWithDetails
		err := rows.Scan(
			&line.ID, &line.DeliveryOrderID, &line.SalesOrderLineID, &line.ProductID,
			&line.QuantityToDeliver, &line.QuantityDelivered, &line.UOM, &line.UnitPrice,
			&line.Notes, &line.LineOrder, &line.CreatedAt, &line.UpdatedAt,
			&line.ProductCode, &line.ProductName, &line.SOLineQuantity,
			&line.SOLineDelivered, &line.RemainingToDeliver,
		)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}

	return lines, rows.Err()
}

// List retrieves delivery orders with filters.
func (r *repository) List(ctx context.Context, req ListRequest) ([]WithDetails, int, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	conditions = append(conditions, fmt.Sprintf("do.company_id = $%d", argPos))
	args = append(args, req.CompanyID)
	argPos++

	if req.SalesOrderID != nil {
		conditions = append(conditions, fmt.Sprintf("do.sales_order_id = $%d", argPos))
		args = append(args, *req.SalesOrderID)
		argPos++
	}

	if req.WarehouseID != nil {
		conditions = append(conditions, fmt.Sprintf("do.warehouse_id = $%d", argPos))
		args = append(args, *req.WarehouseID)
		argPos++
	}

	if req.CustomerID != nil {
		conditions = append(conditions, fmt.Sprintf("do.customer_id = $%d", argPos))
		args = append(args, *req.CustomerID)
		argPos++
	}

	if req.Status != nil {
		conditions = append(conditions, fmt.Sprintf("do.status = $%d", argPos))
		args = append(args, *req.Status)
		argPos++
	}

	if req.DateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("do.delivery_date >= $%d", argPos))
		args = append(args, *req.DateFrom)
		argPos++
	}

	if req.DateTo != nil {
		conditions = append(conditions, fmt.Sprintf("do.delivery_date <= $%d", argPos))
		args = append(args, *req.DateTo)
		argPos++
	}

	if req.Search != nil && *req.Search != "" {
		searchPattern := "%" + strings.ToLower(*req.Search) + "%"
		conditions = append(conditions, fmt.Sprintf(
			"(LOWER(do.doc_number) LIKE $%d OR LOWER(do.driver_name) LIKE $%d OR LOWER(do.tracking_number) LIKE $%d)",
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
	countQuery := fmt.Sprintf(`SELECT COUNT(DISTINCT do.id) FROM delivery_orders do %s`, whereClause)
	var total int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Sort
	orderBy := "do.delivery_date DESC, do.id DESC"
	if req.SortBy != "" {
		dir := "ASC"
		if strings.ToUpper(req.SortDir) == "DESC" {
			dir = "DESC"
		}
		orderBy = fmt.Sprintf("do.%s %s", req.SortBy, dir)
	}

	// Fetch
	query := fmt.Sprintf(`
		SELECT do.id, do.doc_number, do.company_id, do.sales_order_id, do.warehouse_id,
		       do.customer_id, do.delivery_date, do.status, do.driver_name,
		       do.vehicle_number, do.tracking_number, do.notes, do.created_by,
		       do.confirmed_by, do.confirmed_at, do.delivered_at,
		       do.created_at, do.updated_at,
		       so.doc_number AS sales_order_number,
		       w.name AS warehouse_name,
		       c.name AS customer_name,
		       u_created.email AS created_by_name,
		       u_confirmed.email AS confirmed_by_name,
		       COUNT(dol.id) AS line_count,
		       COALESCE(SUM(dol.quantity_to_deliver), 0) AS total_quantity
		FROM delivery_orders do
		INNER JOIN sales_orders so ON so.id = do.sales_order_id
		INNER JOIN warehouses w ON w.id = do.warehouse_id
		INNER JOIN customers c ON c.id = do.customer_id
		INNER JOIN users u_created ON u_created.id = do.created_by
		LEFT JOIN users u_confirmed ON u_confirmed.id = do.confirmed_by
		LEFT JOIN delivery_order_lines dol ON dol.delivery_order_id = do.id
		%s
		GROUP BY do.id, do.doc_number, do.company_id, do.sales_order_id, do.warehouse_id,
		         do.customer_id, do.delivery_date, do.status, do.driver_name,
		         do.vehicle_number, do.tracking_number, do.notes, do.created_by,
		         do.confirmed_by, do.confirmed_at, do.delivered_at,
		         do.created_at, do.updated_at, so.doc_number, w.name, c.name,
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
		err := rows.Scan(
			&wd.ID, &wd.DocNumber, &wd.CompanyID, &wd.SalesOrderID, &wd.WarehouseID,
			&wd.CustomerID, &wd.DeliveryDate, &wd.Status, &wd.DriverName,
			&wd.VehicleNumber, &wd.TrackingNumber, &wd.Notes, &wd.CreatedBy,
			&wd.ConfirmedBy, &wd.ConfirmedAt, &wd.DeliveredAt, &wd.CreatedAt,
			&wd.UpdatedAt, &wd.SalesOrderNumber, &wd.WarehouseName, &wd.CustomerName,
			&wd.CreatedByName, &wd.ConfirmedByName, &wd.LineCount, &wd.TotalQuantity,
		)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, wd)
	}

	return results, total, rows.Err()
}

// GetDeliverableSOLines retrieves SO lines that can still be delivered.
func (r *repository) GetDeliverableSOLines(ctx context.Context, salesOrderID int64) ([]DeliverableSOLine, error) {
	query := `
		SELECT sol.id AS sales_order_line_id,
		       sol.sales_order_id,
		       sol.product_id,
		       p.sku AS product_code,
		       p.name AS product_name,
		       sol.quantity,
		       sol.quantity_delivered,
		       (sol.quantity - sol.quantity_delivered) AS remaining_quantity,
		       sol.uom,
		       sol.unit_price,
		       sol.line_order
		FROM sales_order_lines sol
		INNER JOIN products p ON p.id = sol.product_id
		WHERE sol.sales_order_id = $1
		  AND sol.quantity > sol.quantity_delivered
		ORDER BY sol.line_order, sol.id
	`
	rows, err := r.pool.Query(ctx, query, salesOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []DeliverableSOLine
	for rows.Next() {
		var line DeliverableSOLine
		err := rows.Scan(
			&line.SalesOrderLineID, &line.SalesOrderID, &line.ProductID,
			&line.ProductCode, &line.ProductName, &line.Quantity,
			&line.QuantityDelivered, &line.RemainingQuantity,
			&line.UOM, &line.UnitPrice, &line.LineOrder,
		)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}

	return lines, rows.Err()
}

// GenerateDocNumber generates a unique DO number.
func (r *repository) GenerateDocNumber(ctx context.Context, companyID int64, date time.Time) (string, error) {
	query := `SELECT generate_delivery_order_number($1, $2)`
	var docNumber string
	err := r.pool.QueryRow(ctx, query, companyID, date).Scan(&docNumber)
	return docNumber, err
}

// GetSalesOrderDetails retrieves basic sales order info.
func (r *repository) GetSalesOrderDetails(ctx context.Context, salesOrderID int64) (*SalesOrderInfo, error) {
	query := `
		SELECT id, doc_number, company_id, customer_id, status
		FROM sales_orders
		WHERE id = $1
	`
	var so SalesOrderInfo
	err := r.pool.QueryRow(ctx, query, salesOrderID).Scan(
		&so.ID, &so.DocNumber, &so.CompanyID, &so.CustomerID, &so.Status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &so, nil
}

// CheckWarehouseExists validates warehouse existence.
func (r *repository) CheckWarehouseExists(ctx context.Context, warehouseID int64) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM warehouses WHERE id = $1)`
	var exists bool
	err := r.pool.QueryRow(ctx, query, warehouseID).Scan(&exists)
	return exists, err
}
