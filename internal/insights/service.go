package insights

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	insightsdb "github.com/odyssey-erp/odyssey-erp/internal/insights/db"
)

// Repository exposes the subset of sqlc queries required by the service.
type Repository interface {
	CompareMonthlyNetRevenue(ctx context.Context, arg insightsdb.CompareMonthlyNetRevenueParams) ([]insightsdb.CompareMonthlyNetRevenueRow, error)
	ContributionByBranch(ctx context.Context, arg insightsdb.ContributionByBranchParams) ([]insightsdb.ContributionByBranchRow, error)
}

// Result aggregates all datasets required by the insights view.
type Result struct {
	Series       []MonthlySeries
	Variance     []VarianceMetric
	Contribution []ContributionShare
}

// Service coordinates insights data preparation from the repository.
type Service struct {
	repo Repository
}

// NewService constructs a Service instance.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Load aggregates the comparison series, contribution breakdown, and variance metrics.
func (s *Service) Load(ctx context.Context, filters CompareFilters) (Result, error) {
	if s.repo == nil {
		return Result{}, fmt.Errorf("insights: repository not configured")
	}
	fromTime, err := parseMonth(filters.From)
	if err != nil {
		return Result{}, fmt.Errorf("invalid from period: %w", err)
	}
	toTime, err := parseMonth(filters.To)
	if err != nil {
		return Result{}, fmt.Errorf("invalid to period: %w", err)
	}
	if fromTime.After(toTime) {
		return Result{}, fmt.Errorf("insights: from period must be before to period")
	}

	queryFrom := fromTime
	yoyBaseline := toTime.AddDate(-1, 0, 0)
	if yoyBaseline.Before(queryFrom) {
		queryFrom = yoyBaseline
	}

	rows, err := s.repo.CompareMonthlyNetRevenue(ctx, insightsdb.CompareMonthlyNetRevenueParams{
		FromPeriod: formatMonth(queryFrom),
		ToPeriod:   formatMonth(toTime),
		CompanyID:  valueOrDefault(filters.CompanyID, 1),
		BranchID:   optionalInt(filters.BranchID),
	})
	if err != nil {
		return Result{}, err
	}
	series, lookup := normalizeSeries(rows, fromTime, toTime)

	contributions, err := s.repo.ContributionByBranch(ctx, insightsdb.ContributionByBranchParams{
		Period:    formatMonth(toTime),
		CompanyID: valueOrDefault(filters.CompanyID, 1),
	})
	if err != nil {
		return Result{}, err
	}

	variance := computeVariance(lookup, toTime)
	contribVM := computeContribution(contributions)

	return Result{Series: series, Variance: variance, Contribution: contribVM}, nil
}

func normalizeSeries(rows []insightsdb.CompareMonthlyNetRevenueRow, from, to time.Time) ([]MonthlySeries, map[string]insightsdb.CompareMonthlyNetRevenueRow) {
	lookup := make(map[string]insightsdb.CompareMonthlyNetRevenueRow, len(rows))
	for _, row := range rows {
		lookup[row.Period] = row
	}
	months := enumerateMonths(from, to)
	series := make([]MonthlySeries, 0, len(months))
	for _, month := range months {
		key := formatMonth(month)
		if row, ok := lookup[key]; ok {
			series = append(series, MonthlySeries{Month: key, Net: row.Net, Revenue: row.Revenue})
			continue
		}
		series = append(series, MonthlySeries{Month: key})
	}
	return series, lookup
}

func enumerateMonths(from, to time.Time) []time.Time {
	if from.After(to) {
		return nil
	}
	var months []time.Time
	current := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
	for !current.After(end) {
		months = append(months, current)
		current = current.AddDate(0, 1, 0)
	}
	return months
}

func computeVariance(lookup map[string]insightsdb.CompareMonthlyNetRevenueRow, current time.Time) []VarianceMetric {
	latestKey := formatMonth(current)
	prevKey := formatMonth(current.AddDate(0, -1, 0))
	yoyKey := formatMonth(current.AddDate(-1, 0, 0))

	latest := lookup[latestKey]
	prev := lookup[prevKey]
	yoy := lookup[yoyKey]

	return []VarianceMetric{
		{
			Metric: "Net",
			MoMPct: variancePercent(prev.Net, latest.Net),
			YoYPct: variancePercent(yoy.Net, latest.Net),
		},
		{
			Metric: "Revenue",
			MoMPct: variancePercent(prev.Revenue, latest.Revenue),
			YoYPct: variancePercent(yoy.Revenue, latest.Revenue),
		},
	}
}

func variancePercent(base, current float64) float64 {
	if almostZero(base) {
		if almostZero(current) {
			return 0
		}
		return 100
	}
	return (current - base) / base * 100
}

func computeContribution(rows []insightsdb.ContributionByBranchRow) []ContributionShare {
	if len(rows) == 0 {
		return nil
	}
	var totalNet float64
	var totalRevenue float64
	for _, row := range rows {
		totalNet += row.Net
		totalRevenue += row.Revenue
	}
	type branchShare struct {
		share ContributionShare
		id    int64
	}
	tmp := make([]branchShare, 0, len(rows))
	for _, row := range rows {
		tmp = append(tmp, branchShare{share: ContributionShare{
			Branch:     branchLabel(row.BranchID),
			NetPct:     safePercent(row.Net, totalNet),
			RevenuePct: safePercent(row.Revenue, totalRevenue),
		}, id: row.BranchID})
	}
	sort.Slice(tmp, func(i, j int) bool {
		return tmp[i].id < tmp[j].id
	})
	shares := make([]ContributionShare, 0, len(tmp))
	for _, item := range tmp {
		shares = append(shares, item.share)
	}
	return shares
}

func safePercent(value, total float64) float64 {
	if almostZero(total) {
		return 0
	}
	return (value / total) * 100
}

func almostZero(v float64) bool {
	return v > -0.0001 && v < 0.0001
}

func optionalInt(value *int64) interface{} {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func parseMonth(period string) (time.Time, error) {
	if period == "" {
		return time.Time{}, fmt.Errorf("empty period")
	}
	t, err := time.ParseInLocation("2006-01", period, time.UTC)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC), nil
}

func formatMonth(t time.Time) string {
	return t.Format("2006-01")
}

func branchLabel(id int64) string {
	if id <= 0 {
		return "Unassigned"
	}
	return fmt.Sprintf("Branch %d", id)
}

func valueOrDefault(ptr *int64, fallback int64) int64 {
	if ptr == nil || *ptr <= 0 {
		return fallback
	}
	return *ptr
}
