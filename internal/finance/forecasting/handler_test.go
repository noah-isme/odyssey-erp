package forecasting

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type forecastSettingsReaderFake struct {
	settings automation.Settings
	err      error
}

func (f forecastSettingsReaderFake) Settings(context.Context, int64) (automation.Settings, error) {
	return f.settings, f.err
}

func TestForecastHandlerRejectsMissingIdentifiers(t *testing.T) {
	h := NewHandler(NewService(&forecastRepoFake{}, nil, nil))
	for _, request := range []string{"/runs/latest", "/runs"} {
		r := httptest.NewRequest(http.MethodGet, request, nil)
		if request == "/runs" {
			r = httptest.NewRequest(http.MethodPost, request, nil)
		}
		w := httptest.NewRecorder()
		if request == "/runs" {
			h.TriggerRun(w, r)
		} else {
			h.GetLatestRun(w, r)
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d", request, w.Code)
		}
	}
	service := NewServiceWithFXResolver(&forecastRepoFake{}, []SourceReader{forecastReaderFake{name: "test"}}, fxResolverFake{}, nil)
	request := httptest.NewRequest(http.MethodGet, "/runs/latest", nil)
	request = request.WithContext(forecastTestContext())
	response := httptest.NewRecorder()
	NewHandler(service).GetLatestRun(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing scenario status = %d", response.Code)
	}
}

func TestForecastHandlerReturnsLatestRunAndTriggersSnapshot(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	repo := &forecastRepoFake{
		run:           ForecastRun{ID: 41, CompanyID: 7, ScenarioID: 3, Status: "COMPLETED", CompletedAt: now.Add(-2 * time.Hour)},
		latestBuckets: []ForecastDailyBucket{{ID: 501, RunID: 41, Currency: "USD", BucketDate: time.Now()}},
		sourceLines: []ForecastSourceLine{{
			ID:            901,
			RunID:         41,
			DailyBucketID: 501,
			SourceType:    string(SourceTypeManualAdjustment),
			SourceRef:     "forecast-adjustment:17",
			Amount:        "-125.5000",
			Currency:      "USD",
			ExpectedDate:  time.Now(),
			Certainty:     string(CertaintyProbable),
		}},
	}
	service := NewServiceWithFXResolver(repo, []SourceReader{forecastReaderFake{name: "test"}}, fxResolverFake{}, nil)
	settings := automation.DefaultSettings(7)
	settings.CashForecastEnabled = true
	h := NewHandler(service, forecastSettingsReaderFake{settings: settings})
	h.SetNow(func() time.Time { return now })

	w := httptest.NewRecorder()
	latestRequest := httptest.NewRequest(http.MethodGet, "/runs/latest?scenario_id=3", nil).WithContext(forecastTestContext())
	h.GetLatestRun(w, latestRequest)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("GetLatestRun() status/content type = %d/%q", w.Code, w.Header().Get("Content-Type"))
	}
	var latest struct {
		SourceLines []ForecastSourceLine `json:"source_lines"`
	}
	if err := json.NewDecoder(w.Body).Decode(&latest); err != nil {
		t.Fatalf("decode latest response: %v", err)
	}
	if len(latest.SourceLines) != 1 || latest.SourceLines[0].SourceRef != "forecast-adjustment:17" || latest.SourceLines[0].Amount != "-125.5000" {
		t.Fatalf("source lines = %#v", latest.SourceLines)
	}

	w = httptest.NewRecorder()
	triggerRequest := httptest.NewRequest(http.MethodPost, "/runs?scenario_id=3", nil).WithContext(forecastTestContext())
	h.TriggerRun(w, triggerRequest)
	if w.Code != http.StatusCreated {
		t.Fatalf("TriggerRun() status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestForecastHandlerRejectsUnscopedLatestRun(t *testing.T) {
	tests := []struct {
		name string
		run  ForecastRun
	}{
		{
			name: "different company",
			run:  ForecastRun{ID: 41, CompanyID: 99, ScenarioID: 3, Status: "COMPLETED"},
		},
		{
			name: "different scenario",
			run:  ForecastRun{ID: 41, CompanyID: 7, ScenarioID: 99, Status: "COMPLETED"},
		},
		{
			name: "missing run id",
			run:  ForecastRun{CompanyID: 7, ScenarioID: 3, Status: "COMPLETED"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &forecastRepoFake{run: tt.run}
			service := NewServiceWithFXResolver(repo, []SourceReader{forecastReaderFake{name: "test"}}, fxResolverFake{}, nil)
			request := httptest.NewRequest(http.MethodGet, "/runs/latest?scenario_id=3", nil).WithContext(forecastTestContext())
			response := httptest.NewRecorder()

			NewHandler(service).GetLatestRun(response, request)

			if response.Code != http.StatusNotFound {
				t.Fatalf("GetLatestRun() status = %d, want %d", response.Code, http.StatusNotFound)
			}
			if repo.latestBucketCalls != 0 {
				t.Fatalf("unscoped run should not be used to load buckets: %d calls", repo.latestBucketCalls)
			}
		})
	}
}

func TestForecastHandlerMapsMissingLatestRunToNotFoundButUnexpectedErrorsToServerError(t *testing.T) {
	tests := []struct {
		name      string
		latestErr error
		wantCode  int
	}{
		{
			name:      "missing latest run",
			latestErr: pgx.ErrNoRows,
			wantCode:  http.StatusNotFound,
		},
		{
			name:      "query failure",
			latestErr: errors.New("db unavailable"),
			wantCode:  http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &forecastRepoFake{latestErr: tt.latestErr}
			service := NewServiceWithFXResolver(repo, []SourceReader{forecastReaderFake{name: "test"}}, fxResolverFake{}, nil)
			request := httptest.NewRequest(http.MethodGet, "/runs/latest?scenario_id=3", nil).WithContext(forecastTestContext())
			response := httptest.NewRecorder()

			NewHandler(service).GetLatestRun(response, request)

			if response.Code != tt.wantCode {
				t.Fatalf("GetLatestRun() status = %d, want %d", response.Code, tt.wantCode)
			}
		})
	}
}

func TestForecastHandlerRejectsNonPositiveLatestScenario(t *testing.T) {
	repo := &forecastRepoFake{}
	service := NewServiceWithFXResolver(repo, []SourceReader{forecastReaderFake{name: "test"}}, fxResolverFake{}, nil)
	for _, scenarioID := range []string{"0", "-1"} {
		request := httptest.NewRequest(http.MethodGet, "/runs/latest?scenario_id="+scenarioID, nil).WithContext(forecastTestContext())
		response := httptest.NewRecorder()

		NewHandler(service).GetLatestRun(response, request)

		if response.Code != http.StatusBadRequest {
			t.Fatalf("scenario_id=%s status = %d, want %d", scenarioID, response.Code, http.StatusBadRequest)
		}
	}
}

func TestForecastFreshnessRejectsMissingStaleAndFutureRuns(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		run        ForecastRun
		wantFresh  bool
		wantReason string
	}{
		{
			name:       "missing completion",
			run:        ForecastRun{Status: "COMPLETED"},
			wantReason: "forecast run is not complete",
		},
		{
			name:       "stale",
			run:        ForecastRun{Status: "COMPLETED", CompletedAt: now.Add(-25 * time.Hour)},
			wantReason: "forecast is older than 24 hours",
		},
		{
			name:       "future timestamp",
			run:        ForecastRun{Status: "COMPLETED", CompletedAt: now.Add(time.Hour)},
			wantReason: "forecast completion time is in the future",
		},
		{
			name:      "fresh",
			run:       ForecastRun{Status: "COMPLETED", CompletedAt: now.Add(-23 * time.Hour)},
			wantFresh: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fresh, warning := forecastFreshness(tt.run, now)
			if fresh != tt.wantFresh {
				t.Fatalf("fresh = %v, want %v (warning %q)", fresh, tt.wantFresh, warning)
			}
			if warning != tt.wantReason {
				t.Fatalf("warning = %q, want %q", warning, tt.wantReason)
			}
		})
	}
}

func TestForecastHandlerTriggerRequiresEnabledCompanyFlag(t *testing.T) {
	tests := []struct {
		name     string
		settings SettingsReader
		wantCode int
	}{
		{
			name:     "disabled",
			settings: forecastSettingsReaderFake{settings: automation.DefaultSettings(7)},
			wantCode: http.StatusForbidden,
		},
		{
			name:     "missing settings reader",
			settings: nil,
			wantCode: http.StatusServiceUnavailable,
		},
		{
			name:     "settings error",
			settings: forecastSettingsReaderFake{err: errors.New("settings unavailable")},
			wantCode: http.StatusServiceUnavailable,
		},
		{
			name: "company mismatch",
			settings: func() SettingsReader {
				settings := automation.DefaultSettings(99)
				settings.CashForecastEnabled = true
				return forecastSettingsReaderFake{settings: settings}
			}(),
			wantCode: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &forecastRepoFake{}
			service := NewServiceWithFXResolver(repo, []SourceReader{forecastReaderFake{name: "test"}}, fxResolverFake{}, nil)
			h := NewHandler(service, tt.settings)
			request := httptest.NewRequest(http.MethodPost, "/runs?scenario_id=3", nil).WithContext(forecastTestContext())
			response := httptest.NewRecorder()

			h.TriggerRun(response, request)

			if response.Code != tt.wantCode {
				t.Fatalf("TriggerRun() status = %d, want %d", response.Code, tt.wantCode)
			}
			if repo.run.ID != 0 {
				t.Fatalf("TriggerRun() created run while blocked: %+v", repo.run)
			}
		})
	}
}

func forecastTestContext() context.Context {
	session := &shared.Session{}
	session.SetUser("42")
	session.Set("company_id", "7")
	return shared.ContextWithSession(context.Background(), session)
}
