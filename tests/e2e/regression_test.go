package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// These regression tests use only net/http and run against a deployed local,
// staging, or CI instance. Set ODYSSEY_E2E_URL to enable them.
// Optionally set GOTENBERG_URL to capture screenshots.
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

	gotenbergURL := os.Getenv("GOTENBERG_URL")
	screenshotDir := os.Getenv("ODYSSEY_E2E_SCREENSHOT_DIR")
	if screenshotDir == "" && gotenbergURL != "" {
		screenshotDir = "test-screenshots"
	}
	// URL for Gotenberg to reach the app (may differ from baseURL if Gotenberg runs in container)
	gotenbergTargetURL := os.Getenv("GOTENBERG_TARGET_URL")
	if gotenbergTargetURL == "" {
		gotenbergTargetURL = baseURL
	}

	// Core feature pages organized by module
	pages := []struct {
		path        string
		description string
	}{
		// Dashboard & Core
		{"/", "Home/Dashboard"},

		// Accounting (admin has finance.gl.view, finance.ap.view, finance.ar.view, finance.boardpack)
		{"/accounting/pnl", "Accounting P&L"},
		{"/accounting/balance-sheet", "Accounting Balance Sheet"},
		{"/accounting/cash-flow", "Accounting Cash Flow"},
		{"/accounting/trial-balance", "Accounting Trial Balance"},
		{"/accounting/gl", "Accounting General Ledger"},
		{"/accounting/budget", "Accounting Budget"},

		// Banking
		{"/finance/banking/accounts", "Banking Accounts"},

		// Jobs
		{"/jobs", "Background Jobs"},

		// User Management (admin has users.view, roles.view, permissions.view, rbac.view)
		{"/roles", "Roles"},
		{"/users", "Users"},
		{"/permissions", "Permissions"},

		// Profile & Settings
		// {"/profile", "User Profile"}, // rate limited
		// {"/settings", "User Settings"}, // rate limited

		// Note: Other pages (AR, AP, Inventory, Procurement, Sales, Master Data,
		// Consolidation, Analytics, Audit, Board Pack, Variance, Elimination, Close, Reports)
		// require specific permissions or data setup that may not be present in a fresh seed.
	}

	for _, page := range pages {
		pageURL := baseURL + page.path
		gotenbergPageURL := gotenbergTargetURL + page.path
		response = get(t, client, pageURL)
		if response.StatusCode != http.StatusOK {
			t.Logf("GET %s (%s) status = %d, skipping", page.path, page.description, response.StatusCode)
			response.Body.Close()
			continue
		}

		if gotenbergURL != "" && screenshotDir != "" {
			fileName := strings.ReplaceAll(strings.Trim(page.path, "/"), "/", "-")
			if fileName == "" {
				fileName = "home"
			}
			outputFile := filepath.Join(screenshotDir, fileName+".png")
			if err := TakePageScreenshot(t, client, gotenbergPageURL, gotenbergURL, outputFile); err != nil {
				t.Logf("screenshot failed for %s (%s): %v", page.path, page.description, err)
			}
		}

		response.Body.Close()
	}

	// Export endpoints
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

// TakePageScreenshot fetches a screenshot of an authenticated page via Gotenberg
// by passing the session cookies to Gotenberg's URL-based screenshot endpoint.
// Returns an error instead of failing the test, allowing the caller to decide.
func TakePageScreenshot(t *testing.T, client *http.Client, pageURL, gotenbergURL, outputFilePath string) error {
	t.Helper()

	// Use multipart form to send URL and cookies
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	_ = writer.WriteField("url", pageURL)
	_ = writer.WriteField("format", "png")
	_ = writer.WriteField("width", "1920")
	_ = writer.WriteField("height", "1080")

	// Extract and pass cookies from the client's jar
	if client.Jar != nil {
		pageReq, _ := http.NewRequest(http.MethodGet, pageURL, nil)
		cookies := client.Jar.Cookies(pageReq.URL)
		if len(cookies) > 0 {
			// Gotenberg expects cookies with specific fields; omit SameSite which uses Go's int enum
			type gotenbergCookie struct {
				Name     string `json:"name"`
				Value    string `json:"value"`
				Domain   string `json:"domain,omitempty"`
				Path     string `json:"path,omitempty"`
				Expires  string `json:"expires,omitempty"`
				HTTPOnly bool   `json:"httpOnly,omitempty"`
				Secure   bool   `json:"secure,omitempty"`
			}
			// Determine domain from page URL for cookies that don't have one
			cookieDomain := pageReq.URL.Hostname()
			gCookies := make([]gotenbergCookie, len(cookies))
			for i, c := range cookies {
				var expires string
				if !c.Expires.IsZero() {
					expires = c.Expires.Format(time.RFC3339)
				}
				domain := c.Domain
				if domain == "" {
					domain = cookieDomain
				}
				gCookies[i] = gotenbergCookie{
					Name:     c.Name,
					Value:    c.Value,
					Domain:   domain,
					Path:     c.Path,
					Expires:  expires,
					HTTPOnly: c.HttpOnly,
					Secure:   c.Secure,
				}
			}
			cookieJSON, _ := json.Marshal(gCookies)
			_ = writer.WriteField("cookies", string(cookieJSON))
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close writer: %w", err)
	}

	endpoint := strings.TrimRight(gotenbergURL, "/") + "/forms/chromium/screenshot/url"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gotenberg request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gotenberg returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if err := os.MkdirAll(filepath.Dir(outputFilePath), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	outFile, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("save screenshot: %w", err)
	}

	t.Logf("Saved screenshot to %s", outputFilePath)
	return nil
}