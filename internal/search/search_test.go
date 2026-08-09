package search

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

type searchServiceStub struct {
	results []SearchResult
	err     error
	query   string
	limit   int
}

func (s *searchServiceStub) Search(_ context.Context, query string, limit int) ([]SearchResult, error) {
	s.query = query
	s.limit = limit
	return s.results, s.err
}

func TestEmptySearchDoesNotRequireDatabase(t *testing.T) {
	results, err := NewService(nil).Search(context.Background(), "", 0)
	if err != nil || len(results) != 0 {
		t.Fatalf("results=%v err=%v", results, err)
	}
}

func TestSearchRouteReturnsJSONAndPassesQuery(t *testing.T) {
	svc := &searchServiceStub{results: []SearchResult{{Type: "customer", ID: 8, Title: "Acme", URL: "/sales/customers/8"}}}
	router := chi.NewRouter()
	NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), svc).MountRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=Acme", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status=%d contentType=%q body=%s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	if svc.query != "Acme" || svc.limit != 20 || rec.Body.String() == "[]\n" {
		t.Fatalf("query=%q limit=%d body=%s", svc.query, svc.limit, rec.Body.String())
	}
}

func TestSearchRouteReturnsServerError(t *testing.T) {
	router := chi.NewRouter()
	NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), &searchServiceStub{err: errors.New("database unavailable")}).MountRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=Acme", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
