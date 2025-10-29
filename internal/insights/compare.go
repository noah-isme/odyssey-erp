package insights

import "context"

// CompareFilters menampung parameter filter untuk perbandingan Net dan Revenue.
type CompareFilters struct {
	From      string
	To        string
	CompanyID *int64
	BranchID  *int64
}

// MonthlySeries mewakili satu titik data bulanan.
type MonthlySeries struct {
	Month   string
	Net     float64
	Revenue float64
}

// CompareService mendefinisikan kontrak data yang dibutuhkan handler insights.
type CompareService interface {
	CompareMonthlyNetRevenue(ctx context.Context, filters CompareFilters) ([]MonthlySeries, error)
}
