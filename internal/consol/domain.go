package consol

import "time"

// Filters encapsulates query parameters for consolidated trial balance.
type Filters struct {
	GroupID  int64
	Period   string
	Entities []int64
}

// GroupAccountBalance represents balance for a consolidated group account.
type GroupAccountBalance struct {
	GroupAccountID   int64
	GroupAccountCode string
	GroupAccountName string
	LocalAmount      float64
	GroupAmount      float64
	Members          []MemberShare
}

// MemberShare describes contribution of a member entity for a balance line.
type MemberShare struct {
	CompanyID   int64
	CompanyName string
	LocalAmount float64
}

// TrialBalance aggregates consolidated balances and metadata.
type TrialBalance struct {
	Filters       Filters
	GroupName     string
	ReportingCCY  string
	PeriodDisplay string
	Totals        Totals
	Lines         []GroupAccountBalance
	Contributions []Contribution
	Members       []Member
}

// Totals summarises consolidated debit/credit totals.
type Totals struct {
	Local     float64
	Group     float64
	Balanced  bool
	Refreshed time.Time
}

// Contribution represents proportional amount for a member entity.
type Contribution struct {
	Entity  string
	Amount  float64
	Percent float64
}

// Member describes a consolidation group member entity.
type Member struct {
	CompanyID int64
	Name      string
	Enabled   bool
}
