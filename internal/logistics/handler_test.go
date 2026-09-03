package logistics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestCreateHandlersRejectMalformedJSON(t *testing.T) {
	h := NewHandler(nil)
	for _, handler := range []http.HandlerFunc{h.CreateCarrierHandler, h.CreateFleetHandler, h.RegisterVehicleHandler, h.RegisterDriverHandler, h.CreateShipmentHandler} {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{")))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("content type=%q", rec.Header().Get("Content-Type"))
		}
	}
}

func TestHandlersRejectInvalidPathAndTimestampValues(t *testing.T) {
	h := NewHandler(nil)

	router := chi.NewRouter()
	router.Get("/carriers/{id}", h.GetCarrierHandler)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/carriers/not-a-number", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("carrier status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"carrier_name":"Carrier","carrier_code":"C-1","insurance_expires_at":"not-a-date"}`))
	h.CreateCarrierHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("timestamp status=%d body=%s", rec.Code, rec.Body.String())
	}
}
