package orders

import "time"

// CreateRequest represents request to create a delivery order.
type CreateRequest struct {
	CompanyID      int64            `json:"company_id" validate:"required,gt=0"`
	SalesOrderID   int64            `json:"sales_order_id" validate:"required,gt=0"`
	WarehouseID    int64            `json:"warehouse_id" validate:"required,gt=0"`
	DeliveryDate   time.Time        `json:"delivery_date" validate:"required"`
	DriverName     *string          `json:"driver_name,omitempty" validate:"omitempty,max=200"`
	VehicleNumber  *string          `json:"vehicle_number,omitempty" validate:"omitempty,max=50"`
	TrackingNumber *string          `json:"tracking_number,omitempty" validate:"omitempty,max=100"`
	Notes          *string          `json:"notes,omitempty"`
	Lines          []CreateLineReq  `json:"lines" validate:"required,min=1,dive"`
}

// CreateLineReq represents a line item in create request.
type CreateLineReq struct {
	SalesOrderLineID  int64   `json:"sales_order_line_id" validate:"required,gt=0"`
	ProductID         int64   `json:"product_id" validate:"required,gt=0"`
	QuantityToDeliver float64 `json:"quantity_to_deliver" validate:"required,gt=0"`
	Notes             *string `json:"notes,omitempty"`
	LineOrder         int     `json:"line_order" validate:"gte=0"`
}

// UpdateRequest represents request to update delivery order (DRAFT only).
type UpdateRequest struct {
	DeliveryDate   *time.Time       `json:"delivery_date,omitempty"`
	DriverName     *string          `json:"driver_name,omitempty"`
	VehicleNumber  *string          `json:"vehicle_number,omitempty"`
	TrackingNumber *string          `json:"tracking_number,omitempty"`
	Notes          *string          `json:"notes,omitempty"`
	Lines          *[]CreateLineReq `json:"lines,omitempty" validate:"omitempty,min=1,dive"`
}

// ConfirmRequest represents request to confirm delivery order.
type ConfirmRequest struct {
	ConfirmedBy int64 `json:"confirmed_by" validate:"required,gt=0"`
}

// MarkInTransitRequest represents request to mark DO as in transit.
type MarkInTransitRequest struct {
	TrackingNumber *string `json:"tracking_number,omitempty" validate:"omitempty,max=100"`
	UpdatedBy      int64   `json:"updated_by" validate:"required,gt=0"`
}

// MarkDeliveredRequest represents request to mark DO as delivered.
type MarkDeliveredRequest struct {
	DeliveredAt time.Time `json:"delivered_at" validate:"required"`
	UpdatedBy   int64     `json:"updated_by" validate:"required,gt=0"`
}

// CancelRequest represents request to cancel delivery order.
type CancelRequest struct {
	Reason      string `json:"reason" validate:"required,min=10,max=500"`
	CancelledBy int64  `json:"cancelled_by" validate:"required,gt=0"`
}

// ListRequest represents filters for listing delivery orders.
type ListRequest struct {
	CompanyID    int64   `json:"company_id" validate:"required,gt=0"`
	SalesOrderID *int64  `json:"sales_order_id,omitempty"`
	WarehouseID  *int64  `json:"warehouse_id,omitempty"`
	CustomerID   *int64  `json:"customer_id,omitempty"`
	Status       *Status `json:"status,omitempty"`
	DateFrom     *time.Time `json:"date_from,omitempty"`
	DateTo       *time.Time `json:"date_to,omitempty"`
	Search       *string `json:"search,omitempty"`
	SortBy       string  `json:"sort_by,omitempty"`
	SortDir      string  `json:"sort_dir,omitempty"`
	Limit        int     `json:"limit" validate:"gte=0,lte=1000"`
	Offset       int     `json:"offset" validate:"gte=0"`
}

// ListResponse represents API response for list.
type ListResponse struct {
	DeliveryOrders []WithDetails `json:"delivery_orders"`
	Total          int           `json:"total"`
	Limit          int           `json:"limit"`
	Offset         int           `json:"offset"`
}

// DetailResponse represents API response for delivery order.
type DetailResponse struct {
	DeliveryOrder WithDetails       `json:"delivery_order"`
	Lines         []LineWithDetails `json:"lines"`
}

// DeliverableSOLinesResponse represents available SO lines for delivery.
type DeliverableSOLinesResponse struct {
	SalesOrderID     int64               `json:"sales_order_id"`
	SalesOrderNumber string              `json:"sales_order_number"`
	CustomerID       int64               `json:"customer_id"`
	CustomerName     string              `json:"customer_name"`
	Lines            []DeliverableSOLine `json:"lines"`
}
