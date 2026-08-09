package distribution

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestPlaceholderHandlersReturnJSONContract(t *testing.T) {
	h := NewHandler(nil)
	for _, handler := range []http.HandlerFunc{h.CreatePlanningHorizonHandler, h.CreateLoadHandler, h.PlanRouteHandler, h.CreateTransferHandler} {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodPost, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("content type=%q", rec.Header().Get("Content-Type"))
		}
		var response map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response["status"] != "not implemented" {
			t.Fatalf("response=%v", response)
		}
	}
}

func TestGetLoadRejectsInvalidID(t *testing.T) {
	router := chi.NewRouter()
	h := NewHandler(nil)
	router.Get("/loads/{id}", h.GetLoadHandler)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/loads/not-a-number", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
