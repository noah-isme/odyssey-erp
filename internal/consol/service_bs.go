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

type BalanceSheetFilters struct {
	GroupID  int64
	Period   string
	Entities []int64
	FxOn     bool
}

type BalanceSheetLine struct {
	AccountCode string
	AccountName string
	LocalAmount float64
	GroupAmount float64
	Section     string
}

type BalanceSheetTotals struct {
	Assets     float64
	LiabEquity float64
	Balanced   bool
	DeltaFX    float64
}

type BalanceSheetContribution struct {
	EntityName  string
	GroupAmount float64
	Percent     float64
}

type BalanceSheetReport struct {
	Filters       BalanceSheetFilters
	Assets        []BalanceSheetLine
	LiabilitiesEq []BalanceSheetLine
	Totals        BalanceSheetTotals
	Contributions []BalanceSheetContribution
}

type BalanceSheetRepository interface {
	ConsolBalancesByType(ctx context.Context, groupID int64, periodCode string, entities []int64) ([]ConsolBalanceByTypeQueryRow, error)
	GroupReportingCurrency(ctx context.Context, groupID int64) (string, error)
	MemberCurrencies(ctx context.Context, groupID int64) (map[int64]string, error)
	FxRateForPeriod(ctx context.Context, asOf time.Time, pair string) (fx.Quote, error)
}

type BalanceSheetService struct {
	repo BalanceSheetRepository
}

func NewBalanceSheetService(repo BalanceSheetRepository) *BalanceSheetService {
	return &BalanceSheetService{repo: repo}
}

func (s *BalanceSheetService) Build(ctx context.Context, filters BalanceSheetFilters) (BalanceSheetReport, []string, error) {
	if err := s.validateFilters(filters); err != nil {
		return BalanceSheetReport{}, nil, err
	}

	rows, err := s.repo.ConsolBalancesByType(ctx, filters.GroupID, filters.Period, filters.Entities)
	if err != nil {
		return BalanceSheetReport{}, nil, err
	}

	included := buildIncludedMap(filters.Entities)
	includeAll := len(filters.Entities) == 0

	var fxResult fxSetupResult
	warnings := make([]string, 0)
	if filters.FxOn {
		fxResult, err = setupFXConverter(ctx, s.repo, filters.GroupID, filters.Period, included, includeAll, func(q fx.Quote) bool {
			return q.Closing > 0
		})
		if err != nil {
			return BalanceSheetReport{}, nil, err
		}
		warnings = fxResult.warnings
	}

	assets, liabEq, contributions, totals := s.processRows(rows, included, includeAll, fxResult, filters.Period, &warnings)

	sortLines(assets)
	sortLines(liabEq)
	contributionList := buildContributionList(contributions, totals.contributionBasis)

	report := BalanceSheetReport{
		Filters:       filters,
		Assets:        assets,
		LiabilitiesEq: liabEq,
		Totals: BalanceSheetTotals{
			Assets:     totals.totalAssets,
			LiabEquity: totals.totalLiabEq,
			Balanced:   math.Abs(totals.totalAssets-totals.totalLiabEq) <= 0.01,
			DeltaFX:    totals.deltaFX,
		},
		Contributions: contributionList,
	}
	report.Filters.FxOn = fxResult.applied && fxResult.converter != nil

	return report, warnings, nil
}

func (s *BalanceSheetService) validateFilters(filters BalanceSheetFilters) error {
	if s == nil || s.repo == nil {
		return errors.New("consol: balance sheet service not initialised")
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

type bsTotals struct {
	totalAssets       float64
	totalLiabEq       float64
	deltaFX           float64
	contributionBasis float64
}

func (s *BalanceSheetService) processRows(
	rows []ConsolBalanceByTypeQueryRow,
	included map[int64]struct{},
	includeAll bool,
	fxResult fxSetupResult,
	period string,
	warnings *[]string,
) ([]BalanceSheetLine, []BalanceSheetLine, map[int64]BalanceSheetContribution, bsTotals) {
	assets := make([]BalanceSheetLine, 0, len(rows))
	liabEq := make([]BalanceSheetLine, 0, len(rows))
	contributions := make(map[int64]BalanceSheetContribution)
	var totals bsTotals

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

		section := strings.ToUpper(row.AccountType)
		displayLocal := math.Abs(mb.localTotal)
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
			totals.totalAssets += displayGroup
		} else {
			liabEq = append(liabEq, line)
			totals.totalLiabEq += displayGroup
		}

		s.updateContributions(mb, displayGroup, contributions)
		totals.contributionBasis += displayGroup
	}

	return assets, liabEq, contributions, totals
}

func (s *BalanceSheetService) applyFXConversion(
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

	convertedLines, delta, err := fxResult.converter.ConvertBalanceSheet(fxInput)
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

func (s *BalanceSheetService) updateContributions(
	mb memberBalance,
	displayGroup float64,
	contributions map[int64]BalanceSheetContribution,
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

func buildIncludedMap(entities []int64) map[int64]struct{} {
	included := make(map[int64]struct{}, len(entities))
	for _, id := range entities {
		included[id] = struct{}{}
	}
	return included
}

func sortLines(lines []BalanceSheetLine) {
	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].AccountCode < lines[j].AccountCode
	})
}

func buildContributionList(contributions map[int64]BalanceSheetContribution, basis float64) []BalanceSheetContribution {
	list := make([]BalanceSheetContribution, 0, len(contributions))
	for _, contrib := range contributions {
		if basis != 0 {
			contrib.Percent = (contrib.GroupAmount / basis) * 100
		}
		list = append(list, contrib)
	}
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].GroupAmount > list[j].GroupAmount
	})
	return list
}
