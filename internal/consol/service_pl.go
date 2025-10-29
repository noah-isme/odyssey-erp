package consol

import (
	"context"
	"errors"
)

// ProfitLossFilters defines the supported filters for the consolidated P&L report.
type ProfitLossFilters struct {
	GroupID  int64
	Period   string
	Entities []int64
	FxOn     bool
}

// ProfitLossLine represents a single row in the consolidated P&L statement.
type ProfitLossLine struct {
	AccountCode string
	AccountName string
	LocalAmount float64
	GroupAmount float64
	Section     string
}

// ProfitLossTotals captures common totals for the statement.
type ProfitLossTotals struct {
	Revenue     float64
	COGS        float64
	GrossProfit float64
	Opex        float64
	NetIncome   float64
	DeltaFX     float64
}

// ProfitLossContribution details how each member contributes to the consolidated total.
type ProfitLossContribution struct {
	EntityName  string
	GroupAmount float64
	Percent     float64
}

// ProfitLossReport is the domain representation prepared for downstream consumers.
type ProfitLossReport struct {
	Filters       ProfitLossFilters
	Lines         []ProfitLossLine
	Totals        ProfitLossTotals
	Contributions []ProfitLossContribution
}

// ProfitLossRepository abstracts the data access required by the P&L service.
type ProfitLossRepository interface {
	ConsolBalancesByType(ctx context.Context, groupID int64, periodCode string, entities []int64) ([]ConsolBalanceByTypeQueryRow, error)
}

// ProfitLossService performs aggregation of the consolidated profit and loss statement.
type ProfitLossService struct {
	repo ProfitLossRepository
}

// NewProfitLossService constructs a new service instance.
func NewProfitLossService(repo ProfitLossRepository) *ProfitLossService {
	return &ProfitLossService{repo: repo}
}

// ErrProfitLossNotImplemented is returned while the aggregation logic is under construction.
var ErrProfitLossNotImplemented = errors.New("consol: profit and loss aggregation not implemented yet")

// Build assembles the consolidated profit and loss report for the provided filters.
func (s *ProfitLossService) Build(ctx context.Context, filters ProfitLossFilters) (ProfitLossReport, error) {
	if s == nil || s.repo == nil {
		return ProfitLossReport{}, errors.New("consol: profit loss service not initialised")
	}
	return ProfitLossReport{}, ErrProfitLossNotImplemented
}
