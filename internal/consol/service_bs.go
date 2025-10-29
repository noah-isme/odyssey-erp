package consol

import (
	"context"
	"errors"
)

// BalanceSheetFilters controls the consolidated balance sheet aggregation request.
type BalanceSheetFilters struct {
	GroupID  int64
	Period   string
	Entities []int64
	FxOn     bool
}

// BalanceSheetLine represents a single balance sheet account.
type BalanceSheetLine struct {
	AccountCode string
	AccountName string
	LocalAmount float64
	GroupAmount float64
	Section     string
}

// BalanceSheetTotals contains the aggregated totals for the statement.
type BalanceSheetTotals struct {
	Assets     float64
	LiabEquity float64
	Balanced   bool
	DeltaFX    float64
}

// BalanceSheetContribution reflects an entity contribution for the balance sheet.
type BalanceSheetContribution struct {
	EntityName  string
	GroupAmount float64
	Percent     float64
}

// BalanceSheetReport is the domain output for the balance sheet service.
type BalanceSheetReport struct {
	Filters       BalanceSheetFilters
	Assets        []BalanceSheetLine
	LiabilitiesEq []BalanceSheetLine
	Totals        BalanceSheetTotals
	Contributions []BalanceSheetContribution
}

// BalanceSheetRepository abstracts the persistence needs for the balance sheet service.
type BalanceSheetRepository interface {
	ConsolBalancesByType(ctx context.Context, groupID int64, periodCode string, entities []int64) ([]ConsolBalanceByTypeQueryRow, error)
}

// BalanceSheetService builds consolidated balance sheet view models.
type BalanceSheetService struct {
	repo BalanceSheetRepository
}

// NewBalanceSheetService constructs a balance sheet service instance.
func NewBalanceSheetService(repo BalanceSheetRepository) *BalanceSheetService {
	return &BalanceSheetService{repo: repo}
}

// ErrBalanceSheetNotImplemented is returned while the service is being delivered.
var ErrBalanceSheetNotImplemented = errors.New("consol: balance sheet aggregation not implemented yet")

// Build assembles the consolidated balance sheet.
func (s *BalanceSheetService) Build(ctx context.Context, filters BalanceSheetFilters) (BalanceSheetReport, error) {
	if s == nil || s.repo == nil {
		return BalanceSheetReport{}, errors.New("consol: balance sheet service not initialised")
	}
	return BalanceSheetReport{}, ErrBalanceSheetNotImplemented
}
