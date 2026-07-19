package products

type ProductForm struct {
	Code                string  `json:"code"`
	Name                string  `json:"name"`
	CategoryID          int64   `json:"category_id"`
	UnitID              int64   `json:"unit_id"`
	Price               float64 `json:"price"`
	Cost                float64 `json:"cost"`
	CostMethod          string  `json:"cost_method"`
	MinStock            float64 `json:"min_stock"`
	ReorderTarget       float64 `json:"reorder_target"`
	PreferredSupplierID int64   `json:"preferred_supplier_id"`
	TrackBatch          bool    `json:"track_batch"`
	TrackSerial         bool    `json:"track_serial"`
	TaxID               int64   `json:"tax_id"`
	IsActive            bool    `json:"is_active"`
}
