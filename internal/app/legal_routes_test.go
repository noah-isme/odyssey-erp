package app_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/app"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

func setupTestRouter(t *testing.T) http.Handler {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	engine, err := view.NewEngine()
	require.NoError(t, err)

	sessionManager := shared.NewSessionManager(client, "session", "secret", time.Hour, false)
	csrfManager := shared.NewCSRFManager("csrfsecret")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	params := app.RouterParams{
		Logger:         logger,
		Templates:      engine,
		SessionManager: sessionManager,
		CSRFManager:    csrfManager,
	}

	return app.NewRouter(params)
}

func TestLegalRouteRedirect(t *testing.T) {
	router := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/legal", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "/legal/privacy", rec.Header().Get("Location"))
}

func TestLegalSubroutesRenderOK(t *testing.T) {
	router := setupTestRouter(t)

	tests := []struct {
		path         string
		expectedBody []string
	}{
		{
			path: "/legal/privacy",
			expectedBody: []string{
				"Kebijakan Privasi &amp; Perlindungan Data",
				"UU PDP No. 27/2022",
				"Prinsip Kerahasiaan Data &amp; Pemisahan Tenant",
				"Kebijakan Retensi &amp; Prosedur Pemusnahan Data Finansial",
				"/static/css/pages/legal.css",
			},
		},
		{
			path: "/legal/terms",
			expectedBody: []string{
				"Syarat &amp; Ketentuan Layanan Enterprise",
				"Lisensi Enterprise &amp; Ruang Lingkup Penggunaan",
				"Service Level Agreement (SLA 99.9%)",
				"Portabilitas Penuh &amp; Hak Ekspor Data Terbuka",
				"/static/css/pages/legal.css",
			},
		},
		{
			path: "/legal/security",
			expectedBody: []string{
				"Pernyataan Keamanan Sistem &amp; Enkripsi",
				"AES-256",
				"TLS 1.3",
				"Immutable Journal Audit Trail",
				"DJP e-Faktur",
				"/static/css/pages/legal.css",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.String()
			for _, exp := range tc.expectedBody {
				assert.Contains(t, body, exp)
			}
		})
	}
}
