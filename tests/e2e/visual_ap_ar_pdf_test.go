package e2e

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestVisualAPDebitNotePDF(t *testing.T) {
	client, baseURL, gotenbergURL, gotenbergTargetURL, screenshotDir := loginE2EClient(t)
	if baseURL == "" {
		t.Skip("E2E_BASE_URL not set, skipping visual test")
	}

	var docID string

	t.Run("ui-listing", func(t *testing.T) {
		pagePath := "/finance/ap/debit-notes"
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
		
		name := "ap_debit_note_listing.png"
		uiScreenshotPath := filepath.Join(screenshotDir, "ui", name)
		if err := os.MkdirAll(filepath.Dir(uiScreenshotPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		
		captureVisualPage(t, client, pageURL, targetURL, gotenbergURL, uiScreenshotPath)
		assertPNG(t, uiScreenshotPath)
		
		uiGoldenPath := filepath.Join("golden", "ui", name)
		if _, err := os.Stat(uiGoldenPath); err == nil {
			assertVisualMatch(t, uiGoldenPath, uiScreenshotPath, visualDiffThreshold())
		} else {
			t.Logf("Golden file not found at %s, skipping visual match", uiGoldenPath)
		}
		
		re := regexp.MustCompile(`href="/finance/ap/debit-notes/([0-9]+)"`)
		matches := re.FindStringSubmatch(body)
		
		if len(matches) >= 2 {
			docID = matches[1]
		}
	})

	if docID == "" {
		t.Skip("No seeded AP Debit Note found on /finance/ap/debit-notes page, skipping PDF download")
	}

	t.Run("pdf-download", func(t *testing.T) {
		if !pdfVisualEnabled() {
			t.Skip("PDF visual tests disabled via PDF_VISUAL_TESTS=false")
		}
		pdfURL := fmt.Sprintf("%s/finance/ap/debit-notes/%s/pdf", baseURL, docID)
		pdfBytes := downloadPDF(t, client, pdfURL)
		
		name := "ap_debit_note.pdf"
		pdfOutputPath := filepath.Join(screenshotDir, "pdf", name)
		if err := os.MkdirAll(filepath.Dir(pdfOutputPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(pdfOutputPath, pdfBytes, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	})
}

func TestVisualARCreditNotePDF(t *testing.T) {
	client, baseURL, gotenbergURL, gotenbergTargetURL, screenshotDir := loginE2EClient(t)
	if baseURL == "" {
		t.Skip("E2E_BASE_URL not set, skipping visual test")
	}

	var docID string

	t.Run("ui-listing", func(t *testing.T) {
		pagePath := "/finance/ar/credit-notes"
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
		
		name := "ar_credit_note_listing.png"
		uiScreenshotPath := filepath.Join(screenshotDir, "ui", name)
		if err := os.MkdirAll(filepath.Dir(uiScreenshotPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		
		captureVisualPage(t, client, pageURL, targetURL, gotenbergURL, uiScreenshotPath)
		assertPNG(t, uiScreenshotPath)
		
		uiGoldenPath := filepath.Join("golden", "ui", name)
		if _, err := os.Stat(uiGoldenPath); err == nil {
			assertVisualMatch(t, uiGoldenPath, uiScreenshotPath, visualDiffThreshold())
		} else {
			t.Logf("Golden file not found at %s, skipping visual match", uiGoldenPath)
		}
		
		re := regexp.MustCompile(`href="/finance/ar/credit-notes/([0-9]+)"`)
		matches := re.FindStringSubmatch(body)
		
		if len(matches) >= 2 {
			docID = matches[1]
		}
	})

	if docID == "" {
		t.Skip("No seeded AR Credit Note found on /finance/ar/credit-notes page, skipping PDF download")
	}

	t.Run("pdf-download", func(t *testing.T) {
		if !pdfVisualEnabled() {
			t.Skip("PDF visual tests disabled via PDF_VISUAL_TESTS=false")
		}
		pdfURL := fmt.Sprintf("%s/finance/ar/credit-notes/%s/pdf", baseURL, docID)
		pdfBytes := downloadPDF(t, client, pdfURL)
		
		name := "ar_credit_note.pdf"
		pdfOutputPath := filepath.Join(screenshotDir, "pdf", name)
		if err := os.MkdirAll(filepath.Dir(pdfOutputPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(pdfOutputPath, pdfBytes, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	})
}
