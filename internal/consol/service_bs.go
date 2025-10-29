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

// Build assembles the consolidated balance sheet.
func (s *BalanceSheetService) Build(ctx context.Context, filters BalanceSheetFilters) (BalanceSheetReport, error) {
	if s == nil || s.repo == nil {
		return BalanceSheetReport{}, errors.New("consol: balance sheet service not initialised")
	}
	if filters.GroupID <= 0 {
		return BalanceSheetReport{}, fmt.Errorf("group id wajib diisi")
	}
	if strings.TrimSpace(filters.Period) == "" {
		return BalanceSheetReport{}, fmt.Errorf("periode wajib diisi")
	}
	if _, err := time.Parse("2006-01", filters.Period); err != nil {
		return BalanceSheetReport{}, fmt.Errorf("format periode tidak valid")
	}

	rows, err := s.repo.ConsolBalancesByType(ctx, filters.GroupID, filters.Period, filters.Entities)
	if err != nil {
		return BalanceSheetReport{}, err
	}

	includeAll := len(filters.Entities) == 0
	included := make(map[int64]struct{}, len(filters.Entities))
	for _, id := range filters.Entities {
		included[id] = struct{}{}
	}

	assets := make([]BalanceSheetLine, 0, len(rows))
	liabEq := make([]BalanceSheetLine, 0, len(rows))
	contributions := make(map[int64]BalanceSheetContribution)
	var contributionBasis float64
	var totalAssets float64
	var totalLiabEq float64
	var deltaFX float64

	for _, row := range rows {
		members, err := ParseMembers(row.MembersJSON)
		if err != nil {
			return BalanceSheetReport{}, err
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

		section := strings.ToUpper(row.AccountType)
		displayLocal := math.Abs(localTotal)
		displayGroup := math.Abs(convertedGroup)

		line := BalanceSheetLine{
			AccountCode: row.GroupAccountCode,
			AccountName: row.GroupAccountName,
			LocalAmount: displayLocal,
			GroupAmount: displayGroup,
			Section:     section,
		}

		if section == "ASSET" {
			assets = append(assets, line)
			totalAssets += displayGroup
		} else {
			liabEq = append(liabEq, line)
			totalLiabEq += displayGroup
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
		contributionBasis += displayGroup
	}

	sort.SliceStable(assets, func(i, j int) bool {
		return assets[i].AccountCode < assets[j].AccountCode
	})
	sort.SliceStable(liabEq, func(i, j int) bool {
		return liabEq[i].AccountCode < liabEq[j].AccountCode
	})

	contributionList := make([]BalanceSheetContribution, 0, len(contributions))
	for _, contrib := range contributions {
		if contributionBasis != 0 {
			contrib.Percent = (contrib.GroupAmount / contributionBasis) * 100
		}
		contributionList = append(contributionList, contrib)
	}
	sort.SliceStable(contributionList, func(i, j int) bool {
		return contributionList[i].GroupAmount > contributionList[j].GroupAmount
	})

	report := BalanceSheetReport{
		Filters:       filters,
		Assets:        assets,
		LiabilitiesEq: liabEq,
		Totals: BalanceSheetTotals{
			Assets:     totalAssets,
			LiabEquity: totalLiabEq,
			Balanced:   math.Abs(totalAssets-totalLiabEq) <= 0.01,
			DeltaFX:    deltaFX,
		},
		Contributions: contributionList,
	}

	return report, nil
}
