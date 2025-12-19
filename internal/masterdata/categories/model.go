package categories

// Category represents a product category
type Category struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}
