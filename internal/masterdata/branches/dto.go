package branches

type BranchForm struct {
	CompanyID int64  `json:"company_id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Address   string `json:"address"`
}
