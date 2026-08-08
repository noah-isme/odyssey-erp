package forecasting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type Repository interface {
	CreateForecastRun(ctx context.Context, arg sqlc.CreateForecastRunParams) (sqlc.ForecastRun, error)
	UpdateForecastRunStatus(ctx context.Context, arg sqlc.UpdateForecastRunStatusParams) error
	CreateForecastDailyBucket(ctx context.Context, arg sqlc.CreateForecastDailyBucketParams) (sqlc.ForecastDailyBucket, error)
	CreateForecastSourceLine(ctx context.Context, arg sqlc.CreateForecastSourceLineParams) (sqlc.ForecastSourceLine, error)
	GetLatestForecastRun(ctx context.Context, arg sqlc.GetLatestForecastRunParams) (sqlc.ForecastRun, error)
	ListForecastDailyBucketsByRun(ctx context.Context, runID int64) ([]sqlc.ForecastDailyBucket, error)
}

type Service struct {
	repo    Repository
	readers []SourceReader
	logger  *slog.Logger
}

func NewService(repo Repository, readers []SourceReader, logger *slog.Logger) *Service {
	return &Service{
		repo:    repo,
		readers: readers,
		logger:  logger,
	}
}

// GenerateSnapshot runs a cash forecast snapshot for a 13-week period starting today.
func (s *Service) GenerateSnapshot(ctx context.Context, companyID int64, scenarioID int64) error {
	run, err := s.repo.CreateForecastRun(ctx, sqlc.CreateForecastRunParams{
		CompanyID:   companyID,
		ScenarioID:  scenarioID,
		Status:      "PENDING",
		FxSnapshot:  []byte(`{}`), // Mock FX snapshot for now
	})
	if err != nil {
		return fmt.Errorf("failed to create forecast run: %w", err)
	}

	startDate := time.Now().Truncate(24 * time.Hour)
	endDate := startDate.Add(13 * 7 * 24 * time.Hour)

	var allFlows []ExpectedCashFlow
	var readerErrors []string

	for _, reader := range s.readers {
		flows, err := reader.ReadExpectedFlows(ctx, companyID, startDate, endDate)
		if err != nil {
			s.logger.Error("failed to read expected flows", "reader", reader.Name(), "error", err)
			readerErrors = append(readerErrors, fmt.Sprintf("%s: %s", reader.Name(), err.Error()))
			continue
		}
		allFlows = append(allFlows, flows...)
	}

	if len(readerErrors) > 0 {
		return s.failRun(ctx, run.ID, fmt.Errorf("failed to read all sources: %v", readerErrors))
	}

	// We'll aggregate flows by Currency and Date
	// Mapping: currency -> date string (YYYY-MM-DD) -> bucketData
	type bucketData struct {
		OpeningBalance automation.ExactAmount
		TotalInflow    automation.ExactAmount
		TotalOutflow   automation.ExactAmount
		ClosingBalance automation.ExactAmount
		Lines          []ExpectedCashFlow
	}

	buckets := make(map[string]map[string]*bucketData)

	for _, flow := range allFlows {
		dateStr := flow.Date.Format(time.DateOnly)
		currMap, exists := buckets[flow.Currency]
		if !exists {
			currMap = make(map[string]*bucketData)
			buckets[flow.Currency] = currMap
		}
		b, exists := currMap[dateStr]
		if !exists {
			b = &bucketData{}
			currMap[dateStr] = b
		}

		if flow.SourceType == SourceTypeBankBalance {
			// In a real scenario, opening balance calculation would need to be rolled forward.
			b.OpeningBalance = b.OpeningBalance.Add(flow.Amount)
		} else {
			if flow.Amount.IsPositive() {
				b.TotalInflow = b.TotalInflow.Add(flow.Amount)
			} else {
				b.TotalOutflow = b.TotalOutflow.Add(flow.Amount)
			}
		}
		b.Lines = append(b.Lines, flow)
	}

	// Persist the daily buckets and source lines
	for curr, dateMap := range buckets {
		for dateStr, b := range dateMap {
			date, _ := time.Parse(time.DateOnly, dateStr)
			
			bucket, err := s.repo.CreateForecastDailyBucket(ctx, sqlc.CreateForecastDailyBucketParams{
				RunID:          run.ID,
				Currency:       curr,
				BucketDate:     pgtype.Date{Time: date, Valid: true},
				OpeningBalance: b.OpeningBalance.Numeric(),
				TotalInflow:    b.TotalInflow.Numeric(),
				TotalOutflow:   b.TotalOutflow.Numeric(),
				ClosingBalance: b.OpeningBalance.Add(b.TotalInflow).Add(b.TotalOutflow).Numeric(),
			})
			if err != nil {
				return s.failRun(ctx, run.ID, fmt.Errorf("failed to create daily bucket: %w", err))
			}

			for _, line := range b.Lines {
				_, err = s.repo.CreateForecastSourceLine(ctx, sqlc.CreateForecastSourceLineParams{
					RunID:         run.ID,
					DailyBucketID: bucket.ID,
					SourceType:    string(line.SourceType),
					SourceRef:     line.SourceRef,
					Amount:        line.Amount.Numeric(),
					Currency:      line.Currency,
					ExpectedDate:  pgtype.Date{Time: line.Date, Valid: true},
					Certainty:     string(line.Certainty),
				})
				if err != nil {
					return s.failRun(ctx, run.ID, fmt.Errorf("failed to create source line: %w", err))
				}
			}
		}
	}

	return s.repo.UpdateForecastRunStatus(ctx, sqlc.UpdateForecastRunStatusParams{
		ID:          run.ID,
		Status:      "COMPLETED",
		CompletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
}

func (s *Service) failRun(ctx context.Context, runID int64, err error) error {
	_ = s.repo.UpdateForecastRunStatus(ctx, sqlc.UpdateForecastRunStatusParams{
		ID:           runID,
		Status:       "INCOMPLETE", // Mark as incomplete if there's a missing/stale input or error
		CompletedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ErrorDetails: pgtype.Text{String: err.Error(), Valid: true},
	})
	return err
}
