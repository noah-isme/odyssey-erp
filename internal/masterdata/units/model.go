package units

// Unit represents a unit of measure
type Unit struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}
