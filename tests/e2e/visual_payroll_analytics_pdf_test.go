package e2e

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestVisualPayrollPayslipPDF(t *testing.T) {
	client, baseURL, gotenbergURL, gotenbergTargetURL, screenshotDir := loginE2EClient(t)
	if baseURL == "" {
		t.Skip("E2E_BASE_URL not set, skipping visual test")
	}

	t.Run("UI_PayrollListing", func(t *testing.T) {
		pagePath := "/payroll"
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
		
		uiScreenshotPath := filepath.Join(screenshotDir, "ui", "payroll_listing.png")
		if err := os.MkdirAll(filepath.Dir(uiScreenshotPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		
		captureVisualPage(t, client, pageURL, targetURL, gotenbergURL, uiScreenshotPath)
		
		uiGoldenPath := filepath.Join("golden", "ui", "payroll_listing.png")
		assertVisualMatch(t, uiGoldenPath, uiScreenshotPath, visualDiffThreshold())
		
		re := regexp.MustCompile(`href="/payroll/payslips/([0-9]+)"`)
		matches := re.FindStringSubmatch(body)
		
		if len(matches) < 2 {
			t.Skip("No seeded payslip found on /payroll page")
		}
		payslipID := matches[1]

		t.Run("PDF_PayslipDownload", func(t *testing.T) {
			if !pdfVisualEnabled() {
				t.Skip("PDF visual tests disabled")
			}
			pdfURL := fmt.Sprintf("%s/payroll/payslips/%s.pdf", baseURL, payslipID)
			pdfBytes := downloadPDF(t, client, pdfURL)
			
			pdfOutputPath := filepath.Join(screenshotDir, "pdf", "payroll_payslip.pdf")
			if err := os.MkdirAll(filepath.Dir(pdfOutputPath), 0755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(pdfOutputPath, pdfBytes, 0644); err != nil {
				t.Fatalf("write: %v", err)
			}
		})
	})
}

func TestVisualAnalyticsDashboardPDF(t *testing.T) {
	client, baseURL, gotenbergURL, gotenbergTargetURL, screenshotDir := loginE2EClient(t)
	if baseURL == "" {
		t.Skip("E2E_BASE_URL not set, skipping visual test")
	}

	t.Run("UI_AnalyticsDashboard", func(t *testing.T) {
		pagePath := "/analytics"
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
		
		uiScreenshotPath := filepath.Join(screenshotDir, "ui", "analytics_dashboard.png")
		if err := os.MkdirAll(filepath.Dir(uiScreenshotPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		
		captureVisualPage(t, client, pageURL, targetURL, gotenbergURL, uiScreenshotPath)
		
		uiGoldenPath := filepath.Join("golden", "ui", "analytics_dashboard.png")
		assertVisualMatch(t, uiGoldenPath, uiScreenshotPath, visualDiffThreshold())

		t.Run("PDF_AnalyticsDashboard", func(t *testing.T) {
			if !pdfVisualEnabled() {
				t.Skip("PDF visual tests disabled")
			}
			pdfURL := fmt.Sprintf("%s/analytics/pdf?period=2024-12", baseURL)
			pdfBytes := downloadPDF(t, client, pdfURL)
			
			pdfOutputPath := filepath.Join(screenshotDir, "pdf", "analytics_dashboard.pdf")
			if err := os.MkdirAll(filepath.Dir(pdfOutputPath), 0755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(pdfOutputPath, pdfBytes, 0644); err != nil {
				t.Fatalf("write: %v", err)
			}
		})
	})
}
