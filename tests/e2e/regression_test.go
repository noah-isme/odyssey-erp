package e2e

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
)

// These regression tests use only net/http and run against a deployed local,
// staging, or CI instance. Set ODYSSEY_E2E_URL to enable them.
func TestRegressionFlow(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("ODYSSEY_E2E_URL"), "/")
	if baseURL == "" {
		t.Skip("set ODYSSEY_E2E_URL to run HTTP E2E regression")
	}
	email := envOr("ODYSSEY_E2E_EMAIL", "admin@odyssey.local")
	password := envOr("ODYSSEY_E2E_PASSWORD", "admin123")
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}

	csrf := fetchCSRF(t, client, baseURL+"/auth/login")
	form := url.Values{"email": {email}, "password": {password}, "csrf_token": {csrf}}
	response := postForm(t, client, baseURL+"/auth/login", form)
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", response.StatusCode, http.StatusSeeOther)
	}
	response.Body.Close()

	for _, path := range []string{"/", "/accounting/pnl", "/accounting/budget", "/accounting/dimensions", "/accounting/report-schedules"} {
		response = get(t, client, baseURL+path)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, response.StatusCode)
		}
		response.Body.Close()
	}

	for _, path := range []string{"/accounting/pnl/export.xlsx", "/accounting/budget/export.xlsx"} {
		response = get(t, client, baseURL+path)
		payload, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, response.StatusCode)
		}
		if len(payload) < 4 || string(payload[:2]) != "PK" {
			t.Fatalf("GET %s did not return XLSX data", path)
		}
	}
}

func fetchCSRF(t *testing.T, client *http.Client, endpoint string) string {
	t.Helper()
	response := get(t, client, endpoint)
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET login status = %d", response.StatusCode)
	}
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindSubmatch(body)
	if len(match) != 2 {
		t.Fatal("CSRF token not found")
	}
	return string(match[1])
}
func get(t *testing.T, client *http.Client, endpoint string) *http.Response {
	t.Helper()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
func postForm(t *testing.T, client *http.Client, endpoint string, values url.Values) *http.Response {
	t.Helper()
	response, err := client.PostForm(endpoint, values)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
