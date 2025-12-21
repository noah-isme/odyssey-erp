package customers

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

type ListCustomersRequest struct {
	CompanyID int64   `json:"company_id" validate:"required,gt=0"`
	IsActive  *bool   `json:"is_active,omitempty"`
	Search    *string `json:"search,omitempty"`
	Limit     int     `json:"limit" validate:"gte=0,lte=1000"`
	Offset    int     `json:"offset" validate:"gte=0"`
}
