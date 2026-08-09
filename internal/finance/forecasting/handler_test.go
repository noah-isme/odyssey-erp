package forecasting

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestForecastHandlerRejectsMissingIdentifiers(t *testing.T) {
	h := NewHandler(NewService(&forecastRepoFake{}, nil, nil))
	for _, request := range []string{"/runs/latest", "/runs/latest?company_id=1", "/runs"} {
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
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d", request, w.Code)
		}
	}
}

func TestForecastHandlerReturnsLatestRunAndTriggersSnapshot(t *testing.T) {
	repo := &forecastRepoFake{run: ForecastRun{ID: 41, CompanyID: 7, ScenarioID: 3, Status: "COMPLETED", CompletedAt: time.Now()}}
	service := NewService(repo, nil, nil)
	h := NewHandler(service)

	w := httptest.NewRecorder()
	h.GetLatestRun(w, httptest.NewRequest(http.MethodGet, "/runs/latest?company_id=7&scenario_id=3", nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("GetLatestRun() status/content type = %d/%q", w.Code, w.Header().Get("Content-Type"))
	}

	w = httptest.NewRecorder()
	h.TriggerRun(w, httptest.NewRequest(http.MethodPost, "/runs?company_id=7&scenario_id=3", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("TriggerRun() status = %d, body = %s", w.Code, w.Body.String())
	}
}
