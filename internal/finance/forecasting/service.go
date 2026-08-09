package forecasting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

type ForecastRun struct {
	ID           int64     `json:"id"`
	CompanyID    int64     `json:"company_id"`
	ScenarioID   int64     `json:"scenario_id"`
	Status       string    `json:"status"`
	FxSnapshot   []byte    `json:"fx_snapshot"`
	CompletedAt  time.Time `json:"completed_at"`
	ErrorDetails string    `json:"error_details"`
}

type ForecastDailyBucket struct {
	ID             int64     `json:"id"`
	RunID          int64     `json:"run_id"`
	Currency       string    `json:"currency"`
	BucketDate     time.Time `json:"bucket_date"`
	OpeningBalance float64   `json:"opening_balance"`
	TotalInflow    float64   `json:"total_inflow"`
	TotalOutflow   float64   `json:"total_outflow"`
	ClosingBalance float64   `json:"closing_balance"`
}

type CreateForecastRunInput struct {
	CompanyID  int64
	ScenarioID int64
	Status     string
	FxSnapshot []byte
}

type ForecastRunStatusUpdate struct {
	ID           int64
	Status       string
	CompletedAt  time.Time
	ErrorDetails string
}

type CreateForecastDailyBucketInput struct {
	RunID          int64
	Currency       string
	BucketDate     time.Time
	OpeningBalance automation.ExactAmount
	TotalInflow    automation.ExactAmount
	TotalOutflow   automation.ExactAmount
	ClosingBalance automation.ExactAmount
}

type CreateForecastSourceLineInput struct {
	RunID         int64
	DailyBucketID int64
	SourceType    string
	SourceRef     string
	Amount        automation.ExactAmount
	Currency      string
	ExpectedDate  time.Time
	Certainty     string
}

type ForecastRunQuery struct {
	CompanyID  int64
	ScenarioID int64
}

type Repository interface {
	CreateForecastRun(ctx context.Context, input CreateForecastRunInput) (ForecastRun, error)
	UpdateForecastRunStatus(ctx context.Context, update ForecastRunStatusUpdate) error
	CreateForecastDailyBucket(ctx context.Context, input CreateForecastDailyBucketInput) (ForecastDailyBucket, error)
	CreateForecastSourceLine(ctx context.Context, input CreateForecastSourceLineInput) (int64, error)
	GetLatestForecastRun(ctx context.Context, query ForecastRunQuery) (ForecastRun, error)
	ListForecastDailyBucketsByRun(ctx context.Context, runID int64) ([]ForecastDailyBucket, error)
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
	run, err := s.repo.CreateForecastRun(ctx, CreateForecastRunInput{
		CompanyID:  companyID,
		ScenarioID: scenarioID,
		Status:     "PENDING",
		FxSnapshot: []byte(`{}`), // Mock FX snapshot for now
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

			bucket, err := s.repo.CreateForecastDailyBucket(ctx, CreateForecastDailyBucketInput{
				RunID:          run.ID,
				Currency:       curr,
				BucketDate:     date,
				OpeningBalance: b.OpeningBalance,
				TotalInflow:    b.TotalInflow,
				TotalOutflow:   b.TotalOutflow,
				ClosingBalance: b.OpeningBalance.Add(b.TotalInflow).Add(b.TotalOutflow),
			})
			if err != nil {
				return s.failRun(ctx, run.ID, fmt.Errorf("failed to create daily bucket: %w", err))
			}

			for _, line := range b.Lines {
				_, err = s.repo.CreateForecastSourceLine(ctx, CreateForecastSourceLineInput{
					RunID:         run.ID,
					DailyBucketID: bucket.ID,
					SourceType:    string(line.SourceType),
					SourceRef:     line.SourceRef,
					Amount:        line.Amount,
					Currency:      line.Currency,
					ExpectedDate:  line.Date,
					Certainty:     string(line.Certainty),
				})
				if err != nil {
					return s.failRun(ctx, run.ID, fmt.Errorf("failed to create source line: %w", err))
				}
			}
		}
	}

	return s.repo.UpdateForecastRunStatus(ctx, ForecastRunStatusUpdate{
		ID:          run.ID,
		Status:      "COMPLETED",
		CompletedAt: time.Now(),
	})
}

func (s *Service) failRun(ctx context.Context, runID int64, err error) error {
	_ = s.repo.UpdateForecastRunStatus(ctx, ForecastRunStatusUpdate{
		ID:           runID,
		Status:       "INCOMPLETE", // Mark as incomplete if there's a missing/stale input or error
		CompletedAt:  time.Now(),
		ErrorDetails: err.Error(),
	})
	return err
}
