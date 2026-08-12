package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseReleaseProfile(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  ReleaseProfile
		ok    bool
	}{
		{name: "core", input: "v0.10-core", want: ReleaseProfileV010Core, ok: true},
		{name: "full", input: " FULL ", want: ReleaseProfileFull, ok: true},
		{name: "unknown", input: "preview", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseReleaseProfile(test.input)
			if (err == nil) != test.ok || got != test.want {
				t.Fatalf("ParseReleaseProfile(%q) = %q, %v; want %q, ok=%v", test.input, got, err, test.want, test.ok)
			}
		})
	}
}

func TestCoreProfileAllowsOnlyCoreAndSupportPaths(t *testing.T) {
	profile := ReleaseProfileV010Core
	for _, test := range []struct {
		path string
		want bool
	}{
		{path: "/", want: true},
		{path: "/finance/ap/invoices", want: true},
		{path: "/documents/library", want: true},
		{path: "/cmms/work-orders", want: true},
		{path: "/api/notifications/unread-count", want: true},
		{path: "/static/css/main.css", want: true},
		{path: "/documents/search", want: false},
		{path: "/documents//search", want: false},
		{path: "/documents/library/1/versions/2/ocr", want: false},
		{path: "/cmms/iot/readings", want: false},
		{path: "/cmms/./iot/readings", want: false},
		{path: "/cmms/predictive/evaluate", want: false},
		{path: "/settings/integrations", want: false},
		{path: "/mrp", want: false},
		{path: "/api/v1/projects", want: false},
	} {
		if got := profile.AllowsPath(test.path); got != test.want {
			t.Errorf("AllowsPath(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}

func TestReleaseProfileMiddlewareReturnsNotFoundForPreviewRoute(t *testing.T) {
	handler := ReleaseProfileMiddleware(string(ReleaseProfileV010Core))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/mrp", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("preview route status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestReleaseProfileMiddlewareFailsClosedForInvalidProfile(t *testing.T) {
	handler := ReleaseProfileMiddleware("preview")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("invalid profile status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
