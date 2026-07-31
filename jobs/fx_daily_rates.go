package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	fxdomain "github.com/odyssey-erp/odyssey-erp/internal/fx"
)

const TaskFXDailyRates = "fx:daily-rates"

type FXDailyRatesFetcher interface {
	FetchDailyRates(context.Context, string, time.Time, bool) error
}
type FXCompanyCurrencies interface {
	CompanyBaseCurrencies(context.Context) ([]string, error)
}
type FXDailyRatesPayload struct {
	Date  string `json:"date,omitempty"`
	Force bool   `json:"force,omitempty"`
}

func NewFXDailyRatesTask(date time.Time, force bool) (*asynq.Task, error) {
	var dateValue string
	if !date.IsZero() {
		dateValue = date.Format("2006-01-02")
	}
	body, err := json.Marshal(FXDailyRatesPayload{Date: dateValue, Force: force})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskFXDailyRates, body, asynq.Queue(QueueDefault)), nil
}

func HandleFXDailyRatesTask(fetcher FXDailyRatesFetcher, companies FXCompanyCurrencies, location *time.Location, logger *slog.Logger) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		if fetcher == nil || companies == nil {
			return fmt.Errorf("fx daily rates: dependencies not configured: %w", asynq.SkipRetry)
		}
		var payload FXDailyRatesPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("fx daily rates: invalid payload: %w", asynq.SkipRetry)
		}
		if location == nil {
			location, _ = time.LoadLocation("Asia/Jakarta")
		}
		date := time.Now().In(location)
		if payload.Date != "" {
			parsed, err := time.ParseInLocation("2006-01-02", payload.Date, location)
			if err != nil {
				return fmt.Errorf("fx daily rates: invalid date: %w", asynq.SkipRetry)
			}
			date = parsed
		}
		currencies, err := companies.CompanyBaseCurrencies(ctx)
		if err != nil {
			return err
		}
		for _, currency := range currencies {
			if err := fetcher.FetchDailyRates(ctx, currency, date, payload.Force); err != nil {
				if logger != nil && errors.Is(err, fxdomain.ErrRateStale) {
					logger.Warn("FX rate is stale", "currency", currency, "date", date.Format("2006-01-02"), "error", err)
				}
				return err
			}
		}
		return nil
	}
}
