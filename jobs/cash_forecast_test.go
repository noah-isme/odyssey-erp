package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/forecasting"
)

type cashForecastSettingsReaderFake struct {
	settings automation.Settings
	err      error
}

func (f cashForecastSettingsReaderFake) Settings(context.Context, int64) (automation.Settings, error) {
	return f.settings, f.err
}

func TestCashForecastProcessorRejectsUnconfiguredOrInvalidTasks(t *testing.T) {
	validTask := asynq.NewTask(TypeCashForecastRefresh, []byte(`{"company_id":1,"scenario_id":2}`))
	processor := NewCashForecastProcessor(forecasting.NewService(nil, nil, nil), nil, nil)
	tests := []struct {
		name      string
		processor *CashForecastProcessor
		task      *asynq.Task
	}{
		{name: "processor service", processor: NewCashForecastProcessor(nil, nil, nil), task: validTask},
		{name: "nil processor", task: validTask},
		{name: "nil task", processor: processor},
		{name: "malformed payload", processor: processor, task: asynq.NewTask(TypeCashForecastRefresh, []byte("{"))},
		{name: "missing company", processor: processor, task: asynq.NewTask(TypeCashForecastRefresh, []byte(`{"scenario_id":2}`))},
		{name: "missing scenario", processor: processor, task: asynq.NewTask(TypeCashForecastRefresh, []byte(`{"company_id":1}`))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.processor.ProcessRefreshTask(context.Background(), tt.task)
			require.Error(t, err)
			require.True(t, errors.Is(err, asynq.SkipRetry), "error = %v", err)
		})
	}
}

func TestCashForecastProcessorPropagatesServiceErrorWithoutLogger(t *testing.T) {
	settings := automation.DefaultSettings(1)
	settings.CashForecastEnabled = true
	processor := NewCashForecastProcessor(forecasting.NewService(nil, nil, nil), nil, cashForecastSettingsReaderFake{settings: settings})
	task := asynq.NewTask(TypeCashForecastRefresh, []byte(`{"company_id":1,"scenario_id":2}`))

	err := processor.ProcessRefreshTask(context.Background(), task)
	require.Error(t, err)
	require.Contains(t, err.Error(), "forecast repository is not configured")
}

func TestCashForecastProcessorRechecksCompanyFlagBeforeRefresh(t *testing.T) {
	settings := automation.DefaultSettings(1)
	processor := NewCashForecastProcessor(
		forecasting.NewService(nil, nil, nil),
		nil,
		cashForecastSettingsReaderFake{settings: settings},
	)
	task := asynq.NewTask(TypeCashForecastRefresh, []byte(`{"company_id":1,"scenario_id":2}`))

	err := processor.ProcessRefreshTask(context.Background(), task)
	require.Error(t, err)
	require.ErrorIs(t, err, asynq.SkipRetry)
	require.Contains(t, err.Error(), "is disabled")
}

func TestCashForecastProcessorRetriesSettingsLookupFailure(t *testing.T) {
	processor := NewCashForecastProcessor(
		forecasting.NewService(nil, nil, nil),
		nil,
		cashForecastSettingsReaderFake{err: errors.New("settings unavailable")},
	)
	task := asynq.NewTask(TypeCashForecastRefresh, []byte(`{"company_id":1,"scenario_id":2}`))

	err := processor.ProcessRefreshTask(context.Background(), task)
	require.Error(t, err)
	require.NotErrorIs(t, err, asynq.SkipRetry)
	require.Contains(t, err.Error(), "settings unavailable")
}

func TestCashForecastProcessorRejectsSettingsScopeMismatch(t *testing.T) {
	settings := automation.DefaultSettings(99)
	settings.CashForecastEnabled = true
	processor := NewCashForecastProcessor(
		forecasting.NewService(nil, nil, nil),
		nil,
		cashForecastSettingsReaderFake{settings: settings},
	)
	task := asynq.NewTask(TypeCashForecastRefresh, []byte(`{"company_id":1,"scenario_id":2}`))

	err := processor.ProcessRefreshTask(context.Background(), task)
	require.Error(t, err)
	require.ErrorIs(t, err, asynq.SkipRetry)
	require.Contains(t, err.Error(), "settings company mismatch")
}
