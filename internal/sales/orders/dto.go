package orders

import "time"

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

type ListSalesOrdersRequest struct {
	CompanyID  int64             `json:"company_id" validate:"required,gt=0"`
	CustomerID *int64            `json:"customer_id,omitempty"`
	Status     *SalesOrderStatus `json:"status,omitempty"`
	DateFrom   *time.Time        `json:"date_from,omitempty"`
	DateTo     *time.Time        `json:"date_to,omitempty"`
	Limit      int               `json:"limit" validate:"gte=0,lte=1000"`
	Offset     int               `json:"offset" validate:"gte=0"`
}
