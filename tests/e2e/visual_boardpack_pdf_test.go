package e2e

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestVisualBoardPackDownload(t *testing.T) {
	client, baseURL, gotenbergURL, gotenbergTargetURL, screenshotDir := loginE2EClient(t)
	if baseURL == "" {
		t.Skip("E2E_BASE_URL not set, skipping visual test")
	}

	t.Run("UI_BoardPackListing", func(t *testing.T) {
		pagePath := "/board-packs"
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
		
		uiScreenshotPath := filepath.Join(screenshotDir, "ui", "board_packs_listing.png")
		if err := os.MkdirAll(filepath.Dir(uiScreenshotPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		
		captureVisualPage(t, client, pageURL, targetURL, gotenbergURL, uiScreenshotPath)
		
		uiGoldenPath := filepath.Join("golden", "ui", "board_packs_listing.png")
		assertVisualMatch(t, uiGoldenPath, uiScreenshotPath, visualDiffThreshold())
		
		re := regexp.MustCompile(`href="/board-packs/([0-9]+)"`)
		matches := re.FindStringSubmatch(body)
		
		if len(matches) < 2 {
			t.Skip("No seeded board pack found on /board-packs page")
		}
		packID := matches[1]

		t.Run("PDF_BoardPackDownload", func(t *testing.T) {
			if !pdfVisualEnabled() {
				t.Skip("PDF visual tests disabled")
			}
			pdfURL := fmt.Sprintf("%s/board-packs/%s/download", baseURL, packID)
			pdfBytes := downloadPDF(t, client, pdfURL)
			
			pdfOutputPath := filepath.Join(screenshotDir, "pdf", "board_pack.pdf")
			if err := os.MkdirAll(filepath.Dir(pdfOutputPath), 0755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(pdfOutputPath, pdfBytes, 0644); err != nil {
				t.Fatalf("write: %v", err)
			}
		})
	})
}
