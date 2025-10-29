package insights

import "html/template"

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

// VarianceViewModel menampilkan persentase perubahan.
type VarianceViewModel struct {
        Metric string
        MoMPct float64
        YoYPct float64
}

// ContributionViewModel menampilkan porsi per cabang.
type ContributionViewModel struct {
        Branch     string
        NetPct     float64
        RevenuePct float64
}

// ViewModel adalah struktur utama halaman insights.
type ViewModel struct {
        Filters      FiltersViewModel
        Series       []PointViewModel
        Variances    []VarianceViewModel
        Contribution []ContributionViewModel
        Chart        template.HTML
        Ready        bool
}
