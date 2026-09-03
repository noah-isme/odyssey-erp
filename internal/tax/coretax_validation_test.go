package tax

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCoretaxAdapterContract verifies the local adapter contract. It is not a
// substitute for the tax-staff Coretax staging/import sign-off documented in
// docs/guides/tax-staff-coretax-validation.md.
func TestCoretaxAdapterContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			http.Error(w, `{"message":"unexpected request"}`, http.StatusNotFound)
			return
		}
		if r.Header.Get("X-API-Key") != "test-api-key" {
			http.Error(w, `{"message":"missing API key"}`, http.StatusUnauthorized)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, `{"message":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if payload["npwp"] != "01.234.567.8-090.000" || payload["total_pajak"] != float64(110000) {
			http.Error(w, `{"message":"payload mismatch"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true,"reference":"LOCAL-CONTRACT-1"}`))
	}))
	t.Cleanup(server.Close)

	config := CoretaxConfig{
		BaseURL:    server.URL,
		APIKey:     "test-api-key",
		SubmitPath: "/submit",
	}

	service := NewCoretaxService(config)
	ctx := context.Background()

	t.Run("Validate e-Faktur CSV Structure", func(t *testing.T) {
		period := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
		csvBytes, err := service.GenerateEFaktur(ctx, period)
		if err != nil {
			t.Fatalf("Failed to generate e-Faktur: %v", err)
		}

		csvOutput := string(csvBytes)

		// The checked-in structural contract covers the fields consumed by the
		// current exporter. Official XSD/converter validation remains external.
		expectedHeaders := []string{"FK", "KD_JENIS_TRANSAKSI", "FG_PENGGANTI", "NOMOR_FAKTUR", "MASA_PAJAK", "TAHUN_PAJAK", "TANGGAL_FAKTUR", "NPWP", "NAMA", "ALAMAT_LENGKAP", "JUMLAH_DPP", "JUMLAH_PPN", "JUMLAH_PPNBM"}
		for _, header := range expectedHeaders {
			if !strings.Contains(csvOutput, header) {
				t.Errorf("e-Faktur CSV missing required XSD header: %s", header)
			}
		}

		// Ensure body calculates exactly 11% PPN on DPP 1,000,000 = 110,000
		if !strings.Contains(csvOutput, "1000000,110000") {
			t.Errorf("e-Faktur CSV failed DPP/PPN numerical reconciliation. Expected '1000000,110000' in output.")
		}
	})

	t.Run("Validate Coretax Submission Schema", func(t *testing.T) {
		// Example Portal Import Reconciliation Payload
		payload := map[string]any{
			"npwp":        "01.234.567.8-090.000",
			"masa_pajak":  8,
			"tahun_pajak": 2026,
			"total_pajak": 110000,
		}

		_, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal Coretax JSON payload: %v", err)
		}

		err = service.SubmitCoretax(ctx, payload)
		if err != nil {
			t.Fatalf("Coretax adapter submission failed: %v", err)
		}
	})
}
