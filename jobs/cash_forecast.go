package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/forecasting"
)

const (
	TypeCashForecastRefresh = "cashforecast:refresh"
)

type CashForecastRefreshPayload struct {
	CompanyID  int64 `json:"company_id"`
	ScenarioID int64 `json:"scenario_id"`
}

type CashForecastProcessor struct {
	service *forecasting.Service
	logger  *slog.Logger
}

func NewCashForecastProcessor(service *forecasting.Service, logger *slog.Logger) *CashForecastProcessor {
	return &CashForecastProcessor{
		service: service,
		logger:  logger,
	}
}

func (p *CashForecastProcessor) ProcessRefreshTask(ctx context.Context, t *asynq.Task) error {
	var payload CashForecastRefreshPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	p.logger.Info("Starting cash forecast refresh", "company_id", payload.CompanyID, "scenario_id", payload.ScenarioID)
	err := p.service.GenerateSnapshot(ctx, payload.CompanyID, payload.ScenarioID)
	if err != nil {
		p.logger.Error("Cash forecast refresh failed", "company_id", payload.CompanyID, "error", err)
		return err
	}

	p.logger.Info("Cash forecast refresh completed successfully", "company_id", payload.CompanyID)
	return nil
}
