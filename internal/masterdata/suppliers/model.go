package suppliers

// Supplier represents a supplier entity
// Note: database schema does not have created_at/updated_at columns
type Supplier struct {
	ID       int64  `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	IsActive bool   `json:"is_active"`
}
