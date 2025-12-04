package delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound         = errors.New("record not found")
	ErrInvalidStatus    = errors.New("invalid status transition")
	ErrAlreadyExists    = errors.New("record already exists")
	ErrInsufficientData = errors.New("insufficient data")
)

// Repository provides PostgreSQL backed persistence for delivery operations.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// TxRepository exposes transactional operations.
type TxRepository interface {
	// Delivery Order operations
	CreateDeliveryOrder(ctx context.Context, do DeliveryOrder) (int64, error)
	InsertDeliveryOrderLine(ctx context.Context, line DeliveryOrderLine) (int64, error)
	UpdateDeliveryOrder(ctx context.Context, id int64, updates map[string]interface{}) error
	UpdateDeliveryOrderStatus(ctx context.Context, id int64, status DeliveryOrderStatus, updates map[string]interface{}) error
	DeleteDeliveryOrderLines(ctx context.Context, deliveryOrderID int64) error
	UpdateDeliveryOrderLineQuantity(ctx context.Context, lineID int64, quantityDelivered float64) error
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
// DELIVERY ORDER OPERATIONS
// ============================================================================

// GetDeliveryOrder retrieves a delivery order by ID with all details
func (r *Repository) GetDeliveryOrder(ctx context.Context, id int64) (*DeliveryOrder, error) {
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

	// Load lines
	lines, err := r.getDeliveryOrderLines(ctx, id)
	if err != nil {
		return nil, err
	}
	do.Lines = lines

	return &do, nil
}

// GetDeliveryOrderByDocNumber retrieves a delivery order by document number
func (r *Repository) GetDeliveryOrderByDocNumber(ctx context.Context, companyID int64, docNumber string) (*DeliveryOrder, error) {
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

	// Load lines
	lines, err := r.getDeliveryOrderLines(ctx, do.ID)
	if err != nil {
		return nil, err
	}
	do.Lines = lines

	return &do, nil
}

// getDeliveryOrderLines retrieves lines for a delivery order
func (r *Repository) getDeliveryOrderLines(ctx context.Context, deliveryOrderID int64) ([]DeliveryOrderLine, error) {
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

	var lines []DeliveryOrderLine
	for rows.Next() {
		var line DeliveryOrderLine
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

// GetDeliveryOrderWithDetails retrieves a delivery order with enriched details
func (r *Repository) GetDeliveryOrderWithDetails(ctx context.Context, id int64) (*DeliveryOrderWithDetails, error) {
	query := `
		SELECT do.id, do.doc_number, do.company_id, do.sales_order_id, do.warehouse_id,
		       do.customer_id, do.delivery_date, do.status, do.driver_name,
		       do.vehicle_number, do.tracking_number, do.notes, do.created_by,
		       do.confirmed_by, do.confirmed_at, do.delivered_at,
		       do.created_at, do.updated_at,
		       so.doc_number AS sales_order_number,
		       w.name AS warehouse_name,
		       c.name AS customer_name,
		       u_created.username AS created_by_name,
		       u_confirmed.username AS confirmed_by_name,
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
		         u_created.username, u_confirmed.username
	`
	var dowd DeliveryOrderWithDetails
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&dowd.ID, &dowd.DocNumber, &dowd.CompanyID, &dowd.SalesOrderID, &dowd.WarehouseID,
		&dowd.CustomerID, &dowd.DeliveryDate, &dowd.Status, &dowd.DriverName,
		&dowd.VehicleNumber, &dowd.TrackingNumber, &dowd.Notes, &dowd.CreatedBy,
		&dowd.ConfirmedBy, &dowd.ConfirmedAt, &dowd.DeliveredAt, &dowd.CreatedAt,
		&dowd.UpdatedAt, &dowd.SalesOrderNumber, &dowd.WarehouseName, &dowd.CustomerName,
		&dowd.CreatedByName, &dowd.ConfirmedByName, &dowd.LineCount, &dowd.TotalQuantity,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &dowd, nil
}

// GetDeliveryOrderLinesWithDetails retrieves lines with product details
func (r *Repository) GetDeliveryOrderLinesWithDetails(ctx context.Context, deliveryOrderID int64) ([]DeliveryOrderLineWithDetails, error) {
	query := `
		SELECT dol.id, dol.delivery_order_id, dol.sales_order_line_id, dol.product_id,
		       dol.quantity_to_deliver, dol.quantity_delivered, dol.uom, dol.unit_price,
		       dol.notes, dol.line_order, dol.created_at, dol.updated_at,
		       p.code AS product_code,
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

	var lines []DeliveryOrderLineWithDetails
	for rows.Next() {
		var line DeliveryOrderLineWithDetails
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

// ListDeliveryOrders retrieves delivery orders with filters
func (r *Repository) ListDeliveryOrders(ctx context.Context, req ListDeliveryOrdersRequest) ([]DeliveryOrderWithDetails, int, error) {
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

	// Count total matching records
	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT do.id)
		FROM delivery_orders do
		%s
	`, whereClause)

	var total int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Fetch paginated results
	query := fmt.Sprintf(`
		SELECT do.id, do.doc_number, do.company_id, do.sales_order_id, do.warehouse_id,
		       do.customer_id, do.delivery_date, do.status, do.driver_name,
		       do.vehicle_number, do.tracking_number, do.notes, do.created_by,
		       do.confirmed_by, do.confirmed_at, do.delivered_at,
		       do.created_at, do.updated_at,
		       so.doc_number AS sales_order_number,
		       w.name AS warehouse_name,
		       c.name AS customer_name,
		       u_created.username AS created_by_name,
		       u_confirmed.username AS confirmed_by_name,
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
		         u_created.username, u_confirmed.username
		ORDER BY do.delivery_date DESC, do.id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, req.Limit, req.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var deliveryOrders []DeliveryOrderWithDetails
	for rows.Next() {
		var dowd DeliveryOrderWithDetails
		err := rows.Scan(
			&dowd.ID, &dowd.DocNumber, &dowd.CompanyID, &dowd.SalesOrderID, &dowd.WarehouseID,
			&dowd.CustomerID, &dowd.DeliveryDate, &dowd.Status, &dowd.DriverName,
			&dowd.VehicleNumber, &dowd.TrackingNumber, &dowd.Notes, &dowd.CreatedBy,
			&dowd.ConfirmedBy, &dowd.ConfirmedAt, &dowd.DeliveredAt, &dowd.CreatedAt,
			&dowd.UpdatedAt, &dowd.SalesOrderNumber, &dowd.WarehouseName, &dowd.CustomerName,
			&dowd.CreatedByName, &dowd.ConfirmedByName, &dowd.LineCount, &dowd.TotalQuantity,
		)
		if err != nil {
			return nil, 0, err
		}
		deliveryOrders = append(deliveryOrders, dowd)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, err
	}

	return deliveryOrders, total, nil
}

// ============================================================================
// DELIVERABLE SALES ORDER LINES
// ============================================================================

// GetDeliverableSOLines retrieves sales order lines that can be delivered
func (r *Repository) GetDeliverableSOLines(ctx context.Context, salesOrderID int64) ([]DeliverableSOLine, error) {
	query := `
		SELECT sol.id AS sales_order_line_id,
		       sol.sales_order_id,
		       sol.product_id,
		       p.code AS product_code,
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

// ============================================================================
// TRANSACTIONAL OPERATIONS
// ============================================================================

func (t *txRepo) CreateDeliveryOrder(ctx context.Context, do DeliveryOrder) (int64, error) {
	query := `
		INSERT INTO delivery_orders (
			doc_number, company_id, sales_order_id, warehouse_id, customer_id,
			delivery_date, status, driver_name, vehicle_number, tracking_number,
			notes, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`
	var id int64
	err := t.tx.QueryRow(ctx, query,
		do.DocNumber, do.CompanyID, do.SalesOrderID, do.WarehouseID, do.CustomerID,
		do.DeliveryDate, do.Status, do.DriverName, do.VehicleNumber, do.TrackingNumber,
		do.Notes, do.CreatedBy,
	).Scan(&id)
	return id, err
}

func (t *txRepo) InsertDeliveryOrderLine(ctx context.Context, line DeliveryOrderLine) (int64, error) {
	query := `
		INSERT INTO delivery_order_lines (
			delivery_order_id, sales_order_line_id, product_id,
			quantity_to_deliver, quantity_delivered, uom, unit_price,
			notes, line_order
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`
	var id int64
	err := t.tx.QueryRow(ctx, query,
		line.DeliveryOrderID, line.SalesOrderLineID, line.ProductID,
		line.QuantityToDeliver, line.QuantityDelivered, line.UOM, line.UnitPrice,
		line.Notes, line.LineOrder,
	).Scan(&id)
	return id, err
}

func (t *txRepo) UpdateDeliveryOrder(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	var setClauses []string
	var args []interface{}
	argPos := 1

	for field, value := range updates {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", field, argPos))
		args = append(args, value)
		argPos++
	}

	// Always update updated_at
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argPos))
	args = append(args, time.Now())
	argPos++

	// Add ID for WHERE clause
	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE delivery_orders
		SET %s
		WHERE id = $%d
	`, strings.Join(setClauses, ", "), argPos)

	cmdTag, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *txRepo) UpdateDeliveryOrderStatus(ctx context.Context, id int64, status DeliveryOrderStatus, updates map[string]interface{}) error {
	if updates == nil {
		updates = make(map[string]interface{})
	}
	updates["status"] = status

	return t.UpdateDeliveryOrder(ctx, id, updates)
}

func (t *txRepo) DeleteDeliveryOrderLines(ctx context.Context, deliveryOrderID int64) error {
	query := `DELETE FROM delivery_order_lines WHERE delivery_order_id = $1`
	_, err := t.tx.Exec(ctx, query, deliveryOrderID)
	return err
}

func (t *txRepo) UpdateDeliveryOrderLineQuantity(ctx context.Context, lineID int64, quantityDelivered float64) error {
	query := `
		UPDATE delivery_order_lines
		SET quantity_delivered = $1, updated_at = $2
		WHERE id = $3
	`
	cmdTag, err := t.tx.Exec(ctx, query, quantityDelivered, time.Now(), lineID)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// GenerateDeliveryOrderNumber generates a unique DO number
func (r *Repository) GenerateDeliveryOrderNumber(ctx context.Context, companyID int64, date time.Time) (string, error) {
	query := `SELECT generate_delivery_order_number($1, $2)`
	var docNumber string
	err := r.pool.QueryRow(ctx, query, companyID, date).Scan(&docNumber)
	return docNumber, err
}

// GetSalesOrderDetails retrieves basic sales order info for validation
func (r *Repository) GetSalesOrderDetails(ctx context.Context, salesOrderID int64) (*struct {
	ID         int64
	DocNumber  string
	CompanyID  int64
	CustomerID int64
	Status     string
}, error) {
	query := `
		SELECT id, doc_number, company_id, customer_id, status
		FROM sales_orders
		WHERE id = $1
	`
	var so struct {
		ID         int64
		DocNumber  string
		CompanyID  int64
		CustomerID int64
		Status     string
	}
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

// CheckWarehouseExists validates warehouse existence
func (r *Repository) CheckWarehouseExists(ctx context.Context, warehouseID int64) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM warehouses WHERE id = $1)`
	var exists bool
	err := r.pool.QueryRow(ctx, query, warehouseID).Scan(&exists)
	return exists, err
}

// GetDeliveryOrderIDByDocNumber retrieves DO ID by doc number
func (r *Repository) GetDeliveryOrderIDByDocNumber(ctx context.Context, companyID int64, docNumber string) (int64, error) {
	query := `SELECT id FROM delivery_orders WHERE company_id = $1 AND doc_number = $2`
	var id int64
	err := r.pool.QueryRow(ctx, query, companyID, docNumber).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return id, nil
}
