package forecasting

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// SettingsReader is the narrow company-scoped feature-flag boundary required
// by on-demand forecast runs. Keeping it smaller than SettingsStore prevents
// this read path from acquiring mutation capabilities.
type SettingsReader interface {
	Settings(context.Context, int64) (automation.Settings, error)
}

type Handler struct {
	service  *Service
	settings SettingsReader
	now      func() time.Time
}

// NewHandler constructs the forecast HTTP handler. A settings reader is
// required for the on-demand trigger path; the variadic form preserves source
// compatibility for callers that only mount/read existing-run routes while
// still failing closed if they attempt to trigger a run without one.
func NewHandler(service *Service, settings ...SettingsReader) *Handler {
	var featureSettings SettingsReader
	if len(settings) > 0 {
		featureSettings = settings[0]
	}
	return &Handler{service: service, settings: featureSettings, now: time.Now}
}

// SetNow makes freshness evaluation deterministic for application-level tests.
// Production handlers retain the wall clock by default.
func (h *Handler) SetNow(now func() time.Time) {
	if h != nil && now != nil {
		h.now = now
	}
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
	if h == nil || h.service == nil || h.service.repo == nil {
		http.Error(w, "forecast is unavailable", http.StatusServiceUnavailable)
		return
	}

	scenarioIDStr := r.URL.Query().Get("scenario_id")
	scenarioID, _ := strconv.ParseInt(scenarioIDStr, 10, 64)
	if scenarioID <= 0 {
		http.Error(w, "missing scenario_id", http.StatusBadRequest)
		return
	}

	run, err := h.service.repo.GetLatestForecastRun(r.Context(), ForecastRunQuery{
		CompanyID:  identity.CompanyID,
		ScenarioID: scenarioID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			shared.WriteErrorStatus(w, http.StatusNotFound, err)
			return
		}
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}
	// Keep the tenant and scenario boundary defensive even though the
	// repository query is scoped. A malformed or alternate repository must not
	// cause this handler to fetch buckets for another company's run.
	if run.ID <= 0 || run.CompanyID != identity.CompanyID || run.ScenarioID != scenarioID {
		http.NotFound(w, r)
		return
	}

	buckets, err := h.service.repo.ListForecastDailyBucketsByRun(r.Context(), run.ID)
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}

	var sourceLines []ForecastSourceLine
	if sourceReader, ok := h.service.repo.(SourceLineReader); ok {
		sourceLines, err = sourceReader.ListForecastSourceLinesByRun(r.Context(), identity.CompanyID, run.ID)
		if err != nil {
			shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
			return
		}
	}

	isFresh, freshnessWarning := forecastFreshness(run, h.clockNow())

	response := struct {
		Run              ForecastRun           `json:"run"`
		IsFresh          bool                  `json:"is_fresh"`
		FreshnessWarning string                `json:"freshness_warning,omitempty"`
		Buckets          []ForecastDailyBucket `json:"buckets"`
		SourceLines      []ForecastSourceLine  `json:"source_lines"`
	}{
		Run:              run,
		IsFresh:          isFresh,
		FreshnessWarning: freshnessWarning,
		Buckets:          buckets,
		SourceLines:      sourceLines,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

const forecastFreshnessWindow = 24 * time.Hour

func (h *Handler) clockNow() time.Time {
	if h != nil && h.now != nil {
		return h.now().UTC()
	}
	return time.Now().UTC()
}

// forecastFreshness deliberately treats missing and future completion times as
// stale. A clock-skewed or partially persisted run must never be presented to
// treasury as a fresh snapshot. The 24-hour window is the existing read-path
// contract; the authoritative threshold remains a product/configuration
// decision before production certification.
func forecastFreshness(run ForecastRun, now time.Time) (bool, string) {
	completedAt := run.CompletedAt.UTC()
	if run.Status != "COMPLETED" || completedAt.IsZero() {
		return false, "forecast run is not complete"
	}
	if now.Before(completedAt) {
		return false, "forecast completion time is in the future"
	}
	if now.Sub(completedAt) >= forecastFreshnessWindow {
		return false, "forecast is older than 24 hours"
	}
	return true, ""
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
	if h == nil || h.service == nil || h.settings == nil {
		http.Error(w, "forecast is unavailable", http.StatusServiceUnavailable)
		return
	}
	settings, err := h.settings.Settings(r.Context(), identity.CompanyID)
	if err != nil || settings.CompanyID != identity.CompanyID {
		http.Error(w, "forecast is unavailable", http.StatusServiceUnavailable)
		return
	}
	if !settings.CashForecastEnabled {
		http.Error(w, "forecast is disabled", http.StatusForbidden)
		return
	}

	err = h.service.GenerateSnapshot(r.Context(), identity.CompanyID, scenarioID)
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status": "completed"}`))
}
