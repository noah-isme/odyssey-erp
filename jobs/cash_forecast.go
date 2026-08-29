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
	TypeCashForecastRefresh     = "cashforecast:refresh"
	TypeCashForecastRefreshScan = "cashforecast:refresh_scan"
)

type CashForecastRefreshPayload struct {
	CompanyID  int64 `json:"company_id"`
	ScenarioID int64 `json:"scenario_id"`
}

// NewCashForecastRefreshTask creates one company/scenario refresh task. The
// explicit pair is the scope boundary; a wildcard refresh is never accepted.
func NewCashForecastRefreshTask(companyID, scenarioID int64) (*asynq.Task, error) {
	if companyID <= 0 || scenarioID <= 0 {
		return nil, fmt.Errorf("cash forecast: company and scenario IDs are required")
	}
	body, err := json.Marshal(CashForecastRefreshPayload{CompanyID: companyID, ScenarioID: scenarioID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeCashForecastRefresh, body, asynq.Queue(QueueDefault)), nil
}

// NewCashForecastRefreshScanTask creates the nightly scheduler task. The scan
// enumerates enabled companies and their owned scenarios before enqueueing the
// scoped refresh tasks.
func NewCashForecastRefreshScanTask() *asynq.Task {
	return asynq.NewTask(TypeCashForecastRefreshScan, nil, asynq.Queue(QueueDefault))
}

type CashForecastProcessor struct {
	service  *forecasting.Service
	logger   *slog.Logger
	settings forecasting.SettingsReader
}

// NewCashForecastProcessor constructs the scoped refresh consumer. A settings
// reader is required because a queued task may outlive the feature flag state
// observed by the nightly scan; the worker must re-check the company flag
// immediately before generating a snapshot.
func NewCashForecastProcessor(service *forecasting.Service, logger *slog.Logger, settings ...forecasting.SettingsReader) *CashForecastProcessor {
	var featureSettings forecasting.SettingsReader
	if len(settings) > 0 {
		featureSettings = settings[0]
	}
	return &CashForecastProcessor{
		service:  service,
		logger:   logger,
		settings: featureSettings,
	}
}

func (p *CashForecastProcessor) ProcessRefreshTask(ctx context.Context, t *asynq.Task) error {
	if p == nil || p.service == nil {
		return fmt.Errorf("cash forecast: service not configured: %w", asynq.SkipRetry)
	}
	if t == nil {
		return fmt.Errorf("cash forecast: task not configured: %w", asynq.SkipRetry)
	}
	var payload CashForecastRefreshPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}
	if payload.CompanyID <= 0 || payload.ScenarioID <= 0 {
		return fmt.Errorf("cash forecast: company and scenario IDs are required: %w", asynq.SkipRetry)
	}
	if p.settings == nil {
		return fmt.Errorf("cash forecast: settings reader not configured: %w", asynq.SkipRetry)
	}
	settings, err := p.settings.Settings(ctx, payload.CompanyID)
	if err != nil {
		// Settings lookup failures may be transient; let Asynq retry without
		// invoking the forecast service against an unknown company state.
		return fmt.Errorf("cash forecast: company %d settings: %w", payload.CompanyID, err)
	}
	if settings.CompanyID != payload.CompanyID {
		return fmt.Errorf("cash forecast: company %d settings company mismatch: %w", payload.CompanyID, asynq.SkipRetry)
	}
	if !settings.CashForecastEnabled {
		// A task can remain queued after an operator disables forecasting. Drop
		// that stale task instead of creating a run under the disabled flag.
		return fmt.Errorf("cash forecast: company %d is disabled: %w", payload.CompanyID, asynq.SkipRetry)
	}

	if p.logger != nil {
		p.logger.Info("Starting cash forecast refresh", "company_id", payload.CompanyID, "scenario_id", payload.ScenarioID)
	}
	err = p.service.GenerateSnapshot(ctx, payload.CompanyID, payload.ScenarioID)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("Cash forecast refresh failed", "company_id", payload.CompanyID, "error", err)
		}
		return err
	}

	if p.logger != nil {
		p.logger.Info("Cash forecast refresh completed successfully", "company_id", payload.CompanyID)
	}
	return nil
}
