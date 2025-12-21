package customers

import "time"

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
