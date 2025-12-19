package products

type ProductForm struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	CategoryID int64   `json:"category_id"`
	UnitID     int64   `json:"unit_id"`
	Price      float64 `json:"price"`
	Cost       float64 `json:"cost"`
	TaxID      int64   `json:"tax_id"`
	IsActive   bool    `json:"is_active"`
}
