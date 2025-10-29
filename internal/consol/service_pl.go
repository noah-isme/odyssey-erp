package consol

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
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

// Build assembles the consolidated profit and loss report for the provided filters.
func (s *ProfitLossService) Build(ctx context.Context, filters ProfitLossFilters) (ProfitLossReport, error) {
	if s == nil || s.repo == nil {
		return ProfitLossReport{}, errors.New("consol: profit loss service not initialised")
	}
	if filters.GroupID <= 0 {
		return ProfitLossReport{}, fmt.Errorf("group id wajib diisi")
	}
	if strings.TrimSpace(filters.Period) == "" {
		return ProfitLossReport{}, fmt.Errorf("periode wajib diisi")
	}
	if _, err := time.Parse("2006-01", filters.Period); err != nil {
		return ProfitLossReport{}, fmt.Errorf("format periode tidak valid")
	}

	rows, err := s.repo.ConsolBalancesByType(ctx, filters.GroupID, filters.Period, filters.Entities)
	if err != nil {
		return ProfitLossReport{}, err
	}

	includeAll := len(filters.Entities) == 0
	included := make(map[int64]struct{}, len(filters.Entities))
	for _, id := range filters.Entities {
		included[id] = struct{}{}
	}

	contributions := make(map[int64]ProfitLossContribution)
	var contributionBasis float64
	var totalRevenue float64
	var totalCogs float64
	var totalOpex float64
	var deltaFX float64

	lines := make([]ProfitLossLine, 0, len(rows))

	for _, row := range rows {
		members, err := ParseMembers(row.MembersJSON)
		if err != nil {
			return ProfitLossReport{}, err
		}
		filtered := members[:0]
		var localTotal float64
		var absTotal float64
		for _, m := range members {
			if !includeAll {
				if _, ok := included[m.CompanyID]; !ok {
					continue
				}
			}
			filtered = append(filtered, m)
			localTotal += m.LocalAmount
			absTotal += math.Abs(m.LocalAmount)
		}
		if len(filtered) == 0 {
			continue
		}

		convertedGroup := scaleAmount(row.GroupAmount, row.LocalAmount, localTotal)
		originalGroup := convertedGroup
		deltaFX += convertedGroup - originalGroup

		section := classifyPLSection(row.AccountType, row.GroupAccountCode)
		displayLocal, displayGroup := normalisePLAmounts(section, localTotal, convertedGroup)

		lines = append(lines, ProfitLossLine{
			AccountCode: row.GroupAccountCode,
			AccountName: row.GroupAccountName,
			LocalAmount: displayLocal,
			GroupAmount: displayGroup,
			Section:     section,
		})

		switch section {
		case "REVENUE":
			totalRevenue += displayGroup
		case "COGS":
			totalCogs += displayGroup
		default:
			totalOpex += displayGroup
		}

		for _, member := range filtered {
			var weight float64
			if absTotal == 0 {
				if len(filtered) == 0 {
					weight = 0
				} else {
					weight = 1 / float64(len(filtered))
				}
			} else {
				weight = math.Abs(member.LocalAmount) / absTotal
			}
			share := displayGroup * weight
			contrib := contributions[member.CompanyID]
			if contrib.EntityName == "" {
				contrib.EntityName = member.CompanyName
			}
			contrib.GroupAmount += share
			contributions[member.CompanyID] = contrib
		}
		contributionBasis += math.Abs(displayGroup)
	}

	contributionList := make([]ProfitLossContribution, 0, len(contributions))
	for id, contrib := range contributions {
		_ = id
		if contributionBasis != 0 {
			contrib.Percent = (math.Abs(contrib.GroupAmount) / contributionBasis) * 100
		}
		contributionList = append(contributionList, contrib)
	}
	sort.SliceStable(contributionList, func(i, j int) bool {
		return math.Abs(contributionList[i].GroupAmount) > math.Abs(contributionList[j].GroupAmount)
	})

	grossProfit := totalRevenue - totalCogs
	netIncome := grossProfit - totalOpex

	report := ProfitLossReport{
		Filters: filters,
		Lines:   lines,
		Totals: ProfitLossTotals{
			Revenue:     totalRevenue,
			COGS:        totalCogs,
			GrossProfit: grossProfit,
			Opex:        totalOpex,
			NetIncome:   netIncome,
			DeltaFX:     deltaFX,
		},
		Contributions: contributionList,
	}

	return report, nil
}

func classifyPLSection(accountType, accountCode string) string {
	switch strings.ToUpper(accountType) {
	case "REVENUE", "INCOME":
		return "REVENUE"
	}
	if strings.HasPrefix(accountCode, "5") {
		return "COGS"
	}
	return "OPEX"
}

func normalisePLAmounts(section string, local, group float64) (float64, float64) {
	switch section {
	case "REVENUE":
		return -local, -group
	default:
		return local, group
	}
}

func scaleAmount(base float64, originalTotal float64, filteredTotal float64) float64 {
	if originalTotal == 0 {
		return base
	}
	return base * (filteredTotal / originalTotal)
}
