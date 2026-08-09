package shopify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

type Client struct {
	shopURL     string
	accessToken string
	httpClient  *http.Client
	retryPolicy connectors.RetryPolicy
}

func NewClient(shopURL, accessToken string) *Client {
	return NewClientWithOptions(shopURL, accessToken, connectors.ProviderOptions{})
}

func NewClientWithOptions(shopURL, accessToken string, options connectors.ProviderOptions) *Client {
	return &Client{
		shopURL:     strings.TrimRight(shopURL, "/"),
		accessToken: accessToken,
		httpClient:  options.HTTPClientOrDefault(),
		retryPolicy: options.RetryPolicyOrDefault(),
	}
}

type ShopifyOrder struct {
	ID                int64             `json:"id"`
	Name              string            `json:"name"`
	Email             string            `json:"email"`
	TotalPrice        string            `json:"total_price"`
	Currency          string            `json:"currency"`
	CreatedAt         string            `json:"created_at"`
	LineItems         []ShopifyLineItem `json:"line_items"`
	Customer          ShopifyCustomer   `json:"customer"`
	BillingAddress    ShopifyAddress    `json:"billing_address"`
	ShippingAddress   ShopifyAddress    `json:"shipping_address"`
	FulfillmentStatus string            `json:"fulfillment_status"`
	FinancialStatus   string            `json:"financial_status"`
}

type ShopifyLineItem struct {
	ID        int64  `json:"id"`
	ProductID int64  `json:"product_id"`
	VariantID int64  `json:"variant_id"`
	Title     string `json:"title"`
	Quantity  int    `json:"quantity"`
	Price     string `json:"price"`
	SKU       string `json:"sku"`
}

type ShopifyCustomer struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone"`
}

type ShopifyAddress struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Address1  string `json:"address1"`
	City      string `json:"city"`
	Province  string `json:"province"`
	Zip       string `json:"zip"`
	Country   string `json:"country"`
}

// UpdateInventory updates Shopify inventory using the provider's inventory
// item and location identifiers. Local product/warehouse IDs must be mapped
// before this command is enqueued.
func (c *Client) UpdateInventory(ctx context.Context, locationID, inventoryItemID int64, available int, idempotencyKey string) error {
	if locationID <= 0 || inventoryItemID <= 0 {
		return fmt.Errorf("shopify: location_id and inventory_item_id are required")
	}
	payload := map[string]any{
		"location_id":       locationID,
		"inventory_item_id": inventoryItemID,
		"available":         available,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("shopify: marshal inventory request: %w", err)
	}
	_, responseBody, err := c.request(ctx, http.MethodPost, "/inventory_levels/set.json", body, idempotencyKey)
	if err != nil {
		return err
	}
	_ = responseBody
	return nil
}

func (c *Client) CreateOrder(ctx context.Context, order OrderPayload, idempotencyKey string) error {
	if strings.TrimSpace(order.OrderID) == "" || len(order.LineItems) == 0 {
		return fmt.Errorf("shopify: order_id and at least one line item are required")
	}
	request := struct {
		Order struct {
			Email           string             `json:"email,omitempty"`
			FinancialStatus string             `json:"financial_status,omitempty"`
			Note            string             `json:"note,omitempty"`
			LineItems       []ShopifyOrderLine `json:"line_items"`
		} `json:"order"`
	}{}
	request.Order.Email = order.CustomerEmail
	request.Order.FinancialStatus = order.Status
	request.Order.Note = fmt.Sprintf("Odyssey order %s; total=%s %s", order.OrderID, order.TotalAmount, order.Currency)
	request.Order.LineItems = order.LineItems
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("shopify: marshal order request: %w", err)
	}
	_, _, err = c.request(ctx, http.MethodPost, "/orders.json", body, idempotencyKey)
	return err
}

func (c *Client) CheckHealth(ctx context.Context) error {
	resp, body, err := c.request(ctx, http.MethodGet, "/shop.json", nil, "")
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("shopify: health check returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *Client) request(ctx context.Context, method, path string, body []byte, idempotencyKey string) (*http.Response, []byte, error) {
	url := fmt.Sprintf("%s/admin/api/2024-04%s", c.shopURL, path)
	headers := http.Header{
		"Accept":                 []string{"application/json"},
		"Content-Type":           []string{"application/json"},
		"X-Shopify-Access-Token": []string{c.accessToken},
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		headers.Set("X-Odyssey-Idempotency-Key", idempotencyKey)
	}
	resp, responseBody, err := connectors.DoWithRetry(ctx, c.httpClient, method, url, body, headers, c.retryPolicy)
	if err != nil {
		return nil, nil, fmt.Errorf("shopify: provider request failed: %w", err)
	}
	return resp, responseBody, nil
}

// verifyWebhookSignature validates the HMAC-SHA256 signature from Shopify webhooks.
func verifyWebhookSignature(secret, signature string, payload []byte) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
