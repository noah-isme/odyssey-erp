package e2e

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestVisualSalesOrderFlow covers the primary sales workflow with the same
// authenticated session a user would have: list orders, open the create form,
// and inspect a seeded order detail page. Gotenberg renders each page so this
// catches layout regressions that HTML assertions cannot see.
func TestVisualSalesOrderFlow(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("ODYSSEY_E2E_URL"), "/")
	gotenbergURL := strings.TrimRight(os.Getenv("GOTENBERG_URL"), "/")
	if baseURL == "" || gotenbergURL == "" {
		t.Skip("set ODYSSEY_E2E_URL and GOTENBERG_URL to run sales visual E2E")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	csrf := fetchCSRF(t, client, baseURL+"/auth/login")
	response := postForm(t, client, baseURL+"/auth/login", url.Values{
		"email":      {envOr("ODYSSEY_E2E_EMAIL", "admin@odyssey.local")},
		"password":   {envOr("ODYSSEY_E2E_PASSWORD", "admin123")},
		"csrf_token": {csrf},
	})
	if response.StatusCode != http.StatusSeeOther {
		response.Body.Close()
		t.Fatalf("login status = %d, want %d", response.StatusCode, http.StatusSeeOther)
	}
	response.Body.Close()

	screenshotDir := os.Getenv("ODYSSEY_E2E_SCREENSHOT_DIR")
	if screenshotDir == "" {
		screenshotDir = "test-screenshots"
	}
	gotenbergTargetURL := strings.TrimRight(os.Getenv("GOTENBERG_TARGET_URL"), "/")
	if gotenbergTargetURL == "" {
		gotenbergTargetURL = baseURL
	}

	pages := []struct {
		name string
		path string
	}{
		{name: "sales-orders-list", path: "/sales/orders"},
		{name: "sales-orders-new", path: "/sales/orders/new"},
	}

	var listBody string
	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			_, status, body := fetchPage(t, client, baseURL+page.path)
			if status != http.StatusOK {
				t.Fatalf("GET %s status = %d, want 200", page.path, status)
			}
			assertRenderedPage(t, page.path, body)
			if page.path == "/sales/orders" {
				listBody = body
				if !strings.Contains(body, "Pesanan penjualan / Sales orders") {
					t.Error("sales order list heading is missing")
				}
			} else if !strings.Contains(body, "Create a new sales order") {
				t.Error("sales order create form heading is missing")
			}
			captureVisualPage(t, client, baseURL+page.path, gotenbergTargetURL+page.path,
				gotenbergURL, filepath.Join(screenshotDir, page.name+".png"))
		})
	}

	orderLink := regexp.MustCompile(`href="/sales/orders/([0-9]+)"`).FindStringSubmatch(listBody)
	if len(orderLink) != 2 {
		t.Fatal("sales order list has no seeded order detail link")
	}
	detailPath := "/sales/orders/" + orderLink[1]
	t.Run("sales-order-detail", func(t *testing.T) {
		_, status, body := fetchPage(t, client, baseURL+detailPath)
		if status != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", detailPath, status)
		}
		assertRenderedPage(t, detailPath, body)
		if !strings.Contains(body, "Order Details") || !strings.Contains(body, "Items") {
			t.Error("sales order detail content is incomplete")
		}
		captureVisualPage(t, client, baseURL+detailPath, gotenbergTargetURL+detailPath,
			gotenbergURL, filepath.Join(screenshotDir, "sales-order-detail.png"))
	})
}

func captureVisualPage(t *testing.T, client *http.Client, cookieURL, pageURL, gotenbergURL, output string) {
	t.Helper()
	if err := TakePageScreenshot(t, client, cookieURL, pageURL, gotenbergURL, output); err != nil {
		t.Fatalf("capture %s: %v", pageURL, err)
	}
	assertPNG(t, output)
}
