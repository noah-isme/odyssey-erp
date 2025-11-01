//go:build prod

package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrPDFTimeout indicates the rendering request exceeded the configured timeout.
	ErrPDFTimeout = errors.New("consol pdf exporter: timeout")
	// ErrPDFInvalidResponse indicates Gotenberg returned a non-success status code.
	ErrPDFInvalidResponse = errors.New("consol pdf exporter: invalid response")
	// ErrPDFTooSmall indicates the generated PDF was below the minimum expected size.
	ErrPDFTooSmall = errors.New("consol pdf exporter: pdf below minimum size")
)

const (
	pdfMinSizeBytes   = 1024
	pdfMaxRetry       = 2
	pdfRequestTimeout = 10 * time.Second
)

// NewPDFRenderClient constructs a production PDF exporter backed by Gotenberg.
func NewPDFRenderClient(endpoint string) (PDFRenderClient, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("gotenberg endpoint required")
	}
	endpoint = strings.TrimRight(endpoint, "/")
	return &gotenbergPDFClient{
		endpoint:   endpoint,
		httpClient: &http.Client{Timeout: pdfRequestTimeout},
		retries:    pdfMaxRetry,
		timeout:    pdfRequestTimeout,
		minSize:    pdfMinSizeBytes,
	}, nil
}

type gotenbergPDFClient struct {
	endpoint   string
	httpClient *http.Client
	retries    int
	timeout    time.Duration
	minSize    int
}

func (c *gotenbergPDFClient) RenderHTML(ctx context.Context, html string) ([]byte, error) {
	if c == nil || c.httpClient == nil {
		return nil, fmt.Errorf("pdf client not initialised")
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("files", "report.html")
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(part, html); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	payload := body.Bytes()
	contentType := writer.FormDataContentType()
	attempts := c.retries + 1
	var lastErr error
	for i := 0; i < attempts; i++ {
		attemptCtx := ctx
		var cancel context.CancelFunc
		if c.timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, c.timeout)
		}
		req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, c.endpoint+"/forms/chromium/convert/html", bytes.NewReader(payload))
		if err != nil {
			if cancel != nil {
				cancel()
			}
			return nil, err
		}
		req.Header.Set("Content-Type", contentType)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if cancel != nil {
				cancel()
			}
			if ne := classifyNetError(err); ne != nil {
				lastErr = ne
			} else {
				lastErr = err
			}
			continue
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if cancel != nil {
			cancel()
		}
		if resp.StatusCode >= http.StatusInternalServerError {
			lastErr = fmt.Errorf("%w: status %d", ErrPDFInvalidResponse, resp.StatusCode)
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("%w: status %d", ErrPDFInvalidResponse, resp.StatusCode)
		}
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if len(data) < c.minSize {
			lastErr = ErrPDFTooSmall
			continue
		}
		return data, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: exhausted attempts", ErrPDFInvalidResponse)
	}
	return nil, fmt.Errorf("render consol pdf failed after %d attempts: %w", attempts, lastErr)
}

func classifyNetError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrPDFTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrPDFTimeout
	}
	return err
}
