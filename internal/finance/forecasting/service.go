package forecasting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	fxservice "github.com/odyssey-erp/odyssey-erp/internal/fx"
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
	FxSnapshot   []byte
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
	ScenarioBelongsToCompany(ctx context.Context, scenarioID, companyID int64) (bool, error)
	CreateForecastRun(ctx context.Context, input CreateForecastRunInput) (ForecastRun, error)
	UpdateForecastRunStatus(ctx context.Context, update ForecastRunStatusUpdate) error
	CreateForecastDailyBucket(ctx context.Context, input CreateForecastDailyBucketInput) (ForecastDailyBucket, error)
	CreateForecastSourceLine(ctx context.Context, input CreateForecastSourceLineInput) (int64, error)
	GetLatestForecastRun(ctx context.Context, query ForecastRunQuery) (ForecastRun, error)
	ListForecastDailyBucketsByRun(ctx context.Context, runID int64) ([]ForecastDailyBucket, error)
}

// BaseCurrencyReader is implemented by the persistence adapter so forecasts
// never infer a company's base currency from a mock or from the first flow.
type BaseCurrencyReader interface {
	CompanyBaseCurrency(context.Context, int64) (string, error)
}

// FXResolver is deliberately small so tests can inject a deterministic rate
// repository while production uses fx.Resolver backed by fx_daily_rates.
type FXResolver interface {
	Resolve(context.Context, string, string, time.Time) (fxservice.FXQuote, error)
}

type FXSnapshot struct {
	BaseCurrency string                    `json:"base_currency"`
	RateDate     time.Time                 `json:"rate_date"`
	Rates        map[string]FXSnapshotRate `json:"rates"`
}

type FXSnapshotRate struct {
	Rate   string    `json:"rate"`
	Date   time.Time `json:"date"`
	Source string    `json:"source"`
}

type Service struct {
	repo       Repository
	readers    []SourceReader
	fxResolver FXResolver
	logger     *slog.Logger
	now        func() time.Time
}

func NewService(repo Repository, readers []SourceReader, logger *slog.Logger) *Service {
	return &Service{
		repo:    repo,
		readers: readers,
		logger:  logger,
		now:     time.Now,
	}
}

func NewServiceWithFXResolver(repo Repository, readers []SourceReader, resolver FXResolver, logger *slog.Logger) *Service {
	service := NewService(repo, readers, logger)
	service.fxResolver = resolver
	return service
}

// SetNow makes the forecast clock deterministic for application-level tests.
func (s *Service) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// GenerateSnapshot runs a cash forecast for a 13-week horizon. Opening
// balances are seeded once and every following day rolls forward from the
// prior closing balance.
func (s *Service) GenerateSnapshot(ctx context.Context, companyID int64, scenarioID int64) error {
	if s.repo == nil {
		return errors.New("forecast repository is not configured")
	}
	if companyID <= 0 || scenarioID <= 0 {
		return errors.New("company and scenario are required")
	}
	if s.fxResolver == nil {
		return errors.New("forecast FX resolver is not configured")
	}
	scenarioOwned, err := s.repo.ScenarioBelongsToCompany(ctx, scenarioID, companyID)
	if err != nil {
		return fmt.Errorf("failed to validate forecast scenario: %w", err)
	}
	if !scenarioOwned {
		return errors.New("forecast scenario does not belong to company")
	}

	now := s.now()
	startDate := dateOnlyUTC(now)
	endDate := startDate.AddDate(0, 0, 13*7)

	// Create the run before reading sources so a failure remains visible. The
	// snapshot is replaced with the real rate set when the run completes.
	run, err := s.repo.CreateForecastRun(ctx, CreateForecastRunInput{
		CompanyID:  companyID,
		ScenarioID: scenarioID,
		Status:     "PENDING",
		FxSnapshot: []byte(`{}`),
	})
	if err != nil {
		return fmt.Errorf("failed to create forecast run: %w", err)
	}

	allFlows, err := s.readFlows(ctx, companyID, startDate, endDate)
	if err != nil {
		return s.failRun(ctx, run.ID, err)
	}
	baseCurrency, err := s.companyBaseCurrency(ctx, companyID)
	if err != nil {
		return s.failRun(ctx, run.ID, err)
	}
	fxSnapshot, err := s.buildFXSnapshot(ctx, baseCurrency, startDate, allFlows)
	if err != nil {
		return s.failRun(ctx, run.ID, err)
	}

	currencies, openingBalances, dailyFlows, err := aggregateFlows(startDate, endDate, baseCurrency, allFlows)
	if err != nil {
		return s.failRun(ctx, run.ID, err)
	}

	for _, currency := range currencies {
		opening := openingBalances[currency]
		for date := startDate; date.Before(endDate); date = date.AddDate(0, 0, 1) {
			day := dailyFlows[currency][date.Format(time.DateOnly)]
			if day == nil {
				day = &dailyFlow{inflow: zeroAmount(currency), outflow: zeroAmount(currency)}
			}
			closing := opening.Add(day.inflow).Add(day.outflow)
			bucket, err := s.repo.CreateForecastDailyBucket(ctx, CreateForecastDailyBucketInput{
				RunID:          run.ID,
				Currency:       currency,
				BucketDate:     date,
				OpeningBalance: opening,
				TotalInflow:    day.inflow,
				TotalOutflow:   day.outflow,
				ClosingBalance: closing,
			})
			if err != nil {
				return s.failRun(ctx, run.ID, fmt.Errorf("failed to create daily bucket: %w", err))
			}
			for _, line := range day.lines {
				if _, err := s.repo.CreateForecastSourceLine(ctx, CreateForecastSourceLineInput{
					RunID:         run.ID,
					DailyBucketID: bucket.ID,
					SourceType:    string(line.SourceType),
					SourceRef:     line.SourceRef,
					Amount:        line.Amount,
					Currency:      line.Currency,
					ExpectedDate:  line.Date,
					Certainty:     string(line.Certainty),
				}); err != nil {
					return s.failRun(ctx, run.ID, fmt.Errorf("failed to create source line: %w", err))
				}
			}
			opening = closing
		}
	}

	snapshotBytes, err := json.Marshal(fxSnapshot)
	if err != nil {
		return s.failRun(ctx, run.ID, fmt.Errorf("failed to encode FX snapshot: %w", err))
	}
	return s.repo.UpdateForecastRunStatus(ctx, ForecastRunStatusUpdate{
		ID:          run.ID,
		Status:      "COMPLETED",
		CompletedAt: s.now().UTC(),
		FxSnapshot:  snapshotBytes,
	})
}

func (s *Service) readFlows(ctx context.Context, companyID int64, startDate, endDate time.Time) ([]ExpectedCashFlow, error) {
	var allFlows []ExpectedCashFlow
	var readerErrors []string
	for _, reader := range s.readers {
		if reader == nil {
			continue
		}
		flows, err := reader.ReadExpectedFlows(ctx, companyID, startDate, endDate)
		if err != nil {
			if s.logger != nil {
				s.logger.Error("failed to read expected flows", "reader", reader.Name(), "error", err)
			}
			readerErrors = append(readerErrors, fmt.Sprintf("%s: %s", reader.Name(), err.Error()))
			continue
		}
		allFlows = append(allFlows, flows...)
	}
	if len(readerErrors) > 0 {
		return nil, fmt.Errorf("failed to read all sources: %s", strings.Join(readerErrors, "; "))
	}
	return allFlows, nil
}

func (s *Service) companyBaseCurrency(ctx context.Context, companyID int64) (string, error) {
	for _, reader := range s.readers {
		if provider, ok := reader.(BaseCurrencyReader); ok {
			currency, err := provider.CompanyBaseCurrency(ctx, companyID)
			if err != nil {
				return "", fmt.Errorf("failed to read company base currency: %w", err)
			}
			currency, err = fxservice.Currency(currency)
			if err != nil {
				return "", fmt.Errorf("invalid company base currency: %w", err)
			}
			return currency, nil
		}
	}
	return "", errors.New("forecast base currency reader is not configured")
}

func (s *Service) buildFXSnapshot(ctx context.Context, baseCurrency string, rateDate time.Time, flows []ExpectedCashFlow) (FXSnapshot, error) {
	currencies := map[string]struct{}{baseCurrency: {}}
	for _, flow := range flows {
		currency, err := fxservice.Currency(flow.Currency)
		if err != nil {
			return FXSnapshot{}, fmt.Errorf("invalid flow currency %q: %w", flow.Currency, err)
		}
		currencies[currency] = struct{}{}
	}
	snapshot := FXSnapshot{BaseCurrency: baseCurrency, RateDate: rateDate, Rates: make(map[string]FXSnapshotRate, len(currencies))}
	for currency := range currencies {
		quote, err := s.fxResolver.Resolve(ctx, baseCurrency, currency, rateDate)
		if err != nil {
			return FXSnapshot{}, fmt.Errorf("failed to resolve FX rate %s/%s: %w", baseCurrency, currency, err)
		}
		snapshot.Rates[currency] = FXSnapshotRate{Rate: quote.Rate.String(), Date: quote.RateDate, Source: quote.Source}
	}
	return snapshot, nil
}

type dailyFlow struct {
	inflow  automation.ExactAmount
	outflow automation.ExactAmount
	lines   []ExpectedCashFlow
}

func aggregateFlows(startDate, endDate time.Time, baseCurrency string, flows []ExpectedCashFlow) ([]string, map[string]automation.ExactAmount, map[string]map[string]*dailyFlow, error) {
	currencies := map[string]struct{}{baseCurrency: {}}
	openingBalances := map[string]automation.ExactAmount{baseCurrency: zeroAmount(baseCurrency)}
	dailyFlows := make(map[string]map[string]*dailyFlow)
	for _, flow := range flows {
		currency, err := fxservice.Currency(flow.Currency)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid flow currency %q: %w", flow.Currency, err)
		}
		if flow.Amount.Currency != currency {
			return nil, nil, nil, fmt.Errorf("flow %s has mismatched amount currency", flow.SourceRef)
		}
		date := dateOnlyUTC(flow.Date)
		if flow.SourceType != SourceTypeBankBalance && (date.Before(startDate) || !date.Before(endDate)) {
			return nil, nil, nil, fmt.Errorf("flow %s falls outside forecast horizon", flow.SourceRef)
		}
		currencies[currency] = struct{}{}
		if _, ok := openingBalances[currency]; !ok {
			openingBalances[currency] = zeroAmount(currency)
		}
		if _, ok := dailyFlows[currency]; !ok {
			dailyFlows[currency] = make(map[string]*dailyFlow)
		}
		lineDate := date
		if flow.SourceType == SourceTypeBankBalance {
			openingBalances[currency] = openingBalances[currency].Add(flow.Amount)
			lineDate = startDate
		} else {
			day := dailyFlows[currency][date.Format(time.DateOnly)]
			if day == nil {
				day = &dailyFlow{inflow: zeroAmount(currency), outflow: zeroAmount(currency)}
				dailyFlows[currency][date.Format(time.DateOnly)] = day
			}
			if flow.Amount.IsPositive() {
				day.inflow = day.inflow.Add(flow.Amount)
			} else {
				day.outflow = day.outflow.Add(flow.Amount)
			}
		}
		day := dailyFlows[currency][lineDate.Format(time.DateOnly)]
		if day == nil {
			day = &dailyFlow{inflow: zeroAmount(currency), outflow: zeroAmount(currency)}
			dailyFlows[currency][lineDate.Format(time.DateOnly)] = day
		}
		day.lines = append(day.lines, flow)
	}
	result := make([]string, 0, len(currencies))
	for currency := range currencies {
		result = append(result, currency)
	}
	sort.Strings(result)
	return result, openingBalances, dailyFlows, nil
}

func zeroAmount(currency string) automation.ExactAmount {
	return automation.ExactAmount{Amount: accountingmoney.Must("0", 4), Currency: currency}
}

func dateOnlyUTC(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Service) failRun(ctx context.Context, runID int64, err error) error {
	_ = s.repo.UpdateForecastRunStatus(ctx, ForecastRunStatusUpdate{
		ID:           runID,
		Status:       "INCOMPLETE",
		CompletedAt:  s.now().UTC(),
		ErrorDetails: err.Error(),
	})
	return err
}
