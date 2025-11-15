package boardpack

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	"github.com/odyssey-erp/odyssey-erp/internal/variance"
)

type stubRepo struct {
	balances []reports.AccountBalance
	template Template
}

func (s *stubRepo) AggregateAccountBalances(ctx context.Context, companyID, periodID int64) ([]reports.AccountBalance, error) {
	return s.balances, nil
}

func (s *stubRepo) GetTemplate(ctx context.Context, id int64) (Template, error) {
	return s.template, nil
}

type stubVariance struct {
	rows []variance.VarianceRow
}

func (s stubVariance) LoadSnapshotPayload(ctx context.Context, id int64) ([]variance.VarianceRow, error) {
	return s.rows, nil
}

type stubKPI struct {
	summary analytics.KPISummary
}

func (s stubKPI) GetKPISummary(ctx context.Context, filter analytics.KPIFilter) (analytics.KPISummary, error) {
	return s.summary, nil
}

func TestBuilderBuildProducesSections(t *testing.T) {
	repo := &stubRepo{
		balances: []reports.AccountBalance{
			{Code: "4000", Name: "Revenue", Type: "REVENUE", Debit: 0, Credit: 1500},
			{Code: "5000", Name: "Expense", Type: "EXPENSE", Debit: 700, Credit: 0},
			{Code: "1100", Name: "Cash", Type: "ASSET", Opening: 200, Debit: 50, Credit: 20},
		},
		template: Template{
			ID: 1,
			Sections: []TemplateSection{
				{Type: SectionExecSummary, Title: "Executive"},
				{Type: SectionPLSummary, Title: "P&L"},
				{Type: SectionBSSummary, Title: "Balance Sheet"},
				{Type: SectionCashflow, Title: "Cashflow"},
				{Type: SectionTopVariances, Title: "Top Variances", Options: map[string]any{"limit": 2}},
			},
		},
	}
	varianceRows := []variance.VarianceRow{
		{AccountCode: "6000", Variance: 100},
		{AccountCode: "7000", Variance: 50},
		{AccountCode: "8000", Variance: 25},
	}
	builder := NewBuilder(repo, stubVariance{rows: varianceRows}, stubKPI{summary: analytics.KPISummary{Revenue: 1500, CashIn: 900, CashOut: 400}})
	pack := BoardPack{
		ID:           99,
		CompanyID:    1,
		CompanyName:  "PT Maju",
		CompanyCode:  "MAJU",
		PeriodID:     10,
		PeriodName:   "2024-01",
		PeriodStart:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:    time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
		PeriodStatus: "OPEN",
		TemplateID:   repo.template.ID,
		Template:     &repo.template,
		Status:       StatusPending,
		Metadata:     map[string]any{"requested_by": float64(42)},
	}
	snapshotID := int64(77)
	pack.VarianceSnapshotID = &snapshotID

	data, err := builder.Build(context.Background(), pack)
	require.NoError(t, err)
	require.Equal(t, pack.CompanyName, data.Company.Name)
	require.Len(t, data.Sections, len(repo.template.Sections))

	var varianceSection SectionData
	for _, section := range data.Sections {
		if section.Type == SectionTopVariances {
			varianceSection = section
			break
		}
	}
	rows := varianceSection.Payload.([]variance.VarianceRow)
	require.Len(t, rows, 2)
	require.Equal(t, float64(100), rows[0].Variance)

	var exec SectionData
	for _, section := range data.Sections {
		if section.Type == SectionExecSummary {
			exec = section
			break
		}
	}
	require.NotNil(t, exec.Exec)
	require.Equal(t, int64(42), *exec.Exec.RequestedBy)
	require.NotZero(t, data.GeneratedAt)
}

func TestBuilderBuildWithoutVarianceSnapshot(t *testing.T) {
	repo := &stubRepo{
		balances: []reports.AccountBalance{{Code: "4000", Name: "Revenue", Type: "REVENUE", Debit: 0, Credit: 100}},
		template: Template{
			ID:       2,
			Sections: []TemplateSection{{Type: SectionExecSummary, Title: "Executive"}, {Type: SectionPLSummary, Title: "P&L"}},
		},
	}
	builder := NewBuilder(repo, nil, nil)
	pack := BoardPack{
		ID:           101,
		CompanyID:    3,
		CompanyName:  "PT Tanpa Variance",
		CompanyCode:  "VAR0",
		PeriodID:     11,
		PeriodName:   "2024-02",
		PeriodStart:  time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:    time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
		PeriodStatus: "OPEN",
		TemplateID:   repo.template.ID,
		Template:     &repo.template,
		Status:       StatusPending,
		Metadata:     map[string]any{},
	}
	data, err := builder.Build(context.Background(), pack)
	require.NoError(t, err)
	require.Len(t, data.Sections, len(repo.template.Sections))
	require.Nil(t, pack.VarianceSnapshotID)
}
