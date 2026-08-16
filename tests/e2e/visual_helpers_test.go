package e2e

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Shared login helper
// ---------------------------------------------------------------------------

// loginE2EClient creates an authenticated HTTP client for E2E visual tests.
// Skips the test when required env vars are absent.
func loginE2EClient(t *testing.T) (client *http.Client, baseURL, gotenbergURL, gotenbergTargetURL, screenshotDir string) {
	t.Helper()
	baseURL = strings.TrimRight(os.Getenv("ODYSSEY_E2E_URL"), "/")
	gotenbergURL = strings.TrimRight(os.Getenv("GOTENBERG_URL"), "/")
	if baseURL == "" || gotenbergURL == "" {
		t.Skip("set ODYSSEY_E2E_URL and GOTENBERG_URL to run visual E2E tests")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client = &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	csrf := fetchCSRF(t, client, baseURL+"/auth/login")
	resp := postForm(t, client, baseURL+"/auth/login", url.Values{
		"email":      {envOr("ODYSSEY_E2E_EMAIL", "admin@odyssey.local")},
		"password":   {envOr("ODYSSEY_E2E_PASSWORD", "admin123")},
		"csrf_token": {csrf},
	})
	if resp.StatusCode != http.StatusSeeOther {
		_ = resp.Body.Close()
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	_ = resp.Body.Close()

	screenshotDir = os.Getenv("ODYSSEY_E2E_SCREENSHOT_DIR")
	if screenshotDir == "" {
		screenshotDir = "test-screenshots"
	}
	gotenbergTargetURL = strings.TrimRight(os.Getenv("GOTENBERG_TARGET_URL"), "/")
	if gotenbergTargetURL == "" {
		gotenbergTargetURL = baseURL
	}
	return
}

// ---------------------------------------------------------------------------
// PDF download helpers
// ---------------------------------------------------------------------------

// Blank references keep scaffolded helpers available for upcoming visual
// regression test cases without triggering the unused lint.
var (
	_ = assertPDFHeaders
	_ = screenshotHTML
)

// downloadPDF fetches a PDF from the given URL, validates Content-Type and
// %PDF- magic bytes, and returns the raw bytes.
func downloadPDF(t *testing.T, client *http.Client, pdfURL string) []byte {
	t.Helper()
	resp := get(t, client, pdfURL)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusServiceUnavailable {
		t.Skipf("GET %s returned 503 — PDF exporter not configured (build without pdf tag?)", pdfURL)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s status = %d, want 200; body: %s", pdfURL, resp.StatusCode, string(body))
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/pdf") {
		t.Fatalf("GET %s Content-Type = %q, want application/pdf", pdfURL, ct)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read PDF from %s: %v", pdfURL, err)
	}
	if len(data) < 1024 {
		t.Fatalf("PDF from %s is only %d bytes, likely empty or error", pdfURL, len(data))
	}
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		t.Fatalf("PDF from %s does not start with %%PDF- magic bytes", pdfURL)
	}

	t.Logf("downloaded PDF from %s: %d bytes", pdfURL, len(data))
	return data
}

// assertPDFHeaders validates Content-Type and Content-Disposition on a response.
func assertPDFHeaders(t *testing.T, resp *http.Response, route string) {
	t.Helper()
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/pdf") {
		t.Errorf("GET %s Content-Type = %q, want application/pdf", route, ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" && !strings.Contains(cd, "attachment") && !strings.Contains(cd, "inline") {
		t.Errorf("GET %s Content-Disposition = %q, want attachment or inline", route, cd)
	}
}

// ---------------------------------------------------------------------------
// Gotenberg HTML screenshot (for PDF visual rendering)
// ---------------------------------------------------------------------------

// screenshotHTML sends raw HTML to Gotenberg's /forms/chromium/screenshot/html
// endpoint and saves the resulting PNG to outputPath. This can be used to
// visually capture PDF template output for golden-file comparison.
func screenshotHTML(t *testing.T, html, gotenbergURL, outputPath string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files", "document.html")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(part, html); err != nil {
		t.Fatalf("write HTML: %v", err)
	}
	_ = writer.WriteField("width", "1920")
	_ = writer.WriteField("height", "1080")
	_ = writer.WriteField("format", "png")

	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	endpoint := strings.TrimRight(gotenbergURL, "/") + "/forms/chromium/screenshot/html"
	req, err := http.NewRequest(http.MethodPost, endpoint, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("gotenberg screenshot request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("gotenberg screenshot returned %d: %s", resp.StatusCode, string(respBody))
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("create dir: %v", err)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		t.Fatalf("create %s: %v", outputPath, err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, resp.Body); err != nil {
		t.Fatalf("save screenshot: %v", err)
	}
	t.Logf("saved HTML screenshot to %s", outputPath)
}

// ---------------------------------------------------------------------------
// Pixel-diff comparator (pure Go)
// ---------------------------------------------------------------------------

// comparePNG loads two PNGs and computes per-pixel RGBA diff. Returns the
// percentage of pixels differing beyond the per-channel threshold and a
// highlighted diff image for debugging.
func comparePNG(t *testing.T, goldenPath, actualPath string, channelThreshold uint8) (diffPct float64, diffImg *image.RGBA) {
	t.Helper()

	goldenImg := decodePNGFile(t, goldenPath)
	actualImg := decodePNGFile(t, actualPath)

	gb := goldenImg.Bounds()
	ab := actualImg.Bounds()

	width := maxInt(gb.Dx(), ab.Dx())
	height := maxInt(gb.Dy(), ab.Dy())

	diffImg = image.NewRGBA(image.Rect(0, 0, width, height))
	totalPixels := width * height
	diffPixels := 0

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var gr, gg, gbl, ga uint32
			var ar, ag, abl, aa uint32

			if x < gb.Dx() && y < gb.Dy() {
				gr, gg, gbl, ga = goldenImg.At(x+gb.Min.X, y+gb.Min.Y).RGBA()
			}
			if x < ab.Dx() && y < ab.Dy() {
				ar, ag, abl, aa = actualImg.At(x+ab.Min.X, y+ab.Min.Y).RGBA()
			}

			dr := channelDiff(uint8(gr>>8), uint8(ar>>8))
			dg := channelDiff(uint8(gg>>8), uint8(ag>>8))
			db := channelDiff(uint8(gbl>>8), uint8(abl>>8))
			da := channelDiff(uint8(ga>>8), uint8(aa>>8))

			if dr > channelThreshold || dg > channelThreshold || db > channelThreshold || da > channelThreshold {
				diffPixels++
				diffImg.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				diffImg.Set(x, y, color.RGBA{
					R: uint8(ar>>8) / 3,
					G: uint8(ag>>8) / 3,
					B: uint8(abl>>8) / 3,
					A: 255,
				})
			}
		}
	}

	if totalPixels > 0 {
		diffPct = float64(diffPixels) / float64(totalPixels) * 100.0
	}
	return diffPct, diffImg
}

func decodePNGFile(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode PNG %s: %v", path, err)
	}
	return img
}

func channelDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// Golden file assertion
// ---------------------------------------------------------------------------

// assertVisualMatch compares an actual screenshot against a golden baseline.
//
// When ODYSSEY_E2E_UPDATE_GOLDEN=1 and the golden file is missing, it copies
// the actual as the new baseline. When the golden exists, pixel diff must not
// exceed maxDiffPercent or the test fails and a diff image is saved.
func assertVisualMatch(t *testing.T, goldenPath, actualPath string, maxDiffPercent float64) {
	t.Helper()

	updateGolden := os.Getenv("ODYSSEY_E2E_UPDATE_GOLDEN") == "1"

	if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
		if updateGolden {
			if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
				t.Fatalf("create golden dir: %v", err)
			}
			data, err := os.ReadFile(actualPath)
			if err != nil {
				t.Fatalf("read actual for golden: %v", err)
			}
			if err := os.WriteFile(goldenPath, data, 0o644); err != nil {
				t.Fatalf("write golden: %v", err)
			}
			t.Logf("created golden file %s", goldenPath)
			return
		}
		t.Skipf("golden file %s not found; set ODYSSEY_E2E_UPDATE_GOLDEN=1 to create", goldenPath)
		return
	}

	// Per-channel threshold of 16 (out of 255) absorbs anti-aliasing jitter
	// across different rendering environments.
	diffPct, diffImg := comparePNG(t, goldenPath, actualPath, 16)

	if diffPct > maxDiffPercent {
		diffDir := "test-diffs"
		_ = os.MkdirAll(diffDir, 0o755)
		diffPath := filepath.Join(diffDir, "diff-"+filepath.Base(actualPath))
		saveDiffImage(t, diffImg, diffPath)
		t.Errorf("visual diff %.2f%% exceeds threshold %.2f%% (golden=%s actual=%s diff=%s)",
			diffPct, maxDiffPercent, goldenPath, actualPath, diffPath)
	} else {
		t.Logf("visual match OK: %.2f%% diff (threshold %.2f%%)", diffPct, maxDiffPercent)
	}
}

// saveDiffImage writes a highlighted-diff PNG to disk for debugging.
func saveDiffImage(t *testing.T, img *image.RGBA, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Logf("could not save diff image: %v", err)
		return
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Logf("could not encode diff image: %v", err)
		return
	}
	t.Logf("saved diff image to %s", path)
}

// ---------------------------------------------------------------------------
// Configuration helpers
// ---------------------------------------------------------------------------

// visualDiffThreshold returns the configured pixel diff threshold percentage.
func visualDiffThreshold() float64 {
	if raw := os.Getenv("ODYSSEY_E2E_VISUAL_DIFF_THRESHOLD"); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
			return v
		}
	}
	return 0.5
}

// pdfVisualEnabled returns true if PDF visual tests should run. Defaults to
// true unless ODYSSEY_E2E_PDF_VISUAL is explicitly set to "0".
func pdfVisualEnabled() bool {
	return os.Getenv("ODYSSEY_E2E_PDF_VISUAL") != "0"
}


