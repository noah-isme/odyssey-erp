package midtrans

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	sandboxSnapURL = "https://app.sandbox.midtrans.com/snap/v1/transactions"
	prodSnapURL    = "https://app.midtrans.com/snap/v1/transactions"
)

type Client struct {
	serverKey  string
	httpClient *http.Client
	isProd     bool
}

func NewClient(serverKey string, isProd bool) *Client {
	return &Client{
		serverKey:  serverKey,
		httpClient: &http.Client{},
		isProd:     isProd,
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

func (c *Client) CreateSnapToken(ctx context.Context, req SnapTokenRequest) (*SnapTokenResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := sandboxSnapURL
	if c.isProd {
		url = prodSnapURL
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.SetBasicAuth(c.serverKey, "")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("midtrans API error (status %d): %s", resp.StatusCode, string(body))
	}

	var snapResp SnapTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&snapResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &snapResp, nil
}
