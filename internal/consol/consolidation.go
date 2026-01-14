package consol

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/consol/fx"
)

type fxSetupResult struct {
	converter         *fx.Converter
	reportingCurrency string
	memberCurrencies  map[int64]string
	warnings          []string
	applied           bool
}

type fxRepository interface {
	GroupReportingCurrency(ctx context.Context, groupID int64) (string, error)
	MemberCurrencies(ctx context.Context, groupID int64) (map[int64]string, error)
	FxRateForPeriod(ctx context.Context, asOf time.Time, pair string) (fx.Quote, error)
}

func setupFXConverter(
	ctx context.Context,
	repo fxRepository,
	groupID int64,
	period string,
	included map[int64]struct{},
	includeAll bool,
	rateValidator func(quote fx.Quote) bool,
) (fxSetupResult, error) {
	result := fxSetupResult{warnings: make([]string, 0)}

	reportingCurrency, err := repo.GroupReportingCurrency(ctx, groupID)
	if err != nil {
		return result, err
	}
	result.reportingCurrency = strings.ToUpper(strings.TrimSpace(reportingCurrency))

	memberCurrencies, err := repo.MemberCurrencies(ctx, groupID)
	if err != nil {
		return result, err
	}
	result.memberCurrencies = memberCurrencies

	asOf, _ := time.Parse("2006-01", period)
	asOf = time.Date(asOf.Year(), asOf.Month(), 1, 0, 0, 0, 0, time.UTC)

	requiredCurrencies := collectRequiredCurrencies(memberCurrencies, included, includeAll, result.reportingCurrency)
	quotes, missing := fetchFXQuotes(ctx, repo, asOf, requiredCurrencies, result.reportingCurrency, rateValidator)

	if len(missing) > 0 {
		for _, pair := range missing {
			result.warnings = append(result.warnings, fmt.Sprintf("FX rate missing for %s at %s", pair, period))
		}
		return result, nil
	}

	policy := fx.Policy{
		ReportingCurrency:  result.reportingCurrency,
		ProfitLossMethod:   fx.MethodAverage,
		BalanceSheetMethod: fx.MethodClosing,
	}
	result.converter = fx.NewConverter(policy, quotes)
	result.applied = true

	return result, nil
}

func collectRequiredCurrencies(
	memberCurrencies map[int64]string,
	included map[int64]struct{},
	includeAll bool,
	reportingCurrency string,
) map[string]struct{} {
	required := make(map[string]struct{})

	if includeAll {
		for _, cur := range memberCurrencies {
			cur = strings.ToUpper(strings.TrimSpace(cur))
			if cur != "" && cur != reportingCurrency {
				required[cur] = struct{}{}
			}
		}
	} else {
		for id := range included {
			cur := strings.ToUpper(strings.TrimSpace(memberCurrencies[id]))
			if cur != "" && cur != reportingCurrency {
				required[cur] = struct{}{}
			}
		}
	}
	return required
}

func fetchFXQuotes(
	ctx context.Context,
	repo fxRepository,
	asOf time.Time,
	currencies map[string]struct{},
	reportingCurrency string,
	validator func(quote fx.Quote) bool,
) (map[string]fx.Quote, []string) {
	quotes := make(map[string]fx.Quote, len(currencies))
	missing := make([]string, 0)

	for cur := range currencies {
		pair := cur + reportingCurrency
		quote, err := repo.FxRateForPeriod(ctx, asOf, pair)
		if err != nil || !validator(quote) {
			missing = append(missing, pair)
			continue
		}
		quotes[pair] = quote
	}
	return quotes, missing
}

type memberBalance struct {
	members    []MemberShare
	localTotal float64
	absTotal   float64
}

func filterMembers(members []MemberShare, included map[int64]struct{}, includeAll bool) memberBalance {
	result := memberBalance{members: members[:0]}

	for _, m := range members {
		if !includeAll {
			if _, ok := included[m.CompanyID]; !ok {
				continue
			}
		}
		result.members = append(result.members, m)
		result.localTotal += m.LocalAmount
		result.absTotal += math.Abs(m.LocalAmount)
	}
	return result
}

func buildCurrencyTotals(
	filtered []MemberShare,
	memberCurrencies map[int64]string,
	reportingCurrency string,
) map[string]float64 {
	totals := make(map[string]float64)
	for _, member := range filtered {
		currency := reportingCurrency
		if memberCurrencies != nil {
			if cur, ok := memberCurrencies[member.CompanyID]; ok {
				if trimmed := strings.ToUpper(strings.TrimSpace(cur)); trimmed != "" {
					currency = trimmed
				}
			}
		}
		totals[currency] += member.LocalAmount
	}
	return totals
}

func buildFXLines(
	accountCode string,
	currencyTotals map[string]float64,
	convertedGroup float64,
	localTotal float64,
) []fx.Line {
	lines := make([]fx.Line, 0, len(currencyTotals))
	for currency, amount := range currencyTotals {
		var share float64
		if localTotal != 0 {
			share = convertedGroup * (amount / localTotal)
		}
		lines = append(lines, fx.Line{
			AccountCode:   accountCode,
			LocalCurrency: currency,
			LocalAmount:   amount,
			GroupAmount:   share,
		})
	}
	return lines
}

func calculateContributionWeight(absTotal float64, memberAmount float64, memberCount int) float64 {
	if absTotal == 0 {
		if memberCount == 0 {
			return 0
		}
		return 1 / float64(memberCount)
	}
	return math.Abs(memberAmount) / absTotal
}
