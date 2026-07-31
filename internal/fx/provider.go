package fx

import (
	"errors"
	"net/http"
	"time"
)

type ErrorKind string

const (
	ErrorQuota          ErrorKind = "QUOTA"
	ErrorAuthentication ErrorKind = "AUTHENTICATION"
	ErrorTimeout        ErrorKind = "TIMEOUT"
	ErrorMalformed      ErrorKind = "MALFORMED"
)

type ProviderError struct {
	Kind       ErrorKind
	StatusCode int
	Err        error
}

func (e *ProviderError) Error() string {
	if e.Err == nil {
		return "fx provider error: " + string(e.Kind)
	}
	return "fx provider " + string(e.Kind) + ": " + e.Err.Error()
}
func (e *ProviderError) Unwrap() error { return e.Err }
func IsProviderError(err error, kind ErrorKind) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.Kind == kind
}

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}
type ProviderConfig struct {
	BaseURL, APIKey, Source string
	Timeout                 time.Duration
	Client                  HTTPClient
}
