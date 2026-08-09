package treasury

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHandlerMountsRoutesAndExposesExplicitPlaceholders(t *testing.T) {
	h := NewHandler(nil)
	router := chi.NewRouter()
	h.MountRoutes(router)

	w := httptest.NewRecorder()
	h.ListBankAccounts(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("ListBankAccounts() status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.AddBatchItem(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("AddBatchItem() status = %d", w.Code)
	}
}
