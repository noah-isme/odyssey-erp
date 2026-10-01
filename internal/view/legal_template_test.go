package view_test

import (
	"html"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

func TestLegalTemplateRender(t *testing.T) {
	engine, err := view.NewEngine()
	require.NoError(t, err)

	topics := []struct {
		topic        string
		title        string
		expectedText []string
	}{
		{
			topic: "privacy",
			title: "Kebijakan Privasi · Odyssey ERP",
			expectedText: []string{
				"Kebijakan Privasi &amp; Perlindungan Data",
				"UU PDP No. 27/2022",
				"Prinsip Kerahasiaan Data &amp; Pemisahan Tenant",
				"Kebijakan Retensi &amp; Prosedur Pemusnahan Data Finansial",
				"Isolasi Audit Finansial &amp; Pengelolaan Pihak Ketiga",
				"legal@odyssey-erp.id",
			},
		},
		{
			topic: "terms",
			title: "Syarat & Ketentuan Layanan · Odyssey ERP",
			expectedText: []string{
				"Syarat &amp; Ketentuan Layanan Enterprise",
				"Lisensi Enterprise &amp; Ruang Lingkup Penggunaan",
				"Hak Tenant Multi-Cabang, Multi-Entitas",
				"Service Level Agreement (SLA 99.9%)",
				"Portabilitas Penuh &amp; Hak Ekspor Data Terbuka",
				"CSV",
				"JSON",
				"SQL",
			},
		},
		{
			topic: "security",
			title: "Pernyataan Keamanan Sistem · Odyssey ERP",
			expectedText: []string{
				"Pernyataan Keamanan Sistem &amp; Enkripsi",
				"Enkripsi Data: AES-256 At-Rest &amp; TLS 1.3 In-Transit",
				"Immutable Journal Audit Trail",
				"Keamanan Jalur Integrasi DJP e-Faktur",
				"Proteksi CSRF &amp; Role-Based Access Control (RBAC)",
			},
		},
	}

	for _, tc := range topics {
		t.Run(tc.topic, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			data := view.TemplateData{
				Title:       tc.title,
				CSRFToken:   "test-csrf-token",
				CurrentPath: "/legal/" + tc.topic,
				Data: map[string]any{
					"Topic":         tc.topic,
					"EffectiveDate": "1 Januari 2026",
					"Version":       "2026.2",
				},
			}
			err := engine.Render(recorder, "pages/legal.html", data)
			require.NoError(t, err)

			body := recorder.Body.String()
			assert.Contains(t, body, "<!DOCTYPE html>")
			assert.Contains(t, body, "/static/css/pages/legal.css")
			assert.Contains(t, body, html.EscapeString(tc.title))

			for _, exp := range tc.expectedText {
				assert.Contains(t, body, exp)
			}
		})
	}
}
