package sales

import (
	"time"
)

// ============================================================================
// CUSTOMER
// ============================================================================

type Customer struct {
	ID               int64      `json:"id" db:"id"`
	Code             string     `json:"code" db:"code"`
	Name             string     `json:"name" db:"name"`
	CompanyID        int64      `json:"company_id" db:"company_id"`
	Email            *string    `json:"email,omitempty" db:"email"`
	Phone            *string    `json:"phone,omitempty" db:"phone"`
	TaxID            *string    `json:"tax_id,omitempty" db:"tax_id"`
	CreditLimit      float64    `json:"credit_limit" db:"credit_limit"`
	PaymentTermsDays int        `json:"payment_terms_days" db:"payment_terms_days"`
	AddressLine1     *string    `json:"address_line1,omitempty" db:"address_line1"`
	AddressLine2     *string    `json:"address_line2,omitempty" db:"address_line2"`
	City             *string    `json:"city,omitempty" db:"city"`
	State            *string    `json:"state,omitempty" db:"state"`
	PostalCode       *string    `json:"postal_code,omitempty" db:"postal_code"`
	Country          string     `json:"country" db:"country"`
	IsActive         bool       `json:"is_active" db:"is_active"`
	Notes            *string    `json:"notes,omitempty" db:"notes"`
	CreatedBy        int64      `json:"created_by" db:"created_by"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

type CreateCustomerRequest struct {
	Code             string  `json:"code" validate:"required,max=50"`
	Name             string  `json:"name" validate:"required,max=200"`
	CompanyID        int64   `json:"company_id" validate:"required,gt=0"`
	Email            *string `json:"email,omitempty" validate:"omitempty,email"`
	Phone            *string `json:"phone,omitempty" validate:"omitempty,max=50"`
	TaxID            *string `json:"tax_id,omitempty" validate:"omitempty,max=50"`
	CreditLimit      float64 `json:"credit_limit" validate:"gte=0"`
	PaymentTermsDays int     `json:"payment_terms_days" validate:"gte=0,lte=365"`
	AddressLine1     *string `json:"address_line1,omitempty" validate:"omitempty,max=200"`
	AddressLine2     *string `json:"address_line2,omitempty" validate:"omitempty,max=200"`
	City             *string `json:"city,omitempty" validate:"omitempty,max=100"`
	State            *string `json:"state,omitempty" validate:"omitempty,max=100"`
	PostalCode       *string `json:"postal_code,omitempty" validate:"omitempty,max=20"`
	Country          string  `json:"country" validate:"required,len=2"`
	Notes            *string `json:"notes,omitempty"`
}

type UpdateCustomerRequest struct {
	Name             *string  `json:"name,omitempty" validate:"omitempty,max=200"`
	Email            *string  `json:"email,omitempty" validate:"omitempty,email"`
	Phone            *string  `json:"phone,omitempty" validate:"omitempty,max=50"`
	TaxID            *string  `json:"tax_id,omitempty"`
	CreditLimit      *float64 `json:"credit_limit,omitempty" validate:"omitempty,gte=0"`
	PaymentTermsDays *int     `json:"payment_terms_days,omitempty" validate:"omitempty,gte=0,lte=365"`
	AddressLine1     *string  `json:"address_line1,omitempty"`
	AddressLine2     *string  `json:"address_line2,omitempty"`
	City             *string  `json:"city,omitempty"`
	State            *string  `json:"state,omitempty"`
	PostalCode       *string  `json:"postal_code,omitempty"`
	Country          *string  `json:"country,omitempty" validate:"omitempty,len=2"`
	IsActive         *bool    `json:"is_active,omitempty"`
	Notes            *string  `json:"notes,omitempty"`
}

// ============================================================================
// QUOTATION
// ============================================================================

type QuotationStatus string

const (
	QuotationStatusDraft     QuotationStatus = "DRAFT"
	QuotationStatusSubmitted QuotationStatus = "SUBMITTED"
	QuotationStatusApproved  QuotationStatus = "APPROVED"
	QuotationStatusRejected  QuotationStatus = "REJECTED"
	QuotationStatusConverted QuotationStatus = "CONVERTED"
)

type Quotation struct {
	ID              int64            `json:"id" db:"id"`
	DocNumber       string           `json:"doc_number" db:"doc_number"`
	CompanyID       int64            `json:"company_id" db:"company_id"`
	CustomerID      int64            `json:"customer_id" db:"customer_id"`
	QuoteDate       time.Time        `json:"quote_date" db:"quote_date"`
	ValidUntil      time.Time        `json:"valid_until" db:"valid_until"`
	Status          QuotationStatus  `json:"status" db:"status"`
	Currency        string           `json:"currency" db:"currency"`
	Subtotal        float64          `json:"subtotal" db:"subtotal"`
	TaxAmount       float64          `json:"tax_amount" db:"tax_amount"`
	TotalAmount     float64          `json:"total_amount" db:"total_amount"`
	Notes           *string          `json:"notes,omitempty" db:"notes"`
	CreatedBy       int64            `json:"created_by" db:"created_by"`
	ApprovedBy      *int64           `json:"approved_by,omitempty" db:"approved_by"`
	ApprovedAt      *time.Time       `json:"approved_at,omitempty" db:"approved_at"`
	RejectedBy      *int64           `json:"rejected_by,omitempty" db:"rejected_by"`
	RejectedAt      *time.Time       `json:"rejected_at,omitempty" db:"rejected_at"`
	RejectionReason *string          `json:"rejection_reason,omitempty" db:"rejection_reason"`
	CreatedAt       time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at" db:"updated_at"`
	Lines           []QuotationLine  `json:"lines,omitempty" db:"-"`
}

type QuotationLine struct {
	ID              int64     `json:"id" db:"id"`
	QuotationID     int64     `json:"quotation_id" db:"quotation_id"`
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

type CreateQuotationRequest struct {
	CompanyID  int64                    `json:"company_id" validate:"required,gt=0"`
	CustomerID int64                    `json:"customer_id" validate:"required,gt=0"`
	QuoteDate  time.Time                `json:"quote_date" validate:"required"`
	ValidUntil time.Time                `json:"valid_until" validate:"required"`
	Currency   string                   `json:"currency" validate:"required,len=3"`
	Notes      *string                  `json:"notes,omitempty"`
	Lines      []CreateQuotationLineReq `json:"lines" validate:"required,min=1,dive"`
}

type CreateQuotationLineReq struct {
	ProductID       int64   `json:"product_id" validate:"required,gt=0"`
	Description     *string `json:"description,omitempty"`
	Quantity        float64 `json:"quantity" validate:"required,gt=0"`
	UOM             string  `json:"uom" validate:"required,max=20"`
	UnitPrice       float64 `json:"unit_price" validate:"required,gte=0"`
	DiscountPercent float64 `json:"discount_percent" validate:"gte=0,lte=100"`
	TaxPercent      float64 `json:"tax_percent" validate:"gte=0,lte=100"`
	Notes           *string `json:"notes,omitempty"`
	LineOrder       int     `json:"line_order" validate:"gte=0"`
}

type UpdateQuotationRequest struct {
	QuoteDate  *time.Time                `json:"quote_date,omitempty"`
	ValidUntil *time.Time                `json:"valid_until,omitempty"`
	Notes      *string                   `json:"notes,omitempty"`
	Lines      *[]CreateQuotationLineReq `json:"lines,omitempty" validate:"omitempty,min=1,dive"`
}

type QuotationWithDetails struct {
	Quotation
	CustomerName  string  `json:"customer_name" db:"customer_name"`
	CreatedByName string  `json:"created_by_name" db:"created_by_name"`
	ApprovedByName *string `json:"approved_by_name,omitempty" db:"approved_by_name"`
	RejectedByName *string `json:"rejected_by_name,omitempty" db:"rejected_by_name"`
}

// ============================================================================
// SALES ORDER
// ============================================================================

type SalesOrderStatus string

const (
	SalesOrderStatusDraft      SalesOrderStatus = "DRAFT"
	SalesOrderStatusConfirmed  SalesOrderStatus = "CONFIRMED"
	SalesOrderStatusProcessing SalesOrderStatus = "PROCESSING"
	SalesOrderStatusCompleted  SalesOrderStatus = "COMPLETED"
	SalesOrderStatusCancelled  SalesOrderStatus = "CANCELLED"
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
	ID                int64     `json:"id" db:"id"`
	SalesOrderID      int64     `json:"sales_order_id" db:"sales_order_id"`
	ProductID         int64     `json:"product_id" db:"product_id"`
	Description       *string   `json:"description,omitempty" db:"description"`
	Quantity          float64   `json:"quantity" db:"quantity"`
	QuantityDelivered float64   `json:"quantity_delivered" db:"quantity_delivered"`
	QuantityInvoiced  float64   `json:"quantity_invoiced" db:"quantity_invoiced"`
	UOM               string    `json:"uom" db:"uom"`
	UnitPrice         float64   `json:"unit_price" db:"unit_price"`
	DiscountPercent   float64   `json:"discount_percent" db:"discount_percent"`
	DiscountAmount    float64   `json:"discount_amount" db:"discount_amount"`
	TaxPercent        float64   `json:"tax_percent" db:"tax_percent"`
	TaxAmount         float64   `json:"tax_amount" db:"tax_amount"`
	LineTotal         float64   `json:"line_total" db:"line_total"`
	Notes             *string   `json:"notes,omitempty" db:"notes"`
	LineOrder         int       `json:"line_order" db:"line_order"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

type CreateSalesOrderRequest struct {
	CompanyID            int64                     `json:"company_id" validate:"required,gt=0"`
	CustomerID           int64                     `json:"customer_id" validate:"required,gt=0"`
	QuotationID          *int64                    `json:"quotation_id,omitempty"`
	OrderDate            time.Time                 `json:"order_date" validate:"required"`
	ExpectedDeliveryDate *time.Time                `json:"expected_delivery_date,omitempty"`
	Currency             string                    `json:"currency" validate:"required,len=3"`
	Notes                *string                   `json:"notes,omitempty"`
	Lines                []CreateSalesOrderLineReq `json:"lines" validate:"required,min=1,dive"`
}

type CreateSalesOrderLineReq struct {
	ProductID       int64   `json:"product_id" validate:"required,gt=0"`
	Description     *string `json:"description,omitempty"`
	Quantity        float64 `json:"quantity" validate:"required,gt=0"`
	UOM             string  `json:"uom" validate:"required,max=20"`
	UnitPrice       float64 `json:"unit_price" validate:"required,gte=0"`
	DiscountPercent float64 `json:"discount_percent" validate:"gte=0,lte=100"`
	TaxPercent      float64 `json:"tax_percent" validate:"gte=0,lte=100"`
	Notes           *string `json:"notes,omitempty"`
	LineOrder       int     `json:"line_order" validate:"gte=0"`
}

type UpdateSalesOrderRequest struct {
	OrderDate            *time.Time                 `json:"order_date,omitempty"`
	ExpectedDeliveryDate *time.Time                 `json:"expected_delivery_date,omitempty"`
	Notes                *string                    `json:"notes,omitempty"`
	Lines                *[]CreateSalesOrderLineReq `json:"lines,omitempty" validate:"omitempty,min=1,dive"`
}

type SalesOrderWithDetails struct {
	SalesOrder
	CustomerName   string  `json:"customer_name" db:"customer_name"`
	CreatedByName  string  `json:"created_by_name" db:"created_by_name"`
	ConfirmedByName *string `json:"confirmed_by_name,omitempty" db:"confirmed_by_name"`
	CancelledByName *string `json:"cancelled_by_name,omitempty" db:"cancelled_by_name"`
	QuotationNumber *string `json:"quotation_number,omitempty" db:"quotation_number"`
}

// ============================================================================
// LIST & FILTER REQUESTS
// ============================================================================

type ListCustomersRequest struct {
	CompanyID int64   `json:"company_id" validate:"required,gt=0"`
	IsActive  *bool   `json:"is_active,omitempty"`
	Search    *string `json:"search,omitempty"`
	Limit     int     `json:"limit" validate:"gte=0,lte=1000"`
	Offset    int     `json:"offset" validate:"gte=0"`
}

type ListQuotationsRequest struct {
	CompanyID  int64            `json:"company_id" validate:"required,gt=0"`
	CustomerID *int64           `json:"customer_id,omitempty"`
	Status     *QuotationStatus `json:"status,omitempty"`
	DateFrom   *time.Time       `json:"date_from,omitempty"`
	DateTo     *time.Time       `json:"date_to,omitempty"`
	Limit      int              `json:"limit" validate:"gte=0,lte=1000"`
	Offset     int              `json:"offset" validate:"gte=0"`
}

type ListSalesOrdersRequest struct {
	CompanyID    int64             `json:"company_id" validate:"required,gt=0"`
	CustomerID   *int64            `json:"customer_id,omitempty"`
	Status       *SalesOrderStatus `json:"status,omitempty"`
	DateFrom     *time.Time        `json:"date_from,omitempty"`
	DateTo       *time.Time        `json:"date_to,omitempty"`
	Limit        int               `json:"limit" validate:"gte=0,lte=1000"`
	Offset       int               `json:"offset" validate:"gte=0"`
}
