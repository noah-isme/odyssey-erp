package forecasting

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type forecastRepoFake struct {
	run      sqlc.ForecastRun
	buckets  []sqlc.CreateForecastDailyBucketParams
	lines    []sqlc.CreateForecastSourceLineParams
	statuses []sqlc.UpdateForecastRunStatusParams
	bucketID int64
	lineID   int64
}

func (r *forecastRepoFake) CreateForecastRun(_ context.Context, arg sqlc.CreateForecastRunParams) (sqlc.ForecastRun, error) {
	r.run = sqlc.ForecastRun{ID: 41, CompanyID: arg.CompanyID, ScenarioID: arg.ScenarioID, Status: arg.Status}
	return r.run, nil
}
func (r *forecastRepoFake) UpdateForecastRunStatus(_ context.Context, arg sqlc.UpdateForecastRunStatusParams) error {
	r.statuses = append(r.statuses, arg)
	return nil
}
func (r *forecastRepoFake) CreateForecastDailyBucket(_ context.Context, arg sqlc.CreateForecastDailyBucketParams) (sqlc.ForecastDailyBucket, error) {
	r.bucketID++
	r.buckets = append(r.buckets, arg)
	return sqlc.ForecastDailyBucket{ID: r.bucketID, RunID: arg.RunID, Currency: arg.Currency, BucketDate: arg.BucketDate}, nil
}
func (r *forecastRepoFake) CreateForecastSourceLine(_ context.Context, arg sqlc.CreateForecastSourceLineParams) (sqlc.ForecastSourceLine, error) {
	r.lineID++
	r.lines = append(r.lines, arg)
	return sqlc.ForecastSourceLine{ID: r.lineID, RunID: arg.RunID, DailyBucketID: arg.DailyBucketID}, nil
}
func (r *forecastRepoFake) GetLatestForecastRun(context.Context, sqlc.GetLatestForecastRunParams) (sqlc.ForecastRun, error) {
	return r.run, nil
}
func (r *forecastRepoFake) ListForecastDailyBucketsByRun(context.Context, int64) ([]sqlc.ForecastDailyBucket, error) {
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

func TestGenerateSnapshotAggregatesFlowsAndPersistsSourceLines(t *testing.T) {
	date := time.Now().Truncate(24 * time.Hour)
	reader := forecastReaderFake{
		name: "bank",
		flows: []ExpectedCashFlow{
			{SourceType: SourceTypeBankBalance, SourceRef: "bank-1", Amount: automation.MustParseExact("1000"), Currency: "USD", Date: date, Certainty: CertaintyCommitted},
			{SourceType: SourceTypeOpenAR, SourceRef: "ar-1", Amount: automation.MustParseExact("250"), Currency: "USD", Date: date, Certainty: CertaintyProbable},
			{SourceType: SourceTypePostedAP, SourceRef: "ap-1", Amount: automation.MustParseExact("-75"), Currency: "USD", Date: date, Certainty: CertaintyCommitted},
		},
	}
	repo := &forecastRepoFake{}
	service := NewService(repo, []SourceReader{reader}, slog.Default())

	if err := service.GenerateSnapshot(context.Background(), 7, 3); err != nil {
		t.Fatal(err)
	}
	if len(repo.buckets) != 1 || len(repo.lines) != 3 {
		t.Fatalf("persisted %d buckets and %d lines, want 1 and 3", len(repo.buckets), len(repo.lines))
	}
	bucket := repo.buckets[0]
	if bucket.RunID != 41 || bucket.Currency != "USD" || !bucket.BucketDate.Valid {
		t.Fatalf("bucket params = %#v", bucket)
	}
	if len(repo.statuses) != 1 || repo.statuses[0].Status != "COMPLETED" {
		t.Fatalf("statuses = %#v", repo.statuses)
	}
}

func TestGenerateSnapshotMarksRunIncompleteWhenReaderFails(t *testing.T) {
	repo := &forecastRepoFake{}
	service := NewService(repo, []SourceReader{forecastReaderFake{name: "ledger", err: errors.New("source unavailable")}}, slog.Default())

	err := service.GenerateSnapshot(context.Background(), 7, 0)
	if err == nil || repo.run.ID != 41 {
		t.Fatalf("GenerateSnapshot() error = %v, run = %#v", err, repo.run)
	}
	if len(repo.statuses) != 1 || repo.statuses[0].Status != "INCOMPLETE" || !repo.statuses[0].ErrorDetails.Valid {
		t.Fatalf("failure status = %#v", repo.statuses)
	}
}
