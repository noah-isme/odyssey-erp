package delivery

import (
	"time"
)

// ============================================================================
// DELIVERY ORDER STATUS
// ============================================================================

// DeliveryOrderStatus represents the lifecycle of a delivery order
type DeliveryOrderStatus string

const (
	DOStatusDraft     DeliveryOrderStatus = "DRAFT"      // Initial creation, can be edited
	DOStatusConfirmed DeliveryOrderStatus = "CONFIRMED"  // Confirmed, stock reduced
	DOStatusInTransit DeliveryOrderStatus = "IN_TRANSIT" // Out for delivery
	DOStatusDelivered DeliveryOrderStatus = "DELIVERED"  // Customer received goods
	DOStatusCancelled DeliveryOrderStatus = "CANCELLED"  // Cancelled delivery
)

// IsValid checks if the status is valid
func (s DeliveryOrderStatus) IsValid() bool {
	switch s {
	case DOStatusDraft, DOStatusConfirmed, DOStatusInTransit, DOStatusDelivered, DOStatusCancelled:
		return true
	default:
		return false
	}
}

// CanEdit checks if DO can be edited in this status
func (s DeliveryOrderStatus) CanEdit() bool {
	return s == DOStatusDraft
}

// CanConfirm checks if DO can be confirmed
func (s DeliveryOrderStatus) CanConfirm() bool {
	return s == DOStatusDraft
}

// CanCancel checks if DO can be cancelled
func (s DeliveryOrderStatus) CanCancel() bool {
	return s == DOStatusDraft || s == DOStatusConfirmed
}

// ============================================================================
// DELIVERY ORDER ENTITY
// ============================================================================

// DeliveryOrder represents a delivery from warehouse to customer
type DeliveryOrder struct {
	ID             int64               `json:"id" db:"id"`
	DocNumber      string              `json:"doc_number" db:"doc_number"`
	CompanyID      int64               `json:"company_id" db:"company_id"`
	SalesOrderID   int64               `json:"sales_order_id" db:"sales_order_id"`
	WarehouseID    int64               `json:"warehouse_id" db:"warehouse_id"`
	CustomerID     int64               `json:"customer_id" db:"customer_id"`
	DeliveryDate   time.Time           `json:"delivery_date" db:"delivery_date"`
	Status         DeliveryOrderStatus `json:"status" db:"status"`
	DriverName     *string             `json:"driver_name,omitempty" db:"driver_name"`
	VehicleNumber  *string             `json:"vehicle_number,omitempty" db:"vehicle_number"`
	TrackingNumber *string             `json:"tracking_number,omitempty" db:"tracking_number"`
	Notes          *string             `json:"notes,omitempty" db:"notes"`
	CreatedBy      int64               `json:"created_by" db:"created_by"`
	ConfirmedBy    *int64              `json:"confirmed_by,omitempty" db:"confirmed_by"`
	ConfirmedAt    *time.Time          `json:"confirmed_at,omitempty" db:"confirmed_at"`
	DeliveredAt    *time.Time          `json:"delivered_at,omitempty" db:"delivered_at"`
	CreatedAt      time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at" db:"updated_at"`
	Lines          []DeliveryOrderLine `json:"lines,omitempty" db:"-"`
}

// DeliveryOrderLine represents items in a delivery order
type DeliveryOrderLine struct {
	ID                int64     `json:"id" db:"id"`
	DeliveryOrderID   int64     `json:"delivery_order_id" db:"delivery_order_id"`
	SalesOrderLineID  int64     `json:"sales_order_line_id" db:"sales_order_line_id"`
	ProductID         int64     `json:"product_id" db:"product_id"`
	QuantityToDeliver float64   `json:"quantity_to_deliver" db:"quantity_to_deliver"`
	QuantityDelivered float64   `json:"quantity_delivered" db:"quantity_delivered"`
	UOM               string    `json:"uom" db:"uom"`
	UnitPrice         float64   `json:"unit_price" db:"unit_price"`
	Notes             *string   `json:"notes,omitempty" db:"notes"`
	LineOrder         int       `json:"line_order" db:"line_order"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

// ============================================================================
// DELIVERY ORDER WITH DETAILS
// ============================================================================

// DeliveryOrderWithDetails includes joined data for display
type DeliveryOrderWithDetails struct {
	DeliveryOrder
	SalesOrderNumber string  `json:"sales_order_number" db:"sales_order_number"`
	WarehouseName    string  `json:"warehouse_name" db:"warehouse_name"`
	CustomerName     string  `json:"customer_name" db:"customer_name"`
	CreatedByName    string  `json:"created_by_name" db:"created_by_name"`
	ConfirmedByName  *string `json:"confirmed_by_name,omitempty" db:"confirmed_by_name"`
	LineCount        int     `json:"line_count" db:"line_count"`
	TotalQuantity    float64 `json:"total_quantity" db:"total_quantity"`
}

// DeliveryOrderLineWithDetails includes product information
type DeliveryOrderLineWithDetails struct {
	DeliveryOrderLine
	ProductCode        string  `json:"product_code" db:"product_code"`
	ProductName        string  `json:"product_name" db:"product_name"`
	SOLineQuantity     float64 `json:"so_line_quantity" db:"so_line_quantity"`
	SOLineDelivered    float64 `json:"so_line_delivered" db:"so_line_delivered"`
	RemainingToDeliver float64 `json:"remaining_to_deliver" db:"remaining_to_deliver"`
}

// ============================================================================
// REQUEST DTOs
// ============================================================================

// CreateDeliveryOrderRequest represents request to create a delivery order
type CreateDeliveryOrderRequest struct {
	CompanyID      int64                        `json:"company_id" validate:"required,gt=0"`
	SalesOrderID   int64                        `json:"sales_order_id" validate:"required,gt=0"`
	WarehouseID    int64                        `json:"warehouse_id" validate:"required,gt=0"`
	DeliveryDate   time.Time                    `json:"delivery_date" validate:"required"`
	DriverName     *string                      `json:"driver_name,omitempty" validate:"omitempty,max=200"`
	VehicleNumber  *string                      `json:"vehicle_number,omitempty" validate:"omitempty,max=50"`
	TrackingNumber *string                      `json:"tracking_number,omitempty" validate:"omitempty,max=100"`
	Notes          *string                      `json:"notes,omitempty"`
	Lines          []CreateDeliveryOrderLineReq `json:"lines" validate:"required,min=1,dive"`
}

// CreateDeliveryOrderLineReq represents a line item in create request
type CreateDeliveryOrderLineReq struct {
	SalesOrderLineID  int64   `json:"sales_order_line_id" validate:"required,gt=0"`
	ProductID         int64   `json:"product_id" validate:"required,gt=0"`
	QuantityToDeliver float64 `json:"quantity_to_deliver" validate:"required,gt=0"`
	Notes             *string `json:"notes,omitempty"`
	LineOrder         int     `json:"line_order" validate:"gte=0"`
}

// UpdateDeliveryOrderRequest represents request to update delivery order (DRAFT only)
type UpdateDeliveryOrderRequest struct {
	DeliveryDate   *time.Time                    `json:"delivery_date,omitempty"`
	DriverName     *string                       `json:"driver_name,omitempty"`
	VehicleNumber  *string                       `json:"vehicle_number,omitempty"`
	TrackingNumber *string                       `json:"tracking_number,omitempty"`
	Notes          *string                       `json:"notes,omitempty"`
	Lines          *[]CreateDeliveryOrderLineReq `json:"lines,omitempty" validate:"omitempty,min=1,dive"`
}

// ConfirmDeliveryOrderRequest represents request to confirm delivery order
type ConfirmDeliveryOrderRequest struct {
	ConfirmedBy int64 `json:"confirmed_by" validate:"required,gt=0"`
}

// MarkInTransitRequest represents request to mark DO as in transit
type MarkInTransitRequest struct {
	TrackingNumber *string `json:"tracking_number,omitempty" validate:"omitempty,max=100"`
	UpdatedBy      int64   `json:"updated_by" validate:"required,gt=0"`
}

// MarkDeliveredRequest represents request to mark DO as delivered
type MarkDeliveredRequest struct {
	DeliveredAt time.Time `json:"delivered_at" validate:"required"`
	UpdatedBy   int64     `json:"updated_by" validate:"required,gt=0"`
}

// CancelDeliveryOrderRequest represents request to cancel delivery order
type CancelDeliveryOrderRequest struct {
	Reason      string `json:"reason" validate:"required,min=10,max=500"`
	CancelledBy int64  `json:"cancelled_by" validate:"required,gt=0"`
}

// ============================================================================
// LIST & FILTER REQUESTS
// ============================================================================

// ListDeliveryOrdersRequest represents filters for listing delivery orders
type ListDeliveryOrdersRequest struct {
	CompanyID    int64                `json:"company_id" validate:"required,gt=0"`
	SalesOrderID *int64               `json:"sales_order_id,omitempty"`
	WarehouseID  *int64               `json:"warehouse_id,omitempty"`
	CustomerID   *int64               `json:"customer_id,omitempty"`
	Status       *DeliveryOrderStatus `json:"status,omitempty"`
	DateFrom     *time.Time           `json:"date_from,omitempty"`
	DateTo       *time.Time           `json:"date_to,omitempty"`
	Search       *string              `json:"search,omitempty"` // Search in doc_number, driver_name
	Limit        int                  `json:"limit" validate:"gte=0,lte=1000"`
	Offset       int                  `json:"offset" validate:"gte=0"`
}

// ============================================================================
// DELIVERABLE SO LINES
// ============================================================================

// DeliverableSOLine represents a sales order line that can be delivered
type DeliverableSOLine struct {
	SalesOrderLineID  int64   `json:"sales_order_line_id" db:"sales_order_line_id"`
	SalesOrderID      int64   `json:"sales_order_id" db:"sales_order_id"`
	ProductID         int64   `json:"product_id" db:"product_id"`
	ProductCode       string  `json:"product_code" db:"product_code"`
	ProductName       string  `json:"product_name" db:"product_name"`
	Quantity          float64 `json:"quantity" db:"quantity"`
	QuantityDelivered float64 `json:"quantity_delivered" db:"quantity_delivered"`
	RemainingQuantity float64 `json:"remaining_quantity" db:"remaining_quantity"`
	UOM               string  `json:"uom" db:"uom"`
	UnitPrice         float64 `json:"unit_price" db:"unit_price"`
	LineOrder         int     `json:"line_order" db:"line_order"`
}

// ============================================================================
// INVENTORY INTEGRATION
// ============================================================================

// InventoryTransactionRequest represents request to reduce inventory
type InventoryTransactionRequest struct {
	TransactionType string    `json:"transaction_type"` // "SALES_OUT"
	CompanyID       int64     `json:"company_id"`
	WarehouseID     int64     `json:"warehouse_id"`
	ProductID       int64     `json:"product_id"`
	Quantity        float64   `json:"quantity"` // Negative for outbound
	UnitCost        float64   `json:"unit_cost"`
	ReferenceType   string    `json:"reference_type"` // "delivery_order"
	ReferenceID     int64     `json:"reference_id"`   // DO ID
	TransactionDate time.Time `json:"transaction_date"`
	Notes           string    `json:"notes"`
	PostedBy        int64     `json:"posted_by"`
}

// StockValidationRequest represents request to validate stock availability
type StockValidationRequest struct {
	WarehouseID int64   `json:"warehouse_id"`
	ProductID   int64   `json:"product_id"`
	Quantity    float64 `json:"quantity"`
}

// StockValidationResult represents result of stock validation
type StockValidationResult struct {
	Available    bool    `json:"available"`
	CurrentStock float64 `json:"current_stock"`
	RequestedQty float64 `json:"requested_qty"`
	ShortfallQty float64 `json:"shortfall_qty,omitempty"`
}

// ============================================================================
// RESPONSE DTOs
// ============================================================================

// DeliveryOrderResponse represents API response for delivery order
type DeliveryOrderResponse struct {
	DeliveryOrder DeliveryOrderWithDetails       `json:"delivery_order"`
	Lines         []DeliveryOrderLineWithDetails `json:"lines"`
}

// ListDeliveryOrdersResponse represents API response for list
type ListDeliveryOrdersResponse struct {
	DeliveryOrders []DeliveryOrderWithDetails `json:"delivery_orders"`
	Total          int                        `json:"total"`
	Limit          int                        `json:"limit"`
	Offset         int                        `json:"offset"`
}

// DeliverableSOLinesResponse represents available SO lines for delivery
type DeliverableSOLinesResponse struct {
	SalesOrderID     int64               `json:"sales_order_id"`
	SalesOrderNumber string              `json:"sales_order_number"`
	CustomerID       int64               `json:"customer_id"`
	CustomerName     string              `json:"customer_name"`
	Lines            []DeliverableSOLine `json:"lines"`
}
