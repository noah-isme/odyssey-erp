package taxes

// Tax represents a tax configuration
type Tax struct {
	ID   int64   `json:"id"`
	Code string  `json:"code"`
	Name string  `json:"name"`
	Rate float64 `json:"rate"`
}
