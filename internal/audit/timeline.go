package audit

import "time"

// TimelineFilters menampung filter dasar untuk audit timeline.
type TimelineFilters struct {
	From   time.Time
	To     time.Time
	Actor  string
	Entity string
	Action string
	Limit  int
	Offset int
}

// TimelineRow mewakili satu baris audit timeline.
type TimelineRow struct {
	At       time.Time
	Actor    string
	Action   string
	Entity   string
	EntityID string
}

// PagingInfo menyimpan metadata pagination sederhana.
type PagingInfo struct {
	Page    int
	HasNext bool
}

// ViewModel menyatukan data untuk template audit timeline.
type ViewModel struct {
	Rows   []TimelineRow
	Paging PagingInfo
}
