package analytics

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	sqlc "github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type mockRepo struct {
	kpiRow    sqlc.KpiSummaryRow
	kpiErr    error
	kpiCalls  int
	plRows    []sqlc.MonthlyPLRow
	plCalls   int
	cashRows  []sqlc.MonthlyCashflowRow
	cashCalls int
	arRows    []sqlc.AgingARRow
	arCalls   int
	arParams  sqlc.AgingARParams
	apRows    []sqlc.AgingAPRow
	apCalls   int
}

func (m *mockRepo) KpiSummary(ctx context.Context, arg sqlc.KpiSummaryParams) (sqlc.KpiSummaryRow, error) {
	m.kpiCalls++
	return m.kpiRow, m.kpiErr
}

func (m *mockRepo) MonthlyPL(ctx context.Context, arg sqlc.MonthlyPLParams) ([]sqlc.MonthlyPLRow, error) {
	m.plCalls++
	return m.plRows, nil
}

func (m *mockRepo) MonthlyCashflow(ctx context.Context, arg sqlc.MonthlyCashflowParams) ([]sqlc.MonthlyCashflowRow, error) {
	m.cashCalls++
	return m.cashRows, nil
}

func (m *mockRepo) AgingAR(ctx context.Context, arg sqlc.AgingARParams) ([]sqlc.AgingARRow, error) {
	m.arCalls++
	m.arParams = arg
	return m.arRows, nil
}

func (m *mockRepo) AgingAP(ctx context.Context, arg sqlc.AgingAPParams) ([]sqlc.AgingAPRow, error) {
	m.apCalls++
	return m.apRows, nil
}

func newTestService(t *testing.T, repo Repository) (*Service, func()) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := NewCache(client, time.Minute)
	svc := NewService(repo, cache)
	return svc, func() {
		_ = client.Close()
		mr.Close()
	}
}

func TestGetKPISummaryCaches(t *testing.T) {
	repo := &mockRepo{
		kpiRow: sqlc.KpiSummaryRow{
			NetProfit:     1200.5,
			Revenue:       4200.0,
			Opex:          1800.0,
			Cogs:          800.0,
			CashIn:        3500.0,
			CashOut:       900.0,
			ArOutstanding: 2200.0,
			ApOutstanding: 1400.0,
		},
	}
	svc, cleanup := newTestService(t, repo)
	defer cleanup()

	ctx := context.Background()
	filter := KPIFilter{Period: "2025-01", CompanyID: 7, AsOf: time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)}
	summary, err := svc.GetKPISummary(ctx, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.NetProfit != 1200.5 {
		t.Fatalf("expected net profit 1200.5 got %.2f", summary.NetProfit)
	}
	if repo.kpiCalls != 1 {
		t.Fatalf("expected 1 repo call, got %d", repo.kpiCalls)
	}

	// Second call should hit cache.
	summary, err = svc.GetKPISummary(ctx, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.kpiCalls != 1 {
		t.Fatalf("expected cached result, repo called %d times", repo.kpiCalls)
	}

	// Bumping the cache should trigger reload.
	if err := svc.cache.Bump(ctx); err != nil {
		t.Fatalf("bump failed: %v", err)
	}
	repo.kpiRow.NetProfit = 1500
	summary, err = svc.GetKPISummary(ctx, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.NetProfit != 1500 {
		t.Fatalf("expected refreshed value 1500 got %.2f", summary.NetProfit)
	}
	if repo.kpiCalls != 2 {
		t.Fatalf("expected repo to refresh, calls %d", repo.kpiCalls)
	}
}

func TestGetPLTrendAndCashflow(t *testing.T) {
	repo := &mockRepo{
		plRows: []sqlc.MonthlyPLRow{
			{Period: "2025-01", Revenue: 1000, Cogs: 400, Opex: 300, Net: 300},
			{Period: "2025-02", Revenue: 1100, Cogs: 420, Opex: 320, Net: 360},
		},
		cashRows: []sqlc.MonthlyCashflowRow{
			{Period: "2025-01", CashIn: 900, CashOut: 500},
			{Period: "2025-02", CashIn: 950, CashOut: 480},
		},
	}
	svc, cleanup := newTestService(t, repo)
	defer cleanup()

	ctx := context.Background()
	filter := TrendFilter{From: "2025-01", To: "2025-12", CompanyID: 1}
	points, err := svc.GetPLTrend(ctx, filter)
	if err != nil {
		t.Fatalf("trend error: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points got %d", len(points))
	}
	if repo.plCalls != 1 {
		t.Fatalf("expected 1 repo call, got %d", repo.plCalls)
	}

	_, err = svc.GetPLTrend(ctx, filter)
	if err != nil {
		t.Fatalf("trend cache error: %v", err)
	}
	if repo.plCalls != 1 {
		t.Fatalf("expected cached PL trend, repo calls %d", repo.plCalls)
	}

	cashPoints, err := svc.GetCashflowTrend(ctx, filter)
	if err != nil {
		t.Fatalf("cashflow error: %v", err)
	}
	if len(cashPoints) != 2 {
		t.Fatalf("expected 2 cash points got %d", len(cashPoints))
	}
	if repo.cashCalls != 1 {
		t.Fatalf("expected 1 cash repo call got %d", repo.cashCalls)
	}
}

func TestGetARAgingDefaultsAsOf(t *testing.T) {
	repo := &mockRepo{
		arRows: []sqlc.AgingARRow{
			{Bucket: "0-30", Amount: 1000},
		},
	}
	svc, cleanup := newTestService(t, repo)
	defer cleanup()

	ctx := context.Background()
	filter := AgingFilter{CompanyID: 3}
	buckets, err := svc.GetARAging(ctx, filter)
	if err != nil {
		t.Fatalf("aging error: %v", err)
	}
	if len(buckets) != 1 || buckets[0].Amount != 1000 {
		t.Fatalf("unexpected aging buckets %#v", buckets)
	}
	if repo.arCalls != 1 {
		t.Fatalf("expected repo call once, got %d", repo.arCalls)
	}
	if repo.arParams.AsOf == nil {
		t.Fatalf("expected as_of to be populated")
	}
}
