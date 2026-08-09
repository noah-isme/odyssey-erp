package forecasting

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
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
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	scenarioIDStr := r.URL.Query().Get("scenario_id")
	scenarioID, _ := strconv.ParseInt(scenarioIDStr, 10, 64)
	if scenarioID == 0 {
		http.Error(w, "missing scenario_id", http.StatusBadRequest)
		return
	}

	run, err := h.service.repo.GetLatestForecastRun(r.Context(), ForecastRunQuery{
		CompanyID:  identity.CompanyID,
		ScenarioID: scenarioID,
	})
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusNotFound, err)
		return
	}

	buckets, err := h.service.repo.ListForecastDailyBucketsByRun(r.Context(), run.ID)
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}

	isFresh := time.Since(run.CompletedAt) < 24*time.Hour

	response := struct {
		Run     ForecastRun           `json:"run"`
		IsFresh bool                  `json:"is_fresh"`
		Buckets []ForecastDailyBucket `json:"buckets"`
	}{
		Run:     run,
		IsFresh: isFresh,
		Buckets: buckets,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (h *Handler) TriggerRun(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	scenarioIDStr := r.URL.Query().Get("scenario_id")
	scenarioID, _ := strconv.ParseInt(scenarioIDStr, 10, 64)

	if scenarioID <= 0 {
		http.Error(w, "missing scenario_id", http.StatusBadRequest)
		return
	}

	err := h.service.GenerateSnapshot(r.Context(), identity.CompanyID, scenarioID)
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status": "completed"}`))
}
