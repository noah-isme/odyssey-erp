package consol

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/consol/fx"
)

type ProfitLossFilters struct {
	GroupID  int64
	Period   string
	Entities []int64
	FxOn     bool
}

type ProfitLossLine struct {
	AccountCode string
	AccountName string
	LocalAmount float64
	GroupAmount float64
	Section     string
}

type ProfitLossTotals struct {
	Revenue     float64
	COGS        float64
	GrossProfit float64
	Opex        float64
	NetIncome   float64
	DeltaFX     float64
}

type ProfitLossContribution struct {
	EntityName  string
	GroupAmount float64
	Percent     float64
}

type ProfitLossReport struct {
	Filters       ProfitLossFilters
	Lines         []ProfitLossLine
	Totals        ProfitLossTotals
	Contributions []ProfitLossContribution
}

type ProfitLossRepository interface {
	ConsolBalancesByType(ctx context.Context, groupID int64, periodCode string, entities []int64) ([]ConsolBalanceByTypeQueryRow, error)
	GroupReportingCurrency(ctx context.Context, groupID int64) (string, error)
	MemberCurrencies(ctx context.Context, groupID int64) (map[int64]string, error)
	FxRateForPeriod(ctx context.Context, asOf time.Time, pair string) (fx.Quote, error)
}

type ProfitLossService struct {
	repo ProfitLossRepository
}

func NewProfitLossService(repo ProfitLossRepository) *ProfitLossService {
	return &ProfitLossService{repo: repo}
}

func (s *ProfitLossService) Build(ctx context.Context, filters ProfitLossFilters) (ProfitLossReport, []string, error) {
	if err := s.validateFilters(filters); err != nil {
		return ProfitLossReport{}, nil, err
	}

	rows, err := s.repo.ConsolBalancesByType(ctx, filters.GroupID, filters.Period, filters.Entities)
	if err != nil {
		return ProfitLossReport{}, nil, err
	}

	included := buildIncludedMap(filters.Entities)
	includeAll := len(filters.Entities) == 0

	var fxResult fxSetupResult
	warnings := make([]string, 0)
	if filters.FxOn {
		fxResult, err = setupFXConverter(ctx, s.repo, filters.GroupID, filters.Period, included, includeAll, func(q fx.Quote) bool {
			return q.Average > 0
		})
		if err != nil {
			return ProfitLossReport{}, nil, err
		}
		warnings = fxResult.warnings
	}

	lines, contributions, totals := s.processRows(rows, included, includeAll, fxResult, filters.Period, &warnings)
	contributionList := buildPLContributionList(contributions, totals.contributionBasis)

	grossProfit := totals.totalRevenue - totals.totalCogs
	netIncome := grossProfit - totals.totalOpex

	report := ProfitLossReport{
		Filters: filters,
		Lines:   lines,
		Totals: ProfitLossTotals{
			Revenue:     totals.totalRevenue,
			COGS:        totals.totalCogs,
			GrossProfit: grossProfit,
			Opex:        totals.totalOpex,
			NetIncome:   netIncome,
			DeltaFX:     totals.deltaFX,
		},
		Contributions: contributionList,
	}
	report.Filters.FxOn = fxResult.applied && fxResult.converter != nil

	return report, warnings, nil
}

func (s *ProfitLossService) validateFilters(filters ProfitLossFilters) error {
	if s == nil || s.repo == nil {
		return errors.New("consol: profit loss service not initialised")
	}
	if filters.GroupID <= 0 {
		return fmt.Errorf("group id wajib diisi")
	}
	if strings.TrimSpace(filters.Period) == "" {
		return fmt.Errorf("periode wajib diisi")
	}
	if _, err := time.Parse("2006-01", filters.Period); err != nil {
		return fmt.Errorf("format periode tidak valid")
	}
	return nil
}

type plTotals struct {
	totalRevenue      float64
	totalCogs         float64
	totalOpex         float64
	deltaFX           float64
	contributionBasis float64
}

func (s *ProfitLossService) processRows(
	rows []ConsolBalanceByTypeQueryRow,
	included map[int64]struct{},
	includeAll bool,
	fxResult fxSetupResult,
	period string,
	warnings *[]string,
) ([]ProfitLossLine, map[int64]ProfitLossContribution, plTotals) {
	lines := make([]ProfitLossLine, 0, len(rows))
	contributions := make(map[int64]ProfitLossContribution)
	var totals plTotals

	for _, row := range rows {
		members, err := ParseMembers(row.MembersJSON)
		if err != nil {
			continue
		}

		mb := filterMembers(members, included, includeAll)
		if len(mb.members) == 0 {
			continue
		}

		convertedGroup := scaleAmount(row.GroupAmount, row.LocalAmount, mb.localTotal)
		convertedGroup, delta := s.applyFXConversion(row, mb, fxResult, convertedGroup, period, warnings)
		totals.deltaFX += delta

		section := classifyPLSection(row.AccountType, row.GroupAccountCode)
		displayLocal, displayGroup := normalisePLAmounts(section, mb.localTotal, convertedGroup)

		lines = append(lines, ProfitLossLine{
			AccountCode: row.GroupAccountCode,
			AccountName: row.GroupAccountName,
			LocalAmount: displayLocal,
			GroupAmount: displayGroup,
			Section:     section,
		})

		switch section {
		case "REVENUE":
			totals.totalRevenue += displayGroup
		case "COGS":
			totals.totalCogs += displayGroup
		default:
			totals.totalOpex += displayGroup
		}

		s.updateContributions(mb, displayGroup, contributions)
		totals.contributionBasis += math.Abs(displayGroup)
	}

	return lines, contributions, totals
}

func (s *ProfitLossService) applyFXConversion(
	row ConsolBalanceByTypeQueryRow,
	mb memberBalance,
	fxResult fxSetupResult,
	convertedGroup float64,
	period string,
	warnings *[]string,
) (float64, float64) {
	if !fxResult.applied || fxResult.converter == nil {
		return convertedGroup, 0
	}

	currencyTotals := buildCurrencyTotals(mb.members, fxResult.memberCurrencies, fxResult.reportingCurrency)
	fxInput := buildFXLines(row.GroupAccountCode, currencyTotals, convertedGroup, mb.localTotal)

	if len(fxInput) == 0 {
		return convertedGroup, 0
	}

	convertedLines, delta, err := fxResult.converter.ConvertProfitLoss(fxInput)
	if err != nil {
		if missingErr, ok := err.(*fx.MissingRateError); ok {
			for _, pair := range missingErr.Pairs {
				*warnings = append(*warnings, fmt.Sprintf("FX rate missing for %s at %s", pair, period))
			}
		}
		return convertedGroup, 0
	}

	var convertedTotal float64
	for _, line := range convertedLines {
		convertedTotal += line.GroupAmount
	}
	return convertedTotal, delta
}

func (s *ProfitLossService) updateContributions(
	mb memberBalance,
	displayGroup float64,
	contributions map[int64]ProfitLossContribution,
) {
	for _, member := range mb.members {
		weight := calculateContributionWeight(mb.absTotal, member.LocalAmount, len(mb.members))
		share := displayGroup * weight

		contrib := contributions[member.CompanyID]
		if contrib.EntityName == "" {
			contrib.EntityName = member.CompanyName
		}
		contrib.GroupAmount += share
		contributions[member.CompanyID] = contrib
	}
}

func buildPLContributionList(contributions map[int64]ProfitLossContribution, basis float64) []ProfitLossContribution {
	list := make([]ProfitLossContribution, 0, len(contributions))
	for _, contrib := range contributions {
		if basis != 0 {
			contrib.Percent = (math.Abs(contrib.GroupAmount) / basis) * 100
		}
		list = append(list, contrib)
	}
	sort.SliceStable(list, func(i, j int) bool {
		return math.Abs(list[i].GroupAmount) > math.Abs(list[j].GroupAmount)
	})
	return list
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
