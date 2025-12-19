package suppliers

type SupplierForm struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
}
