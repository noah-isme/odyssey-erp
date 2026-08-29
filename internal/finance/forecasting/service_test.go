package forecasting

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	fxservice "github.com/odyssey-erp/odyssey-erp/internal/fx"
)

type forecastRepoFake struct {
	run               ForecastRun
	buckets           []CreateForecastDailyBucketInput
	latestBuckets     []ForecastDailyBucket
	latestBucketCalls int
	lines             []CreateForecastSourceLineInput
	sourceLines       []ForecastSourceLine
	statuses          []ForecastRunStatusUpdate
	bucketID          int64
	lineID            int64
	latestErr         error
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
	return r.run, r.latestErr
}
func (r *forecastRepoFake) ListForecastDailyBucketsByRun(context.Context, int64) ([]ForecastDailyBucket, error) {
	r.latestBucketCalls++
	return r.latestBuckets, nil
}
func (r *forecastRepoFake) ListForecastSourceLinesByRun(context.Context, int64, int64) ([]ForecastSourceLine, error) {
	return r.sourceLines, nil
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

type scenarioForecastReaderFake struct {
	flows        []ExpectedCashFlow
	normalCalled bool
	scenarioID   int64
}

func (r *scenarioForecastReaderFake) Name() string { return "scenario-adjustments" }
func (r *scenarioForecastReaderFake) ReadExpectedFlows(context.Context, int64, time.Time, time.Time) ([]ExpectedCashFlow, error) {
	r.normalCalled = true
	return nil, nil
}
func (r *scenarioForecastReaderFake) ReadExpectedFlowsForScenario(_ context.Context, _, scenarioID int64, _, _ time.Time) ([]ExpectedCashFlow, error) {
	r.scenarioID = scenarioID
	return r.flows, nil
}
func (r *scenarioForecastReaderFake) CompanyBaseCurrency(context.Context, int64) (string, error) {
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

func TestGenerateSnapshotMarksRunIncompleteWhenSourceReaderIsNil(t *testing.T) {
	repo := &forecastRepoFake{}
	service := NewServiceWithFXResolver(repo, []SourceReader{nil, forecastReaderFake{name: "ledger"}}, fxResolverFake{}, slog.Default())

	err := service.GenerateSnapshot(context.Background(), 7, 3)
	if err == nil || repo.run.ID != 41 {
		t.Fatalf("GenerateSnapshot() error = %v, run = %#v", err, repo.run)
	}
	if !strings.Contains(err.Error(), "source reader is not configured") {
		t.Fatalf("GenerateSnapshot() error = %v, want nil-reader detail", err)
	}
	if len(repo.statuses) != 1 || repo.statuses[0].Status != "INCOMPLETE" {
		t.Fatalf("failure status = %#v", repo.statuses)
	}
}

func TestGenerateSnapshotUsesScenarioAwareReaderForScenarioOwnedFlows(t *testing.T) {
	date := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	reader := &scenarioForecastReaderFake{flows: []ExpectedCashFlow{
		{
			SourceType: SourceTypeManualAdjustment,
			SourceRef:  "forecast-adjustment:17",
			Amount:     automation.MustParseExact("-125.5000"),
			Currency:   "usd",
			Date:       date,
			Certainty:  CertaintyProbable,
		},
	}}
	repo := &forecastRepoFake{}
	service := NewServiceWithFXResolver(repo, []SourceReader{reader}, fxResolverFake{}, nil)
	service.SetNow(func() time.Time { return date })

	if err := service.GenerateSnapshot(context.Background(), 7, 3); err != nil {
		t.Fatal(err)
	}
	if reader.normalCalled {
		t.Fatal("scenario-aware reader fell back to unscoped read")
	}
	if reader.scenarioID != 3 {
		t.Fatalf("scenario ID = %d, want 3", reader.scenarioID)
	}
	if len(repo.lines) != 1 {
		t.Fatalf("persisted %d source lines, want 1", len(repo.lines))
	}
	line := repo.lines[0]
	if line.SourceType != string(SourceTypeManualAdjustment) || line.SourceRef != "forecast-adjustment:17" || line.Certainty != string(CertaintyProbable) || line.Currency != "USD" {
		t.Fatalf("normalized source line = %#v", line)
	}
}

func TestGenerateSnapshotRejectsInvalidOrDuplicateSourceIdentity(t *testing.T) {
	date := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		flows []ExpectedCashFlow
		want  string
	}{
		{
			name: "missing source reference",
			flows: []ExpectedCashFlow{{
				SourceType: SourceTypeOpenAR,
				Amount:     automation.MustParseExact("10"),
				Currency:   "USD",
				Date:       date,
				Certainty:  CertaintyCommitted,
			}},
			want: "source reference is required",
		},
		{
			name: "unknown certainty",
			flows: []ExpectedCashFlow{{
				SourceType: SourceTypeOpenAR,
				SourceRef:  "ar:1",
				Amount:     automation.MustParseExact("10"),
				Currency:   "USD",
				Date:       date,
				Certainty:  Certainty("POSSIBLE"),
			}},
			want: "certainty must be COMMITTED or PROBABLE",
		},
		{
			name: "duplicate stable source",
			flows: []ExpectedCashFlow{
				{SourceType: SourceTypeOpenAR, SourceRef: "ar:1", Amount: automation.MustParseExact("10"), Currency: "USD", Date: date, Certainty: CertaintyCommitted},
				{SourceType: SourceTypeOpenAR, SourceRef: "ar:1", Amount: automation.MustParseExact("12"), Currency: "USD", Date: date.AddDate(0, 0, 1), Certainty: CertaintyCommitted},
			},
			want: "duplicate source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &forecastRepoFake{}
			reader := forecastReaderFake{name: "test", flows: tt.flows}
			service := NewServiceWithFXResolver(repo, []SourceReader{reader}, fxResolverFake{}, nil)
			service.SetNow(func() time.Time { return date })

			err := service.GenerateSnapshot(context.Background(), 7, 3)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("GenerateSnapshot() error = %v, want %q", err, tt.want)
			}
			if len(repo.statuses) != 1 || repo.statuses[0].Status != "INCOMPLETE" {
				t.Fatalf("run status = %#v, want INCOMPLETE", repo.statuses)
			}
		})
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
