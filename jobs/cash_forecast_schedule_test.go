package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

type cashForecastScheduleStoreFake struct {
	companies     []CashForecastScheduleCompany
	enabled       map[int64]bool
	scenarios     map[int64][]CashForecastScheduleScenario
	companiesErr  error
	settingsErr   map[int64]error
	scenariosErr  map[int64]error
	settingsCalls []int64
	scenarioCalls []int64
}

func (f *cashForecastScheduleStoreFake) ListCompanies(context.Context) ([]CashForecastScheduleCompany, error) {
	if f.companiesErr != nil {
		return nil, f.companiesErr
	}
	return f.companies, nil
}

func (f *cashForecastScheduleStoreFake) CashForecastEnabled(_ context.Context, companyID int64) (bool, error) {
	f.settingsCalls = append(f.settingsCalls, companyID)
	if err := f.settingsErr[companyID]; err != nil {
		return false, err
	}
	return f.enabled[companyID], nil
}

func (f *cashForecastScheduleStoreFake) ListForecastScenarios(_ context.Context, companyID int64) ([]CashForecastScheduleScenario, error) {
	f.scenarioCalls = append(f.scenarioCalls, companyID)
	if err := f.scenariosErr[companyID]; err != nil {
		return nil, err
	}
	return f.scenarios[companyID], nil
}

func TestCashForecastRefreshScanEnqueuesEnabledOwnedScenariosOnce(t *testing.T) {
	store := &cashForecastScheduleStoreFake{
		companies: []CashForecastScheduleCompany{{ID: 20}, {ID: 10}, {ID: 20}, {ID: 30}},
		enabled:   map[int64]bool{10: true, 20: true, 30: false},
		scenarios: map[int64][]CashForecastScheduleScenario{
			10: {{ID: 101, CompanyID: 10}, {ID: 102, CompanyID: 10}, {ID: 101, CompanyID: 10}},
			20: {{ID: 201, CompanyID: 20}},
		},
		settingsErr:  map[int64]error{},
		scenariosErr: map[int64]error{},
	}
	var enqueued [][2]int64
	handler := HandleCashForecastRefreshScanTask(store, func(_ context.Context, companyID, scenarioID int64) error {
		enqueued = append(enqueued, [2]int64{companyID, scenarioID})
		return nil
	})

	err := handler(context.Background(), asynq.NewTask(TypeCashForecastRefreshScan, nil))
	require.NoError(t, err)
	require.ElementsMatch(t, [][2]int64{{10, 101}, {10, 102}, {20, 201}}, enqueued)
	require.ElementsMatch(t, []int64{20, 10, 30}, store.settingsCalls)
	require.ElementsMatch(t, []int64{20, 10}, store.scenarioCalls)
}

func TestCashForecastRefreshScanTreatsDuplicateQueueAsSuccess(t *testing.T) {
	store := &cashForecastScheduleStoreFake{
		companies:    []CashForecastScheduleCompany{{ID: 1}},
		enabled:      map[int64]bool{1: true},
		scenarios:    map[int64][]CashForecastScheduleScenario{1: {{ID: 2, CompanyID: 1}}},
		settingsErr:  map[int64]error{},
		scenariosErr: map[int64]error{},
	}
	handler := HandleCashForecastRefreshScanTask(store, func(context.Context, int64, int64) error {
		return asynq.ErrTaskIDConflict
	})

	require.NoError(t, handler(context.Background(), asynq.NewTask(TypeCashForecastRefreshScan, nil)))
}

func TestCashForecastRefreshScanRejectsOutOfScopeScenario(t *testing.T) {
	store := &cashForecastScheduleStoreFake{
		companies:    []CashForecastScheduleCompany{{ID: 1}},
		enabled:      map[int64]bool{1: true},
		scenarios:    map[int64][]CashForecastScheduleScenario{1: {{ID: 2, CompanyID: 99}}},
		settingsErr:  map[int64]error{},
		scenariosErr: map[int64]error{},
	}
	handler := HandleCashForecastRefreshScanTask(store, func(context.Context, int64, int64) error {
		t.Fatalf("out-of-scope scenario must not be enqueued")
		return nil
	})

	err := handler(context.Background(), asynq.NewTask(TypeCashForecastRefreshScan, nil))
	require.Error(t, err)
	require.True(t, errors.Is(err, asynq.SkipRetry), "error = %v", err)
}

func TestCashForecastRefreshScanFailsClosedOnMissingDependenciesAndStoreErrors(t *testing.T) {
	validStore := &cashForecastScheduleStoreFake{
		companies:    []CashForecastScheduleCompany{{ID: 1}},
		enabled:      map[int64]bool{1: true},
		scenarios:    map[int64][]CashForecastScheduleScenario{1: {{ID: 2, CompanyID: 1}}},
		settingsErr:  map[int64]error{},
		scenariosErr: map[int64]error{},
	}
	validEnqueue := func(context.Context, int64, int64) error { return nil }
	tests := []struct {
		name    string
		store   CashForecastScheduleStore
		enqueue CashForecastRefreshEnqueuer
		task    *asynq.Task
		wantErr error
	}{
		{name: "nil store", enqueue: validEnqueue, task: asynq.NewTask(TypeCashForecastRefreshScan, nil), wantErr: asynq.SkipRetry},
		{name: "nil enqueuer", store: validStore, task: asynq.NewTask(TypeCashForecastRefreshScan, nil), wantErr: asynq.SkipRetry},
		{name: "nil task", store: validStore, enqueue: validEnqueue, wantErr: asynq.SkipRetry},
		{name: "company listing", store: &cashForecastScheduleStoreFake{companiesErr: errors.New("database unavailable")}, enqueue: validEnqueue, task: asynq.NewTask(TypeCashForecastRefreshScan, nil), wantErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := HandleCashForecastRefreshScanTask(tt.store, tt.enqueue)(context.Background(), tt.task)
			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}
