package audit

import "time"

// TimelineFilters menampung filter dasar untuk audit timeline.
type TimelineFilters struct {
	From     time.Time
	To       time.Time
	Actor    string
	Entity   string
	Action   string
	Page     int
	PageSize int
}

// TimelineRow mewakili satu baris audit timeline.
type TimelineRow struct {
	At        time.Time
	Actor     string
	Action    string
	Entity    string
	EntityID  string
	Period    string
	JournalNo string
}

// PagingInfo menyimpan metadata pagination sederhana.
type PagingInfo struct {
	Page     int
	HasNext  bool
	PageSize int
	PrevPage int
	NextPage int
}

// FiltersViewModel menampung nilai filter untuk template.
type FiltersViewModel struct {
	From   time.Time
	To     time.Time
	Actor  string
	Entity string
	Action string
}

// ViewModel menyatukan data untuk template audit timeline.
type ViewModel struct {
	Filters FiltersViewModel
	Rows    []TimelineRow
	Paging  PagingInfo
}
