package distribution

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func distributionRequest(t *testing.T, method, path string, body []byte) *http.Request {
	t.Helper()
	session := &shared.Session{}
	session.SetUser("42")
	session.Set("company_id", "7")
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	return request.WithContext(shared.ContextWithSession(request.Context(), session))
}

func TestCreateLoadHandlerUsesAuthenticatedCompany(t *testing.T) {
	repo := newFakeDistributionRepository()
	handler := NewHandler(NewServiceWithDependencies(repo, Dependencies{}))
	recorder := httptest.NewRecorder()
	handler.CreateLoadHandler(recorder, distributionRequest(t, http.MethodPost, "/loads", []byte(`{"origin_warehouse_id":10,"destination_city":"Jakarta"}`)))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Load Load `json:"load"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Load.CompanyID != 7 || response.Load.OriginWarehouseID != 10 {
		t.Fatalf("load=%+v", response.Load)
	}
}

func TestCreateLoadHandlerRejectsUnauthenticatedRequest(t *testing.T) {
	handler := NewHandler(nil)
	recorder := httptest.NewRecorder()
	handler.CreateLoadHandler(recorder, httptest.NewRequest(http.MethodPost, "/loads", bytes.NewBufferString(`{}`)))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGetLoadRejectsInvalidID(t *testing.T) {
	router := chi.NewRouter()
	handler := NewHandler(nil)
	router.Get("/loads/{id}", handler.GetLoadHandler)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/loads/not-a-number", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
