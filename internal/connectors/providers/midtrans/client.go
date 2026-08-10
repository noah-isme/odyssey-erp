package midtrans

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

const (
	sandboxSnapBaseURL = "https://app.sandbox.midtrans.com"
	prodSnapBaseURL    = "https://app.midtrans.com"
	sandboxAPIBaseURL  = "https://api.sandbox.midtrans.com"
	prodAPIBaseURL     = "https://api.midtrans.com"
)

type Client struct {
	serverKey   string
	httpClient  *http.Client
	snapBaseURL string
	apiBaseURL  string
	retryPolicy connectors.RetryPolicy
}

func NewClient(serverKey string, isProd bool) *Client {
	return NewClientWithOptions(serverKey, isProd, connectors.ProviderOptions{})
}

// NewClientWithOptions creates a Midtrans client with explicit transport and
// endpoint configuration. BaseURL is intended for a provider-compatible
// sandbox or contract-test endpoint; when it is empty, the official Midtrans
// sandbox or production endpoints are selected from isProd.
func NewClientWithOptions(serverKey string, isProd bool, options connectors.ProviderOptions) *Client {
	snapBaseURL := sandboxSnapBaseURL
	apiBaseURL := sandboxAPIBaseURL
	if isProd {
		snapBaseURL = prodSnapBaseURL
		apiBaseURL = prodAPIBaseURL
	}
	if strings.TrimSpace(options.BaseURL) != "" {
		snapBaseURL = strings.TrimRight(options.BaseURL, "/")
		apiBaseURL = strings.TrimRight(options.BaseURL, "/")
	}
	return &Client{
		serverKey:   serverKey,
		httpClient:  options.HTTPClientOrDefault(),
		snapBaseURL: snapBaseURL,
		apiBaseURL:  apiBaseURL,
		retryPolicy: options.RetryPolicyOrDefault(),
	}
}

type SnapTokenRequest struct {
	TransactionDetails TransactionDetails `json:"transaction_details"`
	ItemDetails        []ItemDetail       `json:"item_details,omitempty"`
	CustomerDetails    CustomerDetails    `json:"customer_details,omitempty"`
}

type TransactionDetails struct {
	OrderID  string `json:"order_id"`
	GrossAmt int64  `json:"gross_amount"`
}

type ItemDetail struct {
	ID       string `json:"id"`
	Price    int64  `json:"price"`
	Quantity int    `json:"quantity"`
	Name     string `json:"name"`
}

type CustomerDetails struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Email     string `json:"email"`
	Phone     string `json:"phone,omitempty"`
}

type SnapTokenResponse struct {
	Token       string `json:"token"`
	RedirectURL string `json:"redirect_url"`
}

// RefundRequest is the provider request for a full or partial refund. A
// stable RefundKey makes a retry safe: Midtrans treats a repeated refund key
// as the same refund request.
type RefundRequest struct {
	OrderID   string
	RefundKey string
	Amount    int64
	Reason    string
}

func (c *Client) CreateSnapToken(ctx context.Context, req SnapTokenRequest) (*SnapTokenResponse, error) {
	if strings.TrimSpace(c.serverKey) == "" {
		return nil, fmt.Errorf("midtrans: server key is required")
	}
	if strings.TrimSpace(req.TransactionDetails.OrderID) == "" || req.TransactionDetails.GrossAmt <= 0 {
		return nil, fmt.Errorf("midtrans: order_id and positive gross_amount are required")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, responseBody, err := c.request(ctx, http.MethodPost, c.snapBaseURL+"/snap/v1/transactions", payload)
	if err != nil {
		return nil, fmt.Errorf("midtrans: create checkout request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiError(resp.StatusCode, responseBody)
	}

	var snapResp SnapTokenResponse
	if err := json.Unmarshal(responseBody, &snapResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	if strings.TrimSpace(snapResp.Token) == "" || strings.TrimSpace(snapResp.RedirectURL) == "" {
		return nil, fmt.Errorf("midtrans: checkout response did not contain token and redirect_url")
	}
	return &snapResp, nil
}

// GetTransactionStatus retrieves the provider status for an order or
// transaction identifier. It is used by reconciliation and health checks.
func (c *Client) GetTransactionStatus(ctx context.Context, orderID string) (*WebhookNotification, error) {
	if strings.TrimSpace(orderID) == "" {
		return nil, fmt.Errorf("midtrans: order ID is required")
	}
	resp, responseBody, err := c.request(ctx, http.MethodGet, c.apiBaseURL+"/v2/"+url.PathEscape(orderID)+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("midtrans: get transaction status request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiError(resp.StatusCode, responseBody)
	}
	var status WebhookNotification
	if err := json.Unmarshal(responseBody, &status); err != nil {
		return nil, fmt.Errorf("midtrans: invalid transaction status response: %w", err)
	}
	return &status, nil
}

// RefundTransaction requests a full refund when Amount is zero, or a partial
// refund when Amount is positive. Midtrans returns the resulting transaction
// status (`refund` or `partial_refund`) in the same shape used by its status
// and webhook payloads.
func (c *Client) RefundTransaction(ctx context.Context, req RefundRequest) (*WebhookNotification, error) {
	if strings.TrimSpace(req.OrderID) == "" {
		return nil, fmt.Errorf("midtrans: order ID is required for refund")
	}
	if strings.TrimSpace(req.RefundKey) == "" {
		return nil, fmt.Errorf("midtrans: refund key is required")
	}
	if req.Amount < 0 {
		return nil, fmt.Errorf("midtrans: refund amount cannot be negative")
	}

	body := map[string]any{
		"refund_key": req.RefundKey,
	}
	if req.Amount > 0 {
		body["amount"] = req.Amount
	}
	if strings.TrimSpace(req.Reason) != "" {
		body["reason"] = req.Reason
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("midtrans: encode refund request: %w", err)
	}

	resp, responseBody, err := c.request(ctx, http.MethodPost, c.apiBaseURL+"/v2/"+url.PathEscape(req.OrderID)+"/refund", payload)
	if err != nil {
		return nil, fmt.Errorf("midtrans: refund request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiError(resp.StatusCode, responseBody)
	}

	var result WebhookNotification
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("midtrans: invalid refund response: %w", err)
	}
	if result.OrderID == "" {
		result.OrderID = req.OrderID
	}
	if result.TransactionStatus != "refund" && result.TransactionStatus != "partial_refund" {
		return nil, fmt.Errorf("midtrans: refund response has unexpected transaction_status %q", result.TransactionStatus)
	}
	return &result, nil
}

// ProbeTransactionStatus calls the documented status endpoint with a
// deliberately unknown order ID. A 404 is a healthy response: it proves the
// provider is reachable and did not reject the credentials before looking up
// the transaction.
func (c *Client) ProbeTransactionStatus(ctx context.Context, orderID string) (int, []byte, error) {
	if strings.TrimSpace(orderID) == "" {
		return 0, nil, fmt.Errorf("midtrans: health-check order ID is required")
	}
	resp, responseBody, err := c.request(ctx, http.MethodGet, c.apiBaseURL+"/v2/"+url.PathEscape(orderID)+"/status", nil)
	if err != nil {
		return 0, nil, fmt.Errorf("midtrans: health check request failed: %w", err)
	}
	return resp.StatusCode, responseBody, nil
}

func (c *Client) request(ctx context.Context, method, endpoint string, body []byte) (*http.Response, []byte, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, nil, fmt.Errorf("invalid provider endpoint")
	}
	requestHeaders := http.Header{
		"Accept":        []string{"application/json"},
		"Content-Type":  []string{"application/json"},
		"Authorization": []string{"Basic " + base64.StdEncoding.EncodeToString([]byte(c.serverKey+":"))},
	}
	resp, responseBody, err := connectors.DoWithRetry(ctx, c.httpClient, method, endpoint, body, requestHeaders, c.retryPolicy)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute http request: %w", err)
	}
	// DoWithRetry returns a replayable body for callers that need it. This
	// client already has the bytes, so close the body before returning.
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	return resp, responseBody, nil
}

func apiError(status int, body []byte) error {
	var response struct {
		StatusMessage string   `json:"status_message"`
		ErrorMessages []string `json:"error_messages"`
	}
	message := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &response) == nil {
		if response.StatusMessage != "" {
			message = response.StatusMessage
		} else if len(response.ErrorMessages) > 0 {
			message = strings.Join(response.ErrorMessages, "; ")
		}
	}
	if message == "" {
		message = "provider request failed"
	}
	if len(message) > 1024 {
		message = message[:1024]
	}
	return fmt.Errorf("midtrans API error (status %d): %s", status, message)
}
