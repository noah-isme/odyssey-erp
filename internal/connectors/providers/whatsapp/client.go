package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

const (
	graphAPIBaseURL = "https://graph.facebook.com/v19.0"
)

type Client struct {
	accessToken   string
	phoneNumberID string
	httpClient    *http.Client
	baseURL       string
	retryPolicy   connectors.RetryPolicy
}

func NewClient(accessToken string, phoneNumberID string) *Client {
	return NewClientWithOptions(accessToken, phoneNumberID, connectors.ProviderOptions{})
}

func NewClientWithOptions(accessToken string, phoneNumberID string, options connectors.ProviderOptions) *Client {
	baseURL := graphAPIBaseURL
	if strings.TrimSpace(options.BaseURL) != "" {
		baseURL = strings.TrimRight(options.BaseURL, "/")
	}
	return &Client{
		accessToken:   accessToken,
		phoneNumberID: phoneNumberID,
		httpClient:    options.HTTPClientOrDefault(),
		baseURL:       baseURL,
		retryPolicy:   options.RetryPolicyOrDefault(),
	}
}

type MessageRequest struct {
	MessagingProduct string      `json:"messaging_product"`
	To               string      `json:"to"`
	Type             string      `json:"type"`
	Text             *TextObject `json:"text,omitempty"`
}

type TextObject struct {
	Body string `json:"body"`
}

type MessageResponse struct {
	MessagingProduct string `json:"messaging_product"`
	Contacts         []struct {
		Input string `json:"input"`
		WaID  string `json:"wa_id"`
	} `json:"contacts"`
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

func (c *Client) SendTextMessage(ctx context.Context, to string, body string) (*MessageResponse, error) {
	return c.SendTextMessageWithIdempotency(ctx, to, body, "")
}

func (c *Client) SendTextMessageWithIdempotency(ctx context.Context, to string, body string, idempotencyKey string) (*MessageResponse, error) {
	if strings.TrimSpace(to) == "" || strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("whatsapp: recipient and body are required")
	}
	reqData := MessageRequest{
		MessagingProduct: "whatsapp",
		To:               to,
		Type:             "text",
		Text: &TextObject{
			Body: body,
		},
	}

	payload, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s/messages", strings.TrimRight(c.baseURL, "/"), c.phoneNumberID)
	headers := http.Header{
		"Content-Type":  []string{"application/json"},
		"Authorization": []string{"Bearer " + c.accessToken},
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		headers.Set("Idempotency-Key", idempotencyKey)
		headers.Set("X-Odyssey-Correlation-ID", idempotencyKey)
	}
	resp, responseBody, err := connectors.DoWithRetry(ctx, c.httpClient, http.MethodPost, url, payload, headers, c.retryPolicy)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: failed to execute request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("whatsapp API error (status %d): %s", resp.StatusCode, string(responseBody))
	}

	var msgResp MessageResponse
	if err := json.Unmarshal(responseBody, &msgResp); err != nil {
		return nil, fmt.Errorf("whatsapp: failed to decode response: %w", err)
	}
	if len(msgResp.Messages) == 0 || strings.TrimSpace(msgResp.Messages[0].ID) == "" {
		return nil, fmt.Errorf("whatsapp: provider response did not contain a message id")
	}

	return &msgResp, nil
}

// CheckPhoneNumber performs a real authenticated Graph API request.
func (c *Client) CheckPhoneNumber(ctx context.Context) error {
	url := fmt.Sprintf("%s/%s?fields=id", strings.TrimRight(c.baseURL, "/"), c.phoneNumberID)
	headers := http.Header{"Authorization": []string{"Bearer " + c.accessToken}}
	resp, body, err := connectors.DoWithRetry(ctx, c.httpClient, http.MethodGet, url, nil, headers, c.retryPolicy)
	if err != nil {
		return fmt.Errorf("whatsapp: health request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("whatsapp: health request returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
