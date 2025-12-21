package orders

import "time"

type SalesOrderStatus string

const (
	SalesOrderStatusDraft     SalesOrderStatus = "DRAFT"
	SalesOrderStatusConfirmed SalesOrderStatus = "CONFIRMED"
	SalesOrderStatusCancelled SalesOrderStatus = "CANCELLED"
	SalesOrderStatusCompleted SalesOrderStatus = "COMPLETED"
)

type SalesOrder struct {
	ID                   int64            `json:"id" db:"id"`
	DocNumber            string           `json:"doc_number" db:"doc_number"`
	CompanyID            int64            `json:"company_id" db:"company_id"`
	CustomerID           int64            `json:"customer_id" db:"customer_id"`
	QuotationID          *int64           `json:"quotation_id,omitempty" db:"quotation_id"`
	OrderDate            time.Time        `json:"order_date" db:"order_date"`
	ExpectedDeliveryDate *time.Time       `json:"expected_delivery_date,omitempty" db:"expected_delivery_date"`
	Status               SalesOrderStatus `json:"status" db:"status"`
	Currency             string           `json:"currency" db:"currency"`
	Subtotal             float64          `json:"subtotal" db:"subtotal"`
	TaxAmount            float64          `json:"tax_amount" db:"tax_amount"`
	TotalAmount          float64          `json:"total_amount" db:"total_amount"`
	Notes                *string          `json:"notes,omitempty" db:"notes"`
	CreatedBy            int64            `json:"created_by" db:"created_by"`
	ConfirmedBy          *int64           `json:"confirmed_by,omitempty" db:"confirmed_by"`
	ConfirmedAt          *time.Time       `json:"confirmed_at,omitempty" db:"confirmed_at"`
	CancelledBy          *int64           `json:"cancelled_by,omitempty" db:"cancelled_by"`
	CancelledAt          *time.Time       `json:"cancelled_at,omitempty" db:"cancelled_at"`
	CancellationReason   *string          `json:"cancellation_reason,omitempty" db:"cancellation_reason"`
	CreatedAt            time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at" db:"updated_at"`
	Lines                []SalesOrderLine `json:"lines,omitempty" db:"-"`
}

type SalesOrderLine struct {
	ID              int64     `json:"id" db:"id"`
	SalesOrderID    int64     `json:"sales_order_id" db:"sales_order_id"`
	ProductID       int64     `json:"product_id" db:"product_id"`
	Description     *string   `json:"description,omitempty" db:"description"`
	Quantity        float64   `json:"quantity" db:"quantity"`
	UOM             string    `json:"uom" db:"uom"`
	UnitPrice       float64   `json:"unit_price" db:"unit_price"`
	DiscountPercent float64   `json:"discount_percent" db:"discount_percent"`
	DiscountAmount  float64   `json:"discount_amount" db:"discount_amount"`
	TaxPercent      float64   `json:"tax_percent" db:"tax_percent"`
	TaxAmount       float64   `json:"tax_amount" db:"tax_amount"`
	LineTotal       float64   `json:"line_total" db:"line_total"`
	Notes           *string   `json:"notes,omitempty" db:"notes"`
	LineOrder       int       `json:"line_order" db:"line_order"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

type SalesOrderWithDetails struct {
	SalesOrder
	CustomerName    string  `json:"customer_name" db:"customer_name"`
	CreatedByName   string  `json:"created_by_name" db:"created_by_name"`
	ConfirmedByName *string `json:"confirmed_by_name,omitempty" db:"confirmed_by_name"`
	CancelledByName *string `json:"cancelled_by_name,omitempty" db:"cancelled_by_name"`
}
