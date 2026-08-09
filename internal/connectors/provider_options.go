package connectors

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// ErrUnsupportedOperation indicates a provider capability that does not
// exist for the configured authentication or transport model.
var ErrUnsupportedOperation = errors.New("connectors: provider operation unsupported")

// ProviderOptions controls provider runtime behavior. DevelopmentMode is
// deliberately opt-in; production adapters must resolve encrypted secrets
// and execute real provider calls.
type ProviderOptions struct {
	Vault           *shared.Vault
	HTTPClient      *http.Client
	DevelopmentMode bool
	// AllowPlaintextCredentials is intended for in-process provider contract
	// tests only. It is never populated from application configuration.
	AllowPlaintextCredentials bool
	BaseURL                   string
	RetryPolicy               RetryPolicy
}

// RetryPolicy controls retries for provider HTTP requests. Callers should
// only use retries for operations that are idempotent or carry a provider
// supported idempotency key.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func (o ProviderOptions) HTTPClientOrDefault() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (o ProviderOptions) RetryPolicyOrDefault() RetryPolicy {
	policy := o.RetryPolicy
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 3
	}
	if policy.BaseDelay <= 0 {
		policy.BaseDelay = 100 * time.Millisecond
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = 2 * time.Second
	}
	return policy
}

// ResolveSecret decrypts a connection secret through the application vault.
// Only explicit development mode or the test-only AllowPlaintextCredentials
// option permits fixture credentials to bypass the vault.
func (o ProviderOptions) ResolveSecret(conn *Connection) (string, error) {
	if conn == nil || strings.TrimSpace(conn.SecretRef) == "" {
		return "", errors.New("connectors: secret reference is required")
	}
	if o.Vault == nil {
		if o.DevelopmentMode || o.AllowPlaintextCredentials {
			return conn.SecretRef, nil
		}
		return "", errors.New("connectors: credential vault is required")
	}
	secret, err := conn.GetCredentials(o.Vault)
	if err != nil {
		return "", errors.New("connectors: unable to resolve credential secret")
	}
	if strings.TrimSpace(secret) == "" {
		return "", errors.New("connectors: resolved credential secret is empty")
	}
	return secret, nil
}

// Header returns the first matching header using case-insensitive lookup.
func Header(headers map[string]string, names ...string) string {
	for _, name := range names {
		for key, value := range headers {
			if strings.EqualFold(key, name) {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

// ProviderPayloadID returns a stable replay key when a provider does not send
// an explicit event identifier.
func ProviderPayloadID(payload []byte) string {
	digest := sha256.Sum256(payload)
	return "payload-" + hex.EncodeToString(digest[:])
}

// DoWithRetry executes a provider request and retries transport failures,
// HTTP 429, and 5xx responses. The request body is recreated on every attempt.
// The caller owns and must close the returned response body.
func DoWithRetry(ctx context.Context, client *http.Client, method, url string, body []byte, headers http.Header, policy RetryPolicy) (*http.Response, []byte, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	policy = (ProviderOptions{RetryPolicy: policy}).RetryPolicyOrDefault()

	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, nil, err
		}
		for key, values := range headers {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt == policy.MaxAttempts {
				break
			}
			if err := waitRetry(ctx, retryDelay(policy, attempt, nil)); err != nil {
				return nil, nil, err
			}
			continue
		}

		responseBody, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil && readErr == nil {
			readErr = closeErr
		}
		if readErr != nil {
			lastErr = readErr
			if attempt == policy.MaxAttempts {
				break
			}
			if err := waitRetry(ctx, retryDelay(policy, attempt, nil)); err != nil {
				return nil, nil, err
			}
			continue
		}

		if !retryableStatus(resp.StatusCode) || attempt == policy.MaxAttempts {
			resp.Body = io.NopCloser(bytes.NewReader(responseBody))
			return resp, responseBody, nil
		}
		lastErr = errors.New("provider returned retryable HTTP status " + strconv.Itoa(resp.StatusCode))
		if err := waitRetry(ctx, retryDelay(policy, attempt, resp)); err != nil {
			return nil, nil, err
		}
	}

	return nil, nil, lastErr
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func retryDelay(policy RetryPolicy, attempt int, resp *http.Response) time.Duration {
	if resp != nil {
		if raw := strings.TrimSpace(resp.Header.Get("Retry-After")); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
				delay := time.Duration(seconds) * time.Second
				if delay > policy.MaxDelay {
					return policy.MaxDelay
				}
				return delay
			}
		}
	}
	delay := policy.BaseDelay * time.Duration(1<<(attempt-1))
	if delay > policy.MaxDelay {
		return policy.MaxDelay
	}
	return delay
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
