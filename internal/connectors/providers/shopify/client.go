package shopify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	shopURL     string
	accessToken string
	httpClient  *http.Client
}

func NewClient(shopURL, accessToken string) *Client {
	return &Client{
		shopURL:     shopURL,
		accessToken: accessToken,
		httpClient:  &http.Client{},
	}
}

// verifyWebhookSignature validates the HMAC-SHA256 signature from Shopify webhooks.
func verifyWebhookSignature(secret, signature string, payload []byte) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := mac.Sum(nil)
	expectedMACBase64 := base64.StdEncoding.EncodeToString(expectedMAC)
	return hmac.Equal([]byte(expectedMACBase64), []byte(signature))
}

// Shopify Webhook payloads

type ShopifyOrder struct {
	ID              int64              `json:"id"`
	Name            string             `json:"name"`
	Email           string             `json:"email"`
	TotalPrice      string             `json:"total_price"`
	Currency        string             `json:"currency"`
	CreatedAt       string             `json:"created_at"`
	LineItems       []ShopifyLineItem  `json:"line_items"`
	Customer        ShopifyCustomer    `json:"customer"`
	BillingAddress  ShopifyAddress     `json:"billing_address"`
	ShippingAddress ShopifyAddress     `json:"shipping_address"`
	FulfillmentStatus string           `json:"fulfillment_status"`
	FinancialStatus string             `json:"financial_status"`
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

// UpdateInventory updates the available inventory for a specific inventory item.
func (c *Client) UpdateInventory(ctx context.Context, locationID int64, inventoryItemID int64, available int) error {
	payload := map[string]interface{}{
		"location_id":      locationID,
		"inventory_item_id": inventoryItemID,
		"available":        available,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/admin/api/2024-04/inventory_levels/set.json", c.shopURL), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Shopify-Access-Token", c.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("shopify API error %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
