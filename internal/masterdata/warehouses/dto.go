package warehouses

type WarehouseForm struct {
	BranchID int64  `json:"branch_id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Address  string `json:"address"`
}
