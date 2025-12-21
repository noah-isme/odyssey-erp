package quotations

import "time"

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

type ListQuotationsRequest struct {
	CompanyID  int64            `json:"company_id" validate:"required,gt=0"`
	CustomerID *int64           `json:"customer_id,omitempty"`
	Status     *QuotationStatus `json:"status,omitempty"`
	DateFrom   *time.Time       `json:"date_from,omitempty"`
	DateTo     *time.Time       `json:"date_to,omitempty"`
	Limit      int              `json:"limit" validate:"gte=0,lte=1000"`
	Offset     int              `json:"offset" validate:"gte=0"`
}
