package awss3_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/awss3"
)

func TestExecuteCommand_BIExportRequiresExplicitDevelopmentModeForFake(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adapter := awss3.NewAdapter(logger, nil, connectors.ProviderOptions{DevelopmentMode: true})
	cmd := &connectors.OutboxCommand{
		CommandType: "bi.export",
		Payload:     []byte(`{"object_key":"exports/test.csv","content":"Metric,Value\nRevenue,1000\n","mime_type":"text/csv"}`),
	}
	err := adapter.ExecuteCommand(context.Background(), &connectors.Connection{CompanyID: 1}, cmd)
	require.NoError(t, err)
}

func TestExecuteCommand_BIExportUsesS3Contract(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var gotMethod, gotPath string
	var gotBody string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotMethod, gotPath = req.Method, req.URL.Path
		body, _ := io.ReadAll(req.Body)
		gotBody = string(body)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	})
	adapter := awss3.NewAdapter(logger, nil, connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
	})
	secret, _ := json.Marshal(awss3.Credentials{
		AccessKeyID:     "sandbox-key",
		SecretAccessKey: "sandbox-secret",
		Region:          "us-east-1",
		Bucket:          "bi-exports",
		Endpoint:        "https://s3.sandbox.invalid",
		UsePathStyle:    true,
	})
	cmd := &connectors.OutboxCommand{
		ID:            42,
		CommandType:   "bi.export",
		CorrelationID: "corr-42",
		Payload:       []byte(`{"object_key":"exports/2026/kpi.csv","content":"Metric,Value\nRevenue,1000\n","mime_type":"text/csv"}`),
	}
	err := adapter.ExecuteCommand(context.Background(), &connectors.Connection{CompanyID: 1, SecretRef: string(secret)}, cmd)
	require.NoError(t, err)
	require.Equal(t, http.MethodPut, gotMethod)
	require.Contains(t, gotPath, "/bi-exports/exports/2026/kpi.csv")
	require.Equal(t, "Metric,Value\nRevenue,1000\n", gotBody)
}

func TestVerifyCallbackSignatureFailsClosed(t *testing.T) {
	adapter := awss3.NewAdapter(slog.Default(), nil)
	err := adapter.VerifyCallbackSignature(context.Background(), &connectors.Connection{SecretRef: "secret"}, nil, nil)
	require.Error(t, err)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
