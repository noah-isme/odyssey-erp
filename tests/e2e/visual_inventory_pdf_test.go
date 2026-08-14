package e2e

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestVisualGRNPDF(t *testing.T) {
	client, baseURL, _, _, screenshotDir := loginE2EClient(t)
	if baseURL == "" {
		t.Skip("E2E_BASE_URL not set, skipping visual test")
	}

	t.Run("PDF_GRNDownload", func(t *testing.T) {
		if !pdfVisualEnabled() {
			t.Skip("PDF visual tests disabled")
		}
		pdfURL := fmt.Sprintf("%s/report/grn/pdf?number=GRN-202412-0001&supplier_id=1&warehouse_id=1", baseURL)
		pdfBytes := downloadPDF(t, client, pdfURL)

		pdfOutputPath := filepath.Join(screenshotDir, "pdf", "grn.pdf")
		if err := os.MkdirAll(filepath.Dir(pdfOutputPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(pdfOutputPath, pdfBytes, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	})
}

func TestVisualStockCardPDF(t *testing.T) {
	client, baseURL, _, _, screenshotDir := loginE2EClient(t)
	if baseURL == "" {
		t.Skip("E2E_BASE_URL not set, skipping visual test")
	}

	t.Run("PDF_StockCardDownload", func(t *testing.T) {
		if !pdfVisualEnabled() {
			t.Skip("PDF visual tests disabled")
		}
		pdfURL := fmt.Sprintf("%s/report/stock-card/pdf?warehouse_id=1&product_id=1", baseURL)
		pdfBytes := downloadPDF(t, client, pdfURL)

		pdfOutputPath := filepath.Join(screenshotDir, "pdf", "stock_card.pdf")
		if err := os.MkdirAll(filepath.Dir(pdfOutputPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(pdfOutputPath, pdfBytes, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	})
}

func TestVisualPackingListPage(t *testing.T) {
	client, baseURL, gotenbergURL, gotenbergTargetURL, screenshotDir := loginE2EClient(t)
	if baseURL == "" {
		t.Skip("E2E_BASE_URL not set, skipping visual test")
	}

	t.Run("UI_DeliveryOrdersListing", func(t *testing.T) {
		pagePath := "/delivery/orders"
		pageURL := baseURL + pagePath
		targetURL := gotenbergTargetURL + pagePath

		_, status, body := fetchPage(t, client, pageURL)
		if status == http.StatusServiceUnavailable {
			t.Skipf("Service unavailable at %s", pageURL)
		}
		if status != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d", status)
		}

		assertRenderedPage(t, pagePath, body)

		uiScreenshotPath := filepath.Join(screenshotDir, "ui", "delivery_orders_listing.png")
		if err := os.MkdirAll(filepath.Dir(uiScreenshotPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		captureVisualPage(t, client, pageURL, targetURL, gotenbergURL, uiScreenshotPath)

		uiGoldenPath := filepath.Join("golden", "ui", "delivery_orders_listing.png")
		assertVisualMatch(t, uiGoldenPath, uiScreenshotPath, visualDiffThreshold())

		re := regexp.MustCompile(`href="/delivery/orders/([0-9]+)"`)
		matches := re.FindStringSubmatch(body)

		if len(matches) < 2 {
			t.Skip("No seeded delivery order found on /delivery/orders page")
		}
		orderID := matches[1]

		t.Run("UI_DeliveryOrderDetail", func(t *testing.T) {
			detailPath := fmt.Sprintf("/delivery/orders/%s", orderID)
			detailURL := baseURL + detailPath
			detailTargetURL := gotenbergTargetURL + detailPath

			_, status, body := fetchPage(t, client, detailURL)
			if status == http.StatusServiceUnavailable {
				t.Skipf("Service unavailable at %s", detailURL)
			}
			if status != http.StatusOK {
				t.Fatalf("Expected 200 OK, got %d", status)
			}

			assertRenderedPage(t, detailPath, body)

			detailScreenshotPath := filepath.Join(screenshotDir, "ui", "delivery_order_detail.png")
			if err := os.MkdirAll(filepath.Dir(detailScreenshotPath), 0755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			captureVisualPage(t, client, detailURL, detailTargetURL, gotenbergURL, detailScreenshotPath)

			detailGoldenPath := filepath.Join("golden", "ui", "delivery_order_detail.png")
			assertVisualMatch(t, detailGoldenPath, detailScreenshotPath, visualDiffThreshold())
		})
	})
}
