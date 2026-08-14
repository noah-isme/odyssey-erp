package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestVisualConsolTrialBalancePDF(t *testing.T) {
	client, baseURL, gotenbergURL, gotenbergTargetURL, screenshotDir := loginE2EClient(t)

	t.Run("ui-page", func(t *testing.T) {
		pagePath := "/accounting/trial-balance"
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

		uiScreenshotPath := filepath.Join(screenshotDir, "ui", "consol_trial_balance.png")
		if err := os.MkdirAll(filepath.Dir(uiScreenshotPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		captureVisualPage(t, client, pageURL, targetURL, gotenbergURL, uiScreenshotPath)

		uiGoldenPath := filepath.Join("golden", "ui", "consol_trial_balance.png")
		assertVisualMatch(t, uiGoldenPath, uiScreenshotPath, visualDiffThreshold())
	})

	t.Run("pdf-download", func(t *testing.T) {
		if !pdfVisualEnabled() {
			t.Skip("PDF visual tests disabled")
		}
		pdfURL := baseURL + "/finance/consol/tb/pdf?group=1&period=2024-12"

		pdfBytes := downloadPDF(t, client, pdfURL)

		pdfOutputPath := filepath.Join(screenshotDir, "pdf", "consol_trial_balance.pdf")
		if err := os.MkdirAll(filepath.Dir(pdfOutputPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(pdfOutputPath, pdfBytes, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	})
}

func TestVisualConsolBalanceSheetPDF(t *testing.T) {
	client, baseURL, gotenbergURL, gotenbergTargetURL, screenshotDir := loginE2EClient(t)

	t.Run("ui-page", func(t *testing.T) {
		pagePath := "/accounting/balance-sheet"
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

		uiScreenshotPath := filepath.Join(screenshotDir, "ui", "consol_balance_sheet.png")
		if err := os.MkdirAll(filepath.Dir(uiScreenshotPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		captureVisualPage(t, client, pageURL, targetURL, gotenbergURL, uiScreenshotPath)

		uiGoldenPath := filepath.Join("golden", "ui", "consol_balance_sheet.png")
		assertVisualMatch(t, uiGoldenPath, uiScreenshotPath, visualDiffThreshold())
	})

	t.Run("pdf-download", func(t *testing.T) {
		if !pdfVisualEnabled() {
			t.Skip("PDF visual tests disabled")
		}
		pdfURL := baseURL + "/finance/consol/bs/pdf?group=1&period=2024-12"

		pdfBytes := downloadPDF(t, client, pdfURL)

		pdfOutputPath := filepath.Join(screenshotDir, "pdf", "consol_balance_sheet.pdf")
		if err := os.MkdirAll(filepath.Dir(pdfOutputPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(pdfOutputPath, pdfBytes, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	})
}

func TestVisualConsolProfitLossPDF(t *testing.T) {
	client, baseURL, gotenbergURL, gotenbergTargetURL, screenshotDir := loginE2EClient(t)

	t.Run("ui-page", func(t *testing.T) {
		pagePath := "/accounting/pnl"
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

		uiScreenshotPath := filepath.Join(screenshotDir, "ui", "consol_profit_loss.png")
		if err := os.MkdirAll(filepath.Dir(uiScreenshotPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		captureVisualPage(t, client, pageURL, targetURL, gotenbergURL, uiScreenshotPath)

		uiGoldenPath := filepath.Join("golden", "ui", "consol_profit_loss.png")
		assertVisualMatch(t, uiGoldenPath, uiScreenshotPath, visualDiffThreshold())
	})

	t.Run("pdf-download", func(t *testing.T) {
		if !pdfVisualEnabled() {
			t.Skip("PDF visual tests disabled")
		}
		pdfURL := baseURL + "/finance/consol/pl/pdf?group=1&period=2024-12"

		pdfBytes := downloadPDF(t, client, pdfURL)

		pdfOutputPath := filepath.Join(screenshotDir, "pdf", "consol_profit_loss.pdf")
		if err := os.MkdirAll(filepath.Dir(pdfOutputPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(pdfOutputPath, pdfBytes, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	})
}
