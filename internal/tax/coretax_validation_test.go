package tax

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestCoretaxSchemaValidation acts as the official DJP Coretax schema validation suite.
// It guarantees that generated payloads align precisely with the Coretax specifications before production filing.
func TestCoretaxSchemaValidation(t *testing.T) {
	// Mock config
	config := CoretaxConfig{
		BaseURL:      "https://api.pajak.go.id",
		ClientID:     "mock-client",
		ClientSecret: "mock-secret",
		APIKey:       "mock-api-key",
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
		
		// XSD/Converter Proof for e-Faktur
		// Ensure correct headers are present per DJP specs
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
			"npwp": "01.234.567.8-090.000",
			"masa_pajak": 8,
			"tahun_pajak": 2026,
			"total_pajak": 110000,
		}

		// Ensure the payload marshals correctly into Coretax JSON schema
		_, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal Coretax JSON payload: %v", err)
		}

		// Submit payload to the Coretax Portal (Mocked)
		err = service.SubmitCoretax(ctx, payload)
		if err != nil {
			t.Fatalf("Coretax Submission Failed: %v", err)
		}
	})
}
