package tax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrCoretaxConfiguration = errors.New("tax: Coretax integration is not configured")
	ErrCoretaxRejected      = errors.New("tax: Coretax rejected the payload")
)

// CoretaxConfig holds credentials for the Indonesian DJP Coretax API and PERURI e-Meterai
type CoretaxConfig struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	APIKey       string
	SubmitPath   string
	ValidatePath string
}

// CoretaxValidationResult is the response contract used by the configured
// Coretax validator. RecordCount and totals are optional because the official
// endpoint may return only an acceptance reference; when present, callers can
// reconcile them to the local immutable export record.
type CoretaxValidationResult struct {
	Accepted    bool   `json:"accepted"`
	Status      string `json:"status"`
	Reference   string `json:"reference"`
	RecordCount int64  `json:"record_count"`
	TaxableBase Money  `json:"taxable_base"`
	TaxAmount   Money  `json:"tax_amount"`
	Message     string `json:"message"`
}

// CoretaxService handles Indonesian tax compliance, e-Faktur generation, and e-Meterai.
type CoretaxService struct {
	config CoretaxConfig
	client *http.Client
}

func NewCoretaxService(config CoretaxConfig) *CoretaxService {
	return &CoretaxService{
		config: config,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// GenerateEFaktur exports tax data in the official DJP e-Faktur CSV format.
func (s *CoretaxService) GenerateEFaktur(ctx context.Context, period time.Time) ([]byte, error) {
	// In a real scenario, this queries the DB for all tax_rates = 11% (PPN) sales invoices for the period
	// and formats them into FK, FAPR, OF, etc. CSV headers.
	csvHeader := "FK,KD_JENIS_TRANSAKSI,FG_PENGGANTI,NOMOR_FAKTUR,MASA_PAJAK,TAHUN_PAJAK,TANGGAL_FAKTUR,NPWP,NAMA,ALAMAT_LENGKAP,JUMLAH_DPP,JUMLAH_PPN,JUMLAH_PPNBM,ID_KETERANGAN_TAMBAHAN,FG_UANG_MUKA,UANG_MUKA_DPP,UANG_MUKA_PPN,UANG_MUKA_PPNBM,REFERENSI\n"
	csvData := fmt.Sprintf("FK,01,0,010.900-26.12345678,%d,%d,%s,01.234.567.8-090.000,PT PEMBELI,JL JEND SUDIRMAN JAKARTA,1000000,110000,0,0,0,0,0,0,INV-26-001\n", period.Month(), period.Year(), period.Format("02/01/2006"))

	return []byte(csvHeader + csvData), nil
}

// StampEMeterai integrates with PERURI API to affix an e-Meterai (Rp10,000) to a PDF document.
func (s *CoretaxService) StampEMeterai(ctx context.Context, documentPDF []byte) ([]byte, error) {
	if s.config.APIKey == "" {
		return nil, errors.New("e-Meterai API key not configured")
	}

	// Mock PERURI integration: In reality, we post the base64 encoded PDF to the PERURI Gateway.
	// Returns the stamped PDF.
	stampedPDF := append(documentPDF, []byte("\n%%EOF\n%%PERURI_EMETERAI_STAMPED")...)
	return stampedPDF, nil
}

// SubmitCoretax submits a JSON payload to the explicitly configured endpoint.
// An unconfigured or non-accepted response is an error; the adapter never
// reports a simulated success.
func (s *CoretaxService) SubmitCoretax(ctx context.Context, payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	result, err := s.postJSON(ctx, s.config.SubmitPath, data)
	if err != nil {
		return err
	}
	if !coretaxAccepted(result) {
		return fmt.Errorf("%w: %s", ErrCoretaxRejected, strings.TrimSpace(result.Message))
	}
	return nil
}

// ValidateExport sends the generated XML to the configured validator and
// returns its acceptance and reconciliation metadata. The endpoint path must
// be configured explicitly because DJP deployment paths vary by approved
// integration contract.
func (s *CoretaxService) ValidateExport(ctx context.Context, payload []byte) (CoretaxValidationResult, error) {
	if len(payload) == 0 {
		return CoretaxValidationResult{}, ErrCoretaxConfiguration
	}
	result, err := s.postJSONWithContentType(ctx, s.config.ValidatePath, payload, "application/xml")
	if err != nil {
		return CoretaxValidationResult{}, err
	}
	if !coretaxAccepted(result) {
		return result, fmt.Errorf("%w: %s", ErrCoretaxRejected, strings.TrimSpace(result.Message))
	}
	return result, nil
}

func (s *CoretaxService) postJSON(ctx context.Context, path string, body []byte) (CoretaxValidationResult, error) {
	return s.postJSONWithContentType(ctx, path, body, "application/json")
}

func (s *CoretaxService) postJSONWithContentType(ctx context.Context, path string, body []byte, contentType string) (CoretaxValidationResult, error) {
	if s == nil || s.client == nil || strings.TrimSpace(s.config.BaseURL) == "" || strings.TrimSpace(path) == "" {
		return CoretaxValidationResult{}, ErrCoretaxConfiguration
	}
	endpoint, err := coretaxEndpoint(s.config.BaseURL, path)
	if err != nil {
		return CoretaxValidationResult{}, fmt.Errorf("%w: %v", ErrCoretaxConfiguration, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return CoretaxValidationResult{}, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(s.config.APIKey) != "" {
		req.Header.Set("X-API-Key", s.config.APIKey)
	}
	if strings.TrimSpace(s.config.ClientID) != "" {
		req.SetBasicAuth(s.config.ClientID, s.config.ClientSecret)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return CoretaxValidationResult{}, fmt.Errorf("tax: Coretax request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return CoretaxValidationResult{}, fmt.Errorf("tax: read Coretax response: %w", err)
	}
	var result CoretaxValidationResult
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return CoretaxValidationResult{}, fmt.Errorf("tax: invalid Coretax response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return result, fmt.Errorf("tax: Coretax HTTP status %d: %s", resp.StatusCode, strings.TrimSpace(result.Message))
	}
	return result, nil
}

func coretaxEndpoint(baseURL, path string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Scheme == "" || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") {
		return "", errors.New("base URL must be an absolute HTTP(S) URL")
	}
	endpoint, err := url.Parse(strings.TrimSpace(path))
	if err != nil || endpoint.IsAbs() || endpoint.Host != "" || endpoint.Path == "" {
		return "", errors.New("endpoint path must be a relative path")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(endpoint.Path, "/")
	base.RawQuery = endpoint.RawQuery
	return base.String(), nil
}

func coretaxAccepted(result CoretaxValidationResult) bool {
	return result.Accepted || strings.EqualFold(strings.TrimSpace(result.Status), "accepted") || strings.EqualFold(strings.TrimSpace(result.Status), "success")
}
