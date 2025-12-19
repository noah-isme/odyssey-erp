// Package orders provides delivery order entity logic.
package orders

import (
	"time"
)

// Status represents the lifecycle of a delivery order.
type Status string

const (
	StatusDraft     Status = "DRAFT"      // Initial creation, can be edited
	StatusConfirmed Status = "CONFIRMED"  // Confirmed, stock reduced
	StatusInTransit Status = "IN_TRANSIT" // Out for delivery
	StatusDelivered Status = "DELIVERED"  // Customer received goods
	StatusCancelled Status = "CANCELLED"  // Cancelled delivery
)

// IsValid checks if the status is valid.
func (s Status) IsValid() bool {
	switch s {
	case StatusDraft, StatusConfirmed, StatusInTransit, StatusDelivered, StatusCancelled:
		return true
	default:
		return false
	}
}

// CanEdit checks if DO can be edited in this status.
func (s Status) CanEdit() bool {
	return s == StatusDraft
}

// CanConfirm checks if DO can be confirmed.
func (s Status) CanConfirm() bool {
	return s == StatusDraft
}

// CanCancel checks if DO can be cancelled.
func (s Status) CanCancel() bool {
	return s == StatusDraft || s == StatusConfirmed
}

// DeliveryOrder represents a delivery from warehouse to customer.
type DeliveryOrder struct {
	ID             int64      `json:"id" db:"id"`
	DocNumber      string     `json:"doc_number" db:"doc_number"`
	CompanyID      int64      `json:"company_id" db:"company_id"`
	SalesOrderID   int64      `json:"sales_order_id" db:"sales_order_id"`
	WarehouseID    int64      `json:"warehouse_id" db:"warehouse_id"`
	CustomerID     int64      `json:"customer_id" db:"customer_id"`
	DeliveryDate   time.Time  `json:"delivery_date" db:"delivery_date"`
	Status         Status     `json:"status" db:"status"`
	DriverName     *string    `json:"driver_name,omitempty" db:"driver_name"`
	VehicleNumber  *string    `json:"vehicle_number,omitempty" db:"vehicle_number"`
	TrackingNumber *string    `json:"tracking_number,omitempty" db:"tracking_number"`
	Notes          *string    `json:"notes,omitempty" db:"notes"`
	CreatedBy      int64      `json:"created_by" db:"created_by"`
	ConfirmedBy    *int64     `json:"confirmed_by,omitempty" db:"confirmed_by"`
	ConfirmedAt    *time.Time `json:"confirmed_at,omitempty" db:"confirmed_at"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty" db:"delivered_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
	Lines          []Line     `json:"lines,omitempty" db:"-"`
}

// Line represents items in a delivery order.
type Line struct {
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

// WithDetails includes joined data for display.
type WithDetails struct {
	DeliveryOrder
	SalesOrderNumber string  `json:"sales_order_number" db:"sales_order_number"`
	WarehouseName    string  `json:"warehouse_name" db:"warehouse_name"`
	CustomerName     string  `json:"customer_name" db:"customer_name"`
	CreatedByName    string  `json:"created_by_name" db:"created_by_name"`
	ConfirmedByName  *string `json:"confirmed_by_name,omitempty" db:"confirmed_by_name"`
	LineCount        int     `json:"line_count" db:"line_count"`
	TotalQuantity    float64 `json:"total_quantity" db:"total_quantity"`
}

// LineWithDetails includes product information.
type LineWithDetails struct {
	Line
	ProductCode        string  `json:"product_code" db:"product_code"`
	ProductName        string  `json:"product_name" db:"product_name"`
	SOLineQuantity     float64 `json:"so_line_quantity" db:"so_line_quantity"`
	SOLineDelivered    float64 `json:"so_line_delivered" db:"so_line_delivered"`
	RemainingToDeliver float64 `json:"remaining_to_deliver" db:"remaining_to_deliver"`
}

// DeliverableSOLine represents a sales order line that can be delivered.
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
