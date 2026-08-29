package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseReleaseProfile(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  ReleaseProfile
		ok    bool
	}{
		{name: "core", input: "v0.10-core", want: ReleaseProfileV010Core, ok: true},
		{name: "finance", input: " V0.11-FINANCE ", want: ReleaseProfileV011Finance, ok: true},
		{name: "full", input: "full", want: ReleaseProfileFull, ok: true},
		{name: "unknown", input: "preview", ok: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseReleaseProfile(test.input)
			if (err == nil) != test.ok || got != test.want {
				t.Fatalf("ParseReleaseProfile(%q) = %q, %v; want %q, ok=%v", test.input, got, err, test.want, test.ok)
			}
		})
	}
}

func TestCoreProfileBlocksPreviewRoutes(t *testing.T) {
	profile := ReleaseProfileV010Core
	for _, test := range []struct {
		path string
		want bool
	}{
		{path: "/", want: true},
		{path: "/finance/ap/invoices", want: true},
		{path: "/legal/privacy", want: true},
		{path: "/documents/search", want: false},
		{path: "/cmms/iot/readings", want: false},
		{path: "/settings/integrations", want: false},
		{path: "/mrp", want: false},
	} {
		if got := profile.AllowsPath(test.path); got != test.want {
			t.Errorf("AllowsPath(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}

func TestReleaseProfileMiddlewareFailsClosed(t *testing.T) {
	handler := ReleaseProfileMiddleware("preview")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("invalid profile status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
