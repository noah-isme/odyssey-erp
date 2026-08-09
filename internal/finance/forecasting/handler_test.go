package forecasting

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

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
	repo := &forecastRepoFake{run: ForecastRun{ID: 41, CompanyID: 7, ScenarioID: 3, Status: "COMPLETED", CompletedAt: time.Now()}}
	service := NewServiceWithFXResolver(repo, []SourceReader{forecastReaderFake{name: "test"}}, fxResolverFake{}, nil)
	h := NewHandler(service)
	service.SetNow(time.Now)

	w := httptest.NewRecorder()
	latestRequest := httptest.NewRequest(http.MethodGet, "/runs/latest?scenario_id=3", nil).WithContext(forecastTestContext())
	h.GetLatestRun(w, latestRequest)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("GetLatestRun() status/content type = %d/%q", w.Code, w.Header().Get("Content-Type"))
	}

	w = httptest.NewRecorder()
	triggerRequest := httptest.NewRequest(http.MethodPost, "/runs?scenario_id=3", nil).WithContext(forecastTestContext())
	h.TriggerRun(w, triggerRequest)
	if w.Code != http.StatusCreated {
		t.Fatalf("TriggerRun() status = %d, body = %s", w.Code, w.Body.String())
	}
}

func forecastTestContext() context.Context {
	session := &shared.Session{}
	session.SetUser("42")
	session.Set("company_id", "7")
	return shared.ContextWithSession(context.Background(), session)
}
