package insights

import (
	"context"
	"math"
	"testing"

	insightsdb "github.com/odyssey-erp/odyssey-erp/internal/insights/db"
)

type stubRepo struct {
	compareRows []insightsdb.CompareMonthlyNetRevenueRow
	contribRows []insightsdb.ContributionByBranchRow
}

func (s stubRepo) CompareMonthlyNetRevenue(context.Context, insightsdb.CompareMonthlyNetRevenueParams) ([]insightsdb.CompareMonthlyNetRevenueRow, error) {
	return s.compareRows, nil
}

func (s stubRepo) ContributionByBranch(context.Context, insightsdb.ContributionByBranchParams) ([]insightsdb.ContributionByBranchRow, error) {
	return s.contribRows, nil
}

func TestServiceLoadAggregatesData(t *testing.T) {
	repo := stubRepo{
		compareRows: []insightsdb.CompareMonthlyNetRevenueRow{
			{Period: "2023-03", Net: 90, Revenue: 210},
			{Period: "2024-01", Net: 100, Revenue: 200},
			{Period: "2024-02", Net: 150, Revenue: 250},
			{Period: "2024-03", Net: 120, Revenue: 300},
		},
		contribRows: []insightsdb.ContributionByBranchRow{
			{BranchID: 1, Net: 70, Revenue: 200},
			{BranchID: 2, Net: 50, Revenue: 100},
		},
	}
	svc := NewService(repo)
	companyID := int64(2)
	result, err := svc.Load(context.Background(), CompareFilters{
		From:      "2024-01",
		To:        "2024-03",
		CompanyID: &companyID,
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(result.Series) != 3 {
		t.Fatalf("expected 3 series points, got %d", len(result.Series))
	}
	if result.Series[0].Month != "2024-01" || result.Series[2].Month != "2024-03" {
		t.Fatalf("unexpected months order: %+v", result.Series)
	}
	if len(result.Variance) != 2 {
		t.Fatalf("expected variance for two metrics, got %d", len(result.Variance))
	}
	netVariance := result.Variance[0]
	if math.Abs(netVariance.MoMPct-(-20)) > 0.01 {
		t.Fatalf("unexpected net MoM %%: %.2f", netVariance.MoMPct)
	}
	if math.Abs(netVariance.YoYPct-33.33) > 0.1 {
		t.Fatalf("unexpected net YoY %%: %.2f", netVariance.YoYPct)
	}
	if len(result.Contribution) != 2 {
		t.Fatalf("expected 2 contribution rows, got %d", len(result.Contribution))
	}
	if result.Contribution[0].Branch != "Branch 1" {
		t.Fatalf("expected branch label 'Branch 1', got %s", result.Contribution[0].Branch)
	}
}
