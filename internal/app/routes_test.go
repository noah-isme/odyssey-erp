package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestWalkRoutesEnumeratesNestedRouters(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/", func(http.ResponseWriter, *http.Request) {})
	router.Route("/accounting", func(r chi.Router) {
		r.Get("/pnl", func(http.ResponseWriter, *http.Request) {})
		r.Get("/journals/{id}", func(http.ResponseWriter, *http.Request) {})
		r.Post("/journals", func(http.ResponseWriter, *http.Request) {})
	})

	entries, err := WalkRoutes(router)
	if err != nil {
		t.Fatal(err)
	}

	got := make(map[string]string, len(entries))
	for _, entry := range entries {
		got[entry.Pattern] = entry.Method
	}
	for pattern, method := range map[string]string{
		"/":                         http.MethodGet,
		"/accounting/pnl":           http.MethodGet,
		"/accounting/journals/{id}": http.MethodGet,
	} {
		if got[pattern] != method {
			t.Errorf("route %q method = %q, want %q", pattern, got[pattern], method)
		}
	}
	if _, ok := got["/accounting/journals"]; !ok {
		t.Error("POST /accounting/journals missing from walk")
	}

	// Nested routers must not leak doubled or trailing slashes into patterns,
	// since consumers match on the exact string.
	for _, entry := range entries {
		if strings.Contains(entry.Pattern, "//") {
			t.Errorf("pattern %q contains a doubled slash", entry.Pattern)
		}
		if len(entry.Pattern) > 1 && strings.HasSuffix(entry.Pattern, "/") {
			t.Errorf("pattern %q has a trailing slash", entry.Pattern)
		}
	}
}

func TestWalkRoutesRejectsNonChiHandler(t *testing.T) {
	if _, err := WalkRoutes(http.NotFoundHandler()); err == nil {
		t.Fatal("expected an error for a handler that is not a chi router")
	}
}

func TestWriteRoutesEmitsParsableJSON(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/jobs", func(http.ResponseWriter, *http.Request) {})

	recorder := httptest.NewRecorder()
	if err := WriteRoutes(router, recorder.Body); err != nil {
		t.Fatal(err)
	}

	var entries []RouteEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &entries); err != nil {
		t.Fatalf("route dump is not valid JSON: %v", err)
	}
	if len(entries) != 1 || entries[0].Pattern != "/jobs" || entries[0].Method != http.MethodGet {
		t.Fatalf("unexpected dump: %+v", entries)
	}
}
