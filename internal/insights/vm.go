package insights

// FiltersViewModel mewakili filter yang dikirim ke template SSR.
type FiltersViewModel struct {
	From      string
	To        string
	CompanyID *int64
	BranchID  *int64
}

// PointViewModel membungkus angka yang siap ditampilkan.
type PointViewModel struct {
	Month   string
	Net     float64
	Revenue float64
}

// ViewModel adalah struktur utama halaman insights.
type ViewModel struct {
	Filters FiltersViewModel
	Series  []PointViewModel
	HasData bool
}
