package companies

// CompanyForm represents the form data for creating/updating a company
// Currently mirrors the model, but separating it allows for UI-specific fields (e.g. checkbox handling)
type CompanyForm struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Address string `json:"address"`
	TaxID   string `json:"tax_id"`
}
