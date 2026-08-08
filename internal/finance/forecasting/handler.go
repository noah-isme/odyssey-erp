package forecasting

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/runs/latest", h.GetLatestRun)
	r.Post("/runs", h.TriggerRun)
}

func (h *Handler) GetLatestRun(w http.ResponseWriter, r *http.Request) {
	companyIDStr := r.URL.Query().Get("company_id")
	companyID, _ := strconv.ParseInt(companyIDStr, 10, 64)
	if companyID == 0 {
		http.Error(w, "missing company_id", http.StatusBadRequest)
		return
	}

	scenarioIDStr := r.URL.Query().Get("scenario_id")
	scenarioID, _ := strconv.ParseInt(scenarioIDStr, 10, 64)
	if scenarioID == 0 {
		http.Error(w, "missing scenario_id", http.StatusBadRequest)
		return
	}

	run, err := h.service.repo.GetLatestForecastRun(r.Context(), sqlc.GetLatestForecastRunParams{
		CompanyID:  companyID,
		ScenarioID: scenarioID,
	})
	if err != nil {
		http.Error(w, "not found or error: "+err.Error(), http.StatusNotFound)
		return
	}

	buckets, err := h.service.repo.ListForecastDailyBucketsByRun(r.Context(), run.ID)
	if err != nil {
		http.Error(w, "failed to list buckets: "+err.Error(), http.StatusInternalServerError)
		return
	}

	isFresh := time.Since(run.CompletedAt.Time) < 24*time.Hour

	response := struct {
		Run     sqlc.ForecastRun           `json:"run"`
		IsFresh bool                       `json:"is_fresh"`
		Buckets []sqlc.ForecastDailyBucket `json:"buckets"`
	}{
		Run:     run,
		IsFresh: isFresh,
		Buckets: buckets,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (h *Handler) TriggerRun(w http.ResponseWriter, r *http.Request) {
	companyIDStr := r.URL.Query().Get("company_id")
	companyID, _ := strconv.ParseInt(companyIDStr, 10, 64)

	scenarioIDStr := r.URL.Query().Get("scenario_id")
	scenarioID, _ := strconv.ParseInt(scenarioIDStr, 10, 64)

	if companyID == 0 || scenarioID == 0 {
		http.Error(w, "missing company_id or scenario_id", http.StatusBadRequest)
		return
	}

	err := h.service.GenerateSnapshot(r.Context(), companyID, scenarioID)
	if err != nil {
		http.Error(w, "failed to generate snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status": "completed"}`))
}
