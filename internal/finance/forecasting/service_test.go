package forecasting

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	fxservice "github.com/odyssey-erp/odyssey-erp/internal/fx"
)

type forecastRepoFake struct {
	run      ForecastRun
	buckets  []CreateForecastDailyBucketInput
	lines    []CreateForecastSourceLineInput
	statuses []ForecastRunStatusUpdate
	bucketID int64
	lineID   int64
}

func (r *forecastRepoFake) ScenarioBelongsToCompany(_ context.Context, scenarioID, companyID int64) (bool, error) {
	return scenarioID == 3 && companyID == 7, nil
}

func (r *forecastRepoFake) CreateForecastRun(_ context.Context, arg CreateForecastRunInput) (ForecastRun, error) {
	r.run = ForecastRun{ID: 41, CompanyID: arg.CompanyID, ScenarioID: arg.ScenarioID, Status: arg.Status}
	return r.run, nil
}
func (r *forecastRepoFake) UpdateForecastRunStatus(_ context.Context, arg ForecastRunStatusUpdate) error {
	r.statuses = append(r.statuses, arg)
	return nil
}
func (r *forecastRepoFake) CreateForecastDailyBucket(_ context.Context, arg CreateForecastDailyBucketInput) (ForecastDailyBucket, error) {
	r.bucketID++
	r.buckets = append(r.buckets, arg)
	return ForecastDailyBucket{ID: r.bucketID, RunID: arg.RunID, Currency: arg.Currency, BucketDate: arg.BucketDate}, nil
}
func (r *forecastRepoFake) CreateForecastSourceLine(_ context.Context, arg CreateForecastSourceLineInput) (int64, error) {
	r.lineID++
	r.lines = append(r.lines, arg)
	return r.lineID, nil
}
func (r *forecastRepoFake) GetLatestForecastRun(context.Context, ForecastRunQuery) (ForecastRun, error) {
	return r.run, nil
}
func (r *forecastRepoFake) ListForecastDailyBucketsByRun(context.Context, int64) ([]ForecastDailyBucket, error) {
	return nil, nil
}

type forecastReaderFake struct {
	name  string
	flows []ExpectedCashFlow
	err   error
}

func (r forecastReaderFake) Name() string { return r.name }
func (r forecastReaderFake) ReadExpectedFlows(context.Context, int64, time.Time, time.Time) ([]ExpectedCashFlow, error) {
	return r.flows, r.err
}
func (r forecastReaderFake) CompanyBaseCurrency(context.Context, int64) (string, error) {
	return "USD", nil
}

type fxResolverFake struct{}

func (fxResolverFake) Resolve(_ context.Context, base, quote string, date time.Time) (fxservice.FXQuote, error) {
	return fxservice.FXQuote{
		BaseCurrency:  base,
		QuoteCurrency: quote,
		Rate:          fxservice.MustDecimal("1"),
		RateDate:      date,
		Source:        "TEST",
	}, nil
}

func TestGenerateSnapshotAggregatesFlowsAndPersistsSourceLines(t *testing.T) {
	date := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	reader := forecastReaderFake{
		name: "bank",
		flows: []ExpectedCashFlow{
			{SourceType: SourceTypeBankBalance, SourceRef: "bank-1", Amount: automation.MustParseExact("1000"), Currency: "USD", Date: date, Certainty: CertaintyCommitted},
			{SourceType: SourceTypeOpenAR, SourceRef: "ar-1", Amount: automation.MustParseExact("250"), Currency: "USD", Date: date, Certainty: CertaintyProbable},
			{SourceType: SourceTypePostedAP, SourceRef: "ap-1", Amount: automation.MustParseExact("-75"), Currency: "USD", Date: date, Certainty: CertaintyCommitted},
		},
	}
	repo := &forecastRepoFake{}
	service := NewServiceWithFXResolver(repo, []SourceReader{reader}, fxResolverFake{}, slog.Default())
	service.SetNow(func() time.Time { return date })

	if err := service.GenerateSnapshot(context.Background(), 7, 3); err != nil {
		t.Fatal(err)
	}
	if len(repo.buckets) != 13*7 || len(repo.lines) != 3 {
		t.Fatalf("persisted %d buckets and %d lines, want 91 and 3", len(repo.buckets), len(repo.lines))
	}
	bucket := repo.buckets[0]
	if bucket.RunID != 41 || bucket.Currency != "USD" || bucket.BucketDate.IsZero() {
		t.Fatalf("bucket params = %#v", bucket)
	}
	if bucket.OpeningBalance.Amount.String() != "1000.0000" || bucket.TotalInflow.Amount.String() != "250.0000" || bucket.TotalOutflow.Amount.String() != "-75.0000" || bucket.ClosingBalance.Amount.String() != "1175.0000" {
		t.Fatalf("first bucket roll-forward = %#v", bucket)
	}
	if repo.buckets[1].OpeningBalance.Amount.String() != "1175.0000" {
		t.Fatalf("second bucket opening balance = %s, want 1175", repo.buckets[1].OpeningBalance.Amount.String())
	}
	if len(repo.statuses) != 1 || repo.statuses[0].Status != "COMPLETED" {
		t.Fatalf("statuses = %#v", repo.statuses)
	}
}

func TestGenerateSnapshotMarksRunIncompleteWhenReaderFails(t *testing.T) {
	repo := &forecastRepoFake{}
	service := NewServiceWithFXResolver(repo, []SourceReader{forecastReaderFake{name: "ledger", err: errors.New("source unavailable")}}, fxResolverFake{}, slog.Default())

	err := service.GenerateSnapshot(context.Background(), 7, 3)
	if err == nil || repo.run.ID != 41 {
		t.Fatalf("GenerateSnapshot() error = %v, run = %#v", err, repo.run)
	}
	if len(repo.statuses) != 1 || repo.statuses[0].Status != "INCOMPLETE" || repo.statuses[0].ErrorDetails == "" {
		t.Fatalf("failure status = %#v", repo.statuses)
	}
}

func TestGenerateSnapshotRejectsForeignScenarioBeforeCreatingRun(t *testing.T) {
	repo := &forecastRepoFake{}
	service := NewServiceWithFXResolver(repo, nil, fxResolverFake{}, nil)
	service.SetNow(func() time.Time { return time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC) })

	if err := service.GenerateSnapshot(context.Background(), 8, 3); err == nil {
		t.Fatal("expected foreign scenario to be rejected")
	}
	if repo.run.ID != 0 {
		t.Fatalf("foreign scenario created run: %+v", repo.run)
	}
}
