package tax

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// TestCoretaxValidatorContractAndGLReconciliation exercises the complete local
// release contract: reviewed schema -> signed XML -> validator acceptance ->
// persisted totals -> zero-difference GL recap. The tax-staff Coretax import
// evidence is intentionally still a separate operational sign-off.
func TestCoretaxValidatorContractAndGLReconciliation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/validate" {
			http.Error(w, `{"message":"unexpected request"}`, http.StatusNotFound)
			return
		}
		if r.Header.Get("Content-Type") != "application/xml" || r.Header.Get("X-API-Key") != "test-api-key" {
			http.Error(w, `{"message":"invalid validator headers"}`, http.StatusUnauthorized)
			return
		}
		var envelope coretaxEnvelope
		if err := xml.NewDecoder(r.Body).Decode(&envelope); err != nil {
			http.Error(w, `{"message":"invalid XML"}`, http.StatusBadRequest)
			return
		}
		var base, amount Money
		for _, record := range envelope.Records {
			recordBase, err := strconv.ParseInt(record.TaxableBase, 10, 64)
			if err != nil {
				http.Error(w, `{"message":"invalid DPP"}`, http.StatusBadRequest)
				return
			}
			recordAmount, err := strconv.ParseInt(record.TaxAmount, 10, 64)
			if err != nil {
				http.Error(w, `{"message":"invalid tax"}`, http.StatusBadRequest)
				return
			}
			base += Money(recordBase)
			amount += Money(recordAmount)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CoretaxValidationResult{
			Accepted: true, Status: "accepted", Reference: "LOCAL-CORETAX-2026-01",
			RecordCount: int64(len(envelope.Records)), TaxableBase: base, TaxAmount: amount,
		})
	}))
	t.Cleanup(server.Close)

	store := &fakeStore{
		schema:   reviewedSchema(),
		exportID: 17,
		records: []ExportRecord{
			{TaxNumber: "010", DocumentNumber: "INV-2026-01", CounterpartyName: "Buyer", CounterpartyTaxID: "123", TaxableBase: 1000, TaxAmount: 110, Sign: 1},
			{TaxNumber: "011", DocumentNumber: "CN-2026-01", CounterpartyName: "Buyer", CounterpartyTaxID: "123", TaxableBase: 250, TaxAmount: 28, Sign: -1},
		},
		recap: []RecapLine{{Category: "VAT_OUTPUT", AccountCode: "2101", TaxableBase: 750, TaxAmount: 82, GLAmount: 82, Difference: 0}},
	}

	service := NewService(store, ReviewedSchemaValidator{})
	export, err := service.Export(t.Context(), 1, 202601, 9, "CORETAX_OUTPUT_VAT")
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if export.ID != 17 || export.TaxableBase != 750 || export.TaxAmount != 82 || export.Count != 2 {
		t.Fatalf("unexpected export evidence: %+v", export)
	}

	coretax := NewCoretaxService(CoretaxConfig{BaseURL: server.URL, APIKey: "test-api-key", ValidatePath: "/validate"})
	validation, err := coretax.ValidateExport(t.Context(), []byte(export.Content))
	if err != nil {
		t.Fatalf("Coretax validation failed: %v", err)
	}
	if !validation.Accepted || validation.Reference != "LOCAL-CORETAX-2026-01" || validation.RecordCount != export.Count || validation.TaxableBase != export.TaxableBase || validation.TaxAmount != export.TaxAmount {
		t.Fatalf("validator/export mismatch: %+v vs %+v", validation, export)
	}
	if err := service.Lock(t.Context(), 1, 202601, 9); err != nil {
		t.Fatalf("reconciled period did not lock: %v", err)
	}
	if !store.locked {
		t.Fatal("reconciled period was not locked")
	}
}
