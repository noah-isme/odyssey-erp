package report

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// Client wraps interactions with the Gotenberg API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a new client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Ping checks if the remote Gotenberg service is available.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/health", c.baseURL), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("gotenberg returned status %d", resp.StatusCode)
	}
	return nil
}

// RenderHTML converts raw HTML into a PDF document using Gotenberg.
func (c *Client) RenderHTML(ctx context.Context, html string) ([]byte, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("files", "document.html")
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, bytes.NewBufferString(html)); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/forms/chromium/convert/html", c.baseURL), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("render failed with status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
