package shared

// ListFilters represents standard list page filters
type ListFilters struct {
	Page       int
	Limit      int
	Search     string
	SortBy     string
	SortDir    string
	IsActive   *bool
	
	// Entity specific filters
	CompanyID  *int64
	BranchID   *int64
	CategoryID *int64
}
